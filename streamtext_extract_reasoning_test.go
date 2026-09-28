package aisdk

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamText_ExtractReasoningOverlappingBlocks(t *testing.T) {
	model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		parts := []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"},
			{Type: provider.PartTextDelta, ID: "a", Delta: "<think>A"},
			{Type: provider.PartTextDelta, ID: "b", Delta: "<think>B"},
			{Type: provider.PartTextDelta, ID: "a", Delta: "1</think>Alpha."},
			{Type: provider.PartTextDelta, ID: "b", Delta: "2</think>Beta."},
			{Type: provider.PartTextEnd, ID: "a"}, {Type: provider.PartTextEnd, ID: "b"},
			{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
		}
		stream := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			stream <- part
		}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(t.Context(), middleware.WrapLanguageModel(model, middleware.ExtractReasoning(middleware.ExtractReasoningOptions{TagName: "think"})))

	var content []UIMessageChunk
	var all []UIMessageChunk
	for chunk := range result.ToUIMessageStream() {
		all = append(all, chunk)
		switch chunk.Type {
		case ChunkTextStart, ChunkTextDelta, ChunkTextEnd, ChunkReasoningStart, ChunkReasoningDelta, ChunkReasoningEnd:
			content = append(content, chunk)
		}
	}
	require.NoError(t, result.Err())
	assert.Equal(t, []UIMessageChunk{
		{Type: ChunkReasoningStart, ID: "reasoning-0"}, {Type: ChunkReasoningDelta, ID: "reasoning-0", Delta: "A"},
		{Type: ChunkReasoningStart, ID: "reasoning-1"}, {Type: ChunkReasoningDelta, ID: "reasoning-1", Delta: "B"},
		{Type: ChunkReasoningDelta, ID: "reasoning-0", Delta: "1"}, {Type: ChunkReasoningEnd, ID: "reasoning-0"},
		{Type: ChunkTextStart, ID: "a"}, {Type: ChunkTextDelta, ID: "a", Delta: "Alpha."},
		{Type: ChunkReasoningDelta, ID: "reasoning-1", Delta: "2"}, {Type: ChunkReasoningEnd, ID: "reasoning-1"},
		{Type: ChunkTextStart, ID: "b"}, {Type: ChunkTextDelta, ID: "b", Delta: "Beta."},
		{Type: ChunkTextEnd, ID: "a"}, {Type: ChunkTextEnd, ID: "b"},
	}, content)

	message, err := AssembleUIMessage(chunks(all...))
	require.NoError(t, err)
	var text []string
	var reasoning []string
	for _, part := range message.Parts {
		switch part := part.(type) {
		case TextPart:
			text = append(text, part.Text)
		case ReasoningPart:
			reasoning = append(reasoning, part.Text)
		}
	}
	assert.Equal(t, []string{"Alpha.", "Beta."}, text)
	assert.Equal(t, []string{"A1", "B2"}, reasoning)
}
