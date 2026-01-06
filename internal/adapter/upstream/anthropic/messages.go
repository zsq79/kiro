package anthropic

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro2api/config"
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

func (p *Proxy) HandleStream(c *gin.Context, anthropicReq types.AnthropicRequest, tokenWithUsage *types.TokenWithUsage) {
	sender := &shared.AnthropicStreamSender{}
	p.handleGenericStream(c, anthropicReq, tokenWithUsage, sender, createAnthropicStreamEvents)
}

func (p *Proxy) handleGenericStream(
	c *gin.Context,
	anthropicReq types.AnthropicRequest,
	token *types.TokenWithUsage,
	sender shared.StreamEventSender,
	eventCreator func(string, int, string) []map[string]any,
) {
	estimator := utils.NewTokenEstimator()
	countReq := &types.CountTokensRequest{
		Model:    anthropicReq.Model,
		System:   anthropicReq.System,
		Messages: anthropicReq.Messages,
		Tools:    shared.FilterSupportedTools(anthropicReq.Tools),
	}
	inputTokens := estimator.EstimateTokens(countReq)

	if err := shared.InitializeSSEResponse(c); err != nil {
		_ = sender.SendError(c, "连接不支持SSE刷新", err)
		return
	}

	messageID := fmt.Sprintf(config.MessageIDFormat, time.Now().Format(config.MessageIDTimeFormat))
	srvcontext.SetMessageID(c, messageID)

	resp, err := p.reverseProxy.Execute(c, anthropicReq, token.TokenInfo, true)
	if err != nil {
		var modelNotFoundErrorType *types.ModelNotFoundErrorType
		if errors.As(err, &modelNotFoundErrorType) {
			return
		}
		_ = sender.SendError(c, "构建请求失败", err)
		return
	}
	defer resp.Body.Close()

	ctx := shared.NewStreamProcessorContext(c, anthropicReq, token, sender, messageID, inputTokens)
	defer ctx.Cleanup()

	if err := ctx.SendInitialEvents(eventCreator); err != nil {
		return
	}

	processor := shared.NewEventStreamProcessor(ctx)
	if err := processor.ProcessEventStream(resp.Body); err != nil {
		logger.Error("事件流处理失败", logger.Err(err))
		return
	}

	if err := ctx.SendFinalEvents(); err != nil {
		logger.Error("发送结束事件失败", logger.Err(err))
		return
	}
}

func createAnthropicStreamEvents(messageID string, inputTokens int, model string) []map[string]any {
	events := []map[string]any{
		{
			"type": "message_start",
			"message": map[string]any{
				"id":            messageID,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  inputTokens,
					"output_tokens": 0,
				},
			},
		},
		{
			"type": "ping",
		},
	}
	return events
}

func (p *Proxy) HandleNonStream(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo) {
	estimator := utils.NewTokenEstimator()
	countReq := &types.CountTokensRequest{
		Model:    anthropicReq.Model,
		System:   anthropicReq.System,
		Messages: anthropicReq.Messages,
		Tools:    shared.FilterSupportedTools(anthropicReq.Tools),
	}
	inputTokens := estimator.EstimateTokens(countReq)

	resp, err := p.reverseProxy.Execute(c, anthropicReq, token, false)
	if err != nil {
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := utils.ReadHTTPResponse(resp.Body)
	if err != nil {
		support.HandleResponseReadError(c, err)
		return
	}

	compliantParser := parser.NewCompliantEventStreamParser()
	compliantParser.SetMaxErrors(config.ParserMaxErrors)

	result, err := func() (*parser.ParseResult, error) {
		done := make(chan struct{})
		var result *parser.ParseResult
		var err error

		go func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("解析器panic: %v", r)
				}
				close(done)
			}()
			result, err = compliantParser.ParseResponse(body)
		}()

		select {
		case <-done:
			return result, err
		case <-time.After(10 * time.Second):
			logger.Error("非流式解析超时")
			return nil, fmt.Errorf("解析超时")
		}
	}()

	if err != nil {
		logger.Error("非流式解析失败",
			logger.Err(err),
			logger.String("model", anthropicReq.Model),
			logger.Int("response_size", len(body)))

		errorResp := gin.H{
			"error": gin.H{
				"error":   "响应解析失败",
				"type":    "parsing_error",
				"message": "无法解析AWS CodeWhisperer响应格式",
			},
		}

		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "解析超时") {
			statusCode = http.StatusRequestTimeout
			errorResp["error"].(gin.H)["message"] = "请求处理超时，请稍后重试"
		} else if strings.Contains(err.Error(), "格式错误") {
			statusCode = http.StatusBadRequest
			errorResp["error"].(gin.H)["message"] = "请求格式不正确"
		}

		c.JSON(statusCode, errorResp)
		return
	}

	contexts := []map[string]any{}
	textAgg := result.GetCompletionText()

	toolManager := compliantParser.GetToolManager()
	allTools := make([]*parser.ToolExecution, 0)
	for _, tool := range toolManager.GetActiveTools() {
		allTools = append(allTools, tool)
	}
	for _, tool := range toolManager.GetCompletedTools() {
		allTools = append(allTools, tool)
	}

	sawToolUse := len(allTools) > 0

	if textAgg != "" {
		contexts = append(contexts, map[string]any{
			"type": "text",
			"text": textAgg,
		})
	}

	for _, tool := range allTools {
		toolUseBlock := map[string]any{
			"type":  "tool_use",
			"id":    tool.ID,
			"name":  tool.Name,
			"input": tool.Arguments,
		}
		if tool.Arguments == nil {
			toolUseBlock["input"] = map[string]any{}
		}
		contexts = append(contexts, toolUseBlock)
	}

	stopReasonManager := shared.NewStopReasonManager(anthropicReq)

	outputTokens := 0
	for _, contentBlock := range contexts {
		blockType, _ := contentBlock["type"].(string)

		switch blockType {
		case "text":
			if text, ok := contentBlock["text"].(string); ok {
				outputTokens += estimator.EstimateTextTokens(text)
			}
		case "tool_use":
			toolName, _ := contentBlock["name"].(string)
			toolInput, _ := contentBlock["input"].(map[string]any)
			outputTokens += estimator.EstimateToolUseTokens(toolName, toolInput)
		}
	}

	if outputTokens < 1 && len(contexts) > 0 {
		outputTokens = 1
	}

	stopReasonManager.UpdateToolCallStatus(sawToolUse, sawToolUse)
	stopReason := stopReasonManager.DetermineStopReason()

	anthropicResp := map[string]any{
		"content":       contexts,
		"model":         anthropicReq.Model,
		"role":          "assistant",
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"type":          "message",
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}

	logger.Debug("下发非流式响应",
		logutil.AddFields(c,
			logger.String("direction", "downstream_send"),
			logger.Any("contexts", contexts),
			logger.Bool("saw_tool_use", sawToolUse),
			logger.Int("content_count", len(contexts)),
		)...)

	// 记录 token 使用统计
	stats.GetCollector().Record(inputTokens, outputTokens, anthropicReq.Model)

	c.JSON(http.StatusOK, anthropicResp)
}
