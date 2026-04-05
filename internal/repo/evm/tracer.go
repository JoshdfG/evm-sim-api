package evm

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joshdfg/evm-sim-api/internal/entity"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
	"github.com/rs/zerolog"
)

// CallTracer wraps debug_traceCall with callTracer + withLog:true.
//
// Log availability by node tier:
//
//	Alchemy Growth+  → debug_traceCall returns logs field populated ✓
//	Alchemy Free     → debug_traceCall returns call frame, logs field empty
//	Infura           → debug_traceCall supported on paid plans
//	QuickNode        → debug_traceCall + logs supported on all plans
//	Self-hosted geth → full support
type CallTracer struct {
	log zerolog.Logger
}

func NewCallTracer(log zerolog.Logger) *CallTracer {
	return &CallTracer{log: log.With().Str("component", "call_tracer").Logger()}
}

// traceFrame is the raw JSON shape returned by callTracer.
type traceFrame struct {
	Type    string       `json:"type"`
	From    string       `json:"from"`
	To      string       `json:"to"`
	Value   string       `json:"value"`
	Gas     string       `json:"gas"`
	GasUsed string       `json:"gasUsed"`
	Input   string       `json:"input"`
	Output  string       `json:"output"`
	Error   string       `json:"error"`
	Calls   []traceFrame `json:"calls"`
	Logs    []traceLog   `json:"logs"`
}

type traceLog struct {
	Address string   `json:"address"`
	Topics  []string `json:"topics"`
	Data    string   `json:"data"`
}

// TraceResult holds the full output of a trace attempt.
type TraceResult struct {
	Frame     *entity.CallFrame
	Logs      []usecase.RawLog
	LogSource string // "debug_traceCall" | "alchemy_simulate" | "filter_logs" | "none"
}

// TraceCall attempts debug_traceCall first.
// Returns the call frame and whatever logs were found, along with the source used.
func (t *CallTracer) TraceCall(
	ctx context.Context,
	client *ethclient.Client,
	msg ethereum.CallMsg,
	blockNum *big.Int,
) TraceResult {
	blockRef := "latest"
	if blockNum != nil {
		blockRef = fmt.Sprintf("0x%x", blockNum)
	}

	// Use hex.EncodeToString for []byte — fmt.Sprintf("0x%x", slice) drops zero-padding.
	callArg := map[string]any{
		"from": msg.From.Hex(),
		"to":   msg.To.Hex(),
		"gas":  fmt.Sprintf("0x%x", msg.Gas),
		"data": "0x" + hex.EncodeToString(msg.Data),
	}
	if msg.Value != nil {
		callArg["value"] = fmt.Sprintf("0x%x", msg.Value)
	}
	if msg.GasPrice != nil {
		callArg["gasPrice"] = fmt.Sprintf("0x%x", msg.GasPrice)
	}

	tracerOpts := map[string]any{
		"tracer":       "callTracer",
		"tracerConfig": map[string]any{"withLog": true},
	}

	var raw json.RawMessage
	if err := client.Client().CallContext(ctx, &raw, "debug_traceCall", callArg, blockRef, tracerOpts); err != nil {
		t.log.Warn().Err(err).Msg("debug_traceCall failed — call trace unavailable")
		return TraceResult{LogSource: "none"}
	}

	var frame traceFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.log.Warn().Err(err).Msg("debug_traceCall: unmarshal failed")
		return TraceResult{LogSource: "none"}
	}

	logs := collectLogs(frame)
	callFrame := convertFrame(frame)

	t.log.Debug().
		Int("log_count", len(logs)).
		Int("call_depth", callDepth(frame)).
		Msg("debug_traceCall complete")

	return TraceResult{
		Frame:     callFrame,
		Logs:      logs,
		LogSource: "debug_traceCall",
	}
}

