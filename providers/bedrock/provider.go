package bedrock

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
)

// Provider is a registry.Provider that constructs Bedrock language models
// on demand. Captures the construction-time options once so every model
// returned by LanguageModel shares the same region, credentials, HTTP
// client, headers, etc.
//
// Use [NewProvider] to construct an instance and register it under any
// provider id with `registry.NewLanguageModelRegistry`.
type Provider struct {
	opts []Option
}

var _ registry.Provider = (*Provider)(nil)

// NewProvider creates a registry-compatible Bedrock provider value with the
// given module-level options. The options are applied to every model
// returned by LanguageModel.
//
//	reg.RegisterProvider("bedrock", bedrock.NewProvider(
//	    bedrock.WithRegion("us-east-1"),
//	))
//	model, _ := reg.LanguageModel("bedrock:anthropic.claude-sonnet-5")
func NewProvider(opts ...Option) *Provider {
	return &Provider{opts: opts}
}

// LanguageModel implements registry.Provider. The returned model's
// ModelID() equals the supplied modelID verbatim.
func (p *Provider) LanguageModel(modelID string) (provider.LanguageModel, error) {
	return New(modelID, p.opts...), nil
}
