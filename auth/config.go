package auth

import (
	"encoding/json"
	"fmt"
	"os"

	"kiro2api/logger"
)

// AuthConfig 简化的认证配置
type AuthConfig struct {
	AuthType     string `json:"auth"`
	RefreshToken string `json:"refreshToken"`
	ClientID     string `json:"clientId,omitempty"`
	ClientSecret string `json:"clientSecret,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

// 认证方法常量
const (
	AuthMethodSocial = "Social"
	AuthMethodIdC    = "IdC"
)

// loadConfigs 从环境变量或持久化文件加载配置
func loadConfigs() ([]AuthConfig, error) {
	// 🔥 优先从持久化文件加载（容器重启后配置不丢失）
	storage := NewConfigStorage()
	persistedConfigs, err := storage.Load()
	if err == nil && len(persistedConfigs) > 0 {
		logger.Info("从持久化文件加载配置",
			logger.Int("count", len(persistedConfigs)))
		return processConfigs(persistedConfigs), nil
	}

	// 如果持久化文件不存在或为空，从环境变量加载
	logger.Info("持久化文件不存在，从环境变量加载配置")

	// 检测并警告弃用的环境变量
	deprecatedVars := []string{
		"REFRESH_TOKEN",
		"AWS_REFRESHTOKEN",
		"IDC_REFRESH_TOKEN",
		"BULK_REFRESH_TOKENS",
	}

	for _, envVar := range deprecatedVars {
		if os.Getenv(envVar) != "" {
			logger.Warn("检测到已弃用的环境变量",
				logger.String("变量名", envVar),
				logger.String("迁移说明", "请迁移到KIRO_AUTH_TOKEN的JSON格式"))
			logger.Warn("迁移示例",
				logger.String("新格式", `KIRO_AUTH_TOKEN='[{"auth":"Social","refreshToken":"your_token"}]'`))
		}
	}

	// 只支持KIRO_AUTH_TOKEN的JSON格式（支持文件路径或JSON字符串）
	jsonData := os.Getenv("KIRO_AUTH_TOKEN")
	if jsonData == "" {
		// KIRO_AUTH_TOKEN为可选配置，没有时返回空配置
		logger.Info("暂无可用的Token配置",
			logger.String("提示", "请通过Web Dashboard添加Token配置"))
		logger.Info("配置方式",
			logger.String("访问地址", "http://localhost:8080"),
			logger.String("说明", "使用管理员密码登录后在Token管理页面添加配置"))
		return []AuthConfig{}, nil
	}

	// 优先尝试从文件加载，失败后再作为JSON字符串处理
	var configData string
	if fileInfo, err := os.Stat(jsonData); err == nil && !fileInfo.IsDir() {
		// 是文件，读取文件内容
		content, err := os.ReadFile(jsonData)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败: %w\n配置文件路径: %s", err, jsonData)
		}
		configData = string(content)
		logger.Info("从文件加载认证配置", logger.String("文件路径", jsonData))
	} else {
		// 不是文件或文件不存在，作为JSON字符串处理
		configData = jsonData
		logger.Debug("从环境变量加载JSON配置")
	}

	// 解析JSON配置
	configs, err := parseJSONConfig(configData)
	if err != nil {
		return nil, fmt.Errorf("解析KIRO_AUTH_TOKEN失败: %w\n"+
			"请检查JSON格式是否正确\n"+
			"示例: KIRO_AUTH_TOKEN='[{\"auth\":\"Social\",\"refreshToken\":\"token1\"}]'", err)
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("KIRO_AUTH_TOKEN配置为空，请至少提供一个有效的认证配置")
	}

	validConfigs := processConfigs(configs)
	if len(validConfigs) == 0 {
		return nil, fmt.Errorf("没有有效的认证配置\n" +
			"请检查: \n" +
			"1. Social认证需要refreshToken字段\n" +
			"2. IdC认证需要refreshToken、clientId、clientSecret字段")
	}

	logger.Info("成功加载认证配置",
		logger.Int("总配置数", len(configs)),
		logger.Int("有效配置数", len(validConfigs)))

	// 🔥 首次从环境变量加载后，保存到持久化文件（下次重启直接用）
	if err := storage.Save(validConfigs); err != nil {
		logger.Warn("保存初始配置到持久化文件失败（不影响运行）",
			logger.Err(err))
	}

	return validConfigs, nil
}

// GetConfigs 公开的配置获取函数，供其他包调用
func GetConfigs() ([]AuthConfig, error) {
	return loadConfigs()
}

// parseJSONConfig 解析JSON配置字符串
func parseJSONConfig(jsonData string) ([]AuthConfig, error) {
	var configs []AuthConfig

	// 尝试解析为数组
	if err := json.Unmarshal([]byte(jsonData), &configs); err != nil {
		// 尝试解析为单个对象
		var single AuthConfig
		if err := json.Unmarshal([]byte(jsonData), &single); err != nil {
			return nil, fmt.Errorf("JSON格式无效: %w", err)
		}
		configs = []AuthConfig{single}
	}

	return configs, nil
}

// processConfigs 处理和验证配置
func processConfigs(configs []AuthConfig) []AuthConfig {
	var validConfigs []AuthConfig

	for i, config := range configs {
		// 验证必要字段
		if config.RefreshToken == "" {
			continue
		}

		// 设置默认认证类型
		if config.AuthType == "" {
			config.AuthType = AuthMethodSocial
		}

		// 验证IdC认证的必要字段
		if config.AuthType == AuthMethodIdC {
			if config.ClientID == "" || config.ClientSecret == "" {
				continue
			}
		}

		// 跳过禁用的配置
		if config.Disabled {
			continue
		}

		validConfigs = append(validConfigs, config)
		_ = i // 避免未使用变量警告
	}

	return validConfigs
}
