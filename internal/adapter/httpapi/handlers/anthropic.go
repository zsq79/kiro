package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"kiro2api/internal/adapter/httpapi/request"
	"kiro2api/internal/adapter/httpapi/support"
	"kiro2api/logger"
	"kiro2api/types"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleAnthropicMessages(c *gin.Context) {
	reqCtx := &request.Context{
		GinContext:  c,
		AuthService: h.authService,
		RequestType: "Anthropic",
	}

	tokenWithUsage, body, err := reqCtx.GetTokenWithUsageAndBody()
	if err != nil {
		return
	}

	var rawReq map[string]any
	if err := utils.SafeUnmarshal(body, &rawReq); err != nil {
		logger.Error("解析请求体失败", logger.Err(err))
		support.RespondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
		return
	}

	if tools, exists := rawReq["tools"]; exists && tools != nil {
		if toolsArray, ok := tools.([]any); ok {
			normalizedTools := make([]map[string]any, 0, len(toolsArray))
			for _, tool := range toolsArray {
				if toolMap, ok := tool.(map[string]any); ok {
					if name, hasName := toolMap["name"]; hasName {
						if description, hasDesc := toolMap["description"]; hasDesc {
							if inputSchema, hasSchema := toolMap["input_schema"]; hasSchema {
								normalizedTool := map[string]any{
									"name":         name,
									"description":  description,
									"input_schema": inputSchema,
								}
								normalizedTools = append(normalizedTools, normalizedTool)
								continue
							}
						}
					}
					normalizedTools = append(normalizedTools, toolMap)
				}
			}
			rawReq["tools"] = normalizedTools
		}
	}

	normalizedBody, err := utils.SafeMarshal(rawReq)
	if err != nil {
		logger.Error("重新序列化请求失败", logger.Err(err))
		support.RespondError(c, http.StatusBadRequest, "处理请求格式失败: %v", err)
		return
	}

	var anthropicReq types.AnthropicRequest
	if err := utils.SafeUnmarshal(normalizedBody, &anthropicReq); err != nil {
		logger.Error("解析标准化请求体失败", logger.Err(err))
		support.RespondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
		return
	}

	if len(anthropicReq.Messages) == 0 {
		logger.Error("请求中没有消息")
		support.RespondError(c, http.StatusBadRequest, "%s", "messages 数组不能为空")
		return
	}

	lastMsg := anthropicReq.Messages[len(anthropicReq.Messages)-1]
	content, err := utils.GetMessageContent(lastMsg.Content)
	if err != nil {
		logger.Error("获取消息内容失败",
			logger.Err(err),
			logger.String("raw_content", fmt.Sprintf("%v", lastMsg.Content)))
		support.RespondError(c, http.StatusBadRequest, "获取消息内容失败: %v", err)
		return
	}

	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" || trimmedContent == "answer for user question" {
		logger.Error("消息内容为空或无效",
			logger.String("content", content),
			logger.String("trimmed_content", trimmedContent))
		support.RespondError(c, http.StatusBadRequest, "%s", "消息内容不能为空")
		return
	}

	if anthropicReq.Stream {
		h.gateway.HandleAnthropicStream(c, anthropicReq, tokenWithUsage)
		return
	}

	h.gateway.HandleAnthropicNonStream(c, anthropicReq, tokenWithUsage.TokenInfo)
}
