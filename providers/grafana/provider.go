package grafana

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/registry"
	"github.com/grafana/authlib/authn"
)

const (
	defaultAudience   = "ai-sdk"
	providerName      = "grafana"
	accessTokenHeader = "X-Access-Token"
	userIDHeader      = "X-Grafana-Id"
)

// CloudAuthConfig configures internally provisioned Grafana Cloud
// authentication for the hosted provider-wire endpoint.
type CloudAuthConfig struct {
	CAPToken         string
	TokenExchangeURL string
	Namespace        string
	BaseURL          string
	Audience         string
	HTTPClient       *http.Client
}

// AccessTokenConfig configures authentication with a short-lived access token
// JWT (typ=jwt+at) minted by an internal Grafana control plane. The token
// already contains its namespace and audience claims; the provider forwards it
// unchanged and does not refresh it.
type AccessTokenConfig struct {
	// AccessToken is the pre-minted short-lived access token JWT to forward as
	// X-Access-Token. The caller is responsible for refreshing it before expiry.
	AccessToken string
	// BaseURL is the base URL of the Grafana hosted ai-sdk provider-wire
	// endpoint. Namespace and audience are JWT claims, not provider config.
	BaseURL string
	// HTTPClient overrides the HTTP client used for provider-wire requests.
	HTTPClient *http.Client
}

// Option configures a Grafana provider.
type Option func(*providerOptions)

type providerOptions struct {
	httpClient *http.Client
}

// WithHTTPClient sets the HTTP client used for provider-wire and token exchange
// requests when the constructor config does not set HTTPClient.
func WithHTTPClient(client *http.Client) Option {
	return func(opts *providerOptions) { opts.httpClient = client }
}

// Provider creates Grafana hosted language models and satisfies
// registry.Provider.
type Provider struct {
	baseURL        string
	namespace      string
	audience       string
	httpClient     *http.Client
	tokenExchanger authn.TokenExchanger
}

var _ registry.Provider = (*Provider)(nil)

// NewWithCloudAuth creates a Grafana provider for an internal service by
// exchanging an internally provisioned Cloud Access Policy token for
// short-lived access tokens.
func NewWithCloudAuth(cfg CloudAuthConfig, opts ...Option) (*Provider, error) {
	if strings.TrimSpace(cfg.CAPToken) == "" {
		return nil, fmt.Errorf("grafana: CAPToken is required")
	}
	if strings.TrimSpace(cfg.TokenExchangeURL) == "" {
		return nil, fmt.Errorf("grafana: TokenExchangeURL is required")
	}
	if strings.TrimSpace(cfg.Namespace) == "" {
		return nil, fmt.Errorf("grafana: Namespace is required")
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	options := collectProviderOptions(opts)
	httpClient := configuredHTTPClient(cfg.HTTPClient, options.httpClient)

	audience := cfg.Audience
	if audience == "" {
		audience = defaultAudience
	}

	client, err := authn.NewTokenExchangeClient(authn.TokenExchangeConfig{
		Token:            cfg.CAPToken,
		TokenExchangeURL: cfg.TokenExchangeURL,
	}, authn.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("grafana: creating token exchange client: %w", err)
	}

	return &Provider{
		baseURL:        baseURL,
		namespace:      cfg.Namespace,
		audience:       audience,
		httpClient:     httpClient,
		tokenExchanger: client,
	}, nil
}

// NewWithAccessToken creates a Grafana provider that forwards a short-lived
// access token minted by an internal control plane. The caller is responsible for
// refreshing the token before it expires.
func NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" {
		return nil, fmt.Errorf("grafana: AccessToken is required")
	}
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}

	options := collectProviderOptions(opts)
	httpClient := configuredHTTPClient(cfg.HTTPClient, options.httpClient)

	return &Provider{
		baseURL:        baseURL,
		httpClient:     httpClient,
		tokenExchanger: authn.NewStaticTokenExchanger(cfg.AccessToken),
	}, nil
}

// LanguageModel returns a Grafana hosted language model for modelID.
func (p *Provider) LanguageModel(modelID string) (provider.LanguageModel, error) {
	return &model{provider: p, modelID: modelID}, nil
}

func collectProviderOptions(opts []Option) providerOptions {
	options := providerOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	return options
}

func configuredHTTPClient(configClient, optionClient *http.Client) *http.Client {
	if configClient != nil {
		return configClient
	}
	if optionClient != nil {
		return optionClient
	}
	return http.DefaultClient
}

func normalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("grafana: BaseURL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("grafana: invalid BaseURL %q", raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

type userIDTokenKey struct{}

// WithUserIDToken returns a context that forwards idToken as X-Grafana-Id on
// provider-wire requests made with that context.
func WithUserIDToken(ctx context.Context, idToken string) context.Context {
	return context.WithValue(ctx, userIDTokenKey{}, idToken)
}

func userIDTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(userIDTokenKey{}).(string)
	return token
}
