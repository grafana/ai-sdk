package openaicompatible

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDenialReasonPresence(t *testing.T) {
	for _, reason := range []*string{nil, new(""), new("policy")} {
		output := &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: reason}
		want := "Tool call execution denied."
		if reason != nil {
			want = *reason
		}
		got, err := toolResultOutputString(output)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}
