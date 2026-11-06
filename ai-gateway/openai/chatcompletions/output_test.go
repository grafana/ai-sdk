package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartialToolOutput(t *testing.T) {
	r := requestWith(t, `,"tools":[{"type":"function","function":{"name":"weather","strict":true,"parameters":{"type":"object","required":["city"]}}}]`)
	for _, finish := range []provider.UnifiedFinishReason{provider.FinishReasonLength, provider.FinishReasonContentFilter, provider.FinishReasonToolCalls} {
		t.Run(string(finish), func(t *testing.T) {
			result := textResult()
			result.Content = []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "c", ToolName: "weather", Input: json.RawMessage(`{"city":`)}}
			result.FinishReason.Unified = finish
			out, err := mapGenerate(result, r, "id", "model", 1)
			if finish == provider.FinishReasonToolCalls {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, `{"city":`, out.Choices[0].Message.ToolCalls[0].Function.Arguments)
			}
			s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
			for _, p := range []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "c", ToolName: "weather"}, {Type: provider.PartToolInputDelta, ID: "c", Delta: `{"city":`}, {Type: provider.PartToolInputEnd, ID: "c"}, {Type: provider.PartToolCall, ToolCallID: "c", ToolName: "weather", Input: `{"city":`}} {
				_, err := s.consume(p, r)
				require.NoError(t, err)
			}
			_, _, err = s.finish(provider.StreamPart{FinishReason: &result.FinishReason, Usage: &result.Usage}, r)
			if finish == provider.FinishReasonToolCalls {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
func TestEmptyCompletion(t *testing.T) {
	r := requestWith(t, "")
	result := textResult()
	result.Content = nil
	out, err := mapGenerate(result, r, "id", "model", 1)
	require.NoError(t, err)
	require.NotNil(t, out.Choices[0].Message.Content)
	assert.Empty(t, *out.Choices[0].Message.Content)
	s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	_, _, err = s.finish(provider.StreamPart{FinishReason: &result.FinishReason, Usage: &result.Usage}, r)
	require.NoError(t, err)
}
