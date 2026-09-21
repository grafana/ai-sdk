package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackRoute_OrderAndSingleComposition(t *testing.T) {
	file := fallbackCatalogFile()
	constructed, wrapped := 0, 0
	var calls []string
	options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}}
	created, err := buildCatalog(file, fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
		constructed++
		return &observabilityTestModel{generate: func(_ context.Context, got provider.CallOptions) (*provider.GenerateResult, error) {
			calls = append(calls, id)
			assert.Equal(t, options, got)
			if id == "backend-primary" {
				return nil, errors.New("private upstream error")
			}
			return &provider.GenerateResult{}, nil
		}}
	}, func(id string, lower provider.LanguageModel) (provider.LanguageModel, error) {
		wrapped++
		return identityModelFactory(id, lower)
	})
	require.NoError(t, err)
	require.Equal(t, 2, constructed)
	require.Equal(t, 1, wrapped)
	canonical, err := created.ResolveModel(context.Background(), "public")
	require.NoError(t, err)
	alias, err := created.ResolveModel(context.Background(), "alias")
	require.NoError(t, err)
	assert.Same(t, canonical.Model, alias.Model)
	assert.Equal(t, "public", alias.Model.ModelID())
	for range 2 {
		_, err = alias.Model.DoGenerate(context.Background(), options)
		require.NoError(t, err)
	}
	assert.Equal(t, []string{"backend-primary", "backend-secondary", "backend-primary", "backend-secondary"}, calls)
	assert.Equal(t, 2, constructed)
}

func TestFallbackRoute_AutomaticToolChoice(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "unary"
		if streaming {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto}}
			var calls []string
			created, err := buildCatalog(fallbackCatalogFile(), fallbackProviders(), http.DefaultClient, func(_ string, id string, _ ...anthropicprovider.Option) provider.LanguageModel {
				return &observabilityTestModel{
					generate: func(_ context.Context, got provider.CallOptions) (*provider.GenerateResult, error) {
						calls = append(calls, id)
						assert.Equal(t, options, got)
						if id == "backend-primary" {
							return nil, errors.New("private upstream error")
						}
						return &provider.GenerateResult{}, nil
					},
					stream: func(_ context.Context, got provider.CallOptions) (*provider.StreamResult, error) {
						calls = append(calls, id)
						assert.Equal(t, options, got)
						if id == "backend-primary" {
							return nil, errors.New("private upstream error")
						}
						parts := make(chan provider.StreamPart, 1)
						parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
						close(parts)
						return &provider.StreamResult{Stream: parts}, nil
					},
				}
			}, identityModelFactory)
			require.NoError(t, err)
			resolved, err := created.ResolveModel(t.Context(), "public")
			require.NoError(t, err)
			for range 2 {
				if streaming {
					result, err := resolved.Model.DoStream(t.Context(), options)
					require.NoError(t, err)
					for range result.Stream {
					}
				} else {
					_, err := resolved.Model.DoGenerate(t.Context(), options)
					require.NoError(t, err)
				}
			}
			assert.Equal(t, []string{"backend-primary", "backend-secondary", "backend-primary", "backend-secondary"}, calls)
		})
	}
}

func TestFallbackRoute_RejectsEffectsBeforeAnyCandidate(t *testing.T) {
	for _, opts := range []provider.CallOptions{
		{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "secret"}}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "f"}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceType("future")}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto, ToolName: "f"}},
		{Headers: map[string]string{"x-test": "value"}},
		{IncludeRawChunks: true},
		{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON}},
		{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{Type: provider.ContentPartTypeToolCall})}},
		{Prompt: []provider.Message{provider.NewToolMessage()}},
		{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: "future-effect"})}},
		{ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: []byte(`{"secret":true}`)}}},
	} {
		lower := &observabilityTestModel{
			generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				t.Fatal("candidate invoked")
				return nil, nil
			},
			stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				t.Fatal("candidate invoked")
				return nil, nil
			},
		}
		guarded := fallbackTextModel{LanguageModel: lower}
		variants := []provider.CallOptions{opts}
		if opts.ToolChoice == nil {
			opts.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceAuto}
			variants = append(variants, opts)
		}
		for _, variant := range variants {
			_, err := guarded.DoGenerate(context.Background(), variant)
			require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
			_, err = guarded.DoStream(context.Background(), variant)
			require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		}
	}
}

func fallbackCatalogFile() config.File {
	return config.File{Models: map[string]config.Model{"public": {Name: "Public", Aliases: []string{"alias"}, Primary: config.Primary{Provider: "primary-instance", Model: "backend-primary"}, Fallback: []config.Primary{{Provider: "secondary-instance", Model: "backend-secondary"}}}}}
}

func fallbackProviders() map[string]config.ResolvedProvider {
	return map[string]config.ResolvedProvider{"primary-instance": {Type: "anthropic", APIKey: "private-key"}, "secondary-instance": {Type: "anthropic", APIKey: "private-key"}}
}
