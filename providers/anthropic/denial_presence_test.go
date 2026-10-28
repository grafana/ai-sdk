package anthropic

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
		blocks, err := serializeToolOutput(output, nil, nil)
		require.NoError(t, err)
		require.Len(t, blocks, 1)
		got := blocks[0].OfText.Text
		require.Equal(t, want, got)
	}
}
