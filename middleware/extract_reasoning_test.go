package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractReasoning_Generate(t *testing.T) {
	t.Run("BasicExtraction", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "<think>reasoning text</think>actual response"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 2)
		assert.Equal(t, provider.ContentReasoning, got.Content[0].Type)
		assert.Equal(t, "reasoning text", got.Content[0].Text)
		assert.Equal(t, provider.ContentText, got.Content[1].Type)
		assert.Equal(t, "actual response", got.Content[1].Text)
	})

	t.Run("NoTagsPresent", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "just a normal response"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 1)
		assert.Equal(t, provider.ContentText, got.Content[0].Type)
		assert.Equal(t, "just a normal response", got.Content[0].Text)
	})

	t.Run("MultipleReasoningSections", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "<think>first</think>middle<think>second</think>end"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 2)
		assert.Equal(t, provider.ContentReasoning, got.Content[0].Type)
		assert.Equal(t, "first\nsecond", got.Content[0].Text)
		assert.Equal(t, provider.ContentText, got.Content[1].Type)
		assert.Equal(t, "middle\nend", got.Content[1].Text)
	})

	t.Run("StartWithReasoning", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "thinking here</think>the answer"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think", StartWithReasoning: true})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 2)
		assert.Equal(t, provider.ContentReasoning, got.Content[0].Type)
		assert.Equal(t, "thinking here", got.Content[0].Text)
		assert.Equal(t, provider.ContentText, got.Content[1].Type)
		assert.Equal(t, "the answer", got.Content[1].Text)
	})

	t.Run("ReasoningOnly_NoTextAfterTags", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "<think>analyzing the problem\n</think>"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 2)
		assert.Equal(t, provider.ContentReasoning, got.Content[0].Type)
		assert.Equal(t, "analyzing the problem\n", got.Content[0].Text)
		assert.Equal(t, provider.ContentText, got.Content[1].Type)
		assert.Equal(t, "", got.Content[1].Text)
	})

	t.Run("NonTextContent_Preserved", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentToolCall, ToolCallID: "tc1"},
						{Type: provider.ContentText, Text: "<think>r</think>t"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 3)
		assert.Equal(t, provider.ContentToolCall, got.Content[0].Type)
		assert.Equal(t, provider.ContentReasoning, got.Content[1].Type)
		assert.Equal(t, provider.ContentText, got.Content[2].Type)
	})

	t.Run("CustomSeparator", func(t *testing.T) {
		model := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					Content: []provider.GenerateContentPart{
						{Type: provider.ContentText, Text: "<think>a</think>x<think>b</think>y"},
					},
				}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think", Separator: " | "})
		wrapped := WrapLanguageModel(model, mw)
		got, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		require.Len(t, got.Content, 2)
		assert.Equal(t, "a | b", got.Content[0].Text)
		assert.Equal(t, "x | y", got.Content[1].Text)
	})
}

