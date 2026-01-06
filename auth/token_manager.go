package auth

import (
	"fmt"
	"kiro2api/config"
	"kiro2api/logger"
	"kiro2api/types"
	"sync"
	"time"
)

// TokenManager 简化的token管理器
type TokenManager struct {
	cache        *SimpleTokenCache
	configs      []AuthConfig
	mutex        sync.RWMutex
	lastRefresh  time.Time
	configOrder  []string        // 配置顺序
	currentIndex int             // 当前使用的token索引
	exhausted    map[string]bool // 已耗尽的token记录
	storage      *ConfigStorage  // 配置持久化存储
}

// SimpleTokenCache 简化的token缓存（纯数据结构，无锁）
// 所有并发访问由 TokenManager.mutex 统一管理
type SimpleTokenCache struct {
	tokens map[string]*CachedToken
	ttl    time.Duration
}

// CachedToken 缓存的token信息
type CachedToken struct {
	Token     types.TokenInfo
	UsageInfo *types.UsageLimits
	CachedAt  time.Time
	LastUsed  time.Time
	Available float64
}

// NewSimpleTokenCache 创建简单的token缓存
func NewSimpleTokenCache(ttl time.Duration) *SimpleTokenCache {
	return &SimpleTokenCache{
		tokens: make(map[string]*CachedToken),
		ttl:    ttl,
	}
}

// NewTokenManager 创建新的token管理器
func NewTokenManager(configs []AuthConfig) *TokenManager {
	// 生成配置顺序
	configOrder := generateConfigOrder(configs)

	logger.Info("TokenManager初始化（顺序选择策略）",
		logger.Int("config_count", len(configs)),
		logger.Int("config_order_count", len(configOrder)))

	return &TokenManager{
		cache:        NewSimpleTokenCache(config.TokenCacheTTL),
		configs:      configs,
		configOrder:  configOrder,
		currentIndex: 0,
		exhausted:    make(map[string]bool),
		storage:      NewConfigStorage(), // 初始化配置存储
	}
}

// getBestToken 获取最优可用token
// 统一锁管理：所有操作在单一锁保护下完成，避免多次加锁/解锁
func (tm *TokenManager) getBestToken() (types.TokenInfo, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 检查是否需要刷新缓存（在锁内）
	if time.Since(tm.lastRefresh) > config.TokenCacheTTL {
		if err := tm.refreshCacheUnlocked(); err != nil {
			logger.Warn("刷新token缓存失败", logger.Err(err))
		}
	}

	// 选择最优token（内部方法，不加锁）
	bestToken := tm.selectBestTokenUnlocked()
	if bestToken == nil {
		return types.TokenInfo{}, fmt.Errorf("没有可用的token")
	}

	// 更新最后使用时间（在锁内，安全）
	bestToken.LastUsed = time.Now()
	if bestToken.Available > 0 {
		bestToken.Available--
	}

	return bestToken.Token, nil
}

// GetBestTokenWithUsage 获取最优可用token（包含使用信息）
// 统一锁管理：所有操作在单一锁保护下完成
func (tm *TokenManager) GetBestTokenWithUsage() (*types.TokenWithUsage, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	// 检查是否需要刷新缓存（在锁内）
	if time.Since(tm.lastRefresh) > config.TokenCacheTTL {
		if err := tm.refreshCacheUnlocked(); err != nil {
			logger.Warn("刷新token缓存失败", logger.Err(err))
		}
	}

	// 选择最优token（内部方法，不加锁）
	bestToken := tm.selectBestTokenUnlocked()
	if bestToken == nil {
		return nil, fmt.Errorf("没有可用的token")
	}

	// 更新最后使用时间（在锁内，安全）
	bestToken.LastUsed = time.Now()
	available := bestToken.Available
	if bestToken.Available > 0 {
		bestToken.Available--
	}

	// 构造 TokenWithUsage
	tokenWithUsage := &types.TokenWithUsage{
		TokenInfo:       bestToken.Token,
		UsageLimits:     bestToken.UsageInfo,
		AvailableCount:  available, // 使用精确计算的可用次数
		LastUsageCheck:  bestToken.LastUsed,
		IsUsageExceeded: available <= 0,
	}

	logger.Debug("返回TokenWithUsage",
		logger.Float64("available_count", available),
		logger.Bool("is_exceeded", tokenWithUsage.IsUsageExceeded))

	return tokenWithUsage, nil
}

