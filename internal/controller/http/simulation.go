package http

import (
	"errors"
	"net/http"

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

	// Strip verbose call trace unless caller explicitly requested it.
	if !req.IncludeCallTrace {
		result.CallTrace = nil
	}

	c.JSON(http.StatusOK, SimulateResponse{SimulationResult: *result})
}

// GetSimulation godoc
// @Summary      Retrieve a simulation result
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
