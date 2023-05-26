package challenger

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/trie"

	"github.com/ethereum-optimism/optimism/op-challenger/challenger/mocks"
	"github.com/ethereum-optimism/optimism/op-e2e/e2eutils"
	"github.com/ethereum-optimism/optimism/op-node/testlog"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockTraversalSubscription struct {
	errorChan chan error
}

func (m mockTraversalSubscription) Err() <-chan error {
	return m.errorChan
}

func (m mockTraversalSubscription) Unsubscribe() {}

func newLogTraversal(t *testing.T) (*logTraversal, *mocks.TraversalClient) {
	mockTraversalClient := mocks.NewTraversalClient(t)
	return NewLogTraversal(
		mockTraversalClient,
		&ethereum.FilterQuery{},
		testlog.Logger(t, log.LvlError),
	), mockTraversalClient
}

func TestLogTraversal_Start_ReceivesNewHeads_NoTransactions(t *testing.T) {
	logTraversal, mockTraversalClient := newLogTraversal(t)
	require.False(t, logTraversal.Started())

	done := make(chan struct{})
	handleLog := func(log *types.Log) error {
		done <- struct{}{}
		return nil
	}

	sub := mockTraversalSubscription{
		errorChan: make(chan error),
	}
	var headers chan<- *types.Header
	mockTraversalClient.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(
		sub,
		nil,
	).Run(func(args mock.Arguments) {
		headers = args.Get(1).(chan<- *types.Header)
	})

	require.NoError(t, logTraversal.Start(context.Background(), done, handleLog))
	require.True(t, logTraversal.Started())

	firstHeader := &types.Header{
		Number: big.NewInt(1),
	}
	returnBlock := types.NewBlockWithHeader(firstHeader)
	mockTraversalClient.On(
		"BlockByHash",
		mock.Anything,
		mock.Anything,
	).Return(
		returnBlock,
		nil,
	)

	headers <- firstHeader

	timeout, tCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer tCancel()
	err := e2eutils.WaitFor(timeout, 500*time.Millisecond, func() (bool, error) {
		return logTraversal.LastBlockNumber().Cmp(firstHeader.Number) == 0, nil
	})
	require.NoError(t, err)
}

func TestLogTraversal_Start_ReceivesNewHeads_Transactions(t *testing.T) {
	logTraversal, mockTraversalClient := newLogTraversal(t)
	require.False(t, logTraversal.Started())

	done := make(chan struct{})
	handleLog := func(log *types.Log) error {
		log.Address = common.HexToAddress(("0x02"))
		return nil
	}

	sub := mockTraversalSubscription{
		errorChan: make(chan error),
	}
	var headers chan<- *types.Header
	mockTraversalClient.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(
		sub,
		nil,
	).Run(func(args mock.Arguments) {
		headers = args.Get(1).(chan<- *types.Header)
	})

	require.NoError(t, logTraversal.Start(context.Background(), done, handleLog))
	require.True(t, logTraversal.Started())

	firstHeader := &types.Header{
		Number: big.NewInt(1),
	}

	testKey, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	to := common.Address{}
	tx := types.MustSignNewTx(
		testKey,
		types.NewLondonSigner(big.NewInt(1337)),
		&types.DynamicFeeTx{
			ChainID:   big.NewInt(1337),
			Nonce:     uint64(0),
			To:        &to,
			Value:     big.NewInt(0),
			GasTipCap: big.NewInt(0),
			GasFeeCap: big.NewInt(0),
			Gas:       uint64(0),
		})
	txs := []*types.Transaction{tx}
	log := &types.Log{
		Address: to,
		Topics:  []common.Hash{{}},
	}
	receipts := []*types.Receipt{{
		Logs: []*types.Log{log},
	}}
	returnBlock := types.NewBlock(
		firstHeader,
		txs,
		nil,
		receipts,
		trie.NewStackTrie(nil),
	)

	mockTraversalClient.On(
		"BlockByHash",
		mock.Anything,
		mock.Anything,
	).Return(
		returnBlock,
		nil,
	)

	mockTraversalClient.On(
		"TransactionReceipt",
		mock.Anything,
		mock.Anything,
	).Return(
		&types.Receipt{},
		nil,
	)

	headers <- firstHeader

	timeout, tCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer tCancel()
	err := e2eutils.WaitFor(timeout, 500*time.Millisecond, func() (bool, error) {
		return logTraversal.LastBlockNumber().Cmp(firstHeader.Number) == 0, nil
	})
	require.NoError(t, err)
}

func TestLogTraversal_Start_SubscriptionErrors(t *testing.T) {
	logTraversal, mockTraversalClient := newLogTraversal(t)
	require.False(t, logTraversal.Started())

	done := make(chan struct{})
	handleLog := func(log *types.Log) error {
		done <- struct{}{}
		return nil
	}

	errSubscriptionFailed := errors.New("subscription failed")
	mockTraversalClient.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(
		nil,
		errSubscriptionFailed,
	)

	require.EqualError(
		t,
		logTraversal.Start(context.Background(), done, handleLog),
		"operation failed permanently after 4 attempts: subscription failed",
	)
	require.False(t, logTraversal.Started())
}

func TestLogTraversal_Quit_StopsTraversal(t *testing.T) {
	logTraversal, mockTraversalClient := newLogTraversal(t)
	require.False(t, logTraversal.Started())

	done := make(chan struct{})
	handleLog := func(log *types.Log) error {
		done <- struct{}{}
		return nil
	}

	sub := mockTraversalSubscription{
		errorChan: make(chan error),
	}

	mockTraversalClient.On(
		"SubscribeNewHead",
		mock.Anything,
		mock.Anything,
	).Return(
		sub,
		nil,
	)

	require.NoError(t, logTraversal.Start(context.Background(), done, handleLog))
	require.True(t, logTraversal.Started())

	logTraversal.Quit()
	require.False(t, logTraversal.Started())
}
