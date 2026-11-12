package outbound

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
)

var (
	// ErrEndpointForm reports a custom endpoint that is not an absolute URL
	// without userinfo, query or fragment.
	ErrEndpointForm = errors.New("gateway outbound: endpoint must be an absolute URL without userinfo, query or fragment")
	// ErrEndpointScheme reports a custom endpoint that is not https.
	ErrEndpointScheme = errors.New("gateway outbound: endpoint must use https")
)

// CheckEndpoint checks a customer-chosen endpoint without resolving it and
// returns it normalized. An endpoint equal to native, the provider type's
// native endpoint, counts as unset: CheckEndpoint returns "" without applying
// policy, so the model keeps the shared client. Scheme and host compare
// case-insensitively and a trailing "/" is ignored. It fails with
// [ErrEndpointForm], [ErrEndpointScheme], [ErrEndpointHost] or
// [execution.ErrEgressRefused], none of which contains the URL. The egress
// client checks the addresses the endpoint resolves to when connecting.
func CheckEndpoint(raw, native string, policy EgressPolicy) (string, error) {
	parsed, err := ValidateEndpoint(raw)
	if err != nil {
		return "", ErrEndpointForm
	}
	if isNative(parsed, native) {
		return "", nil
	}
	if !strings.EqualFold(parsed.Scheme, httpsScheme) {
		return "", ErrEndpointScheme
	}
	if err := policy.CheckHost(parsed.Hostname()); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func isNative(endpoint *url.URL, native string) bool {
	if native == "" {
		return false
	}
	want, err := url.Parse(native)
	if err != nil {
		return false
	}
	return strings.EqualFold(endpoint.Scheme, want.Scheme) && strings.EqualFold(endpoint.Host, want.Host) &&
		strings.TrimSuffix(endpoint.EscapedPath(), "/") == strings.TrimSuffix(want.EscapedPath(), "/")
}

// ModelClients are the two clients a customer-configured model may use: the
// shared model client for native endpoints and the egress client for custom
// ones.
type ModelClients struct {
	Shared *http.Client
	Egress *http.Client
}

// For returns the client for a model whose checked endpoint is baseURL, and
// whether that endpoint is custom: a custom endpoint gets the egress client,
// and its model must be wrapped with [EgressModel].
func (clients ModelClients) For(baseURL string) (*http.Client, bool) {
	if baseURL == "" {
		return clients.Shared, false
	}
	return clients.Egress, true
}

// EgressModel reports an egress refusal, connection failure or timeout of
// model as the bare [execution.ErrEgressRefused],
// [execution.ErrEgressConnection] or [execution.ErrEgressTimeout], without the
// request URL that HTTP client and provider errors wrap around it.
func EgressModel(model provider.LanguageModel) provider.LanguageModel {
	return egressModel{LanguageModel: model}
}

type egressModel struct {
	provider.LanguageModel
}

func (model egressModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	result, err := model.LanguageModel.DoGenerate(ctx, options)
	return result, bareEgressError(err)
}

func (model egressModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	result, err := model.LanguageModel.DoStream(ctx, options)
	return result, bareEgressError(err)
}

func bareEgressError(err error) error {
	for _, sentinel := range []error{execution.ErrEgressRefused, execution.ErrEgressTimeout, execution.ErrEgressConnection} {
		if errors.Is(err, sentinel) {
			return sentinel
		}
	}
	return err
}
