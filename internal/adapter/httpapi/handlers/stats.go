package handlers

import (
	"net/http"
	"strconv"

	"kiro2api/internal/stats"

	"github.com/gin-gonic/gin"
)

// handleGetStats 获取 token 使用统计
func (h *Handler) handleGetStats(c *gin.Context) {
	// 获取小时数参数，默认 24 小时
	hoursStr := c.DefaultQuery("hours", "24")
	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours < 1 || hours > 168 { // 最多 7 天
		hours = 24
	}

	collector := stats.GetCollector()

	// 获取每小时统计
	hourlyStats := collector.GetHourlyStats(hours)

	// 获取今日总计
	todayInput, todayOutput, todayRequests := collector.GetTodayTotal()

	c.JSON(http.StatusOK, gin.H{
		"hourly_stats": hourlyStats,
		"today_total": gin.H{
			"input_tokens":  todayInput,
			"output_tokens": todayOutput,
			"request_count": todayRequests,
		},
	})
}
