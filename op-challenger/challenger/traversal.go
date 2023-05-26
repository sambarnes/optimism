package challenger

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/backoff"
)

var (
	ErrAlreadyStarted = errors.New("already started")
)

//go:generate mockery --name TraversalClient
type TraversalClient interface {
	SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error)
	HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error)
}

// LogTraversal is a client that can traverse all logs.
type LogTraversal interface {
	Start(context.Context, chan struct{}, func(types.Log) error) error
	Quit()
}

// logTraversal implements LogTraversal.
type logTraversal struct {
	log             log.Logger
	client          TraversalClient
	query           *ethereum.FilterQuery
	quit            chan struct{}
	lastBlockNumber *big.Int
	started         bool
}

// NewLogTraversal creates a new log traversal.
func NewLogTraversal(client TraversalClient, query *ethereum.FilterQuery, log log.Logger) *logTraversal {
	return &logTraversal{
		client:          client,
		query:           query,
		quit:            make(chan struct{}),
		log:             log,
		lastBlockNumber: big.NewInt(0),
	}
}

// lastBlockNumber returns the last block number that was traversed.
func (l *logTraversal) LastBlockNumber() *big.Int {
	return l.lastBlockNumber
}

// buildBackoffStrategy builds a [backoff.Strategy].
func (l *logTraversal) buildBackoffStrategy() backoff.Strategy {
	return &backoff.ExponentialStrategy{
		Min:       1000,
		Max:       20_000,
		MaxJitter: 250,
	}
}

// fetchBlock gracefully fetches a block with a backoff.
func (l *logTraversal) fetchBlock(ctx context.Context, hash common.Hash) (*types.Block, error) {
	backoffStrategy := l.buildBackoffStrategy()
	var block *types.Block
	err := backoff.DoCtx(ctx, 5, backoffStrategy, func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		var err error
		block, err = l.client.BlockByHash(ctx, hash)
		return err
	})
	return block, err
}

// fetchTransactionReceipts fetches receipts for a list of transaction with a backoff.
func (l *logTraversal) fetchTransactionReceipts(ctx context.Context, txs []*types.Transaction) ([]*types.Receipt, error) {
	backoffStrategy := l.buildBackoffStrategy()
	var receipts []*types.Receipt
	for _, tx := range txs {
		err := backoff.DoCtx(ctx, 5, backoffStrategy, func() error {
			ctx, cancel := context.WithTimeout(ctx, time.Second*10)
			defer cancel()
			receipt, err := l.client.TransactionReceipt(ctx, tx.Hash())
			receipts = append(receipts, receipt)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	return receipts, nil
}

// subscribeNewHead subscribes to new heads with a backoff.
func (l *logTraversal) subscribeNewHead(ctx context.Context, headers chan *types.Header) (ethereum.Subscription, error) {
	backoffStrategy := l.buildBackoffStrategy()
	var sub ethereum.Subscription
	err := backoff.DoCtx(ctx, 4, backoffStrategy, func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		var err error
		sub, err = l.client.SubscribeNewHead(ctx, headers)
		return err
	})
	return sub, err
}

// Start starts the log traversal.
func (l *logTraversal) Start(ctx context.Context, done chan struct{}, handleLog func(*types.Log) error) error {
	if l.started {
		return ErrAlreadyStarted
	}
	headers := make(chan *types.Header)
	sub, err := l.subscribeNewHead(ctx, headers)
	if err != nil {
		l.log.Error("failed to subscribe to new heads", "err", err)
		return err
	}
	go l.onNewHead(ctx, headers, handleLog)
	go l.unsubscribeOnDone(done, sub)
	l.started = true
	return nil
}

// Started returns true if the log traversal has started.
func (l *logTraversal) Started() bool {
	return l.started
}

// onNewHead handles a new [types.Header].
func (l *logTraversal) onNewHead(ctx context.Context, headers chan *types.Header, handleLog func(*types.Log) error) {
	for {
		select {
		case <-l.quit:
			l.log.Info("stopping log traversal: received quit signal")
			return
		case header := <-headers:
			l.log.Info("received new head", "number", header.Number)
			l.dispatchNewHead(ctx, header, handleLog, true)
		}
	}
}

// fetchHeaderByNumber gracefully fetches a block header by number with a backoff.
func (l *logTraversal) fetchHeaderByNumber(ctx context.Context, num *big.Int) (*types.Header, error) {
	backoffStrategy := l.buildBackoffStrategy()
	var header *types.Header
	err := backoff.DoCtx(ctx, 5, backoffStrategy, func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		var err error
		header, err = l.client.HeaderByNumber(ctx, num)
		return err
	})
	return header, err
}

// spawnCatchup spawns a new goroutine to "catchup" for missed/skipped blocks.
func (l *logTraversal) spawnCatchup(ctx context.Context, start *big.Int, end *big.Int, handleLog func(*types.Log) error) {
	for {
		// Break if we've caught up (start > end).
		if start.Cmp(end) == 1 {
			l.log.Info("caught up")
			return
		}

		// Fetch the header with a backoff
		header, err := l.fetchHeaderByNumber(ctx, start)
		if err != nil {
			l.log.Error("failed to fetch header", "err", err)
			return
		}
		l.dispatchNewHead(ctx, header, handleLog, false)

		// Increment to the next block to catch up to
		start = start.Add(start, big.NewInt(1))
	}
}

// dispatchNewHead dispatches a new head.
func (l *logTraversal) dispatchNewHead(ctx context.Context, header *types.Header, handleLog func(*types.Log) error, allowCatchup bool) {
	block, err := l.fetchBlock(ctx, header.Hash())
	if err != nil {
		l.log.Error("failed to fetch block", "err", err)
		return
	}
	expectedBlockNumber := l.lastBlockNumber.Add(l.lastBlockNumber, big.NewInt(1))
	currentBlockNumber := block.Number()
	if block.Number().Cmp(expectedBlockNumber) == 1 {
		l.log.Warn("detected skipped block", "expectedBlockNumber", expectedBlockNumber, "currentBlockNumber", currentBlockNumber)
		if allowCatchup {
			endBlockNum := currentBlockNumber.Sub(currentBlockNumber, big.NewInt(1))
			l.log.Warn("spawning catchup thread in range [%d, %d]", expectedBlockNumber, endBlockNum)
			go l.spawnCatchup(ctx, expectedBlockNumber, endBlockNum, handleLog)
		} else {
			l.log.Warn("missed block detected, but catchup disabled")
		}
	}
	// Update the block number before doing network calls
	l.lastBlockNumber = block.Number()
	receipts, err := l.fetchTransactionReceipts(ctx, block.Transactions())
	if err != nil {
		l.log.Error("failed to fetch receipts", "err", err)
		return
	}
	for _, receipt := range receipts {
		for _, log := range receipt.Logs {
			err := handleLog(log)
			if err != nil {
				l.log.Error("failed to handle log", "err", err)
				return
			}
		}
	}
}

// unsubscribeOnDone unsubscribes from new heads when done.
func (l *logTraversal) unsubscribeOnDone(done chan struct{}, sub ethereum.Subscription) {
	<-done
	l.log.Info("stopping log traversal: received done signal")
	sub.Unsubscribe()
	l.started = false
}

// Quit quits the log traversal.
func (l *logTraversal) Quit() {
	l.quit <- struct{}{}
	l.started = false
}
