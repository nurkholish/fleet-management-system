package http

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	headerRequestID = "X-Request-ID"
	ctxLoggerKey    = "logger"
	ctxRequestIDKey = "request_id"
)

// RequestLogger injects a request-scoped logger with a correlation ID.
func RequestLogger(base zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqID := c.GetHeader(headerRequestID)
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set(ctxRequestIDKey, reqID)
		c.Header(headerRequestID, reqID)

		reqLog := base.With().
			Str("request_id", reqID).
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Logger()
		c.Set(ctxLoggerKey, reqLog)

		c.Next()

		reqLog.Info().
			Int("status", c.Writer.Status()).
			Dur("latency", time.Since(start)).
			Str("client_ip", c.ClientIP()).
			Msg("http_request")
	}
}

// LoggerFrom returns the request-scoped logger, or the base logger.
func LoggerFrom(c *gin.Context, base zerolog.Logger) zerolog.Logger {
	if v, ok := c.Get(ctxLoggerKey); ok {
		if l, ok := v.(zerolog.Logger); ok {
			return l
		}
	}
	return base
}
