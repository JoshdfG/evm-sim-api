package usecase

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/joshdfg/evm-sim-api/internal/entity"
)

// maxUint256 is the sentinel value used for unlimited ERC20 approvals.
var maxUint256, _ = new(big.Int).SetString(
	"115792089237316195423570985008687907853269984665640564039457584007913129639935", 10,
)

// DefaultRiskAnalyzer implements RiskAnalyzer with the built-in rule set.
// Add new rules by adding a private method and calling it from Analyze.
type DefaultRiskAnalyzer struct {
	highNativeThresholdWei *big.Int
}

// NewRiskAnalyzer returns a DefaultRiskAnalyzer with a 1 ETH high-transfer threshold.
func NewRiskAnalyzer() *DefaultRiskAnalyzer {
	oneEther := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	return &DefaultRiskAnalyzer{highNativeThresholdWei: oneEther}
}

// Analyze runs all risk rules and returns the combined flag set.
// Works on both the log-decoded path (debug_traceCall) and the Alchemy path.
func (a *DefaultRiskAnalyzer) Analyze(
	req entity.SimulationRequest,
	raw RawSimResult,
	changes []entity.AssetChange,
) []entity.RiskFlag {
	var flags []entity.RiskFlag
	flags = append(flags, a.checkUnlimitedApprovals(changes)...)
	flags = append(flags, a.checkNFTApprovals(raw.Logs)...)
	flags = append(flags, a.checkAlchemyApprovals(raw.AlchemyChanges)...)
	flags = append(flags, a.checkHighNativeTransfer(req, changes)...)
	flags = append(flags, a.checkDelegatecall(raw.CallTrace)...)
	flags = append(flags, a.checkReentrancy(raw.CallTrace)...)
	flags = append(flags, a.checkSelfdestruct(raw.CallTrace)...)
	return flags
}

// checkUnlimitedApprovals detects MAX_UINT256 ERC20 approvals from the log-decoded path.
func (a *DefaultRiskAnalyzer) checkUnlimitedApprovals(changes []entity.AssetChange) []entity.RiskFlag {
	var flags []entity.RiskFlag
	for _, c := range changes {
		if c.Type != entity.AssetChangeERC20 {
			continue
		}
		if c.RawAmount.Int != nil && c.RawAmount.Cmp(maxUint256) == 0 {
			flags = append(flags, entity.RiskFlag{
				Code:     entity.RiskUnlimitedApproval,
				Severity: entity.SeverityCritical,
				Message:  fmt.Sprintf("Unlimited ERC20 approval granted to %s for token %s", c.Address, c.TokenAddress),
				Context: map[string]any{
					"spender":       c.Address,
					"token_address": c.TokenAddress,
					"token_symbol":  c.TokenSymbol,
				},
			})
		}
	}
	return flags
}

// checkAlchemyApprovals inspects the Alchemy-provided change list for APPROVE entries.
// This fires when debug_traceCall is unavailable (free tier) and Alchemy provides
// pre-parsed changes that include changeType: "APPROVE" with the approved amount.
func (a *DefaultRiskAnalyzer) checkAlchemyApprovals(alchemyChanges []entity.AssetChange) []entity.RiskFlag {
	var flags []entity.RiskFlag
	for _, c := range alchemyChanges {
		if c.Type != entity.AssetChangeERC20 {
			continue
		}
		if c.RawAmount.Int == nil {
			continue
		}
		// Alchemy marks approvals as positive amounts to the spender.
		// MAX_UINT256 = unlimited approval.
		if c.RawAmount.Cmp(maxUint256) == 0 {
			flags = append(flags, entity.RiskFlag{
				Code:     entity.RiskUnlimitedApproval,
				Severity: entity.SeverityCritical,
				Message:  fmt.Sprintf("Unlimited ERC20 approval granted to %s for token %s", c.Address, c.TokenAddress),
				Context: map[string]any{
					"spender":       c.Address,
					"token_address": c.TokenAddress,
					"token_symbol":  c.TokenSymbol,
				},
			})
			continue
		}
		// Flag any large approval (>= 1M tokens with 6 decimals, or > 1000 with 18).
		// Threshold: 1_000_000 in base units with the token's own decimals.
		if c.TokenDecimals > 0 {
			threshold := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(c.TokenDecimals)+6), nil)
			if c.RawAmount.Cmp(threshold) >= 0 {
				flags = append(flags, entity.RiskFlag{
					Code:     entity.RiskUnlimitedApproval,
					Severity: entity.SeverityWarning,
					Message:  fmt.Sprintf("Large ERC20 approval of %s %s granted to %s", c.HumanAmount, c.TokenSymbol, c.Address),
					Context: map[string]any{
						"spender":       c.Address,
						"amount_human":  c.HumanAmount,
						"token_address": c.TokenAddress,
						"token_symbol":  c.TokenSymbol,
					},
				})
			}
		}
	}
	return flags
}

