package aisdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intP(v int) *int { return &v }

type testModel struct {
	streamFunc func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error)
}

func (m *testModel) SpecificationVersion() string               { return "v4" }
func (m *testModel) Provider() string                           { return "mock" }
func (m *testModel) ModelID() string                            { return "mock-1" }
func (m *testModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *testModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	return m.streamFunc(ctx, opts)
}
func (m *testModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func textStream(text string) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: text}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intP(10)}, OutputTokens: provider.OutputTokenUsage{Total: intP(5)}}}
	}()
	return ch
}

func mustSchema(t *testing.T, raw string) schema.Schema {
	t.Helper()
	s, err := schema.SchemaFromJSON(json.RawMessage(raw))
	require.NoError(t, err)
	return s
}

func TestStreamText_ObjectOutput(t *testing.T) {
	t.Run("valid output is parsed", func(t *testing.T) {
		model := &testModel{
			streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				require.NotNil(t, opts.ResponseFormat)
				assert.Equal(t, provider.ResponseFormatJSON, opts.ResponseFormat.Type)
				return &provider.StreamResult{
					Stream: textStream(`{"name":"Lasagna","ingredients":["pasta","cheese"]}`),
				}, nil
			},
		}

		type recipe struct {
			Name        string   `json:"name"`
			Ingredients []string `json:"ingredients"`
		}

		s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"ingredients":{"type":"array","items":{"type":"string"}}},"required":["name","ingredients"]}`)
		out, err := output.Object[recipe](s)
		require.NoError(t, err)

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("recipe")),
			aisdk.WithOutput(out),
		)

		for range result.FullStream() {
		}

		require.NoError(t, result.OutputError())
		val := result.OutputValue()
		require.NotNil(t, val)

		r, ok := val.(recipe)
		require.True(t, ok, "expected recipe, got %T", val)
		assert.Equal(t, "Lasagna", r.Name)
	})

	t.Run("invalid output returns ErrNoObjectGenerated", func(t *testing.T) {
		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream(`{"wrong":"format"}`)}, nil
			},
		}

		type recipe struct {
			Name string `json:"name"`
		}

		s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
		out, err := output.Object[recipe](s)
		require.NoError(t, err)

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("recipe")),
			aisdk.WithOutput(out),
		)

		for range result.FullStream() {
		}

		require.Error(t, result.OutputError())
		assert.True(t, errors.Is(result.OutputError(), aisdk.ErrNoObjectGenerated))
		assert.Equal(t, `{"wrong":"format"}`, result.Text(), "raw text should still be available")
	})
}

func TestStreamText_ChoiceOutput(t *testing.T) {
	model := &testModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStream(`{"result":"sunny"}`)}, nil
		},
	}

	out, err := output.Choice("sunny", "rainy", "snowy")
	require.NoError(t, err)

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("weather?")),
		aisdk.WithOutput(out),
	)

	for range result.FullStream() {
	}

	require.NoError(t, result.OutputError())
	choice, ok := result.OutputValue().(string)
	require.True(t, ok, "expected string, got %T", result.OutputValue())
	assert.Equal(t, "sunny", choice)
}

func TestStreamText_ArrayOutput(t *testing.T) {
	model := &testModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{
				Stream: textStream(`{"elements":[{"name":"Paris","pop":2161000},{"name":"London","pop":8982000}]}`),
			}, nil
		},
	}

	type city struct {
		Name string `json:"name"`
		Pop  int    `json:"pop"`
	}

	elemSchema := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"},"pop":{"type":"integer"}},"required":["name","pop"]}`)
	out, err := output.Array[city](elemSchema)
	require.NoError(t, err)

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("recipe")),
		aisdk.WithOutput(out),
	)

	for range result.FullStream() {
	}

	require.NoError(t, result.OutputError())
	cities, ok := result.OutputValue().([]city)
	require.True(t, ok, "expected []city, got %T", result.OutputValue())
	require.Len(t, cities, 2)
	assert.Equal(t, "Paris", cities[0].Name)
}

func TestStreamText_NilOutput_NoRegression(t *testing.T) {
	model := &testModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStream("hello")}, nil
		},
	}

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("hi")),
	)

	for range result.FullStream() {
	}

	assert.Nil(t, result.OutputValue())
	assert.Nil(t, result.OutputError())
	assert.Equal(t, "hello", result.Text())
}

func TestStreamText_OutputTakesPrecedence(t *testing.T) {
	model := &testModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			require.NotNil(t, opts.ResponseFormat)
			assert.Equal(t, provider.ResponseFormatJSON, opts.ResponseFormat.Type, "Output should override ResponseFormat")
			return &provider.StreamResult{Stream: textStream(`{"name":"test"}`)}, nil
		},
	}

	type s struct {
		Name string `json:"name"`
	}

	sch := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := output.Object[s](sch)
	require.NoError(t, err)

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithResponseFormat(provider.ResponseFormat{
			Type: provider.ResponseFormatText,
		}),
		aisdk.WithOutput(out),
	)

	for range result.FullStream() {
	}

	require.NoError(t, result.OutputError())
}

