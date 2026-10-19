package grafana

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExchangeGates_PerSubject(t *testing.T) {
	var gates exchangeGates
	releaseA, err := gates.acquire(context.Background(), "subject-a")
	require.NoError(t, err)

	releaseB, err := gates.acquire(context.Background(), "subject-b")
	require.NoError(t, err, "another subject does not wait")
	releaseB()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = gates.acquire(canceled, "subject-a")
	require.ErrorIs(t, err, context.Canceled, "the same subject waits until released")

	releaseA()
	releaseA, err = gates.acquire(context.Background(), "subject-a")
	require.NoError(t, err, "the subject is admitted again after release")
	releaseA()
	gates.mu.Lock()
	defer gates.mu.Unlock()
	assert.Empty(t, gates.gates)
}
