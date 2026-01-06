package middleware

import (
	"net/http"
	"os"
	"strings"

	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

func PathBasedAuthMiddleware(authToken string, protectedPrefixes []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !requiresAuth(path, protectedPrefixes) {
			logger.Debug("跳过认证", logger.String("path", path))
			c.Next()
			return
		}

		// 🔥 热更新支持：动态读取环境变量
		// 优先使用环境变量的最新值，fallback到启动时的token
		currentToken := os.Getenv("KIRO_CLIENT_TOKEN")
		if currentToken == "" {
			currentToken = authToken // 使用启动时的值作为备用
		}

		if !validateAPIKey(c, currentToken) {
			c.Abort()
			return
		}

		c.Next()
	}
}

func requiresAuth(path string, protectedPrefixes []string) bool {
	for _, prefix := range protectedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func validateAPIKey(c *gin.Context, authToken string) bool {
	providedAPIKey := extractAPIKey(c)
	if providedAPIKey == "" {
		logger.Warn("请求缺少Authorization或x-api-key头")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "401"})
		return false
	}

	if providedAPIKey != authToken {
		logger.Error("authToken验证失败",
			logger.String("expected_suffix", maskTokenSuffix(authToken)),
			logger.String("provided_suffix", maskTokenSuffix(providedAPIKey)))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "401"})
		return false
	}

	return true
}

// maskTokenSuffix 只显示token的最后4位，用于调试
func maskTokenSuffix(token string) string {
	if len(token) <= 4 {
		return "***"
	}
	return "***" + token[len(token)-4:]
}

func extractAPIKey(c *gin.Context) string {
	apiKey := c.GetHeader("Authorization")
	if apiKey == "" {
		apiKey = c.GetHeader("x-api-key")
	} else {
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	}
	return apiKey
}
