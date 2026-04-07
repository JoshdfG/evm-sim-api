package evm

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joshdfg/evm-sim-api/internal/entity"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
	"github.com/rs/zerolog"
)

// ArchiveNodeFork implements usecase.EVMFork.
//
// Log collection strategy (in priority order):
//  1. debug_traceCall with callTracer+withLog  full call tree and logs.
//     Available on Alchemy Growth+, QuickNode, Infura paid, self-hosted geth.
//  2. alchemy_simulateAssetChanges  structured asset changes from Alchemy.
//     Available on ALL Alchemy plans including free tier.
//  3. eth_getLogs fallback  reads actual on-chain logs for the block/address.
//     Only useful when blockNumber is specified (historical simulation).
type ArchiveNodeFork struct {
	client  *ethclient.Client
	tracer  *CallTracer
	timeout time.Duration
	maxGas  uint64
	log     zerolog.Logger
}

func NewArchiveNodeFork(rpcURL string, timeoutSecs int, maxGas uint64, log zerolog.Logger) (*ArchiveNodeFork, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("evm/fork: dial: %w", err)
	}
	l := log.With().Str("component", "archive_fork").Logger()
	return &ArchiveNodeFork{
		client:  client,
		tracer:  NewCallTracer(log),
		timeout: time.Duration(timeoutSecs) * time.Second,
		maxGas:  maxGas,
		log:     l,
	}, nil
}

// Client exposes the underlying ethclient for shared use by the token decoder.
func (f *ArchiveNodeFork) Client() *ethclient.Client {
	return f.client
}

func (f *ArchiveNodeFork) Simulate(ctx context.Context, req entity.SimulationRequest) (*usecase.RawSimResult, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	var blockNum *big.Int
	if req.BlockNumber != nil {
		blockNum = req.BlockNumber
	}

	from := common.HexToAddress(req.From)
	to := common.HexToAddress(req.To)

	var data []byte
	if req.Data != "" {
		b, err := hex.DecodeString(strings.TrimPrefix(req.Data, "0x"))
		if err != nil {
			return nil, fmt.Errorf("evm/fork: decode calldata: %w", err)
		}
		data = b
	}

	gasLimit := req.GasLimit
	if gasLimit == 0 {
		gasLimit = f.maxGas
	}

	msg := ethereum.CallMsg{
		From:     from,
		To:       &to,
		Gas:      gasLimit,
		GasPrice: req.GasPrice,
		Value:    req.Value,
		Data:     data,
	}

	// ── 1. eth_call ───────────────────────────────────────────────────────────
	returnData, callErr := f.client.CallContract(ctx, msg, blockNum)

	// ── 2. eth_estimateGas ────────────────────────────────────────────────────
	gasUsed, _ := f.client.EstimateGas(ctx, msg)

	// ── 3. Resolve simulated block ────────────────────────────────────────────
	var simulatedBlock uint64
	if blockNum != nil {
		simulatedBlock = blockNum.Uint64()
	} else {
		if header, err := f.client.HeaderByNumber(ctx, nil); err == nil {
			simulatedBlock = header.Number.Uint64()
		}
	}

	// ── 4. debug_traceCall (strategy 1) ──────────────────────────────────────
	traceResult := f.tracer.TraceCall(ctx, f.client, msg, blockNum)
	f.log.Debug().
		Str("log_source", traceResult.LogSource).
		Int("log_count", len(traceResult.Logs)).
		Msg("trace complete")

	// ── 5. alchemy_simulateAssetChanges fallback (strategy 2) ─────────────────
	// Triggered when debug_traceCall returns no logs.
	// This is the common case on Alchemy free tier and covers ERC20/native changes.
	var alchemyChanges []entity.AssetChange
	if len(traceResult.Logs) == 0 && callErr == nil {
		f.log.Debug().Msg("no trace logs trying alchemy_simulateAssetChanges")
		if changes, err := f.tracer.AlchemySimulate(ctx, f.client, msg, blockNum); err == nil {
			alchemyChanges = convertAlchemyChanges(changes)
			f.log.Debug().Int("change_count", len(alchemyChanges)).Msg("alchemy_simulateAssetChanges succeeded")
		} else {
			f.log.Debug().Err(err).Msg("alchemy_simulateAssetChanges unavailable")
		}
	}

	raw := &usecase.RawSimResult{
		Success:        callErr == nil,
		GasUsed:        gasUsed,
		Logs:           traceResult.Logs,
		CallTrace:      traceResult.Frame,
		BlockNumber:    simulatedBlock,
		AlchemyChanges: alchemyChanges,
	}

	if callErr != nil {
		raw.RevertReason = decodeRevertReason(callErr, returnData)
	}

	return raw, nil
}

