package entity

// RiskSeverity is the threat level of a detected pattern.
type RiskSeverity string

const (
	SeverityInfo     RiskSeverity = "info"
	SeverityWarning  RiskSeverity = "warning"
	SeverityCritical RiskSeverity = "critical"
)

// RiskCode is a stable machine-readable identifier for a risk pattern.
type RiskCode string

const (
	RiskUnlimitedApproval  RiskCode = "UNLIMITED_APPROVAL"
	RiskHighSlippage       RiskCode = "HIGH_SLIPPAGE"
	RiskUnknownSpender     RiskCode = "UNKNOWN_SPENDER"
	RiskSetApprovalForAll  RiskCode = "SET_APPROVAL_FOR_ALL"
	RiskSelfDestruct       RiskCode = "SELFDESTRUCT"
	RiskProxyDelegation    RiskCode = "UNEXPECTED_DELEGATECALL"
	RiskHighNativeTransfer RiskCode = "HIGH_NATIVE_TRANSFER"
	RiskReentrancy         RiskCode = "REENTRANCY_PATTERN"
)

// RiskFlag is one detected suspicious pattern in a simulated transaction.
type RiskFlag struct {
	Code     RiskCode               `json:"code"`
	Severity RiskSeverity           `json:"severity"`
	Message  string                 `json:"message"`
	Context  map[string]interface{} `json:"context,omitempty"`
}