func TestStreamText_ToolsAndStructuredOutput(t *testing.T) {
	callNum := 0
	model := &testModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			callNum++
			if callNum == 1 {
				ch := make(chan provider.StreamPart, 10)
				go func() {
					defer close(ch)
					ch <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "lookup", Input: `{"q":"data"}`}
					ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intP(10)}, OutputTokens: provider.OutputTokenUsage{Total: intP(3)}}}
				}()
				return &provider.StreamResult{Stream: ch}, nil
			}
			return &provider.StreamResult{Stream: textStream(`{"name":"Result"}`)}, nil
		},
	}

	type s struct {
		Name string `json:"name"`
	}

	sch := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := output.Object[s](sch)
	require.NoError(t, err)

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithOutput(out),
		aisdk.WithTools(aisdk.ToolSet{
			"lookup": aisdk.Tool{
				Description: "Lookup data",
				InputSchema: mustSchema(t, `{"type":"object"}`),
				Execute: func(_ context.Context, _ json.RawMessage, _ aisdk.ToolExecutionOptions) (json.RawMessage, error) {
					return json.RawMessage(`{"found":true}`), nil
				},
			},
		}),
		aisdk.WithStopWhen(aisdk.StepCountIs(5)),
	)

	for range result.FullStream() {
	}

	assert.Equal(t, 2, callNum)
	require.NoError(t, result.OutputError())
	val, ok := result.OutputValue().(s)
	require.True(t, ok, "expected s, got %T", result.OutputValue())
	assert.Equal(t, "Result", val.Name)
}

