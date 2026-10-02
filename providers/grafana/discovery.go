package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

// ModelSpecification identifies the public LanguageModelV4 model.
type ModelSpecification struct {
	SpecificationVersion string `json:"specificationVersion"`
	Provider             string `json:"provider"`
	ModelID              string `json:"modelId"`
}

// ConfiguredCandidate is an explicitly configured invocation destination.
// ProviderInstance is a configuration key; Provider is its effective namespace.
// ModelID is the configured invocation ID, never a provider-reported response ID.
type ConfiguredCandidate struct {
	ProviderInstance string `json:"providerInstance"`
	Provider         string `json:"provider"`
	ModelID          string `json:"modelId"`
}

// ConfiguredRoute contains authorized configured facts, not runtime attempt results.
// Aliases retain configured order; Candidates lists primary before fallbacks.
type ConfiguredRoute struct {
	CanonicalModelID string                `json:"canonicalModelId"`
	Aliases          []string              `json:"aliases"`
	Candidates       []ConfiguredCandidate `json:"candidates"`
}

// ModelInfo is a public catalog row with optional configured-route facts.
// Gateway is nil when the server omits the extension; absence is not an empty route.
// Specifications always identify the public row, even for aliases.
type ModelInfo struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   *string            `json:"description,omitempty"`
	Specification ModelSpecification `json:"specification"`
	Gateway       *ConfiguredRoute   `json:"gateway,omitempty"`
}

// ListModels returns the authenticated catalog in server order, without caching.
// The complete document must fit Limits.DiscoveryBytes and decode into ModelInfo.
// Catalog policy and route consistency belong to server startup validation.
// Malformed or oversized catalogs return no partial results.
func (p *Provider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := p.request(ctx, http.MethodGet, "/config")
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		retryable := true
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "grafana: discovery transport failed", Cause: err, IsRetryable: &retryable})
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, readGatewayError(ctx, resp, p.limits.ErrorBytes)
	}
	body, _, err := readJSON(ctx, resp, p.limits.DiscoveryBytes)
	if err != nil {
		return nil, protocolError("grafana: invalid discovery response", resp.StatusCode, err)
	}
	var document struct {
		Models []ModelInfo `json:"models"`
	}
	if json.Unmarshal(body, &document) != nil || document.Models == nil {
		return nil, protocolError("grafana: invalid discovery document", resp.StatusCode, nil)
	}
	return document.Models, nil
}

func validPublicText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != ""
}

func readJSON(ctx context.Context, resp *http.Response, limit int64) (body []byte, transportFailure bool, err error) {
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" && (!strings.HasPrefix(media, "application/") || !strings.HasSuffix(media, "+json")) {
		return nil, false, errors.New("grafana: expected JSON media type")
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if err != nil {
		return body, true, err
	}
	if int64(len(body)) > limit {
		return nil, false, errors.New("grafana: response byte limit exceeded")
	}
	if !validJSON(body) {
		return nil, false, errors.New("grafana: malformed JSON response")
	}
	return body, false, nil
}

func protocolError(message string, status int, cause error) *provider.APICallError {
	retryable := false
	return provider.NewAPICallError(provider.APICallErrorOptions{Message: message, StatusCode: status, Cause: cause, IsRetryable: &retryable})
}