// checkNFTApprovals detects setApprovalForAll by its event topic (log-decoded path).
func (a *DefaultRiskAnalyzer) checkNFTApprovals(logs []RawLog) []entity.RiskFlag {
	// keccak256("ApprovalForAll(address,address,bool)")
	const topic = "0x17307eab39ab6107e8899845ad3d59bd9653f200f220920489ca2b5937696c31"
	var flags []entity.RiskFlag
	for _, l := range logs {
		for _, t := range l.Topics {
			if strings.EqualFold(t, topic) {
				flags = append(flags, entity.RiskFlag{
					Code:     entity.RiskSetApprovalForAll,
					Severity: entity.SeverityCritical,
					Message:  fmt.Sprintf("setApprovalForAll detected on %s — grants operator full NFT control", l.Address),
					Context:  map[string]any{"contract": l.Address},
				})
			}
		}
	}
	return flags
}

func (a *DefaultRiskAnalyzer) checkHighNativeTransfer(
	req entity.SimulationRequest,
	changes []entity.AssetChange,
) []entity.RiskFlag {
	var flags []entity.RiskFlag
	for _, c := range changes {
		if c.Type != entity.AssetChangeNative {
			continue
		}
		if !strings.EqualFold(c.Address, req.From) {
			continue
		}
		if c.RawAmount.Int == nil {
			continue
		}
		abs := new(big.Int).Abs(c.RawAmount.Int)
		if abs.Cmp(a.highNativeThresholdWei) >= 0 {
			flags = append(flags, entity.RiskFlag{
				Code:     entity.RiskHighNativeTransfer,
				Severity: entity.SeverityWarning,
				Message:  fmt.Sprintf("Transaction sends %s native tokens — verify recipient carefully", c.HumanAmount),
				Context:  map[string]any{"amount_human": c.HumanAmount, "to": req.To},
			})
		}
	}
	return flags
}

func (a *DefaultRiskAnalyzer) checkDelegatecall(frame *entity.CallFrame) []entity.RiskFlag {
	var flags []entity.RiskFlag
	walkFrames(frame, func(f *entity.CallFrame) {
		if strings.EqualFold(f.Type, "DELEGATECALL") {
			flags = append(flags, entity.RiskFlag{
				Code:     entity.RiskProxyDelegation,
				Severity: entity.SeverityWarning,
				Message:  fmt.Sprintf("DELEGATECALL from %s to %s — storage manipulation risk", f.From, f.To),
				Context:  map[string]any{"from": f.From, "to": f.To},
			})
		}
	})
	return flags
}

func (a *DefaultRiskAnalyzer) checkReentrancy(frame *entity.CallFrame) []entity.RiskFlag {
	counts := map[string]int{}
	walkFrames(frame, func(f *entity.CallFrame) {
		counts[strings.ToLower(f.To)]++
	})
	var flags []entity.RiskFlag
	for addr, n := range counts {
		if n > 1 {
			flags = append(flags, entity.RiskFlag{
				Code:     entity.RiskReentrancy,
				Severity: entity.SeverityWarning,
				Message:  fmt.Sprintf("Contract %s called %d times in one tx — possible reentrancy", addr, n),
				Context:  map[string]any{"contract": addr, "call_count": n},
			})
		}
	}
	return flags
}

func (a *DefaultRiskAnalyzer) checkSelfdestruct(frame *entity.CallFrame) []entity.RiskFlag {
	var flags []entity.RiskFlag
	walkFrames(frame, func(f *entity.CallFrame) {
		if strings.EqualFold(f.Type, "SELFDESTRUCT") {
			flags = append(flags, entity.RiskFlag{
				Code:     entity.RiskSelfDestruct,
				Severity: entity.SeverityCritical,
				Message:  fmt.Sprintf("Contract %s will SELFDESTRUCT — all stored ETH drained", f.From),
				Context:  map[string]any{"contract": f.From},
			})
		}
	})
	return flags
}

func walkFrames(frame *entity.CallFrame, fn func(*entity.CallFrame)) {
	if frame == nil {
		return
	}
	fn(frame)
	for i := range frame.Calls {
		walkFrames(&frame.Calls[i], fn)
	}
}

var _ RiskAnalyzer = (*DefaultRiskAnalyzer)(nil)
