package evm

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
)

// knownSelectors maps 4-byte function selectors to their human-readable signatures.
// Used to produce friendlier error messages when a custom error is returned.
var knownSelectors = map[string]string{
	// ── OpenZeppelin / ERC20 ─────────────────────────────────────────────────
	"08c379a0": "Error(string)",

	// ── Uniswap V2 ───────────────────────────────────────────────────────────
	"e6c4247b": "InvalidPath()",
	"675cae38": "Expired()",
	"7939f424": "TransferFailed()",
	"18d6b462": "InsufficientOutputAmount()",
	"4c761cba": "ExcessiveInputAmount()",
	"c9f52c71": "Locked()",

	// ── Uniswap V3 ───────────────────────────────────────────────────────────
	"f645eedf": "PriceLimitAlreadyExceeded()",
	"68efce1b": "PriceLimitOutOfBounds(uint160,uint160)",
	"bd5e88c4": "InsufficientInputAmount()",
	"45b96fe0": "LOK()",
	"0a431745": "NotInitialized()",

	// ── AAVE V3 ──────────────────────────────────────────────────────────────
	"2c5b8f4f": "HEALTH_FACTOR_NOT_BELOW_THRESHOLD()",
	"1c26714c": "COLLATERAL_CANNOT_BE_LIQUIDATED()",
	"ee010539": "INVALID_INTEREST_RATE_MODE_SELECTED()",
	"2af7cbca": "BORROW_ALLOWANCE_NOT_ENOUGH()",
	"a82114a3": "RESERVE_PAUSED()",
	"9ae62c3c": "RESERVE_FROZEN()",
	"7a76a5a8": "RESERVE_INACTIVE()",
	"f87be63e": "NOT_ENOUGH_AVAILABLE_USER_BALANCE()",
	"c2c0e3d2": "SUPPLY_CAP_EXCEEDED()",
	"2c66e7ce": "BORROW_CAP_EXCEEDED()",

	// ── ERC4626 Vault ────────────────────────────────────────────────────────
	"a3f16d3e": "ERC4626ExceededMaxDeposit(address,uint256,uint256)",
	"84504081": "ERC4626ExceededMaxMint(address,uint256,uint256)",
	"00601cff": "ERC4626ExceededMaxWithdraw(address,uint256,uint256)",
	"9c44a452": "ERC4626ExceededMaxRedeem(address,uint256,uint256)",

	// ── Common OpenZeppelin ───────────────────────────────────────────────────
	"e07c8dba": "OwnableUnauthorizedAccount(address)",
	"118cdaa7": "OwnableInvalidOwner(address)",
	"fd2dc6ca": "ERC20InsufficientBalance(address,uint256,uint256)",
	"fce698f7": "ERC20InvalidApprover(address)",
	"94280d62": "ERC20InvalidSpender(address)",
	"e450d38c": "ERC20InsufficientAllowance(address,uint256,uint256)",
	"96c6fd1e": "ERC20InvalidReceiver(address)",
	"ec442f05": "ERC20InvalidSender(address)",

	// ── Permit2 ──────────────────────────────────────────────────────────────
	"ddafbaef": "AllowanceExpired(uint256,uint256)",
	"a966c70e": "InsufficientAllowance(uint256,uint256,uint256)",
	"f9e03f0b": "SignatureExpired(uint256)",
}

// knownRevertMessages maps common revert strings to friendlier explanations.
var knownRevertMessages = map[string]string{
	"ERC20: transfer amount exceeds balance":   "Insufficient token balance — sender does not have enough tokens",
	"ERC20: transfer amount exceeds allowance": "Insufficient allowance — approve the contract to spend tokens first",
	"ERC20: insufficient allowance":            "Insufficient allowance — approve the contract to spend tokens first",
	"ERC20: approve to the zero address":       "Cannot approve the zero address as a spender",
	"ERC20: transfer to the zero address":      "Cannot transfer tokens to the zero address",
	"Ownable: caller is not the owner":         "Access denied — only the contract owner can call this function",
	"Pausable: paused":                         "Contract is paused — try again later",
	"ReentrancyGuard: reentrant call":          "Reentrancy detected — this function cannot be called recursively",
	"TRANSFER_FAILED":                          "Token transfer failed — check balance and allowance",
	"INSUFFICIENT_OUTPUT_AMOUNT":               "Slippage too high — increase slippage tolerance or reduce amount",
	"EXCESSIVE_INPUT_AMOUNT":                   "Input amount exceeds maximum — reduce input or increase slippage",
	"EXPIRED":                                  "Transaction deadline has passed — resubmit with a later deadline",
	"INVALID_PATH":                             "Invalid swap path — token pair may not exist on this DEX",
	"UniswapV2: LOCKED":                        "Reentrancy lock active — Uniswap pair is mid-transaction",
}

