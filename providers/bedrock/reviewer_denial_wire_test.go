package bedrock

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReviewDenialEmptyNativeWire(t *testing.T) {
	count := 0
	var warnings []provider.Warning
	out, err := buildToolResult(provider.ToolResultPart("c", "f", &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: new("")}), &count, false, &warnings)
	require.NoError(t, err)
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"text":""`)
}
