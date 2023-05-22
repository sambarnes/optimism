package challenger

import (
	"testing"

	"github.com/ethereum/go-ethereum"

	"github.com/stretchr/testify/require"
)

// TestLogTraversal_NewLogTraversal tests the NewLogTraversal method on a [logTraversal].
func TestLogTraversal_NewLogTraversal(t *testing.T) {
	logTraversal := NewLogTraversal()
	require.Nil(t, logTraversal.client)
	require.Equal(t, ethereum.FilterQuery{}, logTraversal.query)

}
