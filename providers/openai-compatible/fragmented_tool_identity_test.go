package openaicompatible

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestNativeCustomToolIdentityFragments(t *testing.T) {
	data := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"same","type":"custom"}]}},{"index":1,"delta":{"tool_calls":[{"index":0,"id":"same"}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0}]}},{"index":1,"delta":{"tool_calls":[{"index":0,"custom":{"name":"echo","input":"b"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"custom":{"name":"echo","input":"a"}}]},"finish_reason":"tool_calls"},{"index":1,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
	parts := make(chan provider.StreamPart, 64)
	m := &model{providerName: "custom.chat"}
	m.runStream(t.Context(), "https://example.test", nil, strings.NewReader(data), nil, nil, false, "custom", parts, true, 2)
	close(parts)
	calls := map[int]string{}
	for p := range parts {
		require.NotEqual(t, provider.PartError, p.Type)
		if p.Type == provider.PartToolInputStart {
			assert.Equal(t, provider.ToolInputEncodingText, p.InputEncoding)
		}
		if p.Type == provider.PartToolCall {
			require.NotNil(t, p.ChoiceIndex)
			calls[*p.ChoiceIndex] = p.Input
		}
	}
	assert.Equal(t, map[int]string{0: `"a"`, 1: `"b"`}, calls)
}
