package aisdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
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

func TestStructuredOutputRepair_DirectCalls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		generate bool
		text     string
	}{
		{name: "generate malformed JSON", generate: true, text: `{"name":`},
		{name: "generate schema failure", generate: true, text: `{"wrong":"data"}`},
		{name: "stream malformed JSON", text: `{"name":`},
		{name: "stream schema failure", text: `{"wrong":"data"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			modelCalls, repairCalls := 0, 0
			model := &testModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				modelCalls++
				assert.Equal(t, provider.ResponseFormatJSON, opts.ResponseFormat.Type)
				return &provider.StreamResult{Stream: textStream(tc.text)}, nil
			}}
			out, err := output.Object[struct {
				Name string `json:"name"`
			}](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
			require.NoError(t, err)
			repair := aisdk.WithRepairText(func(text string, parseErr error) (string, bool, error) {
				repairCalls++
				assert.Equal(t, tc.text, text)
				assert.ErrorIs(t, parseErr, aisdk.ErrNoObjectGenerated)
				assert.ErrorIs(t, parseErr, aisdk.ErrInvalidOutputText)
				return `{"name":"repaired"}`, true, nil
			})
			if tc.generate {
				result, err := aisdk.GenerateText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(out), repair)
				require.NoError(t, err)
				assert.Equal(t, "repaired", result.Output.(struct {
					Name string `json:"name"`
				}).Name)
				assert.Equal(t, tc.text, result.Text)
				assert.NoError(t, result.OutputError)
			} else {
				result := aisdk.StreamText(context.Background(), model, aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(out), repair)
				for range result.FullStream() {
				}
				assert.Equal(t, "repaired", result.OutputValue().(struct {
					Name string `json:"name"`
				}).Name)
				assert.Equal(t, tc.text, result.Text())
				assert.NoError(t, result.OutputError())
			}
			assert.Equal(t, 1, repairCalls)
			assert.Equal(t, 1, modelCalls)
		})
	}
}

type repairTestOutput struct {
	calls int
	err   error
}

func (o *repairTestOutput) ResponseFormat() *provider.ResponseFormat {
	return &provider.ResponseFormat{Type: provider.ResponseFormatJSON}
}
func (o *repairTestOutput) ParseComplete(text string) (any, error) {
	o.calls++
	if text == `{"ok":true}` {
		return text, nil
	}
	return nil, o.err
}
func (o *repairTestOutput) ParsePartial(string) (any, bool) { return nil, false }

func TestStructuredOutputRepair_FailureModes(t *testing.T) {
	originalErr := errors.Join(aisdk.ErrNoObjectGenerated, aisdk.ErrInvalidOutputText)
	callbackErr := errors.New("repair failed")
	for _, tc := range []struct {
		name          string
		input         string
		initialErr    error
		repair        func(string, error) (string, bool, error)
		wantErr       error
		wantValue     string
		wantParses    int
		wantCallbacks int
	}{
		{name: "no option", input: `{"bad":true}`, initialErr: originalErr, wantErr: originalErr, wantParses: 1},
		{name: "custom eligible", input: `{"bad":true}`, initialErr: originalErr, repair: func(string, error) (string, bool, error) { return `{"ok":true}`, true, nil }, wantValue: `{"ok":true}`, wantParses: 2, wantCallbacks: 1},
		{name: "decline", input: `{"bad":true}`, initialErr: originalErr, repair: func(string, error) (string, bool, error) { return `{"ok":true}`, false, nil }, wantErr: originalErr, wantParses: 1, wantCallbacks: 1},
		{name: "callback error takes precedence", input: `{"bad":true}`, initialErr: originalErr, repair: func(string, error) (string, bool, error) { return `{"ok":true}`, true, callbackErr }, wantErr: callbackErr, wantParses: 1, wantCallbacks: 1},
		{name: "invalid accepted repair", input: `{"bad":true}`, initialErr: originalErr, repair: func(string, error) (string, bool, error) { return `{"bad":true}`, true, nil }, wantErr: originalErr, wantParses: 2, wantCallbacks: 1},
		{name: "valid original", input: `{"ok":true}`, initialErr: originalErr, repair: func(string, error) (string, bool, error) { return "", true, nil }, wantValue: `{"ok":true}`, wantParses: 1},
		{name: "unrelated error", input: `{"bad":true}`, initialErr: callbackErr, repair: func(string, error) (string, bool, error) { return `{"ok":true}`, true, nil }, wantErr: callbackErr, wantParses: 1},
		{name: "single sentinel", input: `{"bad":true}`, initialErr: aisdk.ErrNoObjectGenerated, repair: func(string, error) (string, bool, error) { return `{"ok":true}`, true, nil }, wantErr: aisdk.ErrNoObjectGenerated, wantParses: 1},
		{name: "empty accepted text", input: `{"bad":true}`, initialErr: originalErr, repair: func(string, error) (string, bool, error) { return "", true, nil }, wantErr: originalErr, wantParses: 2, wantCallbacks: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := &repairTestOutput{err: tc.initialErr}
			modelCalls, repairCalls := 0, 0
			model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				modelCalls++
				return &provider.StreamResult{Stream: textStream(tc.input)}, nil
			}}
			opts := []aisdk.StreamOption{aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(out)}
			if tc.repair != nil {
				opts = append(opts, aisdk.WithRepairText(func(text string, err error) (string, bool, error) {
					repairCalls++
					assert.Equal(t, tc.input, text)
					assert.ErrorIs(t, err, tc.initialErr)
					return tc.repair(text, err)
				}))
			}
			result := aisdk.StreamText(context.Background(), model, opts...)
			for range result.FullStream() {
			}
			if tc.wantErr != nil {
				assert.ErrorIs(t, result.OutputError(), tc.wantErr)
			} else {
				assert.NoError(t, result.OutputError())
				assert.Equal(t, tc.wantValue, result.OutputValue())
			}
			assert.Equal(t, tc.wantParses, out.calls)
			assert.Equal(t, tc.wantCallbacks, repairCalls)
			assert.Equal(t, 1, modelCalls)
		})
	}
}

func TestStructuredOutputRepair_OtherModes(t *testing.T) {
	array, err := output.Array[struct {
		Name string `json:"name"`
	}](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)
	choice, err := output.Choice("sunny")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, original, repaired string
		out                      aisdk.Output
	}{
		{"array", `{"elements":[{"wrong":true}]}`, `{"elements":[{"name":"fixed"}]}`, array},
		{"choice", `{"result":"rainy"}`, `{"result":"sunny"}`, choice},
		{"JSON", `{"bad":`, `{"ok":true}`, output.JSON()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream(tc.original)}, nil
			}}
			calls := 0
			result, err := aisdk.GenerateText(context.Background(), model,
				aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(tc.out),
				aisdk.WithRepairText(func(text string, parseErr error) (string, bool, error) {
					calls++
					assert.Equal(t, tc.original, text)
					assert.ErrorIs(t, parseErr, aisdk.ErrInvalidOutputText)
					return tc.repaired, true, nil
				}),
			)
			require.NoError(t, err)
			assert.NoError(t, result.OutputError)
			assert.NotNil(t, result.Output)
			assert.Equal(t, tc.original, result.Text)
			assert.Equal(t, 1, calls)
		})
	}
}

type repairFailingUnmarshaler struct{}

var errRepairTypedConversion = errors.New("typed conversion failed")

func (*repairFailingUnmarshaler) UnmarshalJSON([]byte) error { return errRepairTypedConversion }

func TestStructuredOutputRepair_TypedConversionDoesNotRepair(t *testing.T) {
	s := mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)
	obj, err := output.Object[struct {
		Name int `json:"name"`
	}](s)
	require.NoError(t, err)
	array, err := output.Array[struct {
		Name int `json:"name"`
	}](s)
	require.NoError(t, err)
	customObject, err := output.Object[repairFailingUnmarshaler](s)
	require.NoError(t, err)
	customArray, err := output.Array[repairFailingUnmarshaler](s)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, text string
		out        aisdk.Output
	}{
		{"object", `{"name":"valid"}`, obj},
		{"array", `{"elements":[{"name":"valid"}]}`, array},
		{"custom object", `{"name":"valid"}`, customObject},
		{"custom array", `{"elements":[{"name":"valid"}]}`, customArray},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: textStream(tc.text)}, nil
			}}
			called := false
			result, err := aisdk.GenerateText(context.Background(), model,
				aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(tc.out),
				aisdk.WithRepairText(func(string, error) (string, bool, error) {
					called = true
					return "", true, nil
				}),
			)
			require.NoError(t, err)
			assert.False(t, called)
			assert.ErrorIs(t, result.OutputError, aisdk.ErrNoObjectGenerated)
			assert.NotErrorIs(t, result.OutputError, aisdk.ErrInvalidOutputText)
			if strings.HasPrefix(tc.name, "custom") {
				assert.ErrorIs(t, result.OutputError, errRepairTypedConversion)
			} else {
				var typeErr *json.UnmarshalTypeError
				assert.ErrorAs(t, result.OutputError, &typeErr)
			}
		})
	}
}

func TestStructuredOutputRepair_ReparsedError(t *testing.T) {
	out, err := output.Object[struct {
		Name string `json:"name"`
	}](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)
	model := &testModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: textStream(`{"name":`)}, nil
	}}
	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(out),
		aisdk.WithRepairText(func(_ string, parseErr error) (string, bool, error) {
			var syntaxErr *json.SyntaxError
			assert.ErrorAs(t, parseErr, &syntaxErr)
			return `{"wrong":"value"}`, true, nil
		}),
	)
	require.NoError(t, err)
	assert.ErrorIs(t, result.OutputError, aisdk.ErrInvalidOutputText)
	var syntaxErr *json.SyntaxError
	assert.NotErrorAs(t, result.OutputError, &syntaxErr)
	var validationErr *jsonschema.ValidationError
	assert.ErrorAs(t, result.OutputError, &validationErr)
}

func TestStructuredOutputRepair_PreservesStreamData(t *testing.T) {
	original := `{"elements":[{"name":"raw"},{"wrong":"value"}]}`
	repaired := `{"elements":[{"name":"fixed"}]}`
	out, err := output.Array[struct {
		Name string `json:"name"`
	}](mustSchema(t, `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)
	newResult := func() *aisdk.StreamTextResult {
		model := &testModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			assert.Equal(t, out.ResponseFormat(), opts.ResponseFormat)
			ch := make(chan provider.StreamPart, 6)
			ch <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "resp-1", ModelID: "mock-1"}
			ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: original}
			ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
			ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: intP(10)}, OutputTokens: provider.OutputTokenUsage{Total: intP(5)}}}
			close(ch)
			return &provider.StreamResult{Stream: ch}, nil
		}}
		return aisdk.StreamText(context.Background(), model,
			aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(out),
			aisdk.WithRepairText(func(text string, _ error) (string, bool, error) {
				assert.Equal(t, original, text)
				return repaired, true, nil
			}),
		)
	}

	result := newResult()
	var full []aisdk.TextStreamPart
	var textDeltas []string
	for part := range result.FullStream() {
		full = append(full, part)
		switch p := part.(type) {
		case aisdk.StreamTextDelta:
			textDeltas = append(textDeltas, p.Text)
		}
	}
	require.Equal(t, []string{original}, textDeltas)
	assert.Equal(t, original, result.Text())
	require.Len(t, result.Steps(), 1)
	assert.Equal(t, original, result.Steps()[0].Text)
	assert.Equal(t, []aisdk.ContentPart{aisdk.TextContent{Text: original}}, result.Content())
	assert.Equal(t, result.Steps()[0].Response, result.Response())
	assert.Equal(t, "resp-1", result.Response().ID)
	assert.Equal(t, 10, *result.Usage().InputTokens.Total)
	assert.Equal(t, 5, *result.TotalUsage().OutputTokens.Total)
	assert.NoError(t, result.OutputError())
	assert.Equal(t, "fixed", result.OutputValue().([]struct {
		Name string `json:"name"`
	})[0].Name)
	assert.NotContains(t, fmt.Sprint(full), "fixed")
	var partials []json.RawMessage
	for partial := range result.PartialOutputStream() {
		partials = append(partials, partial)
		assert.NotContains(t, string(partial), "fixed")
	}
	require.Len(t, partials, 1)
	assert.JSONEq(t, `[{"name":"raw"}]`, string(partials[0]))
	var elements []json.RawMessage
	for element := range result.ElementStream() {
		elements = append(elements, element)
		assert.NotContains(t, string(element), "fixed")
	}
	require.Len(t, elements, 1)
	assert.JSONEq(t, `{"name":"raw"}`, string(elements[0]))

	uiResult := newResult()
	var chunks []aisdk.UIMessageChunk
	for chunk := range uiResult.ToUIMessageStream() {
		chunks = append(chunks, chunk)
	}
	chunkJSON, err := json.Marshal(chunks)
	require.NoError(t, err)
	assert.Contains(t, string(chunkJSON), `\"name\":\"raw\"`)
	assert.NotContains(t, string(chunkJSON), "fixed")
	assert.NoError(t, uiResult.OutputError())
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
	called := false
	model := &testModel{
		streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: textStream("hello")}, nil
		},
	}

	result := aisdk.StreamText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("hi")),
		aisdk.WithRepairText(func(string, error) (string, bool, error) {
			called = true
			return "", true, nil
		}),
	)

	for range result.FullStream() {
	}

	assert.Nil(t, result.OutputValue())
	assert.Nil(t, result.OutputError())
	assert.Equal(t, "hello", result.Text())
	assert.False(t, called)
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
			var value any
			require.NoError(t, json.Unmarshal(raw, &value))
			canonical, err := json.Marshal(value)
			require.NoError(t, err)
			if string(canonical) == last {
				continue
			}
			last = string(canonical)
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

	t.Run("does not republish semantic duplicates", func(t *testing.T) {
		deltas := []string{`{
  "outer": {
    "value": "x"`, "\n  }", "\n}"}
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
			aisdk.WithOutput(output.JSON()),
		)

		var textDeltas []string
		for part := range result.FullStream() {
			if delta, ok := part.(aisdk.StreamTextDelta); ok {
				textDeltas = append(textDeltas, delta.Text)
			}
		}
		var partials []json.RawMessage
		for partial := range result.PartialOutputStream() {
			partials = append(partials, partial)
		}

		require.Len(t, partials, 1)
		assert.JSONEq(t, `{"outer":{"value":"x"}}`, string(partials[0]))
		assert.Equal(t, []string{deltas[0], deltas[1] + deltas[2]}, textDeltas)
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

func TestGenerateText_RepairOutputWithLengthFinishReason(t *testing.T) {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"name":`}
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

	called := false
	result, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithOutput(out),
		aisdk.WithRepairText(func(string, error) (string, bool, error) {
			called = true
			return `{"name":"repaired"}`, true, nil
		}),
	)
	require.NoError(t, err)

	assert.True(t, called)
	assert.Equal(t, provider.FinishReasonLength, result.FinishReason.Unified)
	assert.Equal(t, `{"name":`, result.Text)
	assert.Equal(t, s{Name: "repaired"}, result.Output)
	assert.NoError(t, result.OutputError)
}

func TestStreamText_EffectiveToolChoiceOutput(t *testing.T) {
	for _, mode := range []string{"stream", "agent stream"} {
		for _, choice := range []provider.ToolChoice{{Type: provider.ToolChoiceRequired}, {Type: provider.ToolChoiceTool, ToolName: "lookup"}} {
			for _, tc := range []struct {
				name      string
				text      string
				wantError bool
			}{
				{"valid JSON", `{"value":42}`, false},
				{"malformed JSON", `not JSON`, true},
				{"empty JSON", "", true},
			} {
				t.Run(mode+"/"+string(choice.Type)+"/"+tc.name, func(t *testing.T) {
					providerCalls, executions, errorCalls := 0, 0, 0
					model := &testModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						providerCalls++
						parts := []provider.StreamPart{
							{Type: provider.PartTextStart, ID: "t1"},
							{Type: provider.PartTextDelta, ID: "t1", Delta: tc.text},
							{Type: provider.PartTextEnd, ID: "t1"},
						}
						finish := provider.FinishReason{Unified: provider.FinishReasonStop}
						if choice.Type == provider.ToolChoiceTool {
							parts = append(parts, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "c1", ToolName: "other", Input: `{}`})
							finish.Unified = provider.FinishReasonToolCalls
						}
						parts = append(parts, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish})
						stream := make(chan provider.StreamPart, len(parts))
						for _, part := range parts {
							stream <- part
						}
						close(stream)
						return &provider.StreamResult{Stream: stream}, nil
					}}
					opts := []aisdk.StreamOption{
						aisdk.WithModelMessages(provider.UserText("test")), aisdk.WithOutput(output.JSON()),
						aisdk.WithToolChoice(choice), aisdk.WithStopWhen(aisdk.StepCountIs(3)),
						aisdk.WithTools(aisdk.ToolSet{"lookup": {}, "other": {Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
							executions++
							return json.RawMessage(`"unexpected"`), nil
						}}}),
						aisdk.OnError(func(error) { errorCalls++ }),
					}
					var result *aisdk.StreamTextResult
					if mode == "agent stream" {
						result = aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(opts...)).Stream(t.Context())
					} else {
						result = aisdk.StreamText(t.Context(), model, opts...)
					}
					streamErrors, stepFinishes, finishes := 0, 0, 0
					for event := range result.FullStream() {
						switch part := event.(type) {
						case aisdk.StreamError:
							streamErrors++
							assert.ErrorContains(t, part.Error, "tool choice")
						case aisdk.StreamFinishStep:
							stepFinishes++
							assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
						case aisdk.StreamFinish:
							finishes++
							assert.Equal(t, provider.FinishReasonError, part.FinishReason.Unified)
						}
					}
					require.ErrorContains(t, result.Err(), "tool choice")
					assert.NotErrorIs(t, result.Err(), aisdk.ErrNoObjectGenerated)
					assert.Equal(t, tc.text, result.Text())
					if tc.wantError {
						assert.Nil(t, result.OutputValue())
						require.ErrorIs(t, result.OutputError(), aisdk.ErrNoObjectGenerated)
					} else {
						require.NoError(t, result.OutputError())
						assert.Equal(t, map[string]any{"value": float64(42)}, result.OutputValue())
					}
					assert.Zero(t, executions)
					assert.Equal(t, 1, providerCalls)
					assert.Equal(t, 1, errorCalls)
					assert.Equal(t, 1, streamErrors)
					assert.Equal(t, 1, stepFinishes)
					assert.Equal(t, 1, finishes)
				})
			}
		}
	}
}
