package openai

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"kiro2api/config"
	"kiro2api/converter"
	srvcontext "kiro2api/internal/adapter/httpapi/context"
	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/internal/adapter/httpapi/support"
	"kiro2api/internal/adapter/upstream/shared"
	"kiro2api/internal/stats"
	"kiro2api/logger"
	"kiro2api/parser"
	"kiro2api/types"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

type Proxy struct {
	reverseProxy *shared.ReverseProxy
}

func NewProxy(reverseProxy *shared.ReverseProxy) *Proxy {
	if reverseProxy == nil {
		reverseProxy = shared.NewReverseProxy(nil)
	}
	return &Proxy{reverseProxy: reverseProxy}
}

func (p *Proxy) HandleNonStream(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo) {
	resp, err := p.reverseProxy.Execute(c, anthropicReq, token, false)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	body, err := utils.ReadHTTPResponse(resp.Body)
	if err != nil {
		support.HandleResponseReadError(c, err)
		return
	}

	compliantParser := parser.NewCompliantEventStreamParser()
	result, err := compliantParser.ParseResponse(body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "响应解析失败"})
		return
	}

	contexts := []map[string]any{}
	allContent := result.GetCompletionText()
	sawToolUse := len(result.GetToolCalls()) > 0

	if allContent != "" {
		contexts = append(contexts, map[string]any{
			"type": "text",
			"text": allContent,
		})
	}

	for _, tool := range result.GetToolCalls() {
		contexts = append(contexts, map[string]any{
			"type":  "tool_use",
			"id":    tool.ID,
			"name":  tool.Name,
			"input": tool.Arguments,
		})
	}

	inputContent, _ := utils.GetMessageContent(anthropicReq.Messages[0].Content)
	stopReason := "end_turn"
	if sawToolUse {
		stopReason = "tool_use"
	}

	anthropicResp := map[string]any{
		"content":       contexts,
		"model":         anthropicReq.Model,
		"role":          "assistant",
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"type":          "message",
		"usage": map[string]any{
			"input_tokens":  len(inputContent),
			"output_tokens": len(allContent),
		},
	}

	openaiMessageID := fmt.Sprintf("chatcmpl-%s", time.Now().Format(config.MessageIDTimeFormat))
	openaiResp := converter.ConvertAnthropicToOpenAI(anthropicResp, anthropicReq.Model, openaiMessageID)

	// 记录 token 使用统计
	stats.GetCollector().Record(len(inputContent), len(allContent), anthropicReq.Model)

	logger.Debug("下发OpenAI非流式响应",
		logutil.AddFields(c,
			logger.String("direction", "downstream_send"),
			logger.Bool("saw_tool_use", sawToolUse),
		)...)
	c.JSON(http.StatusOK, openaiResp)
}

func (p *Proxy) HandleStream(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo) {
	if err := shared.InitializeSSEResponse(c); err != nil {
		support.RespondError(c, http.StatusInternalServerError, "%s", "流式响应初始化失败")
		return
	}

	messageID := fmt.Sprintf("chatcmpl-%s", time.Now().Format(config.MessageIDTimeFormat))
	srvcontext.SetMessageID(c, messageID)

	resp, err := p.reverseProxy.Execute(c, anthropicReq, token, true)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	c.Writer.Flush()

	sender := &shared.OpenAIStreamSender{}
	initialEvent := map[string]any{
		"id":      messageID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   anthropicReq.Model,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
				},
				"finish_reason": nil,
			},
		},
	}
	sender.SendEvent(c, initialEvent)

	compliantParser := parser.NewCompliantEventStreamParser()

	toolIndexByToolUseID := make(map[string]int)
	toolUseIDByBlockIndex := make(map[int]string)
	nextToolIndex := 0
	sawToolUse := false
	sentFinal := false

	totalBytesRead := 0
	messageCount := 0
	hasMoreData := true
	consecutiveErrors := 0
	const maxConsecutiveErrors = 3

	buf := make([]byte, 8192)
	for hasMoreData {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			totalBytesRead += n
			consecutiveErrors = 0

			events, parseErr := compliantParser.ParseStream(buf[:n])
			if parseErr != nil {
				continue
			}
			messageCount += len(events)
			for _, event := range events {
				if event.Data == nil {
					continue
				}
				dataMap, ok := event.Data.(map[string]any)
				if !ok {
					continue
				}

				switch dataMap["type"] {
				case "content_block_delta":
					p.handleContentBlockDelta(c, sender, anthropicReq, messageID, dataMap, toolIndexByToolUseID, toolUseIDByBlockIndex)
				case "content_block_start":
					if p.handleContentBlockStart(c, sender, anthropicReq, messageID, dataMap, toolIndexByToolUseID, toolUseIDByBlockIndex, &nextToolIndex) {
						sawToolUse = true
					}
				case "message_delta":
					if p.handleMessageDelta(c, sender, anthropicReq, messageID, dataMap) {
						sentFinal = true
					}
				case "content_block_stop":
					// ignore; final events handled in message_delta
				}
				c.Writer.Flush()
			}
		}

		if err != nil {
			if err == io.EOF {
				hasMoreData = false
			} else if err == io.ErrUnexpectedEOF {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					hasMoreData = false
				} else {
					select {
					case <-time.After(config.RetryDelay):
					case <-c.Request.Context().Done():
						hasMoreData = false
					}
				}
			} else {
				consecutiveErrors++
				if consecutiveErrors >= maxConsecutiveErrors {
					hasMoreData = false
				}
			}
		}
	}

	if !sentFinal && messageCount > 0 {
		finishReason := "stop"
		if sawToolUse {
			finishReason = "tool_calls"
		}

		finalEvent := map[string]any{
			"id":      messageID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   anthropicReq.Model,
			"choices": []map[string]any{
				{
					"index":         0,
					"delta":         map[string]any{},
					"finish_reason": finishReason,
				},
			},
		}
		sender.SendEvent(c, finalEvent)
		c.Writer.Flush()
	}

	fmt.Fprintf(c.Writer, "data: [DONE]\\n\\n")
	c.Writer.Flush()

	logger.Debug("OpenAI流式转发完成",
		logutil.AddFields(c,
			logger.Int("bytes_read", totalBytesRead),
			logger.Int("message_count", messageCount),
			logger.Bool("saw_tool_use", sawToolUse),
		)...)
}

