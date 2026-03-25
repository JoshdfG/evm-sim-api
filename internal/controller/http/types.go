package http

import (
	"fmt"
	"math/big"

	"github.com/joshdfg/evm-sim-api/internal/entity"
)

// SimulateRequest is the JSON body for POST /v1/simulate.
type SimulateRequest struct {
	ChainID          uint64                        `json:"chain_id" binding:"required"`
	BlockNumber      *uint64                       `json:"block_number,omitempty"`
	From             string                        `json:"from" binding:"required"`
	To               string                        `json:"to" binding:"required"`
	Value            string                        `json:"value,omitempty"` // decimal wei string
	GasLimit         uint64                        `json:"gas_limit,omitempty"`
	GasPrice         string                        `json:"gas_price,omitempty"` // decimal wei string
	Data             string                        `json:"data,omitempty"`
	StateOverrides   map[string]AccountOverrideDTO `json:"state_overrides,omitempty"`
	IncludeCallTrace bool                          `json:"include_call_trace,omitempty"`
}

// AccountOverrideDTO is the JSON shape for per-address state injection.
type AccountOverrideDTO struct {
	Balance *string           `json:"balance,omitempty"` // decimal wei string
	Nonce   *uint64           `json:"nonce,omitempty"`
	Code    string            `json:"code,omitempty"`
	Storage map[string]string `json:"storage,omitempty"`
}

// SimulateResponse wraps entity.SimulationResult for the HTTP response.
type SimulateResponse struct {
	entity.SimulationResult
}

// ErrorResponse is the standard JSON error envelope.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// ValidationError carries field-level validation details.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("field %q: %s", e.Field, e.Message)
}

// toEntityRequest converts the HTTP DTO into the domain object.
func (r SimulateRequest) toEntityRequest() (entity.SimulationRequest, error) {
	req := entity.SimulationRequest{
		ChainID:  r.ChainID,
		From:     r.From,
		To:       r.To,
		GasLimit: r.GasLimit,
		Data:     r.Data,
	}

	if r.BlockNumber != nil {
		req.BlockNumber = new(big.Int).SetUint64(*r.BlockNumber)
	}

	if r.Value != "" {
		v, ok := new(big.Int).SetString(r.Value, 10)
		if !ok {
			return req, &ValidationError{Field: "value", Message: "must be a decimal wei string"}
		}
		req.Value = v
	}

	if r.GasPrice != "" {
		gp, ok := new(big.Int).SetString(r.GasPrice, 10)
		if !ok {
			return req, &ValidationError{Field: "gas_price", Message: "must be a decimal wei string"}
		}
		req.GasPrice = gp
	}

	if len(r.StateOverrides) > 0 {
		req.StateOverrides = make(map[string]entity.AccountOverride, len(r.StateOverrides))
		for addr, dto := range r.StateOverrides {
			ov := entity.AccountOverride{
				Code:    dto.Code,
				Nonce:   dto.Nonce,
				Storage: dto.Storage,
			}
			if dto.Balance != nil {
				bal, ok := new(big.Int).SetString(*dto.Balance, 10)
				if !ok {
					return req, &ValidationError{
						Field:   fmt.Sprintf("state_overrides.%s.balance", addr),
						Message: "must be a decimal wei string",
					}
				}
				ov.Balance = bal
			}
			req.StateOverrides[addr] = ov
		}
	}

	return req, nil
}
