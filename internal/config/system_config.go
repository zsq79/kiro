package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"kiro2api/logger"
)

const SystemConfigFile = "data/system_config.json"

// SystemConfig 系统配置（端口由 docker-compose.yml 管理，不持久化）
type SystemConfig struct {
	GinMode         string `json:"gin_mode"`          // Gin运行模式
	LogLevel        string `json:"log_level"`         // 日志级别
	LogFormat       string `json:"log_format"`        // 日志格式
	LogConsole      string `json:"log_console"`       // 控制台输出
	StealthMode     string `json:"stealth_mode"`      // 隐身模式
	HeaderStrategy  string `json:"header_strategy"`   // 请求头策略
	HTTP2Mode       string `json:"http2_mode"`        // HTTP/2模式
	MaxToolLength   string `json:"max_tool_length"`   // 工具描述最大长度
	ClientToken     string `json:"client_token"`      // 客户端Token
	AdminToken      string `json:"admin_token"`       // 管理员Token
}

var (
	systemConfig *SystemConfig
	configMutex  sync.RWMutex
)

// LoadSystemConfig 加载系统配置
func LoadSystemConfig() *SystemConfig {
	configMutex.Lock()
	defer configMutex.Unlock()

	// 如果已经加载过，直接返回
	if systemConfig != nil {
		return systemConfig
	}

	systemConfig = &SystemConfig{}

	// 尝试从文件加载
	if data, err := os.ReadFile(SystemConfigFile); err == nil {
		if err := json.Unmarshal(data, systemConfig); err == nil {
			logger.Info("从持久化文件加载系统配置成功",
				logger.String("file", SystemConfigFile))
			
			// 应用配置到环境变量
			applyConfigToEnv(systemConfig)
			return systemConfig
		}
	}

	// 如果文件不存在或加载失败，从环境变量初始化
	logger.Info("配置文件不存在，从环境变量初始化")
	systemConfig = loadFromEnv()
	
	// 尝试保存到文件（如果失败也不影响启动）
	go func() {
		if err := SaveSystemConfig(systemConfig); err != nil {
			logger.Warn("保存系统配置失败（不影响启动）", logger.Err(err))
		} else {
			logger.Info("系统配置已保存到持久化文件",
				logger.String("file", SystemConfigFile))
		}
	}()

	return systemConfig
}

// loadFromEnv 从环境变量加载配置（端口不持久化）
func loadFromEnv() *SystemConfig {
	return &SystemConfig{
		GinMode:        getEnvOrDefault("GIN_MODE", "release"),
		LogLevel:       getEnvOrDefault("LOG_LEVEL", "info"),
		LogFormat:      getEnvOrDefault("LOG_FORMAT", "json"),
		LogConsole:     getEnvOrDefault("LOG_CONSOLE", "true"),
		StealthMode:    getEnvOrDefault("STEALTH_MODE", "true"),
		HeaderStrategy: getEnvOrDefault("HEADER_STRATEGY", "real_simulation"),
		HTTP2Mode:      getEnvOrDefault("STEALTH_HTTP2_MODE", "auto"),
		MaxToolLength:  getEnvOrDefault("MAX_TOOL_DESCRIPTION_LENGTH", "10000"),
		ClientToken:    os.Getenv("KIRO_CLIENT_TOKEN"),
		AdminToken:     os.Getenv("ADMIN_TOKEN"),
	}
}

// applyConfigToEnv 将配置应用到环境变量（端口不持久化，保持环境变量原值）
func applyConfigToEnv(cfg *SystemConfig) {
	if cfg.GinMode != "" {
		os.Setenv("GIN_MODE", cfg.GinMode)
	}
	if cfg.LogLevel != "" {
		os.Setenv("LOG_LEVEL", cfg.LogLevel)
	}
	if cfg.LogFormat != "" {
		os.Setenv("LOG_FORMAT", cfg.LogFormat)
	}
	if cfg.LogConsole != "" {
		os.Setenv("LOG_CONSOLE", cfg.LogConsole)
	}
	if cfg.StealthMode != "" {
		os.Setenv("STEALTH_MODE", cfg.StealthMode)
	}
	if cfg.HeaderStrategy != "" {
		os.Setenv("HEADER_STRATEGY", cfg.HeaderStrategy)
	}
	if cfg.HTTP2Mode != "" {
		os.Setenv("STEALTH_HTTP2_MODE", cfg.HTTP2Mode)
	}
	if cfg.MaxToolLength != "" {
		os.Setenv("MAX_TOOL_DESCRIPTION_LENGTH", cfg.MaxToolLength)
	}
	if cfg.ClientToken != "" {
		os.Setenv("KIRO_CLIENT_TOKEN", cfg.ClientToken)
	}
	if cfg.AdminToken != "" {
		os.Setenv("ADMIN_TOKEN", cfg.AdminToken)
	}
}

// SaveSystemConfig 保存系统配置
func SaveSystemConfig(cfg *SystemConfig) error {
	configMutex.Lock()
	defer configMutex.Unlock()

	// 确保目录存在
	dir := filepath.Dir(SystemConfigFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	// 序列化为JSON
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	// 写入文件
	if err := os.WriteFile(SystemConfigFile, data, 0644); err != nil {
		return err
	}

	// 更新内存中的配置
	systemConfig = cfg
	
	// 应用到环境变量
	applyConfigToEnv(cfg)

	logger.Info("系统配置已保存",
		logger.String("file", SystemConfigFile))

	return nil
}

// GetSystemConfig 获取当前系统配置
func GetSystemConfig() *SystemConfig {
	configMutex.RLock()
	defer configMutex.RUnlock()

	if systemConfig == nil {
		return loadFromEnv()
	}

	return systemConfig
}

// UpdateSystemConfig 更新系统配置
func UpdateSystemConfig(cfg *SystemConfig) error {
	return SaveSystemConfig(cfg)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

