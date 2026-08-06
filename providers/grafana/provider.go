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

	// DefaultMaxUnaryResponseBytes is the default complete unary success read limit.
	DefaultMaxUnaryResponseBytes int64 = 16 << 20
	// DefaultMaxErrorResponseBytes is the default non-success and diagnostic read limit.
	DefaultMaxErrorResponseBytes int64 = 1 << 20
	// DefaultMaxSSEEventBytes is the default complete framed SSE event read limit.
	DefaultMaxSSEEventBytes int64 = 8 << 20
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

// ProviderWireMode selects the provider-wire codec used by remote models.
type ProviderWireMode string

const (
	// ProviderWireLegacy preserves the deployed tolerant provider-wire codec.
	ProviderWireLegacy ProviderWireMode = "legacy"
	// ProviderWireStrict selects the canonical strict LanguageModelV4 codec.
	ProviderWireStrict ProviderWireMode = "strict"
)

// Option configures a Grafana provider.
type Option func(*providerOptions)

type providerOptions struct {
	httpClient            *http.Client
	providerWireMode      ProviderWireMode
	maxUnaryResponseBytes *int64
	maxErrorResponseBytes *int64
	maxSSEEventBytes      *int64
}

// WithHTTPClient sets the HTTP client used for provider-wire and token exchange
// requests when the constructor config does not set HTTPClient.
func WithHTTPClient(client *http.Client) Option {
	return func(opts *providerOptions) { opts.httpClient = client }
}

// WithProviderWireMode selects the legacy or strict provider-wire codec.
func WithProviderWireMode(mode ProviderWireMode) Option {
	return func(opts *providerOptions) { opts.providerWireMode = mode }
}

// WithStrictProviderWire selects the canonical strict LanguageModelV4 codec.
func WithStrictProviderWire() Option { return WithProviderWireMode(ProviderWireStrict) }

// WithMaxUnaryResponseBytes sets the complete unary success read limit.
func WithMaxUnaryResponseBytes(limit int64) Option {
	return func(opts *providerOptions) { opts.maxUnaryResponseBytes = &limit }
}

// WithMaxErrorResponseBytes sets the non-success and diagnostic read limit.
func WithMaxErrorResponseBytes(limit int64) Option {
	return func(opts *providerOptions) { opts.maxErrorResponseBytes = &limit }
}

// WithMaxSSEEventBytes sets the complete framed SSE event read limit.
func WithMaxSSEEventBytes(limit int64) Option {
	return func(opts *providerOptions) { opts.maxSSEEventBytes = &limit }
}

// Provider creates Grafana hosted language models and satisfies
// registry.Provider.
type Provider struct {
	baseURL               string
	namespace             string
	audience              string
	httpClient            *http.Client
	tokenExchanger        authn.TokenExchanger
	wireCodec             wireCodec
	maxUnaryResponseBytes int64
	maxErrorResponseBytes int64
	maxSSEEventBytes      int64
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

	options, err := collectProviderOptions(opts)
	if err != nil {
		return nil, err
	}
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
		baseURL:               baseURL,
		namespace:             cfg.Namespace,
		audience:              audience,
		httpClient:            httpClient,
		tokenExchanger:        client,
		wireCodec:             codecForMode(options.providerWireMode),
		maxUnaryResponseBytes: resolvedLimit(options.maxUnaryResponseBytes, DefaultMaxUnaryResponseBytes),
		maxErrorResponseBytes: resolvedLimit(options.maxErrorResponseBytes, DefaultMaxErrorResponseBytes),
		maxSSEEventBytes:      resolvedLimit(options.maxSSEEventBytes, DefaultMaxSSEEventBytes),
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

	options, err := collectProviderOptions(opts)
	if err != nil {
		return nil, err
	}
	httpClient := configuredHTTPClient(cfg.HTTPClient, options.httpClient)

	return &Provider{
		baseURL:               baseURL,
		httpClient:            httpClient,
		tokenExchanger:        authn.NewStaticTokenExchanger(cfg.AccessToken),
		wireCodec:             codecForMode(options.providerWireMode),
		maxUnaryResponseBytes: resolvedLimit(options.maxUnaryResponseBytes, DefaultMaxUnaryResponseBytes),
		maxErrorResponseBytes: resolvedLimit(options.maxErrorResponseBytes, DefaultMaxErrorResponseBytes),
		maxSSEEventBytes:      resolvedLimit(options.maxSSEEventBytes, DefaultMaxSSEEventBytes),
	}, nil
}

// LanguageModel returns a Grafana hosted language model for modelID.
func (p *Provider) LanguageModel(modelID string) (provider.LanguageModel, error) {
	return &model{provider: p, modelID: modelID}, nil
}

func collectProviderOptions(opts []Option) (providerOptions, error) {
	options := providerOptions{providerWireMode: ProviderWireLegacy}
	for _, opt := range opts {
		if opt == nil {
			return providerOptions{}, fmt.Errorf("grafana: nil option")
		}
		opt(&options)
	}
	if options.providerWireMode != ProviderWireLegacy && options.providerWireMode != ProviderWireStrict {
		return providerOptions{}, fmt.Errorf("grafana: unsupported provider-wire mode %q", options.providerWireMode)
	}
	for name, limit := range map[string]*int64{
		"unary response": options.maxUnaryResponseBytes,
		"error response": options.maxErrorResponseBytes,
		"SSE event":      options.maxSSEEventBytes,
	} {
		if limit != nil && *limit <= 0 {
			return providerOptions{}, fmt.Errorf("grafana: maximum %s bytes must be positive", name)
		}
	}
	return options, nil
}

func resolvedLimit(configured *int64, defaultValue int64) int64 {
	if configured == nil {
		return defaultValue
	}
	return *configured
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
