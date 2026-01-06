package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro2api/auth"
	"kiro2api/logger"
	"kiro2api/types"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleTokenPool(c *gin.Context) {
	var tokenList []any
	var activeCount int

	// 优先使用 tokenManager 中的配置（支持热更新）
	var configs []auth.AuthConfig
	if h.tokenManager != nil {
		configs = h.tokenManager.GetCurrentConfigs()
	} else {
		// 降级到从环境变量加载
		var err error
		configs, err = auth.GetConfigs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "加载配置失败: " + err.Error(),
			})
			return
		}
	}

	if len(configs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"timestamp":     time.Now().Format(time.RFC3339),
			"total_tokens":  0,
			"active_tokens": 0,
			"tokens":        []any{},
			"pool_stats": map[string]any{
				"total_tokens":  0,
				"active_tokens": 0,
			},
		})
		return
	}

	for i, authConfig := range configs {
		if authConfig.Disabled {
			tokenData := map[string]any{
				"index":           i,
				"user_email":      "已禁用",
				"token_preview":   createTokenPreview(authConfig.RefreshToken),
				"auth_type":       strings.ToLower(authConfig.AuthType),
				"remaining_usage": 0,
				"expires_at":      time.Now().Add(time.Hour).Format(time.RFC3339),
				"last_used":       "未知",
				"status":          "disabled",
			}
			tokenList = append(tokenList, tokenData)
			continue
		}

		tokenInfo, err := refreshSingleTokenByConfig(authConfig)
		if err != nil {
			tokenData := map[string]any{
				"index":           i,
				"user_email":      "获取失败",
				"token_preview":   createTokenPreview(authConfig.RefreshToken),
				"auth_type":       strings.ToLower(authConfig.AuthType),
				"remaining_usage": 0,
				"expires_at":      time.Now().Add(time.Hour).Format(time.RFC3339),
				"last_used":       "未知",
				"status":          "error",
				"error":           err.Error(),
			}
			tokenList = append(tokenList, tokenData)
			continue
		}

		var usageInfo *types.UsageLimits
		var available float64
		userEmail := fmt.Sprintf("用户-%d", i)

		checker := auth.NewUsageLimitsChecker()
		if usage, checkErr := checker.CheckUsageLimits(tokenInfo); checkErr == nil {
			usageInfo = usage
			available = auth.CalculateAvailableCount(usage)

			if usage.UserInfo.Email != "" {
				userEmail = usage.UserInfo.Email
			} else if usage.UserInfo.UserID != "" {
				// 如果没有 email，使用 userId 的后12位
				userId := usage.UserInfo.UserID
				if len(userId) > 12 {
					userEmail = "ID-" + userId[len(userId)-12:]
				} else {
					userEmail = "ID-" + userId
				}
			}
		}

		tokenData := map[string]any{
			"index":           i,
			"user_email":      maskEmail(userEmail),
			"token_preview":   createTokenPreview(tokenInfo.AccessToken),
			"auth_type":       strings.ToLower(authConfig.AuthType),
			"remaining_usage": available,
			"expires_at":      tokenInfo.ExpiresAt.Format(time.RFC3339),
			"last_used":       time.Now().Format(time.RFC3339),
			"status":          "active",
		}

		if usageInfo != nil {
			for _, breakdown := range usageInfo.UsageBreakdownList {
				if breakdown.ResourceType == "CREDIT" {
					var totalLimit float64
					var totalUsed float64

					totalLimit += breakdown.UsageLimitWithPrecision
					totalUsed += breakdown.CurrentUsageWithPrecision

					if breakdown.FreeTrialInfo != nil && breakdown.FreeTrialInfo.FreeTrialStatus == "ACTIVE" {
						totalLimit += breakdown.FreeTrialInfo.UsageLimitWithPrecision
						totalUsed += breakdown.FreeTrialInfo.CurrentUsageWithPrecision
					}

					tokenData["usage_limits"] = map[string]any{
						"total_limit":   totalLimit,
						"current_usage": totalUsed,
						"is_exceeded":   available <= 0,
					}
					break
				}
			}
		}

		if available <= 0 {
			tokenData["status"] = "exhausted"
		} else {
			activeCount++
		}

		if authConfig.AuthType == auth.AuthMethodIdC && authConfig.ClientID != "" {
			tokenData["client_id"] = func() string {
				if len(authConfig.ClientID) > 10 {
					return authConfig.ClientID[:5] + "***" + authConfig.ClientID[len(authConfig.ClientID)-3:]
				}
				return authConfig.ClientID
			}()
		}

		tokenList = append(tokenList, tokenData)
	}

	c.JSON(http.StatusOK, gin.H{
		"timestamp":     time.Now().Format(time.RFC3339),
		"total_tokens":  len(tokenList),
		"active_tokens": activeCount,
		"tokens":        tokenList,
		"pool_stats": map[string]any{
			"total_tokens":  len(configs),
			"active_tokens": activeCount,
		},
	})
}

