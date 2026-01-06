package shared

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro2api/config"
	"kiro2api/converter"
	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/internal/adapter/httpapi/support"
	"kiro2api/logger"
	"kiro2api/types"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

type ReverseProxy struct {
	client         *http.Client
	headers        *HeaderManager
	stealthEnabled bool
}

func NewReverseProxy(client *http.Client) *ReverseProxy {
	if client == nil {
		client = utils.SharedHTTPClient
	}

	return &ReverseProxy{
		client:         client,
		headers:        NewHeaderManager(),
		stealthEnabled: config.IsStealthModeEnabled(),
	}
}

func (rp *ReverseProxy) Execute(c *gin.Context, anthropicReq types.AnthropicRequest, tokenInfo types.TokenInfo, isStream bool) (*http.Response, error) {
	req, err := rp.buildRequest(c, anthropicReq, tokenInfo, isStream)
	if err != nil {
		if _, ok := err.(*types.ModelNotFoundErrorType); ok {
			return nil, err
		}
		support.HandleRequestBuildError(c, err)
		return nil, err
	}

	if rp.stealthEnabled {
		time.Sleep(rp.randomJitter())
	}

	resp, err := rp.client.Do(req)
	if err != nil {
		support.HandleRequestSendError(c, err)
		return nil, err
	}

	if rp.handleCodeWhispererError(c, resp) {
		resp.Body.Close()
		return nil, fmt.Errorf("CodeWhisperer API error")
	}

	logger.Debug("上游响应成功",
		logutil.AddFields(c,
			logger.String("direction", "upstream_response"),
			logger.Int("status_code", resp.StatusCode),
		)...)

	return resp, nil
}

