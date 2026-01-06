package context

import "github.com/gin-gonic/gin"

const (
	requestIDKey = "request_id"
	messageIDKey = "message_id"
)

func SetRequestID(c *gin.Context, id string) {
	c.Set(requestIDKey, id)
	c.Writer.Header().Set("X-Request-ID", id)
}

func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

func SetMessageID(c *gin.Context, id string) {
	c.Set(messageIDKey, id)
}

func GetMessageID(c *gin.Context) string {
	if v, ok := c.Get(messageIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}
