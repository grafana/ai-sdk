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

func TestFallbackRoute_RejectsEffectsBeforeAnyCandidate(t *testing.T) {
	for _, opts := range []provider.CallOptions{
		{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "secret"}}},
		{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone}},
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
		_, err := guarded.DoGenerate(context.Background(), opts)
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		_, err = guarded.DoStream(context.Background(), opts)
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
	}
}

func fallbackCatalogFile() config.File {
	return config.File{Models: map[string]config.Model{"public": {Name: "Public", Aliases: []string{"alias"}, Primary: config.Primary{Provider: "primary-instance", Model: "backend-primary"}, Fallback: []config.Primary{{Provider: "secondary-instance", Model: "backend-secondary"}}}}}
}

func fallbackProviders() map[string]config.ResolvedProvider {
	return map[string]config.ResolvedProvider{"primary-instance": {Type: "anthropic", APIKey: "private-key"}, "secondary-instance": {Type: "anthropic", APIKey: "private-key"}}
}
