package repo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/joshdfg/evm-sim-api/internal/entity"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// SimulationCacheRepo is a Redis caching decorator over EVMFork.
// Same chainID + blockNumber + tx params → same result, cached deterministically.
type SimulationCacheRepo struct {
	inner        usecase.EVMFork
	rdb          *redis.Client
	finalizedTTL time.Duration // 0 means no expiry
	pendingTTL   time.Duration
	log          zerolog.Logger
}

func NewSimulationCacheRepo(
	inner usecase.EVMFork,
	rdb *redis.Client,
	finalizedTTLSecs, pendingTTLSecs int,
	log zerolog.Logger,
) *SimulationCacheRepo {
	var fTTL, pTTL time.Duration
	if finalizedTTLSecs > 0 {
		fTTL = time.Duration(finalizedTTLSecs) * time.Second
	}
	if pendingTTLSecs > 0 {
		pTTL = time.Duration(pendingTTLSecs) * time.Second
	}
	return &SimulationCacheRepo{
		inner:        inner,
		rdb:          rdb,
		finalizedTTL: fTTL,
		pendingTTL:   pTTL,
		log:          log.With().Str("component", "sim_cache").Logger(),
	}
}

// Simulate checks Redis before delegating to the inner EVMFork.
func (c *SimulationCacheRepo) Simulate(ctx context.Context, req entity.SimulationRequest) (*usecase.RawSimResult, error) {
	key := cacheKey(req)

	if blob, err := c.rdb.Get(ctx, key).Bytes(); err == nil {
		var result usecase.RawSimResult
		if json.Unmarshal(blob, &result) == nil {
			c.log.Debug().Str("key", key).Msg("cache HIT")
			return &result, nil
		}
	}
	c.log.Debug().Str("key", key).Msg("cache MISS")

	result, err := c.inner.Simulate(ctx, req)
	if err != nil {
		return nil, err
	}

	ttl := c.finalizedTTL
	if req.BlockNumber == nil {
		ttl = c.pendingTTL // pending block  short TTL
	}

	if blob, err := json.Marshal(result); err == nil {
		if err := c.rdb.Set(ctx, key, blob, ttl).Err(); err != nil {
			c.log.Warn().Err(err).Str("key", key).Msg("cache SET failed")
		} else {
			c.log.Debug().Str("key", key).Msg("cache SET")
		}
	}

	return result, nil
}

// cacheKey produces a deterministic hash from the simulation request parameters.
func cacheKey(req entity.SimulationRequest) string {
	type cacheFields struct {
		ChainID     uint64
		BlockNumber string
		From        string
		To          string
		Value       string
		GasPrice    string
		Data        string
		Overrides   any
	}

	block := "latest"
	if req.BlockNumber != nil {
		block = req.BlockNumber.String()
	}
	value := "0"
	if req.Value != nil {
		value = req.Value.String()
	}
	gp := "basefee"
	if req.GasPrice != nil {
		gp = req.GasPrice.String()
	}

	blob, _ := json.Marshal(cacheFields{
		ChainID:     req.ChainID,
		BlockNumber: block,
		From:        req.From,
		To:          req.To,
		Value:       value,
		GasPrice:    gp,
		Data:        req.Data,
		Overrides:   req.StateOverrides,
	})

	h := sha256.Sum256(blob)
	return fmt.Sprintf("sim:v1:%x", h)
}

// Compile-time interface check.
var _ usecase.EVMFork = (*SimulationCacheRepo)(nil)
