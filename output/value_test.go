package output_test

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

type valTestModel struct {
	streamFunc func(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error)
}

func (m *valTestModel) SpecificationVersion() string               { return "v4" }
func (m *valTestModel) Provider() string                           { return "mock" }
func (m *valTestModel) ModelID() string                            { return "mock-1" }
func (m *valTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *valTestModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	return m.streamFunc(ctx, opts)
}
func (m *valTestModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func valIntPtr(v int) *int { return &v }

func valTextStream(text string) <-chan provider.StreamPart {
	ch := make(chan provider.StreamPart, 10)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: text}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: valIntPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: valIntPtr(5)}}}
	}()
	return ch
}

type valRecipe struct {
	Name string `json:"name"`
}

func valNameSchema(t *testing.T) schema.Schema {
	t.Helper()
	s, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`))
	require.NoError(t, err)
	return s
}

func TestValue_CorrectType(t *testing.T) {
	model := &valTestModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: valTextStream(`{"name":"Lasagna"}`)}, nil
		},
	}

	s := valNameSchema(t)
	out, err := output.Object[valRecipe](s)
	require.NoError(t, err)

	result, err := output.GenerateObject[valRecipe](context.Background(), model, out,
		aisdk.WithModelMessages(provider.UserText("recipe")),
	)
	require.NoError(t, err)

	recipe, err := result.Object()
	require.NoError(t, err)
	assert.Equal(t, "Lasagna", recipe.Name)
}

func TestValue_TypeMismatch(t *testing.T) {
	model := &valTestModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: valTextStream(`{"name":"test"}`)}, nil
		},
	}

	s := valNameSchema(t)
	out, err := output.Object[valRecipe](s)
	require.NoError(t, err)

	genResult, err := aisdk.GenerateText(context.Background(), model,
		aisdk.WithModelMessages(provider.UserText("test")),
		aisdk.WithOutput(out),
	)
	require.NoError(t, err)

	type otherType struct {
		Age int `json:"age"`
	}

	accessor := &genResultAccessor{genResult}
	_, err = output.Value[otherType](accessor)
	require.Error(t, err)
}

type genResultAccessor struct {
	*aisdk.GenerateTextResult
}

func (a *genResultAccessor) OutputValue() any   { return a.Output }
func (a *genResultAccessor) OutputError() error { return a.GenerateTextResult.OutputError }

func TestValue_NilOutput(t *testing.T) {
	accessor := &genResultAccessor{&aisdk.GenerateTextResult{}}
	_, err := output.Value[valRecipe](accessor)
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestStreamObject(t *testing.T) {
	model := &valTestModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: valTextStream(`{"name":"Pizza"}`)}, nil
		},
	}

	s := valNameSchema(t)
	out, err := output.Object[valRecipe](s)
	require.NoError(t, err)

	result := output.StreamObject[valRecipe](context.Background(), model, out,
		aisdk.WithModelMessages(provider.UserText("recipe")),
	)

	for range result.FullStream() {
	}

	recipe, err := result.Object()
	require.NoError(t, err)
	assert.Equal(t, "Pizza", recipe.Name)
}

func TestGenerateObject_OutputError(t *testing.T) {
	model := &valTestModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: valTextStream(`{"wrong":"data"}`)}, nil
		},
	}

	s := valNameSchema(t)
	out, err := output.Object[valRecipe](s)
	require.NoError(t, err)

	result, err := output.GenerateObject[valRecipe](context.Background(), model, out,
		aisdk.WithModelMessages(provider.UserText("recipe")),
	)
	require.NoError(t, err)

	_, err = result.Object()
	require.Error(t, err)
	assert.ErrorIs(t, err, aisdk.ErrNoObjectGenerated)
}

func TestObjectRepair(t *testing.T) {
	for _, tc := range []struct {
		name     string
		stream   bool
		text     string
		repaired string
	}{
		{name: "generate malformed JSON", text: `{"name":`, repaired: `{"name":"Lasagna"}`},
		{name: "generate schema failure", text: `{"wrong":"data"}`, repaired: `{"name":"Lasagna"}`},
		{name: "stream malformed JSON", stream: true, text: `{"name":`, repaired: `{"name":"Lasagna"}`},
		{name: "stream schema failure", stream: true, text: `{"wrong":"data"}`, repaired: `{"name":"Lasagna"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			model := &valTestModel{streamFunc: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
				assert.Equal(t, provider.ResponseFormatJSON, opts.ResponseFormat.Type)
				return &provider.StreamResult{Stream: valTextStream(tc.text)}, nil
			}}
			out, err := output.Object[valRecipe](valNameSchema(t))
			require.NoError(t, err)
			repair := aisdk.WithRepairText(func(text string, parseErr error) (string, bool, error) {
				calls++
				assert.Equal(t, tc.text, text)
				assert.ErrorIs(t, parseErr, aisdk.ErrNoObjectGenerated)
				assert.ErrorIs(t, parseErr, aisdk.ErrInvalidOutputText)
				return tc.repaired, true, nil
			})
			if tc.stream {
				result := output.StreamObject[valRecipe](context.Background(), model, out, aisdk.WithModelMessages(provider.UserText("recipe")), repair)
				for range result.FullStream() {
				}
				value, err := result.Object()
				require.NoError(t, err)
				assert.Equal(t, "Lasagna", value.Name)
				assert.Equal(t, tc.text, result.Text())
			} else {
				result, err := output.GenerateObject[valRecipe](context.Background(), model, out, aisdk.WithModelMessages(provider.UserText("recipe")), repair)
				require.NoError(t, err)
				value, err := result.Object()
				require.NoError(t, err)
				assert.Equal(t, "Lasagna", value.Name)
				assert.Equal(t, tc.text, result.Text)
			}
			assert.Equal(t, 1, calls)
		})
	}
}

