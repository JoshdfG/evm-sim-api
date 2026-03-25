-- name: GetSimulationByID :one
SELECT id, requested_at, chain_id, from_address, to_address, block_number,
       success, revert_reason, gas_used, risk_level, api_key, request_json, result_json
FROM simulations
WHERE id = $1;

-- name: InsertSimulation :exec
INSERT INTO simulations (
    id, requested_at, chain_id, from_address, to_address, block_number,
    success, revert_reason, gas_used, risk_level, api_key, request_json, result_json
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10, $11, $12, $13
);

-- name: ListSimulationsByAPIKey :many
SELECT id, requested_at, chain_id, from_address, to_address,
       success, revert_reason, gas_used, risk_level, result_json
FROM simulations
WHERE api_key = $1
ORDER BY requested_at DESC
LIMIT $2 OFFSET $3;

-- name: CountSimulationsByAPIKey :one
SELECT COUNT(*) FROM simulations WHERE api_key = $1;

-- name: GetAPIKey :one
SELECT id, key, owner_id, plan, label, created_at, revoked_at
FROM api_keys
WHERE key = $1 AND revoked_at IS NULL;

-- name: InsertAPIKey :one
INSERT INTO api_keys (key, owner_id, plan, label)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: RevokeAPIKey :exec
UPDATE api_keys SET revoked_at = NOW() WHERE key = $1;