func TestStreamText_ObjectOutputPreservesMetadataOnlyTextDelta(t *testing.T) {
	metadata := provider.ProviderMetadata{
		"test": json.RawMessage(`{"signature":"test-signature"}`),
	}
	model := &testModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart, 6)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"value":"ok"}`}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", ProviderMetadata: metadata}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithOutput(output.JSON()),
	)

	var metadataDelta *aisdk.StreamTextDelta
	for part := range result.FullStream() {
		if delta, ok := part.(aisdk.StreamTextDelta); ok && delta.Text == "" && delta.ProviderMetadata != nil {
			copy := delta
			metadataDelta = &copy
		}
	}

	require.NotNil(t, metadataDelta)
	assert.Equal(t, "t1", metadataDelta.ID)
	assert.Equal(t, metadata, metadataDelta.ProviderMetadata)
}

func TestStreamText_PartialOutputStream(t *testing.T) {
	t.Run("delivers more partials than the channel buffer", func(t *testing.T) {
		out := output.JSON()
		deltas := []string{`{"value":"`}
		for range 300 {
			deltas = append(deltas, "x")
		}
		deltas = append(deltas, `"}`)

		var expected []json.RawMessage
		var text string
		var last string
		for _, delta := range deltas {
			text += delta
			partial, ok := out.ParsePartial(text)
			if !ok {
				continue
			}
			raw := partial.(json.RawMessage)
			if string(raw) == last {
				continue
			}
			last = string(raw)
			expected = append(expected, append(json.RawMessage(nil), raw...))
		}
		require.Greater(t, len(expected), 256)

		ch := make(chan provider.StreamPart, len(deltas)+3)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		for _, delta := range deltas {
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: delta}
		}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(ch)

		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}
		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("test")),
			aisdk.WithOutput(out),
		)

		for range result.FullStream() {
		}

		var partials []json.RawMessage
		for partial := range result.PartialOutputStream() {
			partials = append(partials, partial)
		}
		require.Len(t, partials, len(expected))
		for i := range expected {
			assert.Equal(t, string(expected[i]), string(partials[i]), "partial %d", i)
		}
	})

	t.Run("emits partial JSON objects", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 20)
		go func() {
			defer close(ch)
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"na`}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `me":"Test"}`}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intP(10)}, OutputTokens: provider.OutputTokenUsage{Total: intP(5)}}}
		}()

		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		type s struct {
			Name string `json:"name"`
		}

		sch := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
		out, err := output.Object[s](sch)
		require.NoError(t, err)

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("test")),
			aisdk.WithOutput(out),
		)

		var partials []json.RawMessage
		done := make(chan struct{})
		go func() {
			defer close(done)
			for p := range result.PartialOutputStream() {
				partials = append(partials, p)
			}
		}()

		for range result.FullStream() {
		}
		<-done

		require.NotEmpty(t, partials)
		assert.True(t, json.Valid(partials[len(partials)-1]), "last partial should be valid JSON")
	})

	t.Run("nil output closes channel immediately", func(t *testing.T) {
		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream("hello")}, nil
			},
		}

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("hi")),
		)

		select {
		case _, ok := <-result.PartialOutputStream():
			assert.False(t, ok)
		default:
			require.FailNow(t, "partial output stream should already be closed")
		}
	})
}

func TestStreamText_ElementStream(t *testing.T) {
	t.Run("delivers more elements than the channel buffer", func(t *testing.T) {
		type item struct {
			Index int `json:"index"`
		}

		elements := make([]item, 300)
		for i := range elements {
			elements[i].Index = i
		}
		text, err := json.Marshal(struct {
			Elements []item `json:"elements"`
		}{Elements: elements})
		require.NoError(t, err)

		elemSchema := mustSchema(t, `{"type":"object","properties":{"index":{"type":"integer"}},"required":["index"]}`)
		out, err := output.Array[item](elemSchema)
		require.NoError(t, err)
		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream(string(text))}, nil
			},
		}
		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("test")),
			aisdk.WithOutput(out),
		)

		for range result.FullStream() {
		}

		var actual []item
		for raw := range result.ElementStream() {
			var element item
			require.NoError(t, json.Unmarshal(raw, &element))
			actual = append(actual, element)
		}
		require.Len(t, actual, len(elements))
		for i := range elements {
			assert.Equal(t, elements[i], actual[i], "element %d", i)
		}

		var partials []json.RawMessage
		for partial := range result.PartialOutputStream() {
			partials = append(partials, partial)
		}
		require.NotEmpty(t, partials)
		expectedPartial, err := json.Marshal(elements)
		require.NoError(t, err)
		assert.JSONEq(t, string(expectedPartial), string(partials[len(partials)-1]))
	})

	t.Run("nil output closes channel immediately", func(t *testing.T) {
		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream("hello")}, nil
			},
		}

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("hi")),
		)

		select {
		case _, ok := <-result.ElementStream():
			assert.False(t, ok)
		default:
			require.FailNow(t, "element stream should already be closed")
		}
	})

	t.Run("non-array mode closes channel", func(t *testing.T) {
		model := &testModel{
			streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream(`{"name":"test"}`)}, nil
			},
		}

		type s struct {
			Name string `json:"name"`
		}

		sch := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
		out, err := output.Object[s](sch)
		require.NoError(t, err)

		result := aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("test")),
			aisdk.WithOutput(out),
		)

		for range result.FullStream() {
		}

		count := 0
		for range result.ElementStream() {
			count++
		}
		assert.Equal(t, 0, count)
	})
}

func TestStreamText_OutputWithLengthFinishReason(t *testing.T) {
	type s struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name  string
		text  string
		check func(t *testing.T, result *aisdk.StreamTextResult)
	}{
		{
			name: "valid output is parsed",
			text: `{"name":"test"}`,
			check: func(t *testing.T, result *aisdk.StreamTextResult) {
				require.NoError(t, result.OutputError())
				assert.Equal(t, s{Name: "test"}, result.OutputValue())
			},
		},
		{
			name: "truncated output returns ErrNoObjectGenerated",
			text: `{"name":"test`,
			check: func(t *testing.T, result *aisdk.StreamTextResult) {
				assert.Nil(t, result.OutputValue())
				assert.ErrorIs(t, result.OutputError(), aisdk.ErrNoObjectGenerated)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan provider.StreamPart, 10)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: tt.text}
				ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
				ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonLength}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intP(10)}, OutputTokens: provider.OutputTokenUsage{Total: intP(5)}}}
			}()

			model := &testModel{
				streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: ch}, nil
				},
			}

			sch := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
			out, err := output.Object[s](sch)
			require.NoError(t, err)

			result := aisdk.StreamText(context.Background(), model,
				aisdk.WithModelMessages(provider.UserText("test")),
				aisdk.WithOutput(out),
			)

			for range result.FullStream() {
			}

			assert.Equal(t, provider.FinishReasonLength, result.FinishReason().Unified)
			tt.check(t, result)
		})
	}
}

func TestGenerateText_OutputWithLengthFinishReason(t *testing.T) {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"name":"test"}`}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonLength}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intP(10)}, OutputTokens: provider.OutputTokenUsage{Total: intP(5)}}}
	}()

	model := &testModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	type s struct {
		Name string `json:"name"`
	}

	sch := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	out, err := output.Object[s](sch)
	require.NoError(t, err)

	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithOutput(out),
	)
	require.NoError(t, err)

	assert.Equal(t, provider.FinishReasonLength, result.FinishReason.Unified)
	assert.Nil(t, result.Output)
	assert.Nil(t, result.OutputError)
}
