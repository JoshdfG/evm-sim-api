package entity

import (
	"math/big"
	"time"
)

// SimulationRequest is the caller-supplied transaction description.
type SimulationRequest struct {
	ChainID        uint64                     `json:"chain_id"`
	BlockNumber    *big.Int                   `json:"block_number,omitempty"`
	From           string                     `json:"from"`
	To             string                     `json:"to"`
	Value          *big.Int                   `json:"value,omitempty"`
	GasLimit       uint64                     `json:"gas_limit,omitempty"`
	GasPrice       *big.Int                   `json:"gas_price,omitempty"`
	Data           string                     `json:"data,omitempty"`
	StateOverrides map[string]AccountOverride `json:"state_overrides,omitempty"`
}

// AccountOverride mirrors the eth_call stateOverride parameter.
type AccountOverride struct {
	Balance *big.Int          `json:"balance,omitempty"`
	Nonce   *uint64           `json:"nonce,omitempty"`
	Code    string            `json:"code,omitempty"`
	Storage map[string]string `json:"storage,omitempty"`
}

// SimulationResult is the full response returned to the API caller.
type SimulationResult struct {
	ID           string    `json:"id"`
	RequestedAt  time.Time `json:"requested_at"`
	Success      bool      `json:"success"`
	RevertReason string    `json:"revert_reason,omitempty"`
	GasUsed      uint64    `json:"gas_used"`
	GasEstimate  uint64    `json:"gas_estimate"`

	AssetChanges []AssetChange `json:"asset_changes"`
	RiskFlags    []RiskFlag    `json:"risk_flags,omitempty"`
	Logs         []DecodedLog  `json:"logs,omitempty"`
	CallTrace    *CallFrame    `json:"call_trace,omitempty"`

	CacheHit    bool   `json:"cache_hit"`
	BlockNumber uint64 `json:"block_number"`
}

// DecodedLog is a human-readable EVM event log.
type DecodedLog struct {
	Address  string                 `json:"address"`
	EventSig string                 `json:"event_sig"`
	Decoded  map[string]interface{} `json:"decoded"`
	Raw      []string               `json:"raw_topics"`
}

// CallFrame is one node in the EVM internal call tree.
type CallFrame struct {
	Type    string      `json:"type"`
	From    string      `json:"from"`
	To      string      `json:"to"`
	Value   string      `json:"value,omitempty"`
	Gas     uint64      `json:"gas"`
	GasUsed uint64      `json:"gas_used"`
	Input   string      `json:"input"`
	Output  string      `json:"output,omitempty"`
	Error   string      `json:"error,omitempty"`
	Calls   []CallFrame `json:"calls,omitempty"`
}
