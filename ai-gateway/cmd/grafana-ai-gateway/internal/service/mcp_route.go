package service

import (
	"context"
	"encoding/json"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
)

type mcpRouteModel struct {
	provider.LanguageModel
	allowMCP bool
}

func (model mcpRouteModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if !model.allowMCP && hasMCPServers(options.ProviderOptions) {
		return nil, catalog.ErrUnsupportedRequest
	}
	return model.LanguageModel.DoGenerate(ctx, options)
}

func (model mcpRouteModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if !model.allowMCP && hasMCPServers(options.ProviderOptions) {
		return nil, catalog.ErrUnsupportedRequest
	}
	return model.LanguageModel.DoStream(ctx, options)
}

func hasMCPServers(options provider.ProviderOptions) bool {
	switch value := options["anthropic"].(type) {
	case anthropicprovider.AnthropicOptions:
		return len(value.MCPServers) > 0
	case provider.RawProviderOption:
		var fields map[string]json.RawMessage
		if json.Unmarshal(value.Raw, &fields) != nil {
			return false
		}
		_, exists := fields["mcpServers"]
		return exists
	default:
		return false
	}
}
