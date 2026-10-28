package grafana

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

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
	// ProviderTimeouts bounds how long BYOK providers may take to start
	// responding.
	ProviderTimeouts *ProviderTimeouts `json:"providerTimeouts,omitempty"`
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

// ProviderTimeouts bounds provider start times.
type ProviderTimeouts struct {
	// BYOK maps provider names to milliseconds a BYOK account may take to send
	// its first response byte before the Gateway tries the next account.
	BYOK map[string]int `json:"byok,omitzero"`
}

// GatewayMetadata is the Gateway's attribution of one call, read from
// providerMetadata.gateway. It is best-effort: absent metadata does not mean
// no provider was tried.
type GatewayMetadata struct {
	// Execution lists the observed attempts, or is nil when absent.
	Execution *ExecutionOverview `json:"execution"`
	// Routing groups the attempts by model, or is nil when absent.
	Routing *Routing `json:"routing"`
}

// ExecutionOverview lists the attempts one call observed, in order.
type ExecutionOverview struct {
	RequestedModelID string `json:"requestedModelId"`
	// CanonicalModelID is empty when no catalog resolution selected the model.
	CanonicalModelID string             `json:"canonicalModelId"`
	Attempts         []ExecutionAttempt `json:"attempts"`
}

// AttemptOutcome is the selection outcome of one attempt.
type AttemptOutcome string

const (
	// AttemptSelected means the provider's result or first stream part was
	// accepted, not that a stream completed.
	AttemptSelected AttemptOutcome = "selected"
	// AttemptFailed means the attempt failed before selection.
	AttemptFailed AttemptOutcome = "failed"
	// AttemptCanceled means the call ended during the attempt.
	AttemptCanceled AttemptOutcome = "canceled"
)

// ExecutionAttempt is one observed attempt.
type ExecutionAttempt struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
	// ProviderInstance is the configured provider instance, empty for BYOK.
	ProviderInstance string          `json:"providerInstance"`
	Outcome          AttemptOutcome  `json:"outcome"`
	Error            *AttemptFailure `json:"error"`
}

// AttemptFailure is the provider's own failure summary for one attempt.
type AttemptFailure struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	// Code is the provider's string or number code as sent, or nil.
	Code       json.RawMessage `json:"code"`
	StatusCode int             `json:"statusCode"`
}

// Routing groups one call's attempts by model, using a subset of the Vercel
// AI Gateway's providerMetadata.gateway.routing fields.
type Routing struct {
	OriginalModelID string `json:"originalModelId"`
	// CanonicalSlug is empty when no catalog resolution selected the model.
	CanonicalSlug    string `json:"canonicalSlug"`
	ResolvedProvider string `json:"resolvedProvider"`
	// FinalProvider is empty when no attempt was selected.
	FinalProvider             string                `json:"finalProvider"`
	ModelAttemptCount         int                   `json:"modelAttemptCount"`
	TotalProviderAttemptCount int                   `json:"totalProviderAttemptCount"`
	ModelAttempts             []RoutingModelAttempt `json:"modelAttempts"`
}

// RoutingModelAttempt is one model with at least one attempt.
type RoutingModelAttempt struct {
	// ModelID is the requested model ID or models entry as sent.
	ModelID              string                   `json:"modelId"`
	CanonicalSlug        string                   `json:"canonicalSlug"`
	Success              bool                     `json:"success"`
	ProviderAttemptCount int                      `json:"providerAttemptCount"`
	ProviderAttempts     []RoutingProviderAttempt `json:"providerAttempts"`
}

// RoutingProviderAttempt is one attempt of a model. Success means the attempt
// was selected, not that a stream completed.
type RoutingProviderAttempt struct {
	Provider   string `json:"provider"`
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode"`
}

// GatewayMetadataFrom reads Gateway attribution from a result's, finish part's
// or other part's provider metadata. It returns nil without an error when the
// gateway namespace is absent or carries neither execution nor routing, and an
// error when either does not decode.
func GatewayMetadataFrom(metadata provider.ProviderMetadata) (*GatewayMetadata, error) {
	raw, ok := metadata["gateway"]
	if !ok {
		return nil, nil
	}
	var members struct {
		Execution json.RawMessage `json:"execution"`
		Routing   json.RawMessage `json:"routing"`
	}
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, fmt.Errorf("grafana: invalid gateway metadata: %w", err)
	}
	var gateway GatewayMetadata
	if err := decodeMember(members.Execution, &gateway.Execution); err != nil {
		return nil, fmt.Errorf("grafana: invalid gateway execution metadata: %w", err)
	}
	if err := decodeMember(members.Routing, &gateway.Routing); err != nil {
		return nil, fmt.Errorf("grafana: invalid gateway routing metadata: %w", err)
	}
	if gateway.Execution == nil && gateway.Routing == nil {
		return nil, nil
	}
	return &gateway, nil
}

// GatewayMetadataFromError reads Gateway attribution from a returned error or
// a stream error part's API call error. It follows [GatewayMetadataFrom], and
// returns nil without an error when err carries no Gateway error document.
func GatewayMetadataFromError(err error) (*GatewayMetadata, error) {
	var api *provider.APICallError
	if !errors.As(err, &api) || len(api.Data) == 0 {
		return nil, nil
	}
	var document struct {
		ProviderMetadata provider.ProviderMetadata `json:"providerMetadata"`
	}
	if json.Unmarshal(api.Data, &document) != nil {
		return nil, nil
	}
	return GatewayMetadataFrom(document.ProviderMetadata)
}

func decodeMember[T any](raw json.RawMessage, target **T) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*target = &value
	return nil
}
