package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddToolInputExamples(t *testing.T) {
	var received provider.CallOptions
	model := &mockModel{
		doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			received = params
			return &provider.GenerateResult{}, nil
		},
	}
	wrapped := WrapLanguageModel(model, AddToolInputExamples(AddToolInputExamplesOptions{}))

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Tools: []provider.Tool{
			{
				Type:        provider.ToolTypeFunction,
				Name:        "weather",
				Description: ptr("Get weather"),
				InputExamples: []provider.InputExample{
					{Input: json.RawMessage("{\n  \"city\": \"London\"\n}")},
					{Input: json.RawMessage(`{"city":"Paris"}`)},
				},
			},
			{Type: provider.ToolTypeProvider, Name: "web", ID: "provider.web"},
		},
	})
	require.NoError(t, err)
	require.Len(t, received.Tools, 2)
	require.NotNil(t, received.Tools[0].Description)
	assert.Equal(t, "Get weather\n\nInput Examples:\n{\"city\":\"London\"}\n{\"city\":\"Paris\"}", *received.Tools[0].Description)
	assert.Nil(t, received.Tools[0].InputExamples)
	assert.Equal(t, "provider.web", received.Tools[1].ID)
}

func TestAddToolInputExamples_KeepExamples(t *testing.T) {
	keep := false
	var received provider.CallOptions
	model := &mockModel{
		doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			received = params
			return &provider.GenerateResult{}, nil
		},
	}
	wrapped := WrapLanguageModel(model, AddToolInputExamples(AddToolInputExamplesOptions{
		Prefix: "Examples:",
		Remove: &keep,
		Format: func(example provider.InputExample, index int) string {
			return string(rune('A'+index)) + ": " + string(example.Input)
		},
	}))

	_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{
		Tools: []provider.Tool{{
			Type:          provider.ToolTypeFunction,
			Name:          "weather",
			InputExamples: []provider.InputExample{{Input: json.RawMessage(`{"city":"London"}`)}},
		}},
	})
	require.NoError(t, err)
	require.Len(t, received.Tools[0].InputExamples, 1)
	require.NotNil(t, received.Tools[0].Description)
	assert.Equal(t, "Examples:\nA: {\"city\":\"London\"}", *received.Tools[0].Description)
}
