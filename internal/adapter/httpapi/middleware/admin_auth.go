package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"

	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

var (
	// 管理员Token（运行时可变）
	currentAdminToken string
)

// InitAdminToken 初始化管理员Token
func InitAdminToken() string {
	enabled := strings.ToLower(os.Getenv("ADMIN_TOKEN_ENABLED"))
	if enabled != "true" && enabled != "1" && enabled != "yes" {
		logger.Info("管理员Token功能未启用")
		return ""
	}

	token := os.Getenv("ADMIN_TOKEN")
	
	// 如果启用但未设置，自动生成随机token
	if token == "" {
		token = generateRandomToken(32)
		logger.Warn("⚠️ 管理员Token未设置，已自动生成随机Token")
		logger.Warn("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Warn("🔑 管理员Token（请妥善保存）: " + token)
		logger.Warn("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		logger.Warn("建议：将此Token保存到.env文件中: ADMIN_TOKEN=" + token)
		
		// 自动设置到环境变量（供后续使用）
		os.Setenv("ADMIN_TOKEN", token)
	} else {
		logger.Info("管理员Token已启用", 
			logger.String("token_preview", "***"+token[len(token)-6:]))
	}
	
	currentAdminToken = token
	return token
}

// AdminAuthMiddleware Dashboard管理员认证中间件
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否启用管理员认证
		if currentAdminToken == "" {
			c.Next()
			return
		}

		path := c.Request.URL.Path
		
		// API端点不需要管理员认证（使用各自的认证机制）
		if strings.HasPrefix(path, "/v1/") {
			c.Next()
			return
		}

		// 登录相关路径和静态资源不需要认证
		if path == "/api/admin/login" || 
		   path == "/api/admin/status" || 
		   path == "/login" ||
		   strings.HasPrefix(path, "/static/") {  // 静态资源不需要认证
			c.Next()
			return
		}

		// 验证管理员Token
		adminToken := c.GetHeader("X-Admin-Token")
		if adminToken == "" {
			// 检查cookie
			adminToken, _ = c.Cookie("admin_token")
		}

		// 动态读取最新的管理员Token（支持热更新）
		expectedToken := os.Getenv("ADMIN_TOKEN")
		if expectedToken == "" {
			expectedToken = currentAdminToken
		}

		if adminToken != expectedToken {
			// Dashboard相关路径需要认证
			if path == "/" || strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/api/") {
				// HTML页面请求：重定向到登录页
				if c.GetHeader("Accept") != "" && strings.Contains(c.GetHeader("Accept"), "text/html") {
					c.Redirect(http.StatusFound, "/login")
					c.Abort()
					return
				}
				
				// API请求：返回401
				c.JSON(http.StatusUnauthorized, gin.H{
					"error":   "unauthorized",
					"message": "需要管理员认证",
				})
				c.Abort()
				return
			}
		}

		c.Next()
	}
}

// UpdateAdminToken 更新管理员Token（热更新）
func UpdateAdminToken(newToken string) {
	currentAdminToken = newToken
	os.Setenv("ADMIN_TOKEN", newToken)
	logger.Info("管理员Token已更新")
}

// GetAdminToken 获取当前管理员Token
func GetAdminToken() string {
	token := os.Getenv("ADMIN_TOKEN")
	if token == "" {
		return currentAdminToken
	}
	return token
}

// generateRandomToken 生成随机token
func generateRandomToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// fallback到简单生成
		return hex.EncodeToString([]byte("admin-token-" + hex.EncodeToString(bytes[:8])))
	}
	return hex.EncodeToString(bytes)
}

