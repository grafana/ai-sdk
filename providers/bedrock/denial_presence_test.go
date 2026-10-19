package bedrock

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
		count := 0
		warnings := []provider.Warning{}
		block, err := buildToolResult(provider.ToolResultPart("call", "lookup", output), &count, false, &warnings)
		require.NoError(t, err)
		require.Len(t, block.Content, 1)
		require.NotNil(t, block.Content[0].Text)
		got := *block.Content[0].Text
		require.Equal(t, want, got)
	}
}
