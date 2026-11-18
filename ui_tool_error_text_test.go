package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/provider"
)

func TestToUIMessageStream_ToolErrorText(t *testing.T) {
	const (
		toolFailure     = "weather backend timeout"
		unavailableText = "AI_NoSuchToolError: Model tried to call unavailable tool 'missing'. Available tools: weather."
		callbackText    = "redacted"
		maskedText      = "An error occurred."
	)

	run := func(t *testing.T, toolName string, opts ...UIMessageStreamOption) ([]UIMessageChunk, *StreamTextResult) {
		t.Helper()
		calls := 0
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			calls++
			if calls == 1 {
				return &provider.StreamResult{Stream: toolCallStreamParts(toolName, `{}`)}, nil
			}
			return &provider.StreamResult{Stream: textStreamParts("done")}, nil
		}}
		result := StreamText(context.Background(), model,
			WithModelMessages(provider.UserText("weather")),
			WithTools(ToolSet{"weather": {Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return nil, errors.New(toolFailure)
			}}}),
			WithStopWhen(StepCountIs(2)),
		)
		var chunks []UIMessageChunk
		for chunk := range result.ToUIMessageStream(opts...) {
			chunks = append(chunks, chunk)
		}
		require.NoError(t, result.Err())
		return chunks, result
	}
	errorChunks := func(chunks []UIMessageChunk) []UIMessageChunk {
		var out []UIMessageChunk
		for _, chunk := range chunks {
			if chunk.Type == ChunkToolInputError || chunk.Type == ChunkToolOutputError {
				out = append(out, chunk)
			}
		}
		return out
	}
	onError := OnUIMessageStreamError(func(error) string { return callbackText })

	for _, tc := range []struct {
		name     string
		toolName string
		opts     []UIMessageStreamOption
		want     UIMessageChunk
	}{
		{name: "execution error keeps the model's text", toolName: "weather", want: UIMessageChunk{Type: ChunkToolOutputError, ErrorText: toolFailure}},
		{name: "unavailable tool keeps the model's text", toolName: "missing", want: UIMessageChunk{Type: ChunkToolInputError, ErrorText: unavailableText}},
		{name: "error callback redacts by default", toolName: "weather", opts: []UIMessageStreamOption{onError}, want: UIMessageChunk{Type: ChunkToolOutputError, ErrorText: callbackText}},
		{name: "error callback redacts an unavailable tool once", toolName: "missing", opts: []UIMessageStreamOption{onError}, want: UIMessageChunk{Type: ChunkToolInputError, ErrorText: callbackText}},
		{name: "redaction without a callback masks", toolName: "weather", opts: []UIMessageStreamOption{WithUIMessageStreamRedactToolErrors(true)}, want: UIMessageChunk{Type: ChunkToolOutputError, ErrorText: maskedText}},
		{name: "disabled redaction limits the callback to stream errors", toolName: "weather", opts: []UIMessageStreamOption{onError, WithUIMessageStreamRedactToolErrors(false)}, want: UIMessageChunk{Type: ChunkToolOutputError, ErrorText: toolFailure}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks, result := run(t, tc.toolName, tc.opts...)
			got := errorChunks(chunks)
			require.Len(t, got, 1)
			assert.Equal(t, tc.want.Type, got[0].Type)
			assert.Equal(t, tc.want.ErrorText, got[0].ErrorText)

			steps := result.Steps()
			require.Len(t, steps[0].ToolResults, 1)
			require.NotNil(t, steps[0].ToolResults[0].ModelOutput)
			modelText := steps[0].ToolResults[0].ModelOutput.Text
			if tc.want.ErrorText != callbackText && tc.want.ErrorText != maskedText {
				assert.Equal(t, modelText, got[0].ErrorText)
			}
		})
	}

	t.Run("redacting callback runs once per invalid tool call", func(t *testing.T) {
		calls := 0
		run(t, "missing", OnUIMessageStreamError(func(error) string { calls++; return callbackText }))
		assert.Equal(t, 1, calls)
	})

	t.Run("no tools", func(t *testing.T) {
		model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: toolCallStreamParts("lookup", `{}`)}, nil
		}}
		result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("x")))
		got := errorChunks(collectUIChunks(result.ToUIMessageStream()))
		require.Len(t, got, 1)
		assert.Equal(t, "AI_NoSuchToolError: Model tried to call unavailable tool 'lookup'. No tools are available.", got[0].ErrorText)
	})
}

func TestTranslateToChunks_ProviderExecutedToolErrorText(t *testing.T) {
	cfg := uiMessageStreamConfig{}
	failure := errors.New("provider detail")

	invalid := translateToChunks(StreamToolCall{ToolCallID: "c1", ToolName: "web_search", Invalid: true, Error: failure, ProviderExecuted: true}, cfg)
	require.Len(t, invalid, 1)
	assert.Equal(t, "An error occurred.", invalid[0].ErrorText, "the model receives no tool result for an invalid provider-executed call")

	executed := translateToChunks(StreamToolError{ToolCallID: "c1", ToolName: "web_search", Error: failure, ProviderExecuted: true}, uiMessageStreamConfig{onError: func(error) string { return "redacted" }})
	require.Len(t, executed, 1)
	assert.Equal(t, "provider detail", executed[0].ErrorText)

	stream := translateToChunks(StreamError{Error: failure}, cfg)
	require.Len(t, stream, 1)
	assert.Equal(t, "An error occurred.", stream[0].ErrorText)
}
