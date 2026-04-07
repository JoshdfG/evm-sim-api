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

// Well-known EVM event topic hashes (keccak256 of the event signature).
const (
	topicERC20Transfer = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	topicERC20Approval = "0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"
)

// ERC20 function selectors first 4 bytes of keccak256(signature).
var (
	selectorSymbol   = []byte{0x95, 0xd8, 0x9b, 0x41} // symbol()
	selectorDecimals = []byte{0x31, 0x3c, 0xe5, 0x67} // decimals()
)

type tokenMeta struct {
	symbol   string
	decimals uint8
}

// EVMTokenDecoder implements usecase.TokenDecoder.
// Converts raw EVM logs + call trace into human-readable asset changes.
// Symbol and decimals are fetched live via eth_call and cached per instance.
type EVMTokenDecoder struct {
	client     *ethclient.Client
	tokenCache map[string]*tokenMeta
	log        zerolog.Logger
}

func NewEVMTokenDecoder(client *ethclient.Client, log zerolog.Logger) *EVMTokenDecoder {
	return &EVMTokenDecoder{
		client:     client,
		tokenCache: map[string]*tokenMeta{},
		log:        log.With().Str("component", "token_decoder").Logger(),
	}
}

// Decode implements usecase.TokenDecoder.
func (d *EVMTokenDecoder) Decode(
	logs []usecase.RawLog,
	callTrace *entity.CallFrame,
	_, _ string,
) ([]entity.AssetChange, []entity.DecodedLog, error) {
	d.log.Debug().Int("raw_log_count", len(logs)).Msg("decoding logs")

	type deltaKey struct{ addr, token string }
	deltas := map[deltaKey]*big.Int{}

	var decodedLogs []entity.DecodedLog
	var approvalChanges []entity.AssetChange

	for _, l := range logs {
		if len(l.Topics) == 0 {
			continue
		}
		topic0 := strings.ToLower(l.Topics[0])

		switch topic0 {
		case topicERC20Transfer:
			if len(l.Topics) < 3 {
				continue
			}
			sender := common.HexToAddress(l.Topics[1]).Hex()
			receiver := common.HexToAddress(l.Topics[2]).Hex()
			amount := new(big.Int).SetBytes(mustDecodeHex(l.Data))

			kOut := deltaKey{strings.ToLower(sender), strings.ToLower(l.Address)}
			kIn := deltaKey{strings.ToLower(receiver), strings.ToLower(l.Address)}
			if deltas[kOut] == nil {
				deltas[kOut] = new(big.Int)
			}
			if deltas[kIn] == nil {
				deltas[kIn] = new(big.Int)
			}
			deltas[kOut].Sub(deltas[kOut], amount)
			deltas[kIn].Add(deltas[kIn], amount)

			decodedLogs = append(decodedLogs, entity.DecodedLog{
				Address:  l.Address,
				EventSig: "Transfer(address,address,uint256)",
				Decoded:  map[string]any{"from": sender, "to": receiver, "value": amount.String()},
				Raw:      l.Topics,
			})
			d.log.Debug().Str("token", l.Address).Str("from", sender).Str("to", receiver).Str("amount", amount.String()).Msg("Transfer decoded")

		case topicERC20Approval:
			if len(l.Topics) < 3 {
				continue
			}
			owner := common.HexToAddress(l.Topics[1]).Hex()
			spender := common.HexToAddress(l.Topics[2]).Hex()
			amount := new(big.Int).SetBytes(mustDecodeHex(l.Data))

			decodedLogs = append(decodedLogs, entity.DecodedLog{
				Address:  l.Address,
				EventSig: "Approval(address,address,uint256)",
				Decoded:  map[string]any{"owner": owner, "spender": spender, "value": amount.String()},
				Raw:      l.Topics,
			})
			// Surface approval amounts to the risk analyzer.
			approvalChanges = append(approvalChanges, entity.AssetChange{
				Type:         entity.AssetChangeERC20,
				Address:      spender,
				TokenAddress: l.Address,
				RawAmount:    entity.NewBigIntString(amount),
				HumanAmount:  entity.NormaliseHumanAmount(amount.String()), // Approval usually raw
			})
			d.log.Debug().Str("token", l.Address).Str("spender", spender).Str("amount", amount.String()).Msg("Approval decoded")
		}
	}

	// Native ETH changes derived from the call tree value fields.
	var changes []entity.AssetChange
	if callTrace != nil {
		nativeChanges := extractNativeChanges(callTrace)
		changes = append(changes, nativeChanges...)
		if len(nativeChanges) > 0 {
			d.log.Debug().Int("native_change_count", len(nativeChanges)).Msg("native ETH changes extracted")
		}
	}
	changes = append(changes, approvalChanges...)

	// Convert ERC20 net deltas into AssetChange entries.
	for k, delta := range deltas {
		if delta.Sign() == 0 {
			continue
		}
		meta := d.lookupToken(k.token)
		changes = append(changes, entity.AssetChange{
			Type:          entity.AssetChangeERC20,
			Address:       k.addr,
			TokenAddress:  k.token,
			TokenSymbol:   meta.symbol,
			TokenDecimals: meta.decimals,
			RawAmount:     entity.NewBigIntString(delta),
			HumanAmount:   formatAmount(delta, meta.decimals),
		})
	}

	d.log.Debug().Int("asset_change_count", len(changes)).Int("decoded_log_count", len(decodedLogs)).Msg("decode complete")
	return changes, decodedLogs, nil
}

