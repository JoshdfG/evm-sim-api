package repo

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/joshdfg/evm-sim-api/internal/entity"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
)

// SimulationLoggingRepo decorates EVMFork with zerolog structured logging.
type SimulationLoggingRepo struct {
	inner usecase.EVMFork
	log   zerolog.Logger
}

func NewSimulationLoggingRepo(inner usecase.EVMFork, log zerolog.Logger) *SimulationLoggingRepo {
	return &SimulationLoggingRepo{
		inner: inner,
		log:   log.With().Str("component", "evm_fork").Logger(),
	}
}

func (l *SimulationLoggingRepo) Simulate(ctx context.Context, req entity.SimulationRequest) (*usecase.RawSimResult, error) {
	start := time.Now()

	block := "latest"
	if req.BlockNumber != nil {
		block = req.BlockNumber.String()
	}

	l.log.Debug().
		Uint64("chain_id", req.ChainID).
		Str("from", req.From).
		Str("to", req.To).
		Str("block", block).
		Msg("simulate start")

	result, err := l.inner.Simulate(ctx, req)
	elapsed := time.Since(start)

	if err != nil {
		l.log.Error().
			Err(err).
			Uint64("chain_id", req.ChainID).
			Dur("elapsed_ms", elapsed).
			Msg("simulate error")
		return nil, err
	}

	l.log.Info().
		Uint64("chain_id", req.ChainID).
		Str("from", req.From).
		Str("to", req.To).
		Bool("success", result.Success).
		Uint64("gas_used", result.GasUsed).
		Uint64("block", result.BlockNumber).
		Dur("elapsed_ms", elapsed).
		Msg("simulate done")

	return result, nil
}

// Compile-time interface check.
var _ usecase.EVMFork = (*SimulationLoggingRepo)(nil)
