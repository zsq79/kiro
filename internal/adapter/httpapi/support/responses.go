package support

import (
	"fmt"
	"net/http"

	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

func RespondErrorWithCode(c *gin.Context, statusCode int, code string, format string, args ...any) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": fmt.Sprintf(format, args...),
			"code":    code,
		},
	})
}

func RespondError(c *gin.Context, statusCode int, format string, args ...any) {
	var code string
	switch statusCode {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusUnauthorized:
		code = "unauthorized"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	default:
		code = "internal_error"
	}
	RespondErrorWithCode(c, statusCode, code, format, args...)
}

func HandleRequestBuildError(c *gin.Context, err error) {
	logger.Error("构建请求失败", logutil.AddFields(c, logger.Err(err))...)
	RespondError(c, http.StatusInternalServerError, "构建请求失败: %v", err)
}

func HandleRequestSendError(c *gin.Context, err error) {
	logger.Error("发送请求失败", logutil.AddFields(c, logger.Err(err))...)
	RespondError(c, http.StatusInternalServerError, "发送请求失败: %v", err)
}

func HandleResponseReadError(c *gin.Context, err error) {
	logger.Error("读取响应体失败", logutil.AddFields(c, logger.Err(err))...)
	RespondError(c, http.StatusInternalServerError, "读取响应体失败: %v", err)
}