func (p *Proxy) handleContentBlockDelta(
	c *gin.Context,
	sender *shared.OpenAIStreamSender,
	anthropicReq types.AnthropicRequest,
	messageID string,
	dataMap map[string]any,
	toolIndexByToolUseID map[string]int,
	toolUseIDByBlockIndex map[int]string,
) {
	delta, ok := dataMap["delta"].(map[string]any)
	if !ok {
		return
	}

	switch delta["type"] {
	case "text_delta":
		if text, ok := delta["text"].(string); ok {
			contentEvent := map[string]any{
				"id":      messageID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   anthropicReq.Model,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"content": text,
						},
						"finish_reason": nil,
					},
				},
			}
			sender.SendEvent(c, contentEvent)
		}
	case "input_json_delta":
		toolBlockIndex := 0
		if idxAny, ok := dataMap["index"]; ok {
			switch v := idxAny.(type) {
			case int:
				toolBlockIndex = v
			case int32:
				toolBlockIndex = int(v)
			case int64:
				toolBlockIndex = int(v)
			case float64:
				toolBlockIndex = int(v)
			}
		}

		toolUseID, ok := toolUseIDByBlockIndex[toolBlockIndex]
		if !ok {
			return
		}
		toolIdx, ok := toolIndexByToolUseID[toolUseID]
		if !ok {
			return
		}

		var partial string
		if pj, ok := delta["partial_json"]; ok {
			switch s := pj.(type) {
			case string:
				partial = s
			case *string:
				if s != nil {
					partial = *s
				}
			}
		}

		if partial == "" {
			return
		}

		toolDelta := map[string]any{
			"id":      messageID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   anthropicReq.Model,
			"choices": []map[string]any{
				{
					"index": 0,
					"delta": map[string]any{
						"tool_calls": []map[string]any{
							{
								"index": toolIdx,
								"type":  "function",
								"function": map[string]any{
									"arguments": partial,
								},
							},
						},
					},
					"finish_reason": nil,
				},
			},
		}
		sender.SendEvent(c, toolDelta)
	}
}

func (p *Proxy) handleContentBlockStart(
	c *gin.Context,
	sender *shared.OpenAIStreamSender,
	anthropicReq types.AnthropicRequest,
	messageID string,
	dataMap map[string]any,
	toolIndexByToolUseID map[string]int,
	toolUseIDByBlockIndex map[int]string,
	nextToolIndex *int,
) bool {
	contentBlock, ok := dataMap["content_block"].(map[string]any)
	if !ok {
		return false
	}
	blockType, _ := contentBlock["type"].(string)
	if blockType != "tool_use" {
		return false
	}

	toolUseID, _ := contentBlock["id"].(string)
	toolName, _ := contentBlock["name"].(string)
	if toolUseID == "" {
		return false
	}

	toolBlockIndex := 0
	if idxAny, ok := dataMap["index"]; ok {
		switch v := idxAny.(type) {
		case int:
			toolBlockIndex = v
		case int32:
			toolBlockIndex = int(v)
		case int64:
			toolBlockIndex = int(v)
		case float64:
			toolBlockIndex = int(v)
		}
	}

	if _, exists := toolIndexByToolUseID[toolUseID]; !exists {
		toolIndexByToolUseID[toolUseID] = *nextToolIndex
		*nextToolIndex++
	}
	toolUseIDByBlockIndex[toolBlockIndex] = toolUseID
	toolIdx := toolIndexByToolUseID[toolUseID]

	toolStart := map[string]any{
		"id":      messageID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   anthropicReq.Model,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"tool_calls": []map[string]any{
						{
							"index": toolIdx,
							"id":    toolUseID,
							"type":  "function",
							"function": map[string]any{
								"name":      toolName,
								"arguments": "",
							},
						},
					},
				},
				"finish_reason": nil,
			},
		},
	}
	sender.SendEvent(c, toolStart)
	return true
}

func (p *Proxy) handleMessageDelta(
	c *gin.Context,
	sender *shared.OpenAIStreamSender,
	anthropicReq types.AnthropicRequest,
	messageID string,
	dataMap map[string]any,
) bool {
	delta, ok := dataMap["delta"].(map[string]any)
	if !ok {
		return false
	}
	stopReason, _ := delta["stop_reason"].(string)
	if stopReason != "tool_use" {
		return false
	}

	endEvent := map[string]any{
		"id":      messageID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   anthropicReq.Model,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": "tool_calls",
			},
		},
	}
	sender.SendEvent(c, endEvent)
	return true
}
