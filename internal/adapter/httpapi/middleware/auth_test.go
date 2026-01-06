package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestPathBasedAuthMiddleware_AllowsValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	router.Use(PathBasedAuthMiddleware("test-token", []string{"/v1"}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Authorization", "Bearer test-token")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestPathBasedAuthMiddleware_RejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	router.Use(PathBasedAuthMiddleware("test-token", []string{"/v1"}))
	router.POST("/v1/messages", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Request.Header.Set("Authorization", "Bearer other-token")

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestPathBasedAuthMiddleware_SkipsUnprotectedPath(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, router := gin.CreateTestContext(w)

	router.Use(PathBasedAuthMiddleware("test-token", []string{"/v1"}))
	router.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)

	router.ServeHTTP(w, c.Request)

	assert.Equal(t, http.StatusOK, w.Code)
}