func (rp *ReverseProxy) buildRequest(c *gin.Context, anthropicReq types.AnthropicRequest, tokenInfo types.TokenInfo, isStream bool) (*http.Request, error) {
	cwReq, err := converter.BuildCodeWhispererRequest(anthropicReq, c)
	if err != nil {
		if modelNotFoundErr, ok := err.(*types.ModelNotFoundErrorType); ok {
			c.JSON(http.StatusBadRequest, modelNotFoundErr.ErrorData)
			return nil, err
		}
		return nil, fmt.Errorf("构建CodeWhisperer请求失败: %v", err)
	}

	cwReqBody, err := converter.MarshalCodeWhispererRequest(cwReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	var toolNamesPreview string
	if len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools) > 0 {
		names := make([]string, 0, len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools))
		for _, t := range cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools {
			if t.ToolSpecification.Name != "" {
				names = append(names, t.ToolSpecification.Name)
			}
		}
		toolNamesPreview = strings.Join(names, ",")
	}

	logger.Debug("发送给CodeWhisperer的请求",
		logger.String("direction", "upstream_request"),
		logger.Int("request_size", len(cwReqBody)),
		logger.String("request_body", string(cwReqBody)),
		logger.Int("tools_count", len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools)),
		logger.String("tools_names", toolNamesPreview))

	req, err := http.NewRequest("POST", config.CodeWhispererURL, bytes.NewReader(cwReqBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	if isStream {
		req.Header.Set("Accept", "text/event-stream")
	}

	// 使用 refreshToken 作为稳定的标识符（同一个 token 在一段时间内保持一致的用户画像）
	// refreshToken 是唯一且稳定的，适合作为用户标识
	tokenIdentifier := tokenInfo.RefreshToken
	if tokenIdentifier == "" {
		// 如果没有 refreshToken，使用 accessToken（虽然会变化，但总比没有好）
		tokenIdentifier = tokenInfo.AccessToken
	}
	
	rp.headers.Apply(req, isStream, tokenIdentifier)

	return req, nil
}

func (rp *ReverseProxy) randomJitter() time.Duration {
	base := utils.RandomIntBetween(5, 50)
	return time.Duration(base) * time.Millisecond
}

func (rp *ReverseProxy) handleCodeWhispererError(c *gin.Context, resp *http.Response) bool {
	if resp.StatusCode == http.StatusOK {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("读取错误响应失败",
			logutil.AddFields(c,
				logger.String("direction", "upstream_response"),
				logger.Err(err),
			)...)
		support.RespondError(c, http.StatusInternalServerError, "%s", "读取响应失败")
		return true
	}

	logger.Error("上游响应错误",
		logutil.AddFields(c,
			logger.String("direction", "upstream_response"),
			logger.Int("status_code", resp.StatusCode),
			logger.Int("response_len", len(body)),
			logger.String("response_body", string(body)),
		)...)

	if resp.StatusCode == http.StatusForbidden {
		logger.Warn("收到403错误，token可能已失效")
		support.RespondErrorWithCode(c, http.StatusUnauthorized, "unauthorized", "%s", "Token已失效，请重试")
		return true
	}

	errorMapper := NewErrorMapper()
	claudeError := errorMapper.MapCodeWhispererError(resp.StatusCode, body)

	if claudeError.StopReason == "max_tokens" {
		logger.Info("内容长度超限，映射为max_tokens stop_reason",
			logutil.AddFields(c,
				logger.String("upstream_reason", "CONTENT_LENGTH_EXCEEDS_THRESHOLD"),
				logger.String("claude_stop_reason", "max_tokens"),
			)...)
		errorMapper.SendClaudeError(c, claudeError)
	} else {
		support.RespondErrorWithCode(c, http.StatusInternalServerError, "cw_error", "CodeWhisperer Error: %s", string(body))
	}

	return true
}

func FilterSupportedTools(tools []types.AnthropicTool) []types.AnthropicTool {
	if len(tools) == 0 {
		return tools
	}

	filtered := make([]types.AnthropicTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "web_search" || tool.Name == "websearch" {
			logger.Debug("过滤不支持的工具（token计算）",
				logger.String("tool_name", tool.Name))
			continue
		}
		filtered = append(filtered, tool)
	}

	return filtered
}

type StreamEventSender interface {
	SendEvent(c *gin.Context, data any) error
	SendError(c *gin.Context, message string, err error) error
}

type AnthropicStreamSender struct{}

type OpenAIStreamSender struct{}

func (s *AnthropicStreamSender) SendEvent(c *gin.Context, data any) error {
	var eventType string

	if dataMap, ok := data.(map[string]any); ok {
		if t, exists := dataMap["type"]; exists {
			eventType = t.(string)
		}
	}

	json, err := utils.SafeMarshal(data)
	if err != nil {
		return err
	}

	logger.Debug("发送SSE事件",
		logutil.AddFields(c,
			logger.String("event", eventType),
			logger.String("payload_preview", string(json)),
		)...)

	fmt.Fprintf(c.Writer, "event: %s\n", eventType)
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(json))
	c.Writer.Flush()
	return nil
}

func (s *AnthropicStreamSender) SendError(c *gin.Context, message string, err error) error {
	logger.Error(message,
		logutil.AddFields(c,
			logger.Err(err),
		)...)

	errorEvent := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "overloaded_error",
			"message": message,
		},
	}

	return s.SendEvent(c, errorEvent)
}

func (s *OpenAIStreamSender) SendEvent(c *gin.Context, data any) error {
	json, err := utils.SafeMarshal(data)
	if err != nil {
		return err
	}

	logger.Debug("发送OpenAI兼容流",
		logutil.AddFields(c,
			logger.String("payload_preview", string(json)),
		)...)

	fmt.Fprintf(c.Writer, "data: %s\n\n", string(json))
	c.Writer.Flush()
	return nil
}

func (s *OpenAIStreamSender) SendError(c *gin.Context, message string, err error) error {
	logger.Error(message, logger.Err(err))
	errorEvent := map[string]any{
		"error": map[string]any{
			"message": message,
		},
	}

	return s.SendEvent(c, errorEvent)
}
