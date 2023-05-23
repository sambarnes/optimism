package challenger

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"

	"github.com/stretchr/testify/require"
)

var (
	testKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	testAddr    = crypto.PubkeyToAddress(testKey.PublicKey)
	testBalance = big.NewInt(2e15)
)

var genesis = &core.Genesis{
	Config:    params.AllEthashProtocolChanges,
	Alloc:     core.GenesisAlloc{testAddr: {Balance: testBalance}},
	ExtraData: []byte("test genesis"),
	Timestamp: 9000,
	BaseFee:   big.NewInt(params.InitialBaseFee),
}

var testTx1 = types.MustSignNewTx(testKey, types.LatestSigner(genesis.Config), &types.LegacyTx{
	Nonce:    0,
	Value:    big.NewInt(12),
	GasPrice: big.NewInt(params.InitialBaseFee),
	Gas:      params.TxGas,
	To:       &common.Address{2},
})

var testTx2 = types.MustSignNewTx(testKey, types.LatestSigner(genesis.Config), &types.LegacyTx{
	Nonce:    1,
	Value:    big.NewInt(8),
	GasPrice: big.NewInt(params.InitialBaseFee),
	Gas:      params.TxGas,
	To:       &common.Address{2},
})

func newTestBackend(t *testing.T) (*node.Node, []*types.Block) {
	// Generate test chain.
	blocks := generateTestChain()

	// Create node
	n, err := node.New(&node.Config{})
	if err != nil {
		t.Fatalf("can't create new node: %v", err)
	}
	// Create Ethereum Service
	config := &ethconfig.Config{Genesis: genesis}
	ethservice, err := eth.New(n, config)
	if err != nil {
		t.Fatalf("can't create new ethereum service: %v", err)
	}
	// Import the test chain.
	if err := n.Start(); err != nil {
		t.Fatalf("can't start test node: %v", err)
	}
	if _, err := ethservice.BlockChain().InsertChain(blocks[1:]); err != nil {
		t.Fatalf("can't import test blocks: %v", err)
	}
	return n, blocks
}

func generateTestChain() []*types.Block {
	generate := func(i int, g *core.BlockGen) {
		g.OffsetTime(5)
		g.SetExtra([]byte("test"))
		if i == 1 {
			// Test transactions are included in block #2.
			g.AddTx(testTx1)
			g.AddTx(testTx2)
		}
	}
	_, blocks, _ := core.GenerateChainWithGenesis(genesis, ethash.NewFaker(), 2, generate)
	return append([]*types.Block{genesis.ToBlock()}, blocks...)
}

// TestLogTraversal_NewLogTraversal tests the NewLogTraversal method on a [logTraversal].
func TestLogTraversal_NewLogTraversal(t *testing.T) {
	logTraversal := NewLogTraversal()
	require.Nil(t, logTraversal.client)
	require.Equal(t, ethereum.FilterQuery{}, logTraversal.query)
	require.Equal(t, logTraversal.lastBlockNumber.Uint64(), uint64(0))
}

// TestLogTraversal_WithClient tests the WithClient method on a [logTraversal].
func TestLogTraversal_WithClient(t *testing.T) {
	backend, _ := newTestBackend(t)
	client, _ := backend.Attach()
	defer backend.Close()
	defer client.Close()

	ec := ethclient.NewClient(client)

	logTraversal := NewLogTraversal()
	logTraversal.WithClient(ec)
	require.Equal(t, ec, logTraversal.client)
}