func TestObjectRepair_FailurePaths(t *testing.T) {
	out, err := output.Object[valRecipe](valNameSchema(t))
	require.NoError(t, err)
	callbackErr := errors.New("repair unavailable")
	for _, tc := range []struct {
		name, original, repaired string
		stream                   bool
		useRepair                bool
		accepted                 bool
		callbackErr              error
		wantErr                  error
		wantCalls                int
	}{
		{name: "generate without repair", original: `{"wrong":1}`, wantErr: aisdk.ErrNoObjectGenerated},
		{name: "stream without repair", original: `{"wrong":1}`, stream: true, wantErr: aisdk.ErrNoObjectGenerated},
		{name: "declined", original: `{"wrong":1}`, repaired: `{"name":"valid"}`, useRepair: true, wantErr: aisdk.ErrInvalidOutputText, wantCalls: 1},
		{name: "callback error", original: `{"wrong":1}`, repaired: `{"name":"valid"}`, useRepair: true, accepted: true, callbackErr: callbackErr, wantErr: callbackErr, wantCalls: 1},
		{name: "invalid accepted repair", original: `{"name":`, repaired: `{"wrong":1}`, useRepair: true, accepted: true, wantErr: aisdk.ErrInvalidOutputText, wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &valTestModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: valTextStream(tc.original)}, nil
			}}
			calls := 0
			repair := aisdk.WithRepairText(func(text string, err error) (string, bool, error) {
				calls++
				assert.Equal(t, tc.original, text)
				return tc.repaired, tc.accepted, tc.callbackErr
			})
			var got error
			if tc.stream {
				opts := []aisdk.StreamOption{aisdk.WithModelMessages(provider.UserText("recipe"))}
				if tc.useRepair {
					opts = append(opts, repair)
				}
				result := output.StreamObject[valRecipe](context.Background(), model, out, opts...)
				for range result.FullStream() {
				}
				_, got = result.Object()
				assert.Equal(t, tc.original, result.Text())
			} else {
				opts := []aisdk.GenerateOption{aisdk.WithModelMessages(provider.UserText("recipe"))}
				if tc.useRepair {
					opts = append(opts, repair)
				}
				result, err := output.GenerateObject[valRecipe](context.Background(), model, out, opts...)
				require.NoError(t, err)
				_, got = result.Object()
				assert.Equal(t, tc.original, result.Text)
			}
			assert.ErrorIs(t, got, tc.wantErr)
			assert.Equal(t, tc.wantCalls, calls)
		})
	}
}

func TestTypedElementStream(t *testing.T) {
	ch := make(chan provider.StreamPart, 20)
	go func() {
		defer close(ch)
		ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `{"elements":[{"name":"A`}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: `lice"},{"name":"Bob"}]}`}
		ch <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
		ch <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{InputTokens: provider.InputTokenUsage{Total: valIntPtr(10)}, OutputTokens: provider.OutputTokenUsage{Total: valIntPtr(5)}}}
	}()

	model := &valTestModel{
		streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: ch}, nil
		},
	}

	elemSchema := valNameSchema(t)
	out, err := output.Array[valRecipe](elemSchema)
	require.NoError(t, err)

	result := output.StreamObject[[]valRecipe](context.Background(), model, out,
		aisdk.WithModelMessages(provider.UserText("names")),
	)

	var elements []valRecipe
	done := make(chan struct{})
	go func() {
		defer close(done)
		for elem := range output.TypedElementStream[valRecipe](result.StreamTextResult) {
			elements = append(elements, elem)
		}
	}()

	for range result.FullStream() {
	}
	<-done

	require.NoError(t, result.OutputError())
}
