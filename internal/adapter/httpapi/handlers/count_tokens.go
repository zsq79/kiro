package handlers

import (
	"fmt"
	"net/http"

	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/logger"
	"kiro2api/types"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleCountTokens(c *gin.Context) {
	var req types.CountTokensRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Warn("token计数请求解析失败",
			logutil.AddFields(c,
				logger.Err(err),
			)...)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request body: %v", err),
			},
		})
		return
	}

	if !utils.IsValidClaudeModel(req.Model) {
		logger.Warn("无效的模型参数",
			logutil.AddFields(c,
				logger.String("model", req.Model),
			)...)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid model: %s", req.Model),
			},
		})
		return
	}

	estimator := utils.NewTokenEstimator()
	tokenCount := estimator.EstimateTokens(&req)

	c.JSON(http.StatusOK, types.CountTokensResponse{
		InputTokens: tokenCount,
	})
}
