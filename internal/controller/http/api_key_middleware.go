package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
)

const (
	ctxKeyAPIKeyInfo = "api_key_info"
	headerAPIKey     = "X-API-Key"
)

// APIKeyAuth validates the X-API-Key header (or Bearer token) on every request.
// On success it injects *usecase.APIKeyInfo into the Gin context.
func APIKeyAuth(repo usecase.APIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(headerAPIKey)
		if key == "" {
			// Also accept "Authorization: Bearer <key>" for SDK convenience.
			auth := c.GetHeader("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				key = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if key == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				Error: "missing API key  provide X-API-Key header",
				Code:  "MISSING_API_KEY",
			})
			return
		}

		info, err := repo.Validate(c.Request.Context(), key)
		if err != nil || info == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{
				Error: "invalid or expired API key",
				Code:  "INVALID_API_KEY",
			})
			return
		}

		c.Set(ctxKeyAPIKeyInfo, info)
		c.Next()
	}
}

// APIKeyFromContext retrieves the validated APIKeyInfo injected by APIKeyAuth.
func APIKeyFromContext(c *gin.Context) *usecase.APIKeyInfo {
	if v, ok := c.Get(ctxKeyAPIKeyInfo); ok {
		if info, ok := v.(*usecase.APIKeyInfo); ok {
			return info
		}
	}
	return nil
}