// selectBestTokenUnlocked 按配置顺序选择下一个可用token
// 内部方法：调用者必须持有 tm.mutex
// 重构说明：从selectBestToken改为Unlocked后缀，明确锁约定
func (tm *TokenManager) selectBestTokenUnlocked() *CachedToken {
	// 调用者已持有 tm.mutex，无需额外加锁

	// 如果没有配置顺序，降级到按map遍历顺序
	if len(tm.configOrder) == 0 {
		for key, cached := range tm.cache.tokens {
			if time.Since(cached.CachedAt) <= tm.cache.ttl && cached.IsUsable() {
				logger.Debug("顺序策略选择token（无顺序配置）",
					logger.String("selected_key", key),
					logger.Float64("available_count", cached.Available))
				return cached
			}
		}
		return nil
	}

	// 从当前索引开始，找到第一个可用的token
	for attempts := 0; attempts < len(tm.configOrder); attempts++ {
		currentKey := tm.configOrder[tm.currentIndex]

		// 检查这个token是否存在且可用
		if cached, exists := tm.cache.tokens[currentKey]; exists {
			// 检查token是否过期
			if time.Since(cached.CachedAt) > tm.cache.ttl {
				tm.exhausted[currentKey] = true
				tm.currentIndex = (tm.currentIndex + 1) % len(tm.configOrder)
				continue
			}

			// 检查token是否可用
			if cached.IsUsable() {
				logger.Debug("顺序策略选择token",
					logger.String("selected_key", currentKey),
					logger.Int("index", tm.currentIndex),
					logger.Float64("available_count", cached.Available))
				return cached
			}
		}

		// 标记当前token为已耗尽，移动到下一个
		tm.exhausted[currentKey] = true
		tm.currentIndex = (tm.currentIndex + 1) % len(tm.configOrder)

		logger.Debug("token不可用，切换到下一个",
			logger.String("exhausted_key", currentKey),
			logger.Int("next_index", tm.currentIndex))
	}

	// 所有token都不可用
	logger.Warn("所有token都不可用",
		logger.Int("total_count", len(tm.configOrder)),
		logger.Int("exhausted_count", len(tm.exhausted)))

	return nil
}

// refreshCacheUnlocked 刷新token缓存
// 内部方法：调用者必须持有 tm.mutex
func (tm *TokenManager) refreshCacheUnlocked() error {
	logger.Debug("开始刷新token缓存")

	for i, cfg := range tm.configs {
		if cfg.Disabled {
			continue
		}

		// 刷新token
		token, err := tm.refreshSingleToken(cfg)
		if err != nil {
			logger.Warn("刷新单个token失败",
				logger.Int("config_index", i),
				logger.String("auth_type", cfg.AuthType),
				logger.Err(err))
			continue
		}

		// 检查使用限制
		var usageInfo *types.UsageLimits
		var available float64

		checker := NewUsageLimitsChecker()
		if usage, checkErr := checker.CheckUsageLimits(token); checkErr == nil {
			usageInfo = usage
			available = CalculateAvailableCount(usage)
		} else {
			logger.Warn("检查使用限制失败", logger.Err(checkErr))
		}

		// 更新缓存（直接访问，已在tm.mutex保护下）
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		tm.cache.tokens[cacheKey] = &CachedToken{
			Token:     token,
			UsageInfo: usageInfo,
			CachedAt:  time.Now(),
			Available: available,
		}

		logger.Debug("token缓存更新",
			logger.String("cache_key", cacheKey),
			logger.Float64("available", available))
	}

	tm.lastRefresh = time.Now()
	return nil
}

// IsUsable 检查缓存的token是否可用
func (ct *CachedToken) IsUsable() bool {
	// 检查token是否过期
	if time.Now().After(ct.Token.ExpiresAt) {
		return false
	}

	// 检查可用次数
	return ct.Available > 0
}

// *** 已删除 set 和 updateLastUsed 方法 ***
// SimpleTokenCache 现在是纯数据结构，所有访问由 TokenManager.mutex 保护
// set 操作：直接通过 tm.cache.tokens[key] = value 完成
// updateLastUsed 操作：已合并到 getBestToken 方法中

