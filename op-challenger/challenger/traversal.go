package challenger

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/backoff"
)

// LogTraversal is a client that can traverse all logs.
type LogTraversal interface {
	WithClient(client *ethclient.Client) LogTraversal
	WithQuery(query *ethereum.FilterQuery) LogTraversal
	Start(context.Context, chan struct{}, func(types.Log) error) error
	Quit() error
}

// logTraversal implements LogTraversal.
type logTraversal struct {
	log    log.Logger
	client *ethclient.Client
	query  ethereum.FilterQuery
	quit   chan struct{}
}

// NewLogTraversal creates a new log traversal.
func NewLogTraversal() *logTraversal {
	return &logTraversal{
		client: nil,
		query:  ethereum.FilterQuery{},
		quit:   make(chan struct{}),
	}
}

// WithClient sets the client.
func (l *logTraversal) WithClient(client *ethclient.Client) *logTraversal {
	l.client = client
	return l
}

// WithQuery sets the query.
func (l *logTraversal) WithQuery(query *ethereum.FilterQuery) *logTraversal {
	l.query = *query
	return l
}

// buildBackoffStrategy builds a [backoff.Strategy].
func (l *logTraversal) buildBackoffStrategy() backoff.Strategy {
	return &backoff.ExponentialStrategy{
		Min:       1000,
		Max:       20_000,
		MaxJitter: 250,
	}
}

// fetchBlock gracfetches a block with a backoff.
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
	err := backoff.DoCtx(ctx, 5, backoffStrategy, func() error {
		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()
		var err error
		sub, err = l.client.SubscribeNewHead(ctx, headers)
		return err
	})
	return sub, err
}

// Start starts the log traversal.
func (l *logTraversal) Start(ctx context.Context, done chan struct{}, handleLog func(*types.Log) error) error {
	headers := make(chan *types.Header)
	sub, err := l.subscribeNewHead(ctx, headers)
	if err != nil {
		l.log.Error("failed to subscribe to new heads", "err", err)
		return err
	}
	go func() {
		for {
			select {
			case <-l.quit:
				l.log.Info("stopping log traversal: received quit signal")
				return
			case header := <-headers:
				block, err := l.fetchBlock(ctx, header.Hash())
				if err != nil {
					l.log.Error("failed to fetch block", "err", err)
					return
				}
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
		}
	}()
	go func() {
		<-done
		l.log.Info("stopping log traversal: received done signal")
		sub.Unsubscribe()
	}()
	return nil
}

// Quit quits the log traversal.
func (l *logTraversal) Quit() error {
	l.quit <- struct{}{}
	return nil
}
