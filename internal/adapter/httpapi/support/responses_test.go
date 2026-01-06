package support

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRespondError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		format         string
		args           []any
		expectedCode   string
		expectedStatus int
	}{
		{
			name:           "BadRequest错误",
			statusCode:     http.StatusBadRequest,
			format:         "无效的请求参数",
			expectedCode:   "bad_request",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Unauthorized错误",
			statusCode:     http.StatusUnauthorized,
			format:         "认证失败",
			expectedCode:   "unauthorized",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "InternalServerError错误",
			statusCode:     http.StatusInternalServerError,
			format:         "服务器内部错误: %v",
			args:           []any{"数据库连接失败"},
			expectedCode:   "internal_error",
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			RespondError(c, tt.statusCode, tt.format, tt.args...)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			errorObj, ok := response["error"].(map[string]any)
			assert.True(t, ok)
			assert.Equal(t, tt.expectedCode, errorObj["code"])
			assert.NotEmpty(t, errorObj["message"])
		})
	}
}