// CalculateAvailableCount 计算可用次数 (基于CREDIT资源类型，返回浮点精度)
func CalculateAvailableCount(usage *types.UsageLimits) float64 {
	for _, breakdown := range usage.UsageBreakdownList {
		if breakdown.ResourceType == "CREDIT" {
			var totalAvailable float64

			// 优先使用免费试用额度 (如果存在且处于ACTIVE状态)
			if breakdown.FreeTrialInfo != nil && breakdown.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
				freeTrialAvailable := breakdown.FreeTrialInfo.UsageLimitWithPrecision - breakdown.FreeTrialInfo.CurrentUsageWithPrecision
				totalAvailable += freeTrialAvailable
			}

			// 加上基础额度
			baseAvailable := breakdown.UsageLimitWithPrecision - breakdown.CurrentUsageWithPrecision
			totalAvailable += baseAvailable

			if totalAvailable < 0 {
				return 0.0
			}
			return totalAvailable
		}
	}
	return 0.0
}

// ReloadConfigs 添加新的token配置（不需要重启服务）
// 注意：这是添加配置，不是替换！原有配置会保留
func (tm *TokenManager) ReloadConfigs(newConfigs []AuthConfig) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	oldCount := len(tm.configs)
	
	logger.Info("开始添加token配置",
		logger.Int("old_count", oldCount),
		logger.Int("add_count", len(newConfigs)))

	tm.configs = append(tm.configs, newConfigs...)
	
	// 重新生成配置顺序
	tm.configOrder = generateConfigOrder(tm.configs)
	
	// 不重置 currentIndex 和 exhausted，保持原有使用状态

	logger.Info("token配置已添加到列表",
		logger.Int("old_count", oldCount),
		logger.Int("total_count", len(tm.configs)))

	// 🔥 持久化保存配置到文件
	if tm.storage != nil {
		if err := tm.storage.Save(tm.configs); err != nil {
			logger.Warn("保存配置到持久化文件失败（但内存配置已更新）",
				logger.Err(err))
		}
	}

	// 刷新新添加的token（只刷新新添加的部分）
	for i := oldCount; i < len(tm.configs); i++ {
		cfg := tm.configs[i]
		if cfg.Disabled {
			logger.Info("跳过禁用的token", logger.Int("index", i))
			continue
		}

		token, err := tm.refreshSingleToken(cfg)
		if err != nil {
			logger.Warn("刷新新添加的token失败（但配置已保存）",
				logger.Int("config_index", i),
				logger.String("auth_type", cfg.AuthType),
				logger.Err(err))
			continue
		}

		// 检查使用限制
		var usageInfo *types.UsageLimits
		var available float64

		checker := NewUsageLimitsChecker()
		if usage, checkErr := checker.CheckUsageLimits(token); checkErr == nil {
			usageInfo = usage
			available = CalculateAvailableCount(usage)
		} else {
			logger.Warn("检查使用限制失败", logger.Err(checkErr))
		}

		// 添加到缓存
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		tm.cache.tokens[cacheKey] = &CachedToken{
			Token:     token,
			UsageInfo: usageInfo,
			CachedAt:  time.Now(),
			Available: available,
		}

		logger.Info("新token已添加到缓存",
			logger.String("cache_key", cacheKey),
			logger.Float64("available", available))
	}

	logger.Info("token配置添加完成",
		logger.Int("total_configs", len(tm.configs)),
		logger.Int("cached_tokens", len(tm.cache.tokens)))

	return nil
}

// GetCurrentConfigs 获取当前配置（用于查看）
func (tm *TokenManager) GetCurrentConfigs() []AuthConfig {
	tm.mutex.RLock()
	defer tm.mutex.RUnlock()

	// 返回配置的副本
	configs := make([]AuthConfig, len(tm.configs))
	copy(configs, tm.configs)
	return configs
}

// ToggleTokenStatus 切换token的启用/停用状态
func (tm *TokenManager) ToggleTokenStatus(index int) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if index < 0 || index >= len(tm.configs) {
		return fmt.Errorf("索引越界: %d", index)
	}

	// 切换状态
	tm.configs[index].Disabled = !tm.configs[index].Disabled
	newStatus := "已启用"
	if tm.configs[index].Disabled {
		newStatus = "已停用"
		// 从缓存中移除
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, index)
		delete(tm.cache.tokens, cacheKey)
		delete(tm.exhausted, cacheKey)
	} else {
		// 重新刷新这个token
		cfg := tm.configs[index]
		token, err := tm.refreshSingleToken(cfg)
		if err != nil {
			logger.Warn("启用token后刷新失败", logger.Err(err))
		} else {
			// 添加到缓存
			var usageInfo *types.UsageLimits
			var available float64
			checker := NewUsageLimitsChecker()
			if usage, checkErr := checker.CheckUsageLimits(token); checkErr == nil {
				usageInfo = usage
				available = CalculateAvailableCount(usage)
			}
			
			cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, index)
			tm.cache.tokens[cacheKey] = &CachedToken{
				Token:     token,
				UsageInfo: usageInfo,
				CachedAt:  time.Now(),
				Available: available,
			}
		}
	}

	logger.Info("token状态已切换",
		logger.Int("index", index),
		logger.String("status", newStatus))

	return nil
}