// AlchemySimulate calls alchemy_simulateAssetChanges — available on all Alchemy plans.
// Returns structured asset changes directly from Alchemy's simulation engine.
// This is used as a fallback when debug_traceCall returns no logs.
func (t *CallTracer) AlchemySimulate(
	ctx context.Context,
	client *ethclient.Client,
	msg ethereum.CallMsg,
	_ *big.Int, // blockNum reserved — alchemy_simulateAssetChanges always simulates at latest
) ([]alchemyAssetChange, error) {
	// Build the transaction object.
	// IMPORTANT: use hex.EncodeToString for []byte fields — fmt.Sprintf("0x%x", slice)
	// drops zero-padding (0x00 → "0" not "00") which corrupts calldata.
	callArg := map[string]interface{}{
		"from": msg.From.Hex(),
		"to":   msg.To.Hex(),
	}
	if len(msg.Data) > 0 {
		callArg["data"] = "0x" + hex.EncodeToString(msg.Data)
	}
	if msg.Value != nil && msg.Value.Sign() > 0 {
		callArg["value"] = fmt.Sprintf("0x%x", msg.Value)
	}
	// Omit gas — letting Alchemy estimate avoids "invalid transaction object"
	// errors caused by our default 30M gas cap looking like a malformed tx.

	type alchemyResp struct {
		Changes []alchemyAssetChange `json:"changes"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	var resp alchemyResp
	// params must be [{txObject}] — single tx object, no block tag.
	// CallContext variadic expansion: CallContext(ctx, result, method, arg) → params: [arg]
	if err := client.Client().CallContext(ctx, &resp, "alchemy_simulateAssetChanges", callArg); err != nil {
		return nil, fmt.Errorf("alchemy_simulateAssetChanges: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("alchemy_simulateAssetChanges: %s", resp.Error.Message)
	}

	t.log.Debug().Int("change_count", len(resp.Changes)).Msg("alchemy_simulateAssetChanges complete")
	return resp.Changes, nil
}

// alchemyAssetChange is the shape of one item in alchemy_simulateAssetChanges response.
type alchemyAssetChange struct {
	AssetType       string `json:"assetType"`  // "ERC20" | "NATIVE" | "ERC721" | "ERC1155"
	ChangeType      string `json:"changeType"` // "TRANSFER" | "APPROVE"
	From            string `json:"from"`
	To              string `json:"to"`
	RawAmount       string `json:"rawAmount"`
	Amount          string `json:"amount"` // human-readable
	Symbol          string `json:"symbol"`
	Decimals        int    `json:"decimals"`
	ContractAddress string `json:"contractAddress"`
	TokenID         string `json:"tokenId,omitempty"`
	Logo            string `json:"logo,omitempty"`
}

func convertFrame(f traceFrame) *entity.CallFrame {
	ef := &entity.CallFrame{
		Type:   f.Type,
		From:   common.HexToAddress(f.From).Hex(),
		To:     common.HexToAddress(f.To).Hex(),
		Value:  f.Value,
		Input:  f.Input,
		Output: f.Output,
		Error:  f.Error,
	}
	ef.Gas = hexToUint64(f.Gas)
	ef.GasUsed = hexToUint64(f.GasUsed)
	for _, child := range f.Calls {
		ef.Calls = append(ef.Calls, *convertFrame(child))
	}
	return ef
}

func collectLogs(f traceFrame) []usecase.RawLog {
	var logs []usecase.RawLog
	for _, l := range f.Logs {
		logs = append(logs, usecase.RawLog{
			Address: l.Address,
			Topics:  l.Topics,
			Data:    l.Data,
		})
	}
	for _, child := range f.Calls {
		logs = append(logs, collectLogs(child)...)
	}
	return logs
}

func callDepth(f traceFrame) int {
	max := 0
	for _, child := range f.Calls {
		if d := callDepth(child); d > max {
			max = d
		}
	}
	return max + 1
}

func hexToUint64(s string) uint64 {
	s = strings.TrimPrefix(s, "0x")
	if s == "" {
		return 0
	}
	n := new(big.Int)
	n.SetString(s, 16)
	return n.Uint64()
}
