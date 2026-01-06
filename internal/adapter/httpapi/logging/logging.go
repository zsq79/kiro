package logging

import (
	srvcontext "kiro2api/internal/adapter/httpapi/context"
	"kiro2api/logger"

	"github.com/gin-gonic/gin"
)

func AddFields(c *gin.Context, fields ...logger.Field) []logger.Field {
	rid := srvcontext.GetRequestID(c)
	mid := srvcontext.GetMessageID(c)
	out := make([]logger.Field, 0, len(fields)+2)
	if rid != "" {
		out = append(out, logger.String("request_id", rid))
	}
	if mid != "" {
		out = append(out, logger.String("message_id", mid))
	}
	out = append(out, fields...)
	return out
}
