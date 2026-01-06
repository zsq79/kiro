package handlers

import (
	"net/http"

	"kiro2api/internal/adapter/httpapi/middleware"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

// handleAdminLogin 管理员登录
func (h *Handler) handleAdminLogin(c *gin.Context) {
	var req struct {
		Token string `json:"token"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "请求参数错误",
		})
		return
	}

	// 验证Token
	expectedToken := middleware.GetAdminToken()
	if expectedToken == "" {
		// 管理员认证未启用
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "管理员认证未启用",
		})
		return
	}

	if req.Token != expectedToken {
		logger.Warn("管理员登录失败：Token错误",
			logger.String("ip", c.ClientIP()))
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   "Token错误",
		})
		return
	}

	logger.Info("管理员登录成功",
		logger.String("ip", c.ClientIP()))

	// 设置cookie（7天有效）
	c.SetCookie("admin_token", req.Token, 7*24*3600, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "登录成功",
	})
}

// handleAdminLogout 管理员登出
func (h *Handler) handleAdminLogout(c *gin.Context) {
	// 清除cookie
	c.SetCookie("admin_token", "", -1, "/", "", false, true)

	logger.Info("管理员已登出",
		logger.String("ip", c.ClientIP()))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已登出",
	})
}

// handleAdminStatus 检查管理员认证状态
func (h *Handler) handleAdminStatus(c *gin.Context) {
	expectedToken := middleware.GetAdminToken()
	enabled := expectedToken != ""
	
	// 检查是否已登录（验证token是否正确）
	loggedIn := false
	if enabled {
		adminToken := c.GetHeader("X-Admin-Token")
		if adminToken == "" {
			adminToken, _ = c.Cookie("admin_token")
		}
		loggedIn = adminToken == expectedToken
	}
	
	c.JSON(http.StatusOK, gin.H{
		"enabled": enabled,
		"logged_in": loggedIn,
	})
}

