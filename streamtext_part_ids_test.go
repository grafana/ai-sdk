package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_PartIDs(t *testing.T) {
	for _, tc := range []struct {
		name string
		ids  []string
		want []string
	}{
		{name: "unique", ids: []string{"first", "second", "third"}, want: []string{"first", "second", "third"}},
		{name: "reused", ids: []string{"0", "0", "0"}, want: []string{"0", "generated", "generated-1"}},
		{name: "generator collision", ids: []string{"generated", "generated", "generated"}, want: []string{"generated", "generated-1", "generated-2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := 0
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				id := tc.ids[call]
				call++
				stream := make(chan provider.StreamPart, 8)
				for _, part := range []provider.StreamPart{
					{Type: provider.PartTextStart, ID: id},
					{Type: provider.PartTextDelta, ID: id, Delta: "text"},
					{Type: provider.PartTextEnd, ID: id},
					{Type: provider.PartReasoningStart, ID: id},
					{Type: provider.PartReasoningDelta, ID: id, Delta: "reason"},
					{Type: provider.PartReasoningEnd, ID: id},
					{Type: provider.PartToolCall, ToolCallID: id, ToolName: "next", Input: `{}`},
					{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}},
				} {
					stream <- part
				}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}}
			result := StreamText(t.Context(), model,
				WithGenerateID(func() string { return "generated" }),
				WithStopWhen(StepCountIs(3)),
				WithTools(ToolSet{"next": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`"ok"`), nil
				}}}),
			)
			var textStarts, textDeltas, textEnds, reasoningStarts, reasoningDeltas, reasoningEnds []string
			for part := range result.FullStream() {
				switch part := part.(type) {
				case StreamTextStart:
					textStarts = append(textStarts, part.ID)
				case StreamTextDelta:
					textDeltas = append(textDeltas, part.ID)
				case StreamTextEnd:
					textEnds = append(textEnds, part.ID)
				case StreamReasoningStart:
					reasoningStarts = append(reasoningStarts, part.ID)
				case StreamReasoningDelta:
					reasoningDeltas = append(reasoningDeltas, part.ID)
				case StreamReasoningEnd:
					reasoningEnds = append(reasoningEnds, part.ID)
				}
			}
			require.NoError(t, result.Err())
			assert.Equal(t, 3, call)
			for _, ids := range [][]string{textStarts, textDeltas, textEnds, reasoningStarts, reasoningDeltas, reasoningEnds} {
				assert.Equal(t, tc.want, ids)
			}
		})
	}
}