func TestExtractReasoning_Stream(t *testing.T) {
	makeTextStream := func(deltas ...string) *mockModel {
		return &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, len(deltas)+2)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "0"}
				for _, d := range deltas {
					ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: d}
				}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "0"}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}
	}

	collectParts := func(t *testing.T, result *provider.StreamResult) []provider.StreamPart {
		t.Helper()
		var parts []provider.StreamPart
		for p := range result.Stream {
			parts = append(parts, p)
		}
		return parts
	}

	t.Run("BasicStreamExtraction", func(t *testing.T) {
		model := makeTextStream("<think>reasoning</think>answer")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var reasoning, text string
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningDelta:
				reasoning += p.Delta
			case provider.PartTextDelta:
				text += p.Delta
			}
		}
		assert.Equal(t, "reasoning", reasoning)
		assert.Equal(t, "answer", text)
	})

	t.Run("TagSplitAcrossChunks", func(t *testing.T) {
		model := makeTextStream("<thi", "nk>reason", "ing</th", "ink>answer")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var reasoning, text string
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningDelta:
				reasoning += p.Delta
			case provider.PartTextDelta:
				text += p.Delta
			}
		}
		assert.Equal(t, "reasoning", reasoning)
		assert.Equal(t, "answer", text)
	})

	t.Run("StartWithReasoning", func(t *testing.T) {
		model := makeTextStream("thinking here</think>the answer")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think", StartWithReasoning: true})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var reasoning, text string
		var hasReasoningStart, hasReasoningEnd bool
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningStart:
				hasReasoningStart = true
			case provider.PartReasoningDelta:
				reasoning += p.Delta
			case provider.PartReasoningEnd:
				hasReasoningEnd = true
			case provider.PartTextDelta:
				text += p.Delta
			}
		}
		assert.True(t, hasReasoningStart)
		assert.True(t, hasReasoningEnd)
		assert.Equal(t, "thinking here", reasoning)
		assert.Equal(t, "the answer", text)
	})

	t.Run("EmptyReasoningBlock", func(t *testing.T) {
		model := makeTextStream("<think></think>answer")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var reasoning, text string
		var reasoningStartCount, reasoningEndCount int
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningStart:
				reasoningStartCount++
			case provider.PartReasoningDelta:
				reasoning += p.Delta
			case provider.PartReasoningEnd:
				reasoningEndCount++
			case provider.PartTextDelta:
				text += p.Delta
			}
		}
		assert.Equal(t, "", reasoning)
		assert.Equal(t, "answer", text)
		assert.Equal(t, 1, reasoningStartCount)
		assert.Equal(t, 1, reasoningEndCount)
	})

	t.Run("TransitionBetweenReasoningAndText", func(t *testing.T) {
		model := makeTextStream("<think>r1</think>t1<think>r2</think>t2")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var reasoningParts []string
		var textParts []string
		var reasoningStartCount, reasoningEndCount int
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningStart:
				reasoningStartCount++
			case provider.PartReasoningDelta:
				reasoningParts = append(reasoningParts, p.Delta)
			case provider.PartReasoningEnd:
				reasoningEndCount++
			case provider.PartTextDelta:
				textParts = append(textParts, p.Delta)
			}
		}

		fullReasoning := ""
		for _, r := range reasoningParts {
			fullReasoning += r
		}
		fullText := ""
		for _, t := range textParts {
			fullText += t
		}

		assert.Contains(t, fullReasoning, "r1")
		assert.Contains(t, fullReasoning, "r2")
		assert.Contains(t, fullText, "t1")
		assert.Contains(t, fullText, "t2")
		assert.Equal(t, 2, reasoningStartCount)
		assert.Equal(t, 2, reasoningEndCount)
	})

	t.Run("ReasoningOnly_NoTextAfterTags", func(t *testing.T) {
		model := makeTextStream("<think>just reasoning</think>")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var reasoning, text string
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningDelta:
				reasoning += p.Delta
			case provider.PartTextDelta:
				text += p.Delta
			}
		}
		assert.Equal(t, "just reasoning", reasoning)
		assert.Equal(t, "", text)
	})

	t.Run("NoReasoningTags_TextPassesThrough", func(t *testing.T) {
		model := makeTextStream("just plain text")
		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var text string
		var hasReasoning bool
		for _, p := range parts {
			switch p.Type {
			case provider.PartReasoningDelta:
				hasReasoning = true
			case provider.PartTextDelta:
				text += p.Delta
			}
		}
		assert.False(t, hasReasoning)
		assert.Equal(t, "just plain text", text)
	})

	t.Run("NonTextParts_PassThrough", func(t *testing.T) {
		model := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 4)
				ch <- provider.StreamPart{Type: provider.PartStreamStart}
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "0"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "<think>r</think>t"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "0"}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		hasStreamStart := false
		for _, p := range parts {
			if p.Type == provider.PartStreamStart {
				hasStreamStart = true
			}
		}
		assert.True(t, hasStreamStart, "stream-start should pass through")
	})

	t.Run("DelayedTextStart_BeforeReasoning", func(t *testing.T) {
		model := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, 4)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "0"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "0", Delta: "<think>reasoning</think>text"}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "0"}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		mw := ExtractReasoning(ExtractReasoningOptions{TagName: "think"})
		wrapped := WrapLanguageModel(model, mw)
		result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		parts := collectParts(t, result)

		var types []provider.StreamPartType
		for _, p := range parts {
			types = append(types, p.Type)
		}

		reasoningStartIdx := -1
		textStartIdx := -1
		for i, typ := range types {
			if typ == provider.PartReasoningStart && reasoningStartIdx == -1 {
				reasoningStartIdx = i
			}
			if typ == provider.PartTextStart && textStartIdx == -1 {
				textStartIdx = i
			}
		}

		require.NotEqual(t, -1, reasoningStartIdx, "should have reasoning-start")
		require.NotEqual(t, -1, textStartIdx, "should have text-start")
		assert.Less(t, reasoningStartIdx, textStartIdx, "reasoning-start should come before text-start")
	})

	for _, tc := range []struct {
		name  string
		input []provider.StreamPart
		want  []provider.StreamPart
	}{
		{
			name: "OverlappingPlainText",
			input: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "Alpha."},
				{Type: provider.PartTextDelta, ID: "b", Delta: "Beta."},
				{Type: provider.PartTextEnd, ID: "b"}, {Type: provider.PartTextEnd, ID: "a"},
			},
			want: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextDelta, ID: "a", Delta: "Alpha."},
				{Type: provider.PartTextStart, ID: "b"}, {Type: provider.PartTextDelta, ID: "b", Delta: "Beta."},
				{Type: provider.PartTextEnd, ID: "b"}, {Type: provider.PartTextEnd, ID: "a"},
			},
		},
		{
			name: "OverlappingSplitReasoning",
			input: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "<think>A"},
				{Type: provider.PartTextDelta, ID: "b", Delta: "<thi"},
				{Type: provider.PartTextDelta, ID: "b", Delta: "nk>B"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "1</think>Alpha."},
				{Type: provider.PartTextDelta, ID: "b", Delta: "2</think>Beta."},
				{Type: provider.PartTextEnd, ID: "b"}, {Type: provider.PartTextEnd, ID: "a"},
			},
			want: []provider.StreamPart{
				{Type: provider.PartReasoningStart, ID: "reasoning-0"}, {Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "A"},
				{Type: provider.PartReasoningStart, ID: "reasoning-1"}, {Type: provider.PartReasoningDelta, ID: "reasoning-1", Delta: "B"},
				{Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "1"}, {Type: provider.PartReasoningEnd, ID: "reasoning-0"},
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextDelta, ID: "a", Delta: "Alpha."},
				{Type: provider.PartReasoningDelta, ID: "reasoning-1", Delta: "2"}, {Type: provider.PartReasoningEnd, ID: "reasoning-1"},
				{Type: provider.PartTextStart, ID: "b"}, {Type: provider.PartTextDelta, ID: "b", Delta: "Beta."},
				{Type: provider.PartTextEnd, ID: "b"}, {Type: provider.PartTextEnd, ID: "a"},
			},
		},
		{
			name: "ReasoningOnlyBlock",
			input: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "<think>A</think>Alpha."},
				{Type: provider.PartTextDelta, ID: "b", Delta: "<think>B</think>"},
				{Type: provider.PartTextEnd, ID: "b"}, {Type: provider.PartTextEnd, ID: "a"},
			},
			want: []provider.StreamPart{
				{Type: provider.PartReasoningStart, ID: "reasoning-0"}, {Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "A"}, {Type: provider.PartReasoningEnd, ID: "reasoning-0"},
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextDelta, ID: "a", Delta: "Alpha."},
				{Type: provider.PartReasoningStart, ID: "reasoning-1"}, {Type: provider.PartReasoningDelta, ID: "reasoning-1", Delta: "B"}, {Type: provider.PartReasoningEnd, ID: "reasoning-1"},
				{Type: provider.PartTextStart, ID: "b"}, {Type: provider.PartTextEnd, ID: "b"}, {Type: provider.PartTextEnd, ID: "a"},
			},
		},
		{
			name: "FirstEmptyAndLaterSegments",
			input: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextStart, ID: "b"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "<think></think>x<think>R</think>y"},
				{Type: provider.PartTextDelta, ID: "b", Delta: "<think>B</think>z"},
				{Type: provider.PartTextEnd, ID: "a"}, {Type: provider.PartTextEnd, ID: "b"},
			},
			want: []provider.StreamPart{
				{Type: provider.PartReasoningStart, ID: "reasoning-0"}, {Type: provider.PartReasoningEnd, ID: "reasoning-0"},
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextDelta, ID: "a", Delta: "x"},
				{Type: provider.PartReasoningStart, ID: "reasoning-1"}, {Type: provider.PartReasoningDelta, ID: "reasoning-1", Delta: "R"}, {Type: provider.PartReasoningEnd, ID: "reasoning-1"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "\ny"},
				{Type: provider.PartReasoningStart, ID: "reasoning-2"}, {Type: provider.PartReasoningDelta, ID: "reasoning-2", Delta: "B"}, {Type: provider.PartReasoningEnd, ID: "reasoning-2"},
				{Type: provider.PartTextStart, ID: "b"}, {Type: provider.PartTextDelta, ID: "b", Delta: "z"},
				{Type: provider.PartTextEnd, ID: "a"}, {Type: provider.PartTextEnd, ID: "b"},
			},
		},
		{
			name: "LaterEmptyPreservesUpstreamBehavior",
			input: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "a"},
				{Type: provider.PartTextDelta, ID: "a", Delta: "<think>A</think>x<think></think>y"},
				{Type: provider.PartTextEnd, ID: "a"},
			},
			want: []provider.StreamPart{
				{Type: provider.PartReasoningStart, ID: "reasoning-0"}, {Type: provider.PartReasoningDelta, ID: "reasoning-0", Delta: "A"}, {Type: provider.PartReasoningEnd, ID: "reasoning-0"},
				{Type: provider.PartTextStart, ID: "a"}, {Type: provider.PartTextDelta, ID: "a", Delta: "x"},
				{Type: provider.PartReasoningEnd, ID: "reasoning-1"}, {Type: provider.PartTextDelta, ID: "a", Delta: "\ny"}, {Type: provider.PartTextEnd, ID: "a"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, len(tc.input))
				for _, part := range tc.input {
					ch <- part
				}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			}}
			result, err := WrapLanguageModel(model, ExtractReasoning(ExtractReasoningOptions{TagName: "think"})).DoStream(t.Context(), provider.CallOptions{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, collectParts(t, result))
		})
	}

	t.Run("CancelWithOverlappingPendingStarts", func(t *testing.T) {
		input := make(chan provider.StreamPart)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		model := &mockModel{doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: input}, nil
		}}
		result, err := WrapLanguageModel(model, ExtractReasoning(ExtractReasoningOptions{TagName: "think"})).DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)

		input <- provider.StreamPart{Type: provider.PartTextStart, ID: "a"}
		input <- provider.StreamPart{Type: provider.PartTextStart, ID: "b"}
		cancel()

		select {
		case part, ok := <-result.Stream:
			assert.False(t, ok, "pending text starts should not be flushed on cancellation: %+v", part)
		case <-time.After(time.Second):
			t.Fatal("transformed stream did not close after cancellation")
		}
	})
}

func TestGetPotentialStartIndex(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		search   string
		expected int
	}{
		{"empty search", "hello", "", -1},
		{"direct match", "hello <think> world", "<think>", 6},
		{"no match", "hello world", "<think>", -1},
		{"partial match at end", "hello <thi", "<think>", 6},
		{"partial match single char", "hello <", "<think>", 6},
		{"full match at start", "<think>rest", "<think>", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getPotentialStartIndex(tc.text, tc.search)
			assert.Equal(t, tc.expected, got)
		})
	}
}