// convertAlchemyChanges maps alchemy_simulateAssetChanges output to entity.AssetChange.
func convertAlchemyChanges(in []alchemyAssetChange) []entity.AssetChange {
	out := make([]entity.AssetChange, 0, len(in))
	for _, c := range in {
		raw := new(big.Int)
		raw.SetString(c.RawAmount, 10)

		switch strings.ToUpper(c.AssetType) {
		case "NATIVE":
			// Sender (Outflow)
			out = append(out, entity.AssetChange{
				Type:        entity.AssetChangeNative,
				Address:     c.From,
				RawAmount:   entity.NewBigIntString(new(big.Int).Neg(raw)),
				HumanAmount: entity.NormaliseHumanAmount("-" + c.Amount),
			})
			// Receiver (Inflow)
			out = append(out, entity.AssetChange{
				Type:        entity.AssetChangeNative,
				Address:     c.To,
				RawAmount:   entity.NewBigIntString(raw),
				HumanAmount: entity.NormaliseHumanAmount(c.Amount),
			})

		case "ERC20":
			decimals := uint8(c.Decimals)
			// Sender (Outflow)
			out = append(out, entity.AssetChange{
				Type:          entity.AssetChangeERC20,
				Address:       c.From,
				TokenAddress:  c.ContractAddress,
				TokenSymbol:   c.Symbol,
				TokenDecimals: decimals,
				RawAmount:     entity.NewBigIntString(new(big.Int).Neg(raw)),
				HumanAmount:   entity.NormaliseHumanAmount("-" + c.Amount),
			})
			// Receiver (Inflow)
			out = append(out, entity.AssetChange{
				Type:          entity.AssetChangeERC20,
				Address:       c.To,
				TokenAddress:  c.ContractAddress,
				TokenSymbol:   c.Symbol,
				TokenDecimals: decimals,
				RawAmount:     entity.NewBigIntString(raw),
				HumanAmount:   entity.NormaliseHumanAmount(c.Amount),
			})

		case "ERC721":
			tokenID := new(big.Int)
			tokenID.SetString(c.TokenID, 10)
			out = append(out, entity.AssetChange{
				Type:         entity.AssetChangeERC721,
				Address:      c.To,
				TokenAddress: c.ContractAddress,
				TokenSymbol:  c.Symbol,
				TokenID:      &entity.BigIntString{Int: tokenID},
				RawAmount:    entity.BigIntString{Int: big.NewInt(1)},
				HumanAmount:  "1.0", // Hardcoded normalized string
			})
		}
	}
	return out
}

func decodeRevertReason(err error, returnData []byte) string {
	if len(returnData) > 4 {
		sel := returnData[:4]
		if sel[0] == 0x08 && sel[1] == 0xc3 && sel[2] == 0x79 && sel[3] == 0xa0 {
			if len(returnData) >= 68 {
				msgLen := new(big.Int).SetBytes(returnData[36:68]).Int64()
				if int64(len(returnData)) >= 68+msgLen {
					return string(returnData[68 : 68+msgLen])
				}
			}
		}
	}
	if err != nil {
		return err.Error()
	}
	return "unknown revert"
}

var _ usecase.EVMFork = (*ArchiveNodeFork)(nil)
