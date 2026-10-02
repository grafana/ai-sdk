package process

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun_RejectsIncompleteDiscoveryBeforeListener(t *testing.T) {
	listeners := 0
	err := Run(context.Background(), []string{"--config.file=" + writeProcessConfig(t, "http://127.0.0.1:1"), "--deployment.mode=development", "--auth.unsafe", "--server.listen-address=127.0.0.1:0", "--discovery.response-bytes=256"}, func(name string) (string, bool) { return "dummy-api-key", name == "ANTHROPIC_SECRET" }, func(string, string) (net.Listener, error) { listeners++; return nil, assert.AnError }, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating configured discovery")
	assert.NotContains(t, err.Error(), "dummy-api-key")
	assert.Zero(t, listeners)
}
