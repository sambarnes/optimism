package challenger

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-challenger/metrics"
	"github.com/ethereum-optimism/optimism/op-node/eth"
	"github.com/ethereum-optimism/optimism/op-node/testlog"
)

func TestChallenger_ValidateOutput_PanicsWithoutRollupClient(t *testing.T) {
	challenger := Challenger{}
	require.Panics(t, func() {
		_, _, err := challenger.ValidateOutput(context.Background(), nil, eth.Bytes32{})
		require.NoError(t, err)
	})
}

func TestChallenger_ValidateOutput_PanicsWithoutLog(t *testing.T) {
	outputApi := &mockOutputApi{}
	challenger := Challenger{
		rollupClient: outputApi,
	}
	require.Panics(t, func() {
		_, _, err := challenger.ValidateOutput(context.Background(), nil, eth.Bytes32{})
		require.NoError(t, err)
	})
}

func TestChallenger_ValidateOutput_PanicsWithoutMetricer(t *testing.T) {
	outputApi := mockOutputApi{}
	log := testlog.Logger(t, log.LvlError)
	metr := metrics.NewMetrics("test")
	challenger := &Challenger{
		rollupClient:   &outputApi,
		log:            log,
		metr:           metr,
		networkTimeout: time.Duration(5) * time.Second,
	}
	require.Panics(t, func() {
		_, _, err := challenger.ValidateOutput(context.Background(), nil, eth.Bytes32{})
		require.NoError(t, err)
	})
}

func TestChallenger_ValidateOutput_RollupClientErrors(t *testing.T) {
	output := eth.OutputResponse{
		Version:    supportedL2OutputVersion,
		OutputRoot: eth.Bytes32{},
		BlockRef:   eth.L2BlockRef{},
	}

	outputApi := newMockOutputApi(output, true)
	log := testlog.Logger(t, log.LvlError)
	metr := metrics.NewMetrics("test")
	challenger := Challenger{
		rollupClient:   outputApi,
		log:            log,
		metr:           metr,
		networkTimeout: time.Duration(5) * time.Second,
	}

	l2BlockNumber := big.NewInt(0)
	valid, received, err := challenger.ValidateOutput(context.Background(), l2BlockNumber, output.OutputRoot)
	require.False(t, valid)
	require.Nil(t, received)
	require.EqualError(t, err, mockOutputApiError.Error())
}

func TestChallenger_ValidateOutput_ErrorsWithWrongVersion(t *testing.T) {
	output := eth.OutputResponse{
		Version:    eth.Bytes32{0x01},
		OutputRoot: eth.Bytes32{0x01},
		BlockRef:   eth.L2BlockRef{},
	}

	outputApi := newMockOutputApi(output, false)
	log := testlog.Logger(t, log.LvlError)
	metr := metrics.NewMetrics("test")
	challenger := Challenger{
		rollupClient:   outputApi,
		log:            log,
		metr:           metr,
		networkTimeout: time.Duration(5) * time.Second,
	}

	l2BlockNumber := big.NewInt(0)
	valid, received, err := challenger.ValidateOutput(context.Background(), l2BlockNumber, eth.Bytes32{})
	require.False(t, valid)
	require.Nil(t, received)
	require.EqualError(t, err, ErrUnsupportedL2OOVersion.Error())
}

func TestChallenger_ValidateOutput_ErrorsInvalidBlockNumber(t *testing.T) {
	output := eth.OutputResponse{
		Version:    supportedL2OutputVersion,
		OutputRoot: eth.Bytes32{0x01},
		BlockRef:   eth.L2BlockRef{},
	}

	outputApi := newMockOutputApi(output, false)
	log := testlog.Logger(t, log.LvlError)
	metr := metrics.NewMetrics("test")
	challenger := Challenger{
		rollupClient:   outputApi,
		log:            log,
		metr:           metr,
		networkTimeout: time.Duration(5) * time.Second,
	}

	l2BlockNumber := big.NewInt(10)
	valid, received, err := challenger.ValidateOutput(context.Background(), l2BlockNumber, output.OutputRoot)
	require.False(t, valid)
	require.Nil(t, received)
	require.EqualError(t, err, ErrInvalidBlockNumber.Error())
}

// TestOutput_ValidateOutput tests that an Output is correctly validated.
func TestOutput_ValidateOutput(t *testing.T) {
	output := eth.OutputResponse{
		Version:    eth.Bytes32{},
		OutputRoot: eth.Bytes32{},
		BlockRef:   eth.L2BlockRef{},
	}

	outputApi := newMockOutputApi(output, false)
	log := testlog.Logger(t, log.LvlError)
	metr := metrics.NewMetrics("test")
	challenger := Challenger{
		rollupClient:   outputApi,
		log:            log,
		metr:           metr,
		networkTimeout: time.Duration(5) * time.Second,
	}

	l2BlockNumber := big.NewInt(0)
	valid, expected, err := challenger.ValidateOutput(context.Background(), l2BlockNumber, output.OutputRoot)
	require.Equal(t, *expected, output.OutputRoot)
	require.True(t, valid)
	require.NoError(t, err)
}

var mockOutputApiError = errors.New("mock output api error")

type mockOutputApi struct {
	mock.Mock
	expected eth.OutputResponse
	errors   bool
}

func newMockOutputApi(output eth.OutputResponse, errors bool) *mockOutputApi {
	return &mockOutputApi{
		expected: output,
		errors:   errors,
	}
}

func (m *mockOutputApi) OutputAtBlock(ctx context.Context, blockNumber uint64) (*eth.OutputResponse, error) {
	if m.errors {
		return nil, mockOutputApiError
	}
	return &m.expected, nil
}
