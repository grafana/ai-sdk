package grafana

import (
	"encoding/json"

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
		var fields map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(raw, &fields))
		for _, field := range []string{"ChoiceCount", "choiceCount", "RejectUnsupported", "rejectUnsupported"} {
			require.NotContains(t, fields, field)
		}
	}
}