// DecodeRevertReason converts raw ABI-encoded revert data + an optional RPC error
// string into the most human-readable explanation available.
//
// Priority:
//  1. ABI-decoded Error(string) — standard Solidity revert("message")
//  2. Known custom error selector — matched against knownSelectors
//  3. Known revert string lookup in knownRevertMessages
//  4. Raw selector hex with signature if known
//  5. Original RPC error string
func DecodeRevertReason(rpcErr error, returnData []byte) string {
	raw := returnData
	if len(raw) == 0 && rpcErr != nil {
		// Some nodes return the revert data hex-encoded in the error string.
		raw = extractHexFromError(rpcErr.Error())
	}

	if len(raw) < 4 {
		if rpcErr != nil {
			return friendlyRPCError(rpcErr.Error())
		}
		return "unknown revert"
	}

	selector := fmt.Sprintf("%x", raw[:4])

	// ── 1. Standard Error(string) — selector 0x08c379a0 ──────────────────────
	if selector == "08c379a0" && len(raw) >= 68 {
		msgLen := new(big.Int).SetBytes(raw[36:68]).Int64()
		if msgLen > 0 && int64(len(raw)) >= 68+msgLen {
			msg := string(raw[68 : 68+msgLen])
			if friendly, ok := knownRevertMessages[msg]; ok {
				return friendly
			}
			return msg
		}
	}

	// ── 2. Panic(uint256) — selector 0x4e487b71 ───────────────────────────────
	if selector == "4e487b71" && len(raw) >= 36 {
		code := new(big.Int).SetBytes(raw[4:36]).Uint64()
		return decodePanicCode(code)
	}

	// ── 3. Known custom error selector ────────────────────────────────────────
	if sig, ok := knownSelectors[selector]; ok {
		return fmt.Sprintf("Custom error: %s", sig)
	}

	// ── 4. Unknown selector — return hex for debugging ────────────────────────
	if rpcErr != nil {
		return fmt.Sprintf("Custom error (0x%s): %s", selector, friendlyRPCError(rpcErr.Error()))
	}
	return fmt.Sprintf("Custom error (0x%s)", selector)
}

// decodePanicCode maps Solidity panic codes to human-readable messages.
// See: https://docs.soliditylang.org/en/latest/control-structures.html#panic-via-assert-and-error-via-require
func decodePanicCode(code uint64) string {
	messages := map[uint64]string{
		0x00: "Generic compiler-inserted panic",
		0x01: "Assert failed — condition was false",
		0x11: "Arithmetic overflow or underflow",
		0x12: "Division or modulo by zero",
		0x21: "Invalid enum value conversion",
		0x22: "Corrupted storage byte array",
		0x31: "Pop on empty array",
		0x32: "Array index out of bounds",
		0x41: "Out of memory (too much memory allocated)",
		0x51: "Called a zero-initialized function pointer",
	}
	if msg, ok := messages[code]; ok {
		return fmt.Sprintf("Panic: %s (code 0x%02x)", msg, code)
	}
	return fmt.Sprintf("Panic: unknown code 0x%02x", code)
}

// friendlyRPCError strips JSON-RPC boilerplate from node error strings.
func friendlyRPCError(errStr string) string {
	// Strip common prefixes from go-ethereum RPC error messages.
	for _, prefix := range []string{
		"execution reverted: ",
		"VM Exception while processing transaction: revert ",
	} {
		if strings.HasPrefix(errStr, prefix) {
			return strings.TrimPrefix(errStr, prefix)
		}
	}
	return errStr
}

// extractHexFromError tries to find a 0x-prefixed hex string in an error message
// and decode it as ABI revert data.
func extractHexFromError(errStr string) []byte {
	// Look for "0x" followed by hex chars.
	idx := strings.Index(errStr, "0x")
	if idx < 0 {
		return nil
	}
	hexPart := errStr[idx+2:]
	// Trim to only hex chars.
	end := 0
	for end < len(hexPart) && isHexChar(hexPart[end]) {
		end++
	}
	if end < 8 { // need at least a 4-byte selector
		return nil
	}
	b, err := hex.DecodeString(hexPart[:end])
	if err != nil {
		return nil
	}
	return b
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
