package grafana

// GatewayOptions are the Gateway host controls of one call, sent as
// providerOptions.gateway. They are the subset of the Vercel AI Gateway
// GatewayProviderOptions that the Grafana AI Gateway accepts. The client
// encodes them without validation; the Gateway decides and reports invalid
// values as a [GatewayError].
type GatewayOptions struct {
	// Models are fallback model IDs, tried in order after the requested model.
	// Callers with configured access name public model IDs or aliases; callers
	// with their own keys name provider/model selectors backed by BYOK.
	Models []string `json:"models,omitzero"`
	// Order is a provider preference: matching candidates of each model are
	// tried first, in this order. It does not remove candidates.
	Order []string `json:"order,omitzero"`
	// Only restricts each model's candidates to these providers. A nil Only is
	// absent; an empty, non-nil Only excludes every candidate.
	Only []string `json:"only,omitzero"`
	// BYOK maps provider names to ordered request accounts.
	BYOK map[string][]BYOKAccount `json:"byok,omitzero"`
}

// ProviderKey returns the gateway provider-option namespace.
func (GatewayOptions) ProviderKey() string { return "gateway" }

// BYOKAccount is one request-supplied provider account.
type BYOKAccount struct {
	APIKey       string `json:"apiKey"`
	BaseURL      string `json:"baseURL,omitempty"`
	Organization string `json:"organization,omitempty"`
	Project      string `json:"project,omitempty"`
}
