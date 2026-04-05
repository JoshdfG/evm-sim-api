package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/joshdfg/evm-sim-api/internal/entity"
)

// SimulationUseCase orchestrates: fork → decode → risk-analyze → persist → return.
// It has no knowledge of HTTP, databases, or EVM internals.
type SimulationUseCase struct {
	fork    EVMFork
	repo    SimulationRepository
	decoder TokenDecoder
	risk    RiskAnalyzer
}

func NewSimulationUseCase(
	fork EVMFork,
	repo SimulationRepository,
	decoder TokenDecoder,
	risk RiskAnalyzer,
) *SimulationUseCase {
	return &SimulationUseCase{fork: fork, repo: repo, decoder: decoder, risk: risk}
}

// Run is the primary entry point called by the HTTP handler.
func (uc *SimulationUseCase) Run(ctx context.Context, req entity.SimulationRequest) (*entity.SimulationResult, error) {
	// 1. Execute against the EVM fork.
	raw, err := uc.fork.Simulate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("simulation: fork: %w", err)
	}

	// 2. Resolve asset changes.
	//
	// Priority:
	//   a) AlchemyChanges — pre-parsed by alchemy_simulateAssetChanges (free tier).
	//      Available even when debug_traceCall logs are empty.
	//   b) Decode from raw EVM logs (debug_traceCall with withLog:true).
	//      Available on Alchemy Growth+, QuickNode, self-hosted geth.
	//
	// Risk analysis always runs — it operates on whatever changes we have.
	var changes []entity.AssetChange
	var decodedLogs []entity.DecodedLog

	if len(raw.AlchemyChanges) > 0 {
		// Strategy (a): Alchemy gave us pre-parsed changes. Use them directly.
		// Still run the log decoder for the decodedLogs array (events display).
		changes = raw.AlchemyChanges
		_, decodedLogs, err = uc.decoder.Decode(raw.Logs, raw.CallTrace, req.From, req.To)
		if err != nil {
			decodedLogs = append(decodedLogs, entity.DecodedLog{
				EventSig: "DECODE_ERROR",
				Decoded:  map[string]any{"error": err.Error()},
			})
		}
	} else {
		// Strategy (b): Decode from raw EVM trace logs.
		changes, decodedLogs, err = uc.decoder.Decode(raw.Logs, raw.CallTrace, req.From, req.To)
		if err != nil {
			decodedLogs = append(decodedLogs, entity.DecodedLog{
				EventSig: "DECODE_ERROR",
				Decoded:  map[string]any{"error": err.Error()},
			})
		}
	}

	// 3. Risk analysis runs on all changes regardless of source.
	flags := uc.risk.Analyze(req, *raw, changes)

	// 4. Build the result.
	result := entity.SimulationResult{
		ID:           uuid.New().String(),
		RequestedAt:  time.Now().UTC(),
		Success:      raw.Success,
		RevertReason: raw.RevertReason,
		GasUsed:      raw.GasUsed,
		GasEstimate:  gasWithBuffer(raw.GasUsed),
		AssetChanges: changes,
		RiskFlags:    flags,
		Logs:         decodedLogs,
		CallTrace:    raw.CallTrace,
		BlockNumber:  raw.BlockNumber,
	}

	// 5. Persist fire-and-forget — DB error must never fail the caller.
	if saveErr := uc.repo.Save(ctx, req, result); saveErr != nil {
		_ = saveErr // logged by the repo's logging decorator
	}

	return &result, nil
}

// GetByID retrieves a previously stored simulation result.
func (uc *SimulationUseCase) GetByID(ctx context.Context, id string) (*entity.SimulationResult, error) {
	res, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("simulation: GetByID: %w", err)
	}
	return res, nil
}

func gasWithBuffer(gasUsed uint64) uint64 {
	return gasUsed + (gasUsed / 5)
}