// RemoveToken 删除指定索引的token
func (tm *TokenManager) RemoveToken(index int) error {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	if index < 0 || index >= len(tm.configs) {
		return fmt.Errorf("索引越界: %d", index)
	}

	logger.Info("删除token配置",
		logger.Int("index", index),
		logger.Int("total_before", len(tm.configs)))

	// 删除配置
	tm.configs = append(tm.configs[:index], tm.configs[index+1:]...)
	
	// 重新生成配置顺序
	tm.configOrder = generateConfigOrder(tm.configs)
	
	// 清空缓存，重新刷新（因为索引变了）
	tm.cache.tokens = make(map[string]*CachedToken)
	tm.exhausted = make(map[string]bool)
	tm.currentIndex = 0
	
	// 重新刷新所有token
	if err := tm.refreshCacheUnlocked(); err != nil {
		logger.Warn("删除token后重新刷新失败", logger.Err(err))
	}

	logger.Info("token已删除",
		logger.Int("total_after", len(tm.configs)),
		logger.Int("cached_tokens", len(tm.cache.tokens)))

	return nil
}

// generateConfigOrder 生成token配置的顺序
func generateConfigOrder(configs []AuthConfig) []string {
	var order []string

	for i := range configs {
		// 使用索引生成cache key，与refreshCache中的逻辑保持一致
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		order = append(order, cacheKey)
	}

	logger.Debug("生成配置顺序",
		logger.Int("config_count", len(configs)),
		logger.Any("order", order))

	return order
}

// RefreshAllTokens 刷新所有token的用量信息
func (tm *TokenManager) RefreshAllTokens() (int, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	logger.Info("开始刷新所有token", logger.Int("total", len(tm.configs)))

	// 直接调用内部刷新方法
	if err := tm.refreshCacheUnlocked(); err != nil {
		return 0, err
	}

	refreshedCount := len(tm.cache.tokens)

	logger.Info("token刷新完成",
		logger.Int("refreshed", refreshedCount),
		logger.Int("total", len(tm.configs)))

	return refreshedCount, nil
}

// CleanupInvalidTokens 清理失效token（过期或已耗尽）
func (tm *TokenManager) CleanupInvalidTokens() (int, error) {
	tm.mutex.Lock()
	defer tm.mutex.Unlock()

	logger.Info("开始清理失效token", logger.Int("total", len(tm.configs)))

	now := time.Now()
	var validConfigs []AuthConfig
	removedCount := 0

	for i, cfg := range tm.configs {
		cacheKey := fmt.Sprintf(config.TokenCacheKeyFormat, i)
		cached, exists := tm.cache.tokens[cacheKey]

		shouldRemove := false
		reason := ""

		if exists && cached != nil {
			// 检查是否过期
			if cached.Token.ExpiresAt.Before(now) {
				shouldRemove = true
				reason = "已过期"
			}

			// 检查是否已耗尽
			if cached.Available <= 0 {
				shouldRemove = true
				reason = "已耗尽"
			}
		}

		if shouldRemove {
			logger.Info("清理失效token",
				logger.Int("index", i),
				logger.String("reason", reason))
			removedCount++
		} else {
			validConfigs = append(validConfigs, cfg)
		}
	}

	// 更新配置
	tm.configs = validConfigs
	tm.configOrder = generateConfigOrder(tm.configs)

	// 清空缓存，重新刷新
	tm.cache.tokens = make(map[string]*CachedToken)
	tm.exhausted = make(map[string]bool)
	tm.currentIndex = 0

	// 重新刷新所有token
	if len(tm.configs) > 0 {
		if err := tm.refreshCacheUnlocked(); err != nil {
			logger.Warn("清理后重新刷新失败", logger.Err(err))
		}
	}

	logger.Info("token清理完成",
		logger.Int("removed", removedCount),
		logger.Int("remaining", len(tm.configs)))

	return removedCount, nil
}
