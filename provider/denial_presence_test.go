package provider

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestToolResultOutput_DenialReasonPresence(t *testing.T) {
	for _, raw := range []string{`{"type":"execution-denied"}`, `{"type":"execution-denied","reason":""}`, `{"type":"execution-denied","reason":"policy"}`} {
		t.Run(raw, func(t *testing.T) {
			var output ToolResultOutput
			require.NoError(t, json.Unmarshal([]byte(raw), &output))
			got, err := json.Marshal(output)
			require.NoError(t, err)
			require.JSONEq(t, raw, string(got))
		})
	}
}
