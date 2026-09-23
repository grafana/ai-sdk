package service

import (
	"context"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type mcpRouteModel struct {
	provider.LanguageModel
	allowMCP bool
}

func (model mcpRouteModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if !model.allowMCP && len(options.ProviderOptions) != 0 {
		return nil, catalog.ErrUnsupportedRequest
	}
	return model.LanguageModel.DoGenerate(ctx, options)
}

func (model mcpRouteModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if !model.allowMCP && len(options.ProviderOptions) != 0 {
		return nil, catalog.ErrUnsupportedRequest
	}
	return model.LanguageModel.DoStream(ctx, options)
}
