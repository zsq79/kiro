package shared

import (
	"encoding/json"
	"fmt"
	"net/http"

	logutil "kiro2api/internal/adapter/httpapi/logging"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

type ErrorMappingStrategy interface {
	MapError(statusCode int, responseBody []byte) (*ClaudeErrorResponse, bool)
	GetErrorType() string
}

type ClaudeErrorResponse struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	StopReason string `json:"stop_reason,omitempty"`
}

type CodeWhispererErrorBody struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

type ContentLengthExceedsStrategy struct{}

func (s *ContentLengthExceedsStrategy) MapError(statusCode int, responseBody []byte) (*ClaudeErrorResponse, bool) {
	if statusCode != http.StatusBadRequest {
		return nil, false
	}

	var errorBody CodeWhispererErrorBody
	if err := json.Unmarshal(responseBody, &errorBody); err != nil {
		return nil, false
	}

	if errorBody.Reason == "CONTENT_LENGTH_EXCEEDS_THRESHOLD" {
		return &ClaudeErrorResponse{
			Type:       "message_delta",
			StopReason: "max_tokens",
			Message:    "Content length exceeds threshold, response truncated",
		}, true
	}

	return nil, false
}

func (s *ContentLengthExceedsStrategy) GetErrorType() string {
	return "content_length_exceeds"
}

type DefaultErrorStrategy struct{}

func (s *DefaultErrorStrategy) MapError(statusCode int, responseBody []byte) (*ClaudeErrorResponse, bool) {
	return &ClaudeErrorResponse{
		Type:    "error",
		Message: fmt.Sprintf("Upstream error: %s", string(responseBody)),
	}, true
}

func (s *DefaultErrorStrategy) GetErrorType() string {
	return "default"
}

type ErrorMapper struct {
	strategies []ErrorMappingStrategy
}

func NewErrorMapper() *ErrorMapper {
	return &ErrorMapper{
		strategies: []ErrorMappingStrategy{
			&ContentLengthExceedsStrategy{},
			&DefaultErrorStrategy{},
		},
	}
}

func (em *ErrorMapper) MapCodeWhispererError(statusCode int, responseBody []byte) *ClaudeErrorResponse {
	for _, strategy := range em.strategies {
		if response, handled := strategy.MapError(statusCode, responseBody); handled {
			logger.Debug("错误映射成功",
				logger.String("strategy", strategy.GetErrorType()),
				logger.Int("status_code", statusCode),
				logger.String("mapped_type", response.Type),
				logger.String("stop_reason", response.StopReason))
			return response
		}
	}

	return &ClaudeErrorResponse{
		Type:    "error",
		Message: "Unknown error",
	}
}

func (em *ErrorMapper) SendClaudeError(c *gin.Context, claudeError *ClaudeErrorResponse) {
	if claudeError.StopReason == "max_tokens" {
		em.sendMaxTokensResponse(c, claudeError)
	} else {
		em.sendStandardError(c, claudeError)
	}
}

func (em *ErrorMapper) sendMaxTokensResponse(c *gin.Context, claudeError *ClaudeErrorResponse) {
	response := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   "max_tokens",
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	}

	sender := &AnthropicStreamSender{}
	if err := sender.SendEvent(c, response); err != nil {
		logger.Error("发送max_tokens响应失败",
			logger.Err(err),
			logger.String("original_message", claudeError.Message))
	}

	logger.Info("已发送max_tokens stop_reason响应",
		logutil.AddFields(c,
			logger.String("stop_reason", "max_tokens"),
			logger.String("original_message", claudeError.Message))...)
}

func (em *ErrorMapper) sendStandardError(c *gin.Context, claudeError *ClaudeErrorResponse) {
	errorResp := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "overloaded_error",
			"message": claudeError.Message,
		},
	}

	sender := &AnthropicStreamSender{}
	if err := sender.SendEvent(c, errorResp); err != nil {
		logger.Error("发送标准错误响应失败", logger.Err(err))
	}
}
