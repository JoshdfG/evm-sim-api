package http

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
)

// SimulationHandler handles simulation-related HTTP routes.
type SimulationHandler struct {
	uc *usecase.SimulationUseCase
}

func NewSimulationHandler(uc *usecase.SimulationUseCase) *SimulationHandler {
	return &SimulationHandler{uc: uc}
}

// Simulate godoc
// @Summary      Simulate an EVM transaction
// @Description  Forks mainnet state and dry-runs the transaction. Returns token deltas, gas, revert reason, and risk flags.
// @Tags         simulation
// @Accept       json
// @Produce      json
// @Security     ApiKeyAuth
// @Param        request body    SimulateRequest  true  "Transaction to simulate"
// @Success      200             {object}         SimulateResponse
// @Failure      400             {object}         ErrorResponse
// @Failure      401             {object}         ErrorResponse
// @Failure      422             {object}         ErrorResponse
// @Failure      429             {object}         ErrorResponse
// @Failure      500             {object}         ErrorResponse
// @Router       /v1/simulate [post]
func (h *SimulationHandler) Simulate(c *gin.Context) {
	var req SimulateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid request body",
			Details: err.Error(),
		})
		return
	}

	entityReq, err := req.toEntityRequest()
	if err != nil {
		var ve *ValidationError
		if errors.As(err, &ve) {
			c.JSON(http.StatusUnprocessableEntity, ErrorResponse{
				Error:   "validation failed",
				Details: err.Error(),
			})
			return
		}
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.uc.Run(c.Request.Context(), entityReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "simulation failed",
			Details: err.Error(),
		})
		return
	}

	if !req.IncludeCallTrace {
		result.CallTrace = nil
	}

	c.JSON(http.StatusOK, SimulateResponse{SimulationResult: *result})
}

// GetSimulation godoc
// @Summary      Retrieve a simulation result by ID
// @Description  Fetches a previously run simulation result from history by its UUID.
// @Tags         simulation
// @Produce      json
// @Security     ApiKeyAuth
// @Param        id   path     string  true  "Simulation UUID"
// @Success      200  {object} SimulateResponse
// @Failure      404  {object} ErrorResponse
// @Failure      500  {object} ErrorResponse
// @Router       /v1/simulations/{id} [get]
func (h *SimulationHandler) GetSimulation(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "id is required"})
		return
	}

	result, err := h.uc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to retrieve simulation",
			Details: err.Error(),
		})
		return
	}
	if result == nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "simulation not found"})
		return
	}

	c.JSON(http.StatusOK, SimulateResponse{SimulationResult: *result})
}

// ListSimulations godoc
// @Summary      List simulation history
// @Description  Returns a paginated list of simulations run by the authenticated API key.
// @Tags         simulation
// @Produce      json
// @Security     ApiKeyAuth
// @Param        limit   query  int  false  "Max results to return (default 20, max 100)"
// @Param        offset  query  int  false  "Pagination offset (default 0)"
// @Success      200  {object}  ListSimulationsResponse
// @Failure      401  {object}  ErrorResponse
// @Failure      500  {object}  ErrorResponse
// @Router       /v1/simulations [get]
func (h *SimulationHandler) ListSimulations(c *gin.Context) {
	info := APIKeyFromContext(c)
	if info == nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "missing API key context"})
		return
	}

	limit := clampInt(c.Query("limit"), 20, 1, 100)
	offset := clampInt(c.Query("offset"), 0, 0, 1<<31)

	results, err := h.uc.List(c.Request.Context(), info.OwnerID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "failed to list simulations",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ListSimulationsResponse{
		Results: results,
		Limit:   limit,
		Offset:  offset,
		Count:   len(results),
	})
}

// clampInt parses s as an integer, returns defaultVal if empty/invalid,
// and clamps the result to [min, max].
func clampInt(s string, defaultVal, min, max int) int {
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
