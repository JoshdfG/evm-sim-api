package usecase_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/joshdfg/evm-sim-api/internal/entity"
	"github.com/joshdfg/evm-sim-api/internal/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── mock EVMFork ──────────────────────────────────────────────────────────────

type mockFork struct {
	result *usecase.RawSimResult
	err    error
}

func (m *mockFork) Simulate(_ context.Context, _ entity.SimulationRequest) (*usecase.RawSimResult, error) {
	return m.result, m.err
}

// ── mock SimulationRepository ─────────────────────────────────────────────────

type mockRepo struct {
	saved []entity.SimulationResult
}

func (m *mockRepo) Save(_ context.Context, _ entity.SimulationRequest, r entity.SimulationResult) error {
	m.saved = append(m.saved, r)
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id string) (*entity.SimulationResult, error) {
	for _, r := range m.saved {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, nil
}

func (m *mockRepo) ListByAPIKey(_ context.Context, _ string, _, _ int) ([]entity.SimulationResult, error) {
	return m.saved, nil
}

// ── mock TokenDecoder ─────────────────────────────────────────────────────────

type mockDecoder struct{}

func (m *mockDecoder) Decode(_ []usecase.RawLog, _ *entity.CallFrame, _, _ string) ([]entity.AssetChange, []entity.DecodedLog, error) {
	return nil, nil, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newUC(fork usecase.EVMFork) (*usecase.SimulationUseCase, *mockRepo) {
	repo := &mockRepo{}
	return usecase.NewSimulationUseCase(fork, repo, &mockDecoder{}, usecase.NewRiskAnalyzer()), repo
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestSimulate_Success(t *testing.T) {
	fork := &mockFork{result: &usecase.RawSimResult{
		Success:     true,
		GasUsed:     21000,
		BlockNumber: 19_000_000,
	}}
	uc, repo := newUC(fork)

	result, err := uc.Run(context.Background(), entity.SimulationRequest{
		ChainID: 1,
		From:    "0xabc",
		To:      "0xdef",
	})

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, uint64(21000), result.GasUsed)
	assert.Equal(t, uint64(25200), result.GasEstimate) // 21000 + 20%
	assert.Len(t, repo.saved, 1)
}

func TestSimulate_Revert(t *testing.T) {
	fork := &mockFork{result: &usecase.RawSimResult{
		Success:      false,
		RevertReason: "ERC20: insufficient allowance",
		GasUsed:      8000,
		BlockNumber:  19_000_000,
	}}
	uc, _ := newUC(fork)

	result, err := uc.Run(context.Background(), entity.SimulationRequest{ChainID: 1})
	require.NoError(t, err)
	assert.False(t, result.Success)
	assert.Equal(t, "ERC20: insufficient allowance", result.RevertReason)
}

func TestRiskAnalyzer_UnlimitedApproval(t *testing.T) {
	analyzer := usecase.NewRiskAnalyzer()
	maxU256, _ := new(big.Int).SetString(
		"115792089237316195423570985008687907853269984665640564039457584007913129639935", 10,
	)

	changes := []entity.AssetChange{
		{
			Type:         entity.AssetChangeERC20,
			Address:      "0xspender",
			TokenAddress: "0xusdc",
			TokenSymbol:  "USDC",
			RawAmount:    entity.BigIntString{Int: maxU256},
		},
	}

	flags := analyzer.Analyze(
		entity.SimulationRequest{From: "0xuser"},
		usecase.RawSimResult{},
		changes,
	)

	require.Len(t, flags, 1)
	assert.Equal(t, entity.RiskUnlimitedApproval, flags[0].Code)
	assert.Equal(t, entity.SeverityCritical, flags[0].Severity)
}

func TestRiskAnalyzer_HighNativeTransfer(t *testing.T) {
	analyzer := usecase.NewRiskAnalyzer()
	twoETH := new(big.Int).Mul(
		big.NewInt(2),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil),
	)

	changes := []entity.AssetChange{
		{
			Type:        entity.AssetChangeNative,
			Address:     "0xsender",
			RawAmount:   entity.BigIntString{Int: new(big.Int).Neg(twoETH)},
			HumanAmount: "-2.0",
		},
	}

	flags := analyzer.Analyze(
		entity.SimulationRequest{From: "0xsender"},
		usecase.RawSimResult{},
		changes,
	)

	require.Len(t, flags, 1)
	assert.Equal(t, entity.RiskHighNativeTransfer, flags[0].Code)
}

func TestRiskAnalyzer_CleanTxNoFlags(t *testing.T) {
	analyzer := usecase.NewRiskAnalyzer()
	changes := []entity.AssetChange{
		{
			Type:      entity.AssetChangeNative,
			Address:   "0xsender",
			RawAmount: entity.BigIntString{Int: big.NewInt(-1000)},
		},
	}

	flags := analyzer.Analyze(
		entity.SimulationRequest{From: "0xsender"},
		usecase.RawSimResult{},
		changes,
	)

	assert.Empty(t, flags)
}

func TestRiskAnalyzer_Reentrancy(t *testing.T) {
	analyzer := usecase.NewRiskAnalyzer()

	frame := &entity.CallFrame{
		Type: "CALL",
		From: "0xuser",
		To:   "0xcontract",
		Calls: []entity.CallFrame{
			{Type: "CALL", From: "0xcontract", To: "0xvictim"},
			{Type: "CALL", From: "0xvictim", To: "0xcontract"}, // re-enters 0xcontract
		},
	}

	flags := analyzer.Analyze(
		entity.SimulationRequest{From: "0xuser"},
		usecase.RawSimResult{CallTrace: frame},
		nil,
	)

	codes := make([]entity.RiskCode, len(flags))
	for i, f := range flags {
		codes[i] = f.Code
	}
	assert.Contains(t, codes, entity.RiskReentrancy)
}
