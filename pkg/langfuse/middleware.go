package langfuse

import (
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// TraceKey is the Gin context key used to store the Langfuse trace ID for
	// the current request so that relay handlers can retrieve it later.
	TraceKey = "langfuse_trace_id"

	// StartTimeKey is the Gin context key used to store the time at which the
	// relay request was first received.
	StartTimeKey = "langfuse_start_time"
)

// Middleware returns a Gin middleware that:
//  1. Records the request start time in the context.
//  2. Creates a Langfuse trace and stores its ID in the context.
//
// Downstream relay handlers should call TrackGeneration with the trace ID
// retrieved via TraceIDFromContext to attach generation observations to the
// same trace.
//
// The middleware is a no-op when the Langfuse client is disabled so that it
// can be registered unconditionally in the router setup.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		c.Set(StartTimeKey, startTime)

		if GetClient().IsEnabled() {
			userID := c.GetString("username")
			sessionID := c.GetHeader("X-Session-Id")

			traceID := StartTrace(
				"new-api-relay",
				userID,
				sessionID,
				map[string]any{
					"path":   c.Request.URL.Path,
					"method": c.Request.Method,
				},
				map[string]any{
					"remote_addr": c.ClientIP(),
					"user_agent":  c.Request.UserAgent(),
				},
				nil,
			)
			c.Set(TraceKey, traceID)
		}

		c.Next()
	}
}

// TraceIDFromContext retrieves the Langfuse trace ID stored by Middleware.
// Returns an empty string when the middleware has not run or the client is
// disabled.
func TraceIDFromContext(c *gin.Context) string {
	traceID, _ := c.Get(TraceKey)
	if id, ok := traceID.(string); ok {
		return id
	}
	return ""
}

// StartTimeFromContext retrieves the request start time stored by Middleware.
// Returns the current time as a fallback when the value is absent.
func StartTimeFromContext(c *gin.Context) time.Time {
	v, exists := c.Get(StartTimeKey)
	if !exists {
		return time.Now()
	}
	if t, ok := v.(time.Time); ok {
		return t
	}
	return time.Now()
}
