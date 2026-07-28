package middleware

import (
	"context"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractJSON_Generate(t *testing.T) {
	model := &mockModel{
		doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{
				Content: []provider.GenerateContentPart{
					{Type: provider.ContentText, Text: "```json\n{\"ok\":true}\n```"},
					{Type: provider.ContentReasoning, Text: "kept"},
				},
			}, nil
		},
	}
	wrapped := WrapLanguageModel(model, ExtractJSON(ExtractJSONOptions{}))

	got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	require.Len(t, got.Content, 2)
	assert.Equal(t, `{"ok":true}`, got.Content[0].Text)
	assert.Equal(t, provider.ContentReasoning, got.Content[1].Type)
}

func TestExtractJSON_Stream(t *testing.T) {
	model := &mockModel{
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 5)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "```json\n"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "{\"ok\":true}\n```"}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}
	wrapped := WrapLanguageModel(model, ExtractJSON(ExtractJSONOptions{}))

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.Len(t, parts, 5)
	assert.Equal(t, provider.PartTextStart, parts[0].Type)
	assert.Equal(t, `{"ok":true}`, parts[1].Delta+parts[2].Delta)
	assert.Equal(t, provider.PartTextEnd, parts[3].Type)
	assert.Equal(t, provider.PartFinish, parts[4].Type)
}

func TestExtractJSON_StreamPreservesLeadingWhitespaceInFinalSuffix(t *testing.T) {
	model := &mockModel{
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 5)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "```json\n"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "{\"ok\":   true}\n```"}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}
	wrapped := WrapLanguageModel(model, ExtractJSON(ExtractJSONOptions{}))

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	var text strings.Builder
	for part := range result.Stream {
		if part.Type == provider.PartTextDelta {
			text.WriteString(part.Delta)
		}
	}
	assert.Equal(t, `{"ok":   true}`, text.String())
}

func TestExtractJSON_StreamCustomTransformBuffersUntilEnd(t *testing.T) {
	model := &mockModel{
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 5)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "abc"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "def"}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}
	wrapped := WrapLanguageModel(model, ExtractJSON(ExtractJSONOptions{
		Transform: strings.ToUpper,
	}))

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.Len(t, parts, 4)
	assert.Equal(t, provider.PartTextStart, parts[0].Type)
	assert.Equal(t, "ABCDEF", parts[1].Delta)
	assert.Equal(t, provider.PartTextEnd, parts[2].Type)
	assert.Equal(t, provider.PartFinish, parts[3].Type)
}
