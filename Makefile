APP_NAME  := evm-sim-api
MAIN      := ./cmd/app
BUILD_DIR := ./bin

.PHONY: run build test test-all lint swag sqlc migrate seed infra infra-down tidy help

## run: start the API server (reads .env automatically)
run:
	go run $(MAIN)

## build: compile a production binary into ./bin/
build:
	CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o $(BUILD_DIR)/$(APP_NAME) $(MAIN)

## test: run unit tests — no database or network required
test:
	go test ./internal/usecase/... -v -race -count=1

## test-all: run all tests including integration (requires `make infra` first)
test-all:
	go test ./... -v -race -count=1

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## swag: regenerate Swagger docs from handler annotations
swag:
	swag init -g cmd/app/main.go --output docs

## sqlc: regenerate type-safe DB queries from internal/repo/sqlc/query.sql
sqlc:
	sqlc generate

## tidy: tidy and verify go modules
tidy:
	go mod tidy
	go mod verify

## infra: start Postgres + Redis via Docker Compose
infra:
	docker compose up -d postgres redis

## infra-down: stop and remove containers + volumes
infra-down:
	docker compose down -v

## migrate: apply all SQL migrations to the local Postgres instance
migrate:
	@for f in migrations/*.sql; do \
		echo "▶ applying $$f"; \
		docker exec -i evm-sim-api-postgres-1 psql -U simapi -d simapi < $$f; \
	done

## seed: insert a local development API key (idempotent)
seed:
	docker exec -i evm-sim-api-postgres-1 psql -U simapi -d simapi -c \
		"INSERT INTO api_keys (key, owner_id, plan, label) \
		 VALUES ('dev-test-key-00000000', 'local-dev', 'enterprise', 'local') \
		 ON CONFLICT (key) DO NOTHING;"
	@echo "Dev API key: dev-test-key-00000000"

## help: list available make targets
help:
	@grep -E '^##' Makefile | sed 's/## /  /'

swag-docs:
	curl -X POST http://localhost:8081/v1/simulate \
  -H "X-API-Key: dev-test-key-00000000" \
  -H "Content-Type: application/json" \
  -d '{"chain_id":1,"from":"0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045","to":"0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48","data":"0x70a08231000000000000000000000000d8da6bf26964af9d7eed9e03e53415d37aa96045"}'
