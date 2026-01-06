package request

import (
	"fmt"
	"net/http"

	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/internal/adapter/httpapi/support"
	"kiro2api/logger"
	"kiro2api/types"

	"github.com/gin-gonic/gin"
)

type TokenProvider interface {
	GetToken() (types.TokenInfo, error)
	GetTokenWithUsage() (*types.TokenWithUsage, error)
}

type Context struct {
	GinContext  *gin.Context
	AuthService TokenProvider
	RequestType string
}

func (rc *Context) GetTokenAndBody() (types.TokenInfo, []byte, error) {
	tokenInfo, err := rc.AuthService.GetToken()
	if err != nil {
		logger.Error("获取token失败", logger.Err(err))
		support.RespondError(rc.GinContext, http.StatusInternalServerError, "获取token失败: %v", err)
		return types.TokenInfo{}, nil, err
	}

	body, err := rc.GinContext.GetRawData()
	if err != nil {
		logger.Error("读取请求体失败", logger.Err(err))
		support.RespondError(rc.GinContext, http.StatusBadRequest, "读取请求体失败: %v", err)
		return types.TokenInfo{}, nil, err
	}

	logger.Debug(fmt.Sprintf("收到%s请求", rc.RequestType),
		logutil.AddFields(rc.GinContext,
			logger.String("direction", "client_request"),
			logger.String("body", string(body)),
			logger.Int("body_size", len(body)),
			logger.String("remote_addr", rc.GinContext.ClientIP()),
			logger.String("user_agent", rc.GinContext.GetHeader("User-Agent")),
		)...)

	return tokenInfo, body, nil
}

func (rc *Context) GetTokenWithUsageAndBody() (*types.TokenWithUsage, []byte, error) {
	tokenWithUsage, err := rc.AuthService.GetTokenWithUsage()
	if err != nil {
		logger.Error("获取token失败", logger.Err(err))
		support.RespondError(rc.GinContext, http.StatusInternalServerError, "获取token失败: %v", err)
		return nil, nil, err
	}

	body, err := rc.GinContext.GetRawData()
	if err != nil {
		logger.Error("读取请求体失败", logger.Err(err))
		support.RespondError(rc.GinContext, http.StatusBadRequest, "读取请求体失败: %v", err)
		return nil, nil, err
	}

	logger.Debug(fmt.Sprintf("收到%s请求", rc.RequestType),
		logutil.AddFields(rc.GinContext,
			logger.String("direction", "client_request"),
			logger.String("body", string(body)),
			logger.Int("body_size", len(body)),
			logger.String("remote_addr", rc.GinContext.ClientIP()),
			logger.String("user_agent", rc.GinContext.GetHeader("User-Agent")),
			logger.Float64("available_count", tokenWithUsage.AvailableCount),
		)...)

	return tokenWithUsage, body, nil
}
