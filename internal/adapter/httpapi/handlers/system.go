package handlers

import (
	"net/http"
	"os"
	"os/exec"
	"time"

	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

// handleRestartService 重启服务
// 云平台环境（Zeabur/Railway等）：通过退出进程让平台自动重启
// Docker Compose 环境：使用 docker restart 命令重启容器
func (h *Handler) handleRestartService(c *gin.Context) {
	logger.Warn("收到重启服务请求",
		logger.String("ip", c.ClientIP()))

	// 检测是否在云平台环境中运行
	isCloudPlatform := os.Getenv("ZEABUR") != "" || 
		os.Getenv("RAILWAY_ENVIRONMENT") != "" ||
		os.Getenv("KUBERNETES_SERVICE_HOST") != "" ||
		os.Getenv("FLY_APP_NAME") != "" ||
		os.Getenv("RENDER") != ""

	// 返回响应
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "服务将在2秒后重启...",
	})

	// 延迟2秒后重启（给响应时间发送）
	go func() {
		time.Sleep(2 * time.Second)
		
		if isCloudPlatform {
			// 云平台环境：直接退出进程，由平台自动重启
			logger.Warn("检测到云平台环境，退出进程由平台自动重启...")
			os.Exit(0)
		} else {
			// Docker Compose 环境：尝试使用 docker restart
			containerName := os.Getenv("CONTAINER_NAME")
			if containerName == "" {
				containerName = "kiro2api" // 默认容器名
			}
			
			logger.Warn("执行容器重启...", logger.String("container", containerName))
			cmd := exec.Command("docker", "restart", containerName)
			if err := cmd.Run(); err != nil {
				logger.Error("容器重启失败，回退到进程退出方式", logger.Err(err))
				os.Exit(0)
			}
		}
	}()
}

// handleGetSystemInfo 获取系统信息
func (h *Handler) handleGetSystemInfo(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"port":     os.Getenv("PORT"),
		"gin_mode": os.Getenv("GIN_MODE"),
		"version":  "1.0.0",
	})
}

