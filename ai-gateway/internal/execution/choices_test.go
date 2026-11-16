package execution

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func choicePart(index int, p provider.StreamPart) provider.StreamPart {
	p.ChoiceIndex = &index
	return p
}
func choiceFinish(index int) provider.StreamPart {
	return choicePart(index, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}})
}

func TestNativeChoices_Unary(t *testing.T) {
	choice := provider.GenerateChoice{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "weather"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
	result := &provider.GenerateResult{Choices: []provider.GenerateChoice{choice, choice}}
	result.Choices[1].Index = 1
	require.NoError(t, ValidateGenerateResult(result, nil, 2))
	require.ErrorIs(t, ValidateGenerateResult(result, nil), ErrInvalidOutput)
	require.ErrorIs(t, ValidateGenerateResult(result, nil, 3), ErrInvalidOutput)
	result.Choices[1].Index = 0
	require.ErrorIs(t, ValidateGenerateResult(result, nil, 2), ErrInvalidOutput)
	result.Choices[1].Index = 1
	require.ErrorIs(t, ValidateGenerateResult(result, ToolHistory{"call": "historical"}, 2), ErrInvalidOutput)
	result.Choices[1].Content = append(result.Choices[1].Content, provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: "missing", ToolName: "weather", Result: json.RawMessage(`{}`)})
	require.ErrorIs(t, ValidateGenerateResult(result, nil, 2), ErrInvalidOutput)
}

func TestNativeChoices_InterleavedStream(t *testing.T) {
	parts := []provider.StreamPart{
		{Type: provider.PartStreamStart}, {Type: provider.PartResponseMeta, ResponseID: "native"},
		choicePart(1, provider.StreamPart{Type: provider.PartTextStart, ID: "text"}),
		choicePart(0, provider.StreamPart{Type: provider.PartTextStart, ID: "text"}),
		choicePart(1, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "one"}),
		choicePart(0, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "zero"}),
		choicePart(1, provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}), choiceFinish(1),
		choicePart(0, provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}), choiceFinish(0),
		{Type: provider.PartChoicesFinish, Usage: &provider.Usage{}},
	}
	v := newOutputValidator(nil, 2)
	for _, p := range parts {
		require.NoError(t, v.Check(p))
	}
	assert.True(t, v.SegmentFinished())
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{"index outside request", []provider.StreamPart{choiceFinish(2)}},
		{"negative index", []provider.StreamPart{choiceFinish(-1)}},
		{"missing choice", []provider.StreamPart{choiceFinish(0), {Type: provider.PartChoicesFinish, Usage: &provider.Usage{}}}},
		{"duplicate finish", []provider.StreamPart{choiceFinish(0), choiceFinish(0)}},
		{"unindexed content", []provider.StreamPart{{Type: provider.PartTextStart, ID: "text"}}},
		{"delta belongs to other choice", []provider.StreamPart{choicePart(0, provider.StreamPart{Type: provider.PartTextStart, ID: "text"}), choicePart(1, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "wrong"})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newOutputValidator(nil, 2)
			var err error
			for _, p := range tc.parts {
				err = v.Check(p)
				if err != nil {
					break
				}
			}
			require.ErrorIs(t, err, ErrInvalidOutput)
		})
	}
	single := newOutputValidator(nil, 1)
	require.ErrorIs(t, single.Check(choiceFinish(0)), ErrInvalidOutput)
	require.ErrorIs(t, NewStreamValidator(nil).Check(provider.StreamPart{Type: provider.PartChoicesFinish, Usage: &provider.Usage{}}), ErrInvalidOutput)
}
