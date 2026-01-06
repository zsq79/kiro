package middleware

import (
	"kiro2api/internal/adapter/httpapi/context"
	"kiro2api/utils"

	"github.com/gin-gonic/gin"
)

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = "req_" + utils.GenerateUUID()
		}
		context.SetRequestID(c, rid)
		c.Next()
	}
}
