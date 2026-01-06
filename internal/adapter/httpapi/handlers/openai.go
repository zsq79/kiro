package handlers

import (
	"net/http"

	"kiro2api/converter"
	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/internal/adapter/httpapi/request"
	"kiro2api/internal/adapter/httpapi/support"
	"kiro2api/logger"
	"kiro2api/types"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleOpenAICompletions(c *gin.Context) {
	reqCtx := &request.Context{
		GinContext:  c,
		AuthService: h.authService,
		RequestType: "OpenAI",
	}

	tokenInfo, body, err := reqCtx.GetTokenAndBody()
	if err != nil {
		return
	}

	var openaiReq types.OpenAIRequest
	if err := utils.SafeUnmarshal(body, &openaiReq); err != nil {
		logger.Error("解析OpenAI请求体失败", logger.Err(err))
		support.RespondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
		return
	}

	logger.Debug("OpenAI请求解析成功",
		logutil.AddFields(c,
			logger.String("model", openaiReq.Model),
			logger.Bool("stream", openaiReq.Stream != nil && *openaiReq.Stream),
			logger.Int("max_tokens", func() int {
				if openaiReq.MaxTokens != nil {
					return *openaiReq.MaxTokens
				}
				return 16384
			}()),
		)...)

	anthropicReq := converter.ConvertOpenAIToAnthropic(openaiReq)

	if anthropicReq.Stream {
		h.gateway.HandleOpenAIStream(c, anthropicReq, tokenInfo)
		return
	}

	h.gateway.HandleOpenAINonStream(c, anthropicReq, tokenInfo)
}
