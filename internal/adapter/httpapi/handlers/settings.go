package handlers

import (
	"net/http"
	"os"
	"strings"
	"time"

	"kiro2api/internal/adapter/httpapi/middleware"
	"kiro2api/internal/config"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

// handleGetSettings 获取当前环境配置
func (h *Handler) handleGetSettings(c *gin.Context) {
	settings := map[string]string{
		"ADMIN_TOKEN":                  os.Getenv("ADMIN_TOKEN"), // Dashboard访问Token
		"KIRO_CLIENT_TOKEN":            os.Getenv("KIRO_CLIENT_TOKEN"), // API访问Token
		"STEALTH_MODE":                 getEnvOrDefault("STEALTH_MODE", "true"),
		"HEADER_STRATEGY":              getEnvOrDefault("HEADER_STRATEGY", "real_simulation"),
		"STEALTH_HTTP2_MODE":           getEnvOrDefault("STEALTH_HTTP2_MODE", "auto"),
		"PORT":                         getEnvOrDefault("PORT", "8080"),
		"GIN_MODE":                     getEnvOrDefault("GIN_MODE", "release"),
		"LOG_LEVEL":                    getEnvOrDefault("LOG_LEVEL", "info"),
		"LOG_FORMAT":                   getEnvOrDefault("LOG_FORMAT", "json"),
		"LOG_CONSOLE":                  getEnvOrDefault("LOG_CONSOLE", "true"),
		"MAX_TOOL_DESCRIPTION_LENGTH":  getEnvOrDefault("MAX_TOOL_DESCRIPTION_LENGTH", "10000"),
	}

	c.JSON(http.StatusOK, settings)
}

// handleSaveSettings 保存系统配置到持久化文件
func (h *Handler) handleSaveSettings(c *gin.Context) {
	var settings map[string]string

	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误: " + err.Error(),
		})
		return
	}

	logger.Info("收到保存设置请求", logger.Any("settings_count", len(settings)))

	// 获取当前配置
	currentConfig := config.GetSystemConfig()

	// 更新配置（PORT 由 docker-compose.yml 管理，不持久化）
	newConfig := &config.SystemConfig{
		GinMode:        getSettingValue(settings, "GIN_MODE", currentConfig.GinMode),
		LogLevel:       getSettingValue(settings, "LOG_LEVEL", currentConfig.LogLevel),
		LogFormat:      getSettingValue(settings, "LOG_FORMAT", currentConfig.LogFormat),
		LogConsole:     getSettingValue(settings, "LOG_CONSOLE", currentConfig.LogConsole),
		StealthMode:    getSettingValue(settings, "STEALTH_MODE", currentConfig.StealthMode),
		HeaderStrategy: getSettingValue(settings, "HEADER_STRATEGY", currentConfig.HeaderStrategy),
		HTTP2Mode:      getSettingValue(settings, "STEALTH_HTTP2_MODE", currentConfig.HTTP2Mode),
		MaxToolLength:  getSettingValue(settings, "MAX_TOOL_DESCRIPTION_LENGTH", currentConfig.MaxToolLength),
	}

	// 处理 Token（如果包含*则不更新）
	adminTokenChanged := false
	if clientToken := settings["KIRO_CLIENT_TOKEN"]; clientToken != "" && !strings.Contains(clientToken, "*") {
		newConfig.ClientToken = clientToken
		logger.Info("KIRO_CLIENT_TOKEN已更新，立即生效")
	} else {
		newConfig.ClientToken = currentConfig.ClientToken
	}

	if adminToken := settings["ADMIN_TOKEN"]; adminToken != "" && !strings.Contains(adminToken, "*") {
		// 只有当新Token与当前Token不同时，才标记为已修改
		if adminToken != currentConfig.AdminToken {
			newConfig.AdminToken = adminToken
			middleware.UpdateAdminToken(adminToken)
			adminTokenChanged = true
			logger.Info("ADMIN_TOKEN已更新，用户需要重新登录")
		} else {
			newConfig.AdminToken = currentConfig.AdminToken
		}
	} else {
		newConfig.AdminToken = currentConfig.AdminToken
	}

	// 保存到持久化文件
	if err := config.SaveSystemConfig(newConfig); err != nil {
		logger.Error("保存系统配置失败", logger.Err(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "保存配置失败: " + err.Error(),
		})
		return
	}

	logger.Info("系统配置已保存到持久化文件")

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"message":             "配置保存成功",
		"restart_required":    false,  // 端口不再支持修改，其他配置都是热更新
		"admin_token_changed": adminTokenChanged,
		"timestamp":           time.Now().Format(time.RFC3339),
	})
}

// getSettingValue 获取设置值，如果为空则使用默认值
func getSettingValue(settings map[string]string, key, defaultValue string) string {
	if value, ok := settings[key]; ok && value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func maskToken(token string) string {
	if token == "" {
		return ""
	}
	if len(token) <= 10 {
		return strings.Repeat("*", len(token))
	}
	return strings.Repeat("*", len(token)-6) + token[len(token)-6:]
}

