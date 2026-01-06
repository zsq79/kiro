package shared

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestContentLengthExceedsStrategy_MapError(t *testing.T) {
	strategy := &ContentLengthExceedsStrategy{}

	response := []byte(`{"message":"Content length exceeds threshold","reason":"CONTENT_LENGTH_EXCEEDS_THRESHOLD"}`)
	mapped, handled := strategy.MapError(http.StatusBadRequest, response)
	assert.True(t, handled)
	assert.Equal(t, "message_delta", mapped.Type)
	assert.Equal(t, "max_tokens", mapped.StopReason)
}

func TestDefaultErrorStrategy_MapError(t *testing.T) {
	strategy := &DefaultErrorStrategy{}

	mapped, handled := strategy.MapError(http.StatusInternalServerError, []byte(`{"error":"internal"}`))
	assert.True(t, handled)
	assert.Equal(t, "error", mapped.Type)
	assert.Contains(t, mapped.Message, "Upstream error")
}

func TestErrorMapper_SendClaudeError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mapper := NewErrorMapper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	mapper.SendClaudeError(c, &ClaudeErrorResponse{
		Type:       "message_delta",
		StopReason: "max_tokens",
	})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "message_delta")
}

func TestHandleCodeWhispererError_Default(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	buf := &bytes.Buffer{}
	json.NewEncoder(buf).Encode(map[string]any{"error": "bad gateway"})
	resp := &http.Response{
		StatusCode: http.StatusBadGateway,
		Body:       io.NopCloser(bytes.NewReader(buf.Bytes())),
	}

	rp := NewReverseProxy(nil)
	handled := rp.handleCodeWhispererError(c, resp)

	assert.True(t, handled)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
