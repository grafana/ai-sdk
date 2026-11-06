package grafana

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRequestRejectsGoOnlyCapabilities(t *testing.T) {
	for _, options := range []provider.CallOptions{{RejectUnsupported: true}, {ChoiceCount: 2}, {ChoiceCount: -1}} {
		_, err := encodeRequest(options)
		require.ErrorIs(t, err, errRequest)
	}
	for _, count := range []int{0, 1} {
		raw, err := encodeRequest(provider.CallOptions{ChoiceCount: count})
		require.NoError(t, err)
		require.NotContains(t, string(raw), "ChoiceCount")
	}
}
