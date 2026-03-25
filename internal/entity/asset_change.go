package entity

import "math/big"

// AssetChangeType distinguishes native coin moves from token moves.
type AssetChangeType string

const (
	AssetChangeNative  AssetChangeType = "native"
	AssetChangeERC20   AssetChangeType = "erc20"
	AssetChangeERC721  AssetChangeType = "erc721"
	AssetChangeERC1155 AssetChangeType = "erc1155"
)

// AssetChange is one row in the "what changed" table for a simulation.
// Positive RawAmount = inflow. Negative = outflow.
type AssetChange struct {
	Type          AssetChangeType `json:"type"`
	Address       string          `json:"address"`
	TokenAddress  string          `json:"token_address,omitempty"`
	TokenSymbol   string          `json:"token_symbol,omitempty"`
	TokenDecimals uint8           `json:"token_decimals,omitempty"`
	TokenID       *big.Int        `json:"token_id,omitempty" swaggertype:"string"`
	RawAmount     *big.Int        `json:"raw_amount" swaggertype:"string"`
	HumanAmount   string          `json:"human_amount"`
}
