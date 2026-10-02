package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

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

func TestFallbackRoute_EmptyMessageOptionsPreserveTextFailover(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "unary"
		if streaming {
			name = "streaming"
		}
		t.Run(name, func(t *testing.T) {
			message := provider.UserText("hello")
			message.ProviderOptions = provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: []byte(` { } `)}}
			options := provider.CallOptions{Prompt: []provider.Message{message}}
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
			if streaming {
				result, err := resolved.Model.DoStream(t.Context(), options)
				require.NoError(t, err)
				for range result.Stream {
				}
			} else {
				_, err = resolved.Model.DoGenerate(t.Context(), options)
				require.NoError(t, err)
			}
			assert.Equal(t, []string{"backend-primary", "backend-secondary"}, calls)
		})
	}
}

func fallbackCatalogFile() config.File {
	return config.File{Models: map[string]config.Model{"public": {Name: "Public", Aliases: []string{"alias"}, Primary: config.Primary{Provider: "primary-instance", Model: "backend-primary"}, Fallback: []config.Primary{{Provider: "secondary-instance", Model: "backend-secondary"}}}}}
}

func fallbackProviders() map[string]config.ResolvedProvider {
	return map[string]config.ResolvedProvider{"primary-instance": {Type: "anthropic", APIKey: "private-key"}, "secondary-instance": {Type: "anthropic", APIKey: "private-key"}}
}
