package output_test

import (
	"context"
	"encoding/json"
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
