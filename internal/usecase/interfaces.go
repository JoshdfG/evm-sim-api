package usecase

import (
	"context"

	"github.com/joshdfg/evm-sim-api/internal/entity"
)

// SimulationRepository persists and retrieves simulation history.
// Defined in the inner layer; implemented in internal/repo/.
type SimulationRepository interface {
	Save(ctx context.Context, req entity.SimulationRequest, result entity.SimulationResult) error
	GetByID(ctx context.Context, id string) (*entity.SimulationResult, error)
	ListByAPIKey(ctx context.Context, apiKey string, limit, offset int) ([]entity.SimulationResult, error)
}

// EVMFork executes a transaction against a fork of chain state.
// Defined in the inner layer; implemented in internal/repo/evm/.
type EVMFork interface {
	Simulate(ctx context.Context, req entity.SimulationRequest) (*RawSimResult, error)
}

// RawSimResult is the low-level output from the EVM engine before business logic transforms it.
type RawSimResult struct {
	Success      bool
	RevertReason string
	GasUsed      uint64
	Logs         []RawLog
	CallTrace    *entity.CallFrame
	BlockNumber  uint64

	// AlchemyChanges are pre-parsed asset changes from alchemy_simulateAssetChanges.
	// Populated when debug_traceCall returns no logs (Alchemy free tier).
	// The usecase prefers these over re-decoding logs when present.
	AlchemyChanges []entity.AssetChange
}

// RawLog is a pre-decoded EVM event log from the tracer.
type RawLog struct {
	Address string
	Topics  []string
	Data    string // hex-encoded
}

// TokenDecoder converts raw EVM logs into human-readable asset changes.
// Defined in the inner layer; implemented in internal/repo/evm/.
type TokenDecoder interface {
	Decode(logs []RawLog, callTrace *entity.CallFrame, from, to string) ([]entity.AssetChange, []entity.DecodedLog, error)
}

// RiskAnalyzer inspects simulation output for suspicious patterns.
// Defined in the inner layer; implemented in internal/usecase/risk_analyzer.go.
type RiskAnalyzer interface {
	Analyze(req entity.SimulationRequest, raw RawSimResult, changes []entity.AssetChange) []entity.RiskFlag
}

// APIKeyRepository validates API keys for the auth middleware.
// Defined in the inner layer; implemented in internal/repo/.
type APIKeyRepository interface {
	Validate(ctx context.Context, key string) (*APIKeyInfo, error)
}

// APIKeyInfo is returned after a successful key validation.
type APIKeyInfo struct {
	OwnerID string
	Plan    string // "free" | "pro" | "enterprise"
}
