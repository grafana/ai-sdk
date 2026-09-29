package service

import (
	"context"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type fallbackTextModel struct{ provider.LanguageModel }

func (model fallbackTextModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	if !fallbackTextRequest(options) {
		return nil, catalog.ErrUnsupportedRequest
	}
	return model.LanguageModel.DoGenerate(ctx, options)
}

func (model fallbackTextModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	if !fallbackTextRequest(options) {
		return nil, catalog.ErrUnsupportedRequest
	}
	return model.LanguageModel.DoStream(ctx, options)
}

func fallbackTextRequest(options provider.CallOptions) bool {
	if options.ToolChoice != nil && (options.ToolChoice.Type != provider.ToolChoiceAuto || options.ToolChoice.ToolName != "") {
		return false
	}
	if len(options.Tools) != 0 || len(options.ProviderOptions) != 0 || len(options.Headers) != 0 || options.IncludeRawChunks {
		return false
	}
	if options.ResponseFormat != nil && options.ResponseFormat.Type != provider.ResponseFormatText {
		return false
	}
	for _, message := range options.Prompt {
		switch message.Role {
		case provider.RoleSystem, provider.RoleUser, provider.RoleAssistant:
		default:
			return false
		}
		if len(message.ProviderOptions) != 0 {
			return false
		}
		for _, part := range message.Content {
			if part.Type != provider.ContentPartTypeText || len(part.ProviderOptions) != 0 {
				return false
			}
		}
	}
	return true
}
