package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
	"github.com/rs/zerolog"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// NewRouter wires all routes, middleware, and handlers into a *gin.Engine.
func NewRouter(
	simUC *usecase.SimulationUseCase,
	apiKeyRepo usecase.APIKeyRepository,
	log zerolog.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(Recovery(log))
	r.Use(RequestLogger(log))
	r.Use(CORS())
	r.Use(RateLimiter("600-M")) // outer guard: 600 req/min per IP (burst protection)

	// ── Public routes ─────────────────────────────────────────────────────────
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ── Authenticated API routes ──────────────────────────────────────────────
	v1 := r.Group("/v1")
	v1.Use(APIKeyAuth(apiKeyRepo))

	// Inject rate-limit key header so PlanRateLimiter can key by owner, not IP.
	v1.Use(func(c *gin.Context) {
		if info := APIKeyFromContext(c); info != nil {
			c.Request.Header.Set("X-Rate-Limit-Key", info.OwnerID)
		}
		c.Next()
	})

	// Plan-aware rate limiting runs after auth so plan is known.
	v1.Use(PlanRateLimiter())

	simHandler := NewSimulationHandler(simUC)
	v1.POST("/simulate", simHandler.Simulate)
	v1.GET("/simulations/:id", simHandler.GetSimulation)
	v1.GET("/simulations", simHandler.ListSimulations)

	return r
}
