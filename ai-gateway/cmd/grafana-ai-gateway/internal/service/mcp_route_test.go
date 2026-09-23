package service

import (
	"context"
	"encoding/json"
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

func TestBuildCatalog_MCPRouteCapability(t *testing.T) {
	file := config.File{Models: map[string]config.Model{
		"direct":     {Name: "Direct", Primary: config.Primary{Provider: "anthropic", Model: "backend-direct"}},
		"compatible": {Name: "Compatible", Primary: config.Primary{Provider: "compatible", Model: "backend-compatible"}},
		"fallback":   {Name: "Fallback", Primary: config.Primary{Provider: "anthropic", Model: "backend-primary"}, Fallback: []config.Primary{{Provider: "anthropic", Model: "backend-secondary"}}},
	}}
	providers := map[string]config.ResolvedProvider{
		"anthropic":  {Type: "anthropic", APIKey: "private-key"},
		"compatible": {Type: "openai-compatible", APIKey: "private-key", BaseURL: "https://compatible.invalid/v1"},
	}
	calls := 0
	created, err := buildCatalog(file, providers, http.DefaultClient, func(_ string, _ string, _ ...anthropicprovider.Option) provider.LanguageModel {
		return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			calls++
			return &provider.GenerateResult{}, nil
		}}
	}, func(id string, lower provider.LanguageModel) (provider.LanguageModel, error) {
		gated, ok := lower.(mcpRouteModel)
		require.True(t, ok)
		assert.Equal(t, id == "direct", gated.allowMCP)
		return identityModelFactory(id, lower)
	})
	require.NoError(t, err)
	options := provider.CallOptions{ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}`)}}}
	for _, id := range []string{"compatible", "fallback", "direct"} {
		resolved, err := created.ResolveModel(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, "grafana", resolved.Model.Provider())
		_, err = resolved.Model.DoGenerate(context.Background(), options)
		if id == "direct" {
			require.NoError(t, err)
			assert.Equal(t, 1, calls)
		} else {
			assert.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
			assert.Zero(t, calls)
		}
	}
}

func TestMCPRouteModel_BoundToConfiguredRoute(t *testing.T) {
	calls := 0
	lower := &observabilityTestModel{
		generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			calls++
			return &provider.GenerateResult{}, nil
		},
		stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			calls++
			return &provider.StreamResult{}, nil
		},
	}
	options := provider.CallOptions{ProviderOptions: provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example.test"}]}`)}}}
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "non-Anthropic or fallback", true: "direct Anthropic"}[allow], func(t *testing.T) {
			model := mcpRouteModel{LanguageModel: lower, allowMCP: allow}
			_, generateErr := model.DoGenerate(context.Background(), options)
			_, streamErr := model.DoStream(context.Background(), options)
			if allow {
				require.NoError(t, generateErr)
				require.NoError(t, streamErr)
				assert.Equal(t, 2, calls)
			} else {
				assert.True(t, errors.Is(generateErr, catalog.ErrUnsupportedRequest))
				assert.True(t, errors.Is(streamErr, catalog.ErrUnsupportedRequest))
				assert.Zero(t, calls)
			}
		})
	}
}
