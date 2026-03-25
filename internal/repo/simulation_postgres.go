package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joshdfg/evm-sim-api/internal/entity"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
)

// SimulationPostgresRepo persists simulation results to PostgreSQL.
type SimulationPostgresRepo struct {
	pool *pgxpool.Pool
}

func NewSimulationPostgresRepo(pool *pgxpool.Pool) *SimulationPostgresRepo {
	return &SimulationPostgresRepo{pool: pool}
}

func (r *SimulationPostgresRepo) Save(
	ctx context.Context,
	req entity.SimulationRequest,
	result entity.SimulationResult,
) error {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("sim repo: marshal request: %w", err)
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("sim repo: marshal result: %w", err)
	}

	riskLevel := deriveRiskLevel(result.RiskFlags)

	_, err = r.pool.Exec(ctx, `
		INSERT INTO simulations (
			id, requested_at, chain_id, from_address, to_address,
			block_number, success, revert_reason, gas_used,
			risk_level, request_json, result_json
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`,
		result.ID,
		result.RequestedAt,
		req.ChainID,
		req.From,
		req.To,
		result.BlockNumber,
		result.Success,
		result.RevertReason,
		result.GasUsed,
		riskLevel,
		reqJSON,
		resultJSON,
	)
	if err != nil {
		return fmt.Errorf("sim repo: insert: %w", err)
	}
	return nil
}

func (r *SimulationPostgresRepo) GetByID(ctx context.Context, id string) (*entity.SimulationResult, error) {
	var blob []byte
	err := r.pool.QueryRow(ctx,
		`SELECT result_json FROM simulations WHERE id = $1`, id,
	).Scan(&blob)
	if err != nil {
		return nil, fmt.Errorf("sim repo: get by id: %w", err)
	}
	var result entity.SimulationResult
	if err := json.Unmarshal(blob, &result); err != nil {
		return nil, fmt.Errorf("sim repo: unmarshal: %w", err)
	}
	return &result, nil
}

func (r *SimulationPostgresRepo) ListByAPIKey(
	ctx context.Context,
	apiKey string,
	limit, offset int,
) ([]entity.SimulationResult, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT result_json FROM simulations
		WHERE api_key = $1
		ORDER BY requested_at DESC
		LIMIT $2 OFFSET $3
	`, apiKey, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("sim repo: list: %w", err)
	}
	defer rows.Close()

	var results []entity.SimulationResult
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		var res entity.SimulationResult
		if err := json.Unmarshal(blob, &res); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func deriveRiskLevel(flags []entity.RiskFlag) string {
	for _, f := range flags {
		if f.Severity == entity.SeverityCritical {
			return "critical"
		}
	}
	for _, f := range flags {
		if f.Severity == entity.SeverityWarning {
			return "warning"
		}
	}
	return "none"
}

// Compile-time interface check.
var _ usecase.SimulationRepository = (*SimulationPostgresRepo)(nil)
