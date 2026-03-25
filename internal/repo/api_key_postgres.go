package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
)

// APIKeyPostgresRepo implements usecase.APIKeyRepository.
type APIKeyPostgresRepo struct {
	pool *pgxpool.Pool
}

func NewAPIKeyPostgresRepo(pool *pgxpool.Pool) *APIKeyPostgresRepo {
	return &APIKeyPostgresRepo{pool: pool}
}

func (r *APIKeyPostgresRepo) Validate(ctx context.Context, key string) (*usecase.APIKeyInfo, error) {
	var info usecase.APIKeyInfo
	err := r.pool.QueryRow(ctx, `
		SELECT owner_id, plan
		FROM api_keys
		WHERE key = $1 AND revoked_at IS NULL
	`, key).Scan(&info.OwnerID, &info.Plan)
	if err != nil {
		return nil, fmt.Errorf("api_key repo: validate: %w", err)
	}
	return &info, nil
}

// Compile-time interface check.
var _ usecase.APIKeyRepository = (*APIKeyPostgresRepo)(nil)
