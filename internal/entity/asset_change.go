package entity

import (
	"fmt"
	"math/big"
	"strings"
)

// BigIntString is a wrapper around big.Int that marshals/unmarshals as a JSON string.
type BigIntString struct {
	*big.Int
}

func (b BigIntString) MarshalJSON() ([]byte, error) {
	if b.Int == nil {
		return []byte("null"), nil
	}
	return fmt.Appendf(nil, "%q", b.Int.String()), nil
}

func (b *BigIntString) UnmarshalJSON(p []byte) error {
	s := strings.Trim(string(p), "\"")
	if s == "null" {
		b.Int = nil
		return nil
	}
	b.Int = new(big.Int)
	_, ok := b.Int.SetString(s, 10)
	if !ok {
		return fmt.Errorf("invalid big.Int string: %s", s)
	}
	return nil
}

func NewBigIntString(i *big.Int) BigIntString {
	return BigIntString{Int: i}
}

type AssetChangeType string

const (
	AssetChangeNative  AssetChangeType = "native"
	AssetChangeERC20   AssetChangeType = "erc20"
	AssetChangeERC721  AssetChangeType = "erc721"
	AssetChangeERC1155 AssetChangeType = "erc1155"
)

type AssetChange struct {
	Type          AssetChangeType `json:"type"`
	Address       string          `json:"address"`
	TokenAddress  string          `json:"token_address,omitempty"`
	TokenSymbol   string          `json:"token_symbol,omitempty"`
	TokenDecimals uint8           `json:"token_decimals,omitempty"`
	TokenID       *BigIntString   `json:"token_id,omitempty" swaggertype:"string"`
	RawAmount     BigIntString    `json:"raw_amount" swaggertype:"string"`
	HumanAmount   string          `json:"human_amount"`
}

// NormaliseHumanAmount ensures the string always contains a decimal point.
// e.g. "2000" -> "2000.0", "-2000" -> "-2000.0"
func NormaliseHumanAmount(s string) string {
	if s == "" || s == "-" {
		return "0.0"
	}
	// If it's just "-2000", make it "-2000.0"
	if !strings.Contains(s, ".") {
		return s + ".0"
	}
	// If it's "2000.", make it "2000.0"
	if strings.HasSuffix(s, ".") {
		return s + "0"
	}
	return s
}
