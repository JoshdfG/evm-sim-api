package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

// RequestLogger returns a zerolog-based Gin middleware that logs every request.
func RequestLogger(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info().
			Str("method", c.Request.Method).
			Str("path", c.Request.URL.Path).
			Int("status", c.Writer.Status()).
			Dur("latency_ms", time.Since(start)).
			Str("ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Msg("request")
	}
}

// Recovery returns a middleware that recovers from panics, logs them, and
// returns a 500 to the caller instead of crashing the process.
func Recovery(log zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Error().
					Interface("panic", err).
					Str("path", c.Request.URL.Path).
					Msg("panic recovered")
				c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{
					Error: "internal server error",
				})
			}
		}()
		c.Next()
	}
}

// CORS returns permissive CORS headers suitable for a public API.
// Lock AllowOrigins to your known consumers in production.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RateLimiter returns a per-IP in-memory rate limiter.
// rate format: "N-period" e.g. "100-M" = 100 req/minute, "1000-H" = 1000/hour.
func RateLimiter(rate string) gin.HandlerFunc {
	r, err := limiter.NewRateFromFormatted(rate)
	if err != nil {
		panic("rate limiter: invalid rate format: " + rate)
	}
	store := memory.NewStore()
	instance := limiter.New(store, r)
	return ginlimiter.NewMiddleware(instance)
}