// extractNativeChanges walks the call tree summing ETH value transfers per address.
func extractNativeChanges(frame *entity.CallFrame) []entity.AssetChange {
	deltas := map[string]*big.Int{}

	var walk func(f *entity.CallFrame)
	walk = func(f *entity.CallFrame) {
		if f.Value != "" && f.Value != "0x0" && f.Value != "0x" {
			v := new(big.Int)
			v.SetString(strings.TrimPrefix(f.Value, "0x"), 16)
			if v.Sign() > 0 {
				from := strings.ToLower(f.From)
				to := strings.ToLower(f.To)
				if deltas[from] == nil {
					deltas[from] = new(big.Int)
				}
				if deltas[to] == nil {
					deltas[to] = new(big.Int)
				}
				deltas[from].Sub(deltas[from], v)
				deltas[to].Add(deltas[to], v)
			}
		}
		for i := range f.Calls {
			walk(&f.Calls[i])
		}
	}
	walk(frame)

	var changes []entity.AssetChange
	for addr, delta := range deltas {
		if delta.Sign() == 0 {
			continue
		}
		changes = append(changes, entity.AssetChange{
			Type:        entity.AssetChangeNative,
			Address:     addr,
			RawAmount:   entity.NewBigIntString(delta),
			HumanAmount: formatAmount(delta, 18),
		})
	}
	return changes
}

// lookupToken fetches symbol() and decimals() from the token contract via eth_call.
// Results are cached in-process  no repeat RPC calls for the same token.
func (d *EVMTokenDecoder) lookupToken(address string) *tokenMeta {
	if m, ok := d.tokenCache[address]; ok {
		return m
	}

	m := &tokenMeta{symbol: "UNKNOWN", decimals: 18}

	if d.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		addr := common.HexToAddress(address)

		// symbol()
		if symData, err := d.client.CallContract(ctx, ethereum.CallMsg{
			To:   &addr,
			Data: selectorSymbol,
		}, nil); err == nil {
			m.symbol = decodeStringReturn(symData)
		} else {
			d.log.Debug().Err(err).Str("token", address).Msg("symbol() lookup failed")
		}

		// decimals()
		if decData, err := d.client.CallContract(ctx, ethereum.CallMsg{
			To:   &addr,
			Data: selectorDecimals,
		}, nil); err == nil && len(decData) >= 32 {
			m.decimals = uint8(new(big.Int).SetBytes(decData[len(decData)-32:]).Uint64())
		} else if err != nil {
			d.log.Debug().Err(err).Str("token", address).Msg("decimals() lookup failed")
		}
	}

	d.log.Debug().Str("token", address).Str("symbol", m.symbol).Uint("decimals", uint(m.decimals)).Msg("token meta resolved")
	d.tokenCache[address] = m
	return m
}

// decodeStringReturn ABI-decodes a bytes-encoded string return value.
// Handles standard ABI (32-byte offset + 32-byte length + data) and
// legacy bytes32 encoding used by some old tokens (MKR, etc.).
func decodeStringReturn(data []byte) string {
	if len(data) == 0 {
		return "UNKNOWN"
	}
	// Standard ABI string: offset(32) + length(32) + data
	if len(data) >= 96 {
		offset := new(big.Int).SetBytes(data[:32]).Int64()
		if offset == 32 {
			strLen := new(big.Int).SetBytes(data[32:64]).Int64()
			if strLen > 0 && int64(len(data)) >= 64+strLen {
				return strings.TrimRight(string(data[64:64+strLen]), "\x00")
			}
		}
	}
	// Legacy bytes32 (right-padded with null bytes): MKR, SNX, etc.
	if len(data) == 32 {
		return strings.TrimRight(string(data), "\x00")
	}
	return "UNKNOWN"
}

// formatAmount converts a raw big.Int to a human-readable decimal string.
// e.g. 2_000_000_000 with decimals=6 → "2000.0"
func formatAmount(amount *big.Int, decimals uint8) string {
	if decimals == 0 {
		return amount.String()
	}
	neg := amount.Sign() < 0
	abs := new(big.Int).Abs(amount)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	whole := new(big.Int).Div(abs, divisor)
	frac := new(big.Int).Mod(abs, divisor)
	fracStr := strings.TrimRight(fmt.Sprintf("%0*s", int(decimals), frac.String()), "0")
	if fracStr == "" {
		fracStr = "0"
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%s.%s", sign, whole.String(), fracStr)
}

func mustDecodeHex(s string) []byte {
	s = strings.TrimPrefix(s, "0x")
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, _ := hex.DecodeString(s)
	return b
}

// Compile-time interface check.
var _ usecase.TokenDecoder = (*EVMTokenDecoder)(nil)
