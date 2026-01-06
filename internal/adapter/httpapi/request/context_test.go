package request

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"kiro2api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

type mockAuthService struct {
	token      types.TokenInfo
	tokenUsage *types.TokenWithUsage
	err        error
}

func (m *mockAuthService) GetToken() (types.TokenInfo, error) {
	return m.token, m.err
}

func (m *mockAuthService) GetTokenWithUsage() (*types.TokenWithUsage, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.tokenUsage != nil {
		return m.tokenUsage, nil
	}
	return &types.TokenWithUsage{
		TokenInfo:      m.token,
		AvailableCount: 100,
	}, nil
}

func TestContext_GetTokenAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		mockToken     types.TokenInfo
		mockError     error
		requestBody   string
		expectError   bool
		expectedToken types.TokenInfo
	}{
		{
			name: "成功获取token和body",
			mockToken: types.TokenInfo{
				AccessToken: "test-token-123",
			},
			requestBody:   `{"test": "data"}`,
			expectError:   false,
			expectedToken: types.TokenInfo{AccessToken: "test-token-123"},
		},
		{
			name:        "获取token失败",
			mockError:   assert.AnError,
			requestBody: `{"test": "data"}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/test", bytes.NewBufferString(tt.requestBody))

			mockAuth := &mockAuthService{
				token: tt.mockToken,
				err:   tt.mockError,
			}

			reqCtx := &Context{
				GinContext:  c,
				AuthService: mockAuth,
				RequestType: "test",
			}

			tokenInfo, body, err := reqCtx.GetTokenAndBody()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedToken.AccessToken, tokenInfo.AccessToken)
				assert.NotNil(t, body)
			}
		})
	}
}

func TestContext_GetTokenWithUsageAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		tokenUsage  *types.TokenWithUsage
		err         error
		expectError bool
	}{
		{
			name: "成功获取token usage",
			tokenUsage: &types.TokenWithUsage{
				TokenInfo: types.TokenInfo{AccessToken: "usage-token"},
			},
		},
		{
			name:        "获取token usage失败",
			err:         assert.AnError,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"test": "data"}`))

			mockAuth := &mockAuthService{
				tokenUsage: tt.tokenUsage,
				err:        tt.err,
			}

			reqCtx := &Context{
				GinContext:  c,
				AuthService: mockAuth,
				RequestType: "test",
			}

			token, body, err := reqCtx.GetTokenWithUsageAndBody()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, token)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, token)
				assert.NotNil(t, body)
			}
		})
	}
}
