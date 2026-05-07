package http

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	ginlimiter "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

// planLimits maps API key plan names to ulule rate format strings.
// Format: "N-period" where period is S=second, M=minute, H=hour, D=day.
var planLimits = map[string]string{
	"free": "10-M",  // 10 req/minute
	"pro":  "200-M", // 200 req/minute
	// "enterprise" is handled as unlimited — no limiter created.
}

// planMiddlewares caches one middleware per plan to avoid re-creating stores.
var (
	planMiddlewaresMu sync.RWMutex
	planMiddlewares   = map[string]gin.HandlerFunc{}
)

// PlanRateLimiter returns a middleware that enforces per-API-key rate limits
// based on the plan stored in the validated APIKeyInfo context value.
// Must be registered AFTER APIKeyAuth so the plan is already in context.
func PlanRateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		info := APIKeyFromContext(c)
		if info == nil {
			// No key info outer IP limiter already covers unauthenticated paths.
			c.Next()
			return
		}

		plan := info.Plan
		if plan == "" {
			plan = "free"
		}

		// Enterprise plan is unlimited — bypass the limiter entirely.
		if plan == "enterprise" {
			c.Next()
			return
		}

		mw := getPlanMiddleware(plan)
		mw(c)
	}
}

// getPlanMiddleware returns (creating once) the rate limiter middleware for a plan.
// The limiter keys by OwnerID injected into X-Rate-Limit-Key header in the router,
// so a customer's quota spans all their IPs.
func getPlanMiddleware(plan string) gin.HandlerFunc {
	planMiddlewaresMu.RLock()
	if mw, ok := planMiddlewares[plan]; ok {
		planMiddlewaresMu.RUnlock()
		return mw
	}
	planMiddlewaresMu.RUnlock()

	planMiddlewaresMu.Lock()
	defer planMiddlewaresMu.Unlock()

	// Double-check after acquiring write lock.
	if mw, ok := planMiddlewares[plan]; ok {
		return mw
	}

	rateStr, ok := planLimits[plan]
	if !ok {
		rateStr = planLimits["free"] // unknown plans fall back to free
	}

	rate, err := limiter.NewRateFromFormatted(rateStr)
	if err != nil {
		rate, _ = limiter.NewRateFromFormatted(planLimits["free"])
	}

	store := memory.NewStore()
	instance := limiter.New(store, rate)

	mw := ginlimiter.NewMiddleware(instance,
		// WithKeyGetter extracts the rate limit key from the gin context.
		// X-Rate-Limit-Key is set to OwnerID by the router middleware so
		// a customer's full quota applies across all their IPs.
		ginlimiter.WithKeyGetter(func(c *gin.Context) string {
			if key := c.GetHeader("X-Rate-Limit-Key"); key != "" {
				return key
			}
			return c.ClientIP() // fallback
		}),
		ginlimiter.WithLimitReachedHandler(func(c *gin.Context) {
			plan := "free"
			if info := APIKeyFromContext(c); info != nil {
				plan = info.Plan
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, ErrorResponse{
				Error:   "rate limit exceeded",
				Code:    "RATE_LIMITED",
				Details: planLimitMessage(plan),
			})
		}),
	)

	planMiddlewares[plan] = mw
	return mw
}

func planLimitMessage(plan string) string {
	switch plan {
	case "free":
		return "Free plan: 10 requests/minute. Upgrade to Pro for 200 req/min."
	case "pro":
		return "Pro plan: 200 requests/minute. Contact us for Enterprise."
	default:
		return "Rate limit exceeded. Please try again shortly."
	}
}
