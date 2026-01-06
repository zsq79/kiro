package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kiro2api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHandleCountTokens_SimpleRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	request := types.CountTokensRequest{
		Model: "claude-sonnet-4-20250514",
		Messages: []types.AnthropicRequestMessage{
			{
				Role:    "user",
				Content: "Hello, how are you?",
			},
		},
	}

	jsonBytes, err := json.Marshal(request)
	assert.NoError(t, err)

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(jsonBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := &Handler{}
	handler.handleCountTokens(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response types.CountTokensResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Greater(t, response.InputTokens, 0)
}

func TestHandleCountTokens_InvalidModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	request := types.CountTokensRequest{
		Model: "invalid-model",
	}

	jsonBytes, err := json.Marshal(request)
	assert.NoError(t, err)

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(jsonBytes))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := &Handler{}
	handler.handleCountTokens(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}