func refreshSingleTokenByConfig(config auth.AuthConfig) (types.TokenInfo, error) {
	switch config.AuthType {
	case auth.AuthMethodSocial:
		return auth.RefreshSocialToken(config.RefreshToken)
	case auth.AuthMethodIdC:
		return auth.RefreshIdCToken(config)
	default:
		return types.TokenInfo{}, fmt.Errorf("不支持的认证类型: %s", config.AuthType)
	}
}

func createTokenPreview(token string) string {
	if len(token) <= 10 {
		return strings.Repeat("*", len(token))
	}
	suffix := token[len(token)-10:]
	return "***" + suffix
}

func maskEmail(email string) string {
	if email == "" {
		return ""
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}

	username := parts[0]
	domain := parts[1]

	var maskedUsername string
	if len(username) <= 4 {
		maskedUsername = strings.Repeat("*", len(username))
	} else {
		prefix := username[:2]
		suffix := username[len(username)-2:]
		middleLen := len(username) - 4
		maskedUsername = prefix + strings.Repeat("*", middleLen) + suffix
	}

	domainParts := strings.Split(domain, ".")
	var maskedDomain string

	if len(domainParts) == 1 {
		maskedDomain = strings.Repeat("*", len(domain))
	} else if len(domainParts) == 2 {
		maskedDomain = strings.Repeat("*", len(domainParts[0])) + "." + domainParts[1]
	} else {
		maskedParts := make([]string, len(domainParts))
		for i := 0; i < len(domainParts)-2; i++ {
			maskedParts[i] = strings.Repeat("*", len(domainParts[i]))
		}
		maskedParts[len(domainParts)-2] = domainParts[len(domainParts)-2]
		maskedParts[len(domainParts)-1] = domainParts[len(domainParts)-1]
		maskedDomain = strings.Join(maskedParts, ".")
	}

	return maskedUsername + "@" + maskedDomain
}

