package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config is the single source of truth for all runtime settings.
// Populated from environment variables or a .env file at startup.
type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	EVM      EVMConfig
	Log      LogConfig
}

type AppConfig struct {
	Name    string `env:"APP_NAME"    env-default:"evm-sim-api"`
	Version string `env:"APP_VERSION" env-default:"1.0.0"`
	Env     string `env:"APP_ENV"     env-default:"development"`
}

type HTTPConfig struct {
	Port string `env:"HTTP_PORT" env-default:"8080"`
}

type PostgresConfig struct {
	User     string `env:"POSTGRES_USER"     env-required:"true"`
	Password string `env:"POSTGRES_PASSWORD" env-required:"true"`
	DB       string `env:"POSTGRES_DB"       env-required:"true"`
	Host     string `env:"POSTGRES_HOST"     env-default:"localhost"`
	Port     string `env:"POSTGRES_PORT"     env-default:"5432"`
	SSLMode  string `env:"POSTGRES_SSLMODE"  env-default:"disable"`
}

// DSN builds the postgres connection string.
func (p PostgresConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, p.DB, p.SSLMode,
	)
}

type RedisConfig struct {
	Host     string `env:"REDIS_HOST"     env-default:"localhost"`
	Port     string `env:"REDIS_PORT"     env-default:"6379"`
	Password string `env:"REDIS_PASSWORD" env-default:""`
	DB       int    `env:"REDIS_DB"       env-default:"0"`
}

func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%s", r.Host, r.Port)
}

type EVMConfig struct {
	// ArchiveNodeURL is an Ethereum archive node supporting eth_call + debug_traceCall.
	// Works with Alchemy, Infura, QuickNode.
	ArchiveNodeURL string `env:"EVM_ARCHIVE_NODE_URL" env-required:"true"`

	// SimulationTimeoutSeconds caps how long a single simulation may run.
	SimulationTimeoutSeconds int `env:"EVM_SIM_TIMEOUT_SECONDS" env-default:"30"`

	// MaxGasLimit is the gas cap applied when the caller sends gas_limit = 0.
	MaxGasLimit uint64 `env:"EVM_MAX_GAS_LIMIT" env-default:"30000000"`

	// CacheTTLSeconds for finalized-block results. 0 = cache forever (recommended).
	CacheTTLSeconds int `env:"EVM_CACHE_TTL_SECONDS" env-default:"0"`

	// PendingCacheTTLSeconds for latest-block results.
	PendingCacheTTLSeconds int `env:"EVM_PENDING_CACHE_TTL_SECONDS" env-default:"15"`
}

type LogConfig struct {
	Level string `env:"LOG_LEVEL" env-default:"info"`
}

// New loads config from a .env file (optional) then environment variables.
// The process exits immediately if any required field is absent.
func New() (*Config, error) {
	cfg := &Config{}
	// ReadConfig falls back to ReadEnv when .env is absent  that is intentional.
	if err := cleanenv.ReadConfig(".env", cfg); err != nil {
		if err2 := cleanenv.ReadEnv(cfg); err2 != nil {
			return nil, fmt.Errorf("config: %w", err2)
		}
	}
	return cfg, nil
}