// handleTokenReload 处理token配置更新
func (h *Handler) handleTokenReload(c *gin.Context) {
	var newConfigs []auth.AuthConfig

	// 检查请求类型：JSON 或文件上传
	contentType := c.GetHeader("Content-Type")
	
	if strings.Contains(contentType, "multipart/form-data") {
		// 处理文件上传
		file, err := c.FormFile("config")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "未找到上传文件: " + err.Error(),
			})
			return
		}

		// 打开文件
		fileContent, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "无法打开文件: " + err.Error(),
			})
			return
		}
		defer fileContent.Close()

		// 读取文件内容
		data, err := io.ReadAll(fileContent)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "读取文件失败: " + err.Error(),
			})
			return
		}

		// 解析 JSON
		if err := json.Unmarshal(data, &newConfigs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "解析JSON失败: " + err.Error(),
			})
			return
		}
	} else {
		// 处理JSON请求体
		if err := c.ShouldBindJSON(&newConfigs); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   "解析请求失败: " + err.Error(),
			})
			return
		}
	}

	// 验证配置
	if len(newConfigs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "配置不能为空",
		})
		return
	}

	// 验证每个配置的必要字段
	for i, cfg := range newConfigs {
		if cfg.RefreshToken == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("配置 #%d: refreshToken 不能为空", i),
			})
			return
		}
		if cfg.AuthType != auth.AuthMethodSocial && cfg.AuthType != auth.AuthMethodIdC {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("配置 #%d: auth 必须是 'Social' 或 'IdC'", i),
			})
			return
		}
		if cfg.AuthType == auth.AuthMethodIdC && (cfg.ClientID == "" || cfg.ClientSecret == "") {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   fmt.Sprintf("配置 #%d: IdC 认证需要 clientId 和 clientSecret", i),
			})
			return
		}
	}

	logger.Info("收到token配置更新请求",
		logger.Int("new_config_count", len(newConfigs)),
		logger.String("content_type", contentType))

	// 执行热更新
	if err := h.tokenManager.ReloadConfigs(newConfigs); err != nil {
		logger.Error("token配置更新失败", logger.Err(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "更新失败: " + err.Error(),
		})
		return
	}

	logger.Info("token配置更新成功", logger.Int("config_count", len(newConfigs)))

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "配置更新成功",
		"config_count": len(newConfigs),
		"timestamp":    time.Now().Format(time.RFC3339),
	})
}

// handleTokenToggle 切换token启用/停用状态
func (h *Handler) handleTokenToggle(c *gin.Context) {
	var req struct {
		Index int `json:"index"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("解析toggle请求失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	logger.Info("收到toggle请求", logger.Int("index", req.Index))

	if err := h.tokenManager.ToggleTokenStatus(req.Index); err != nil {
		logger.Error("切换token状态失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logger.Info("token状态已切换", logger.Int("index", req.Index))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "状态切换成功",
	})
}

// handleTokenDelete 删除token
func (h *Handler) handleTokenDelete(c *gin.Context) {
	var req struct {
		Index int `json:"index"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Error("解析delete请求失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	logger.Info("收到delete请求", logger.Int("index", req.Index))

	if err := h.tokenManager.RemoveToken(req.Index); err != nil {
		logger.Error("删除token失败", logger.Err(err))
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logger.Info("token已删除", logger.Int("index", req.Index))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "删除成功",
	})
}

// handleExportTokens 导出token配置
func (h *Handler) handleExportTokens(c *gin.Context) {
	// 获取当前配置
	configs := h.tokenManager.GetCurrentConfigs()
	
	if len(configs) == 0 {
		c.JSON(http.StatusOK, []auth.AuthConfig{})
		return
	}
	
	logger.Info("导出token配置", logger.Int("count", len(configs)))
	
	// 直接返回配置数组（JSON格式）
	c.JSON(http.StatusOK, configs)
}

// handleRefreshAllTokens 刷新所有token的用量信息
func (h *Handler) handleRefreshAllTokens(c *gin.Context) {
	logger.Info("收到刷新全部token请求")

	refreshedCount, err := h.tokenManager.RefreshAllTokens()
	if err != nil {
		logger.Error("刷新token失败", logger.Err(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logger.Info("token刷新完成", logger.Int("refreshed_count", refreshedCount))

	c.JSON(http.StatusOK, gin.H{
		"success":         true,
		"message":         "刷新完成",
		"refreshed_count": refreshedCount,
	})
}

// handleCleanupTokens 清理失效token（过期或已耗尽）
func (h *Handler) handleCleanupTokens(c *gin.Context) {
	logger.Info("收到清理失效token请求")

	removedCount, err := h.tokenManager.CleanupInvalidTokens()
	if err != nil {
		logger.Error("清理token失败", logger.Err(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	logger.Info("token清理完成", logger.Int("removed_count", removedCount))

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "清理完成",
		"removed_count": removedCount,
	})
}
