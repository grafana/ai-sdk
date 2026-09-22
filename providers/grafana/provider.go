package grafana

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/authn"
)

// Limits bounds response processing. A nil Limits configuration selects defaults;
// an explicit configuration must set every field to a positive safe value.
type Limits struct {
	DiscoveryBytes int64
	UnaryBytes     int64
	ErrorBytes     int64
	StreamBytes    int64
	// StreamEventBytes bounds a complete event after CRLF normalization, including fields and delimiters.
	StreamEventBytes int64
	StreamEvents     int64
}

// DefaultLimits returns a fresh copy of the client processing limits.
func DefaultLimits() Limits {
	return Limits{DiscoveryBytes: 4 << 20, UnaryBytes: 16 << 20, ErrorBytes: 64 << 10, StreamBytes: 64 << 20, StreamEventBytes: 1 << 20, StreamEvents: 100000}
}

// CloudAuthConfig configures CAP exchange for an internally provisioned service.
type CloudAuthConfig struct {
	CAPToken         string
	TokenExchangeURL string
	Namespace        string
	Audience         string
	BaseURL          string
	HTTPClient       *http.Client
	Headers          http.Header
	Limits           *Limits
}

// AccessTokenConfig configures a caller-managed, short-lived access token.
type AccessTokenConfig struct {
	AccessToken string
	BaseURL     string
	HTTPClient  *http.Client
	Headers     http.Header
	Limits      *Limits
}

// Option configures provider construction.
type Option func(*providerOptions)
type providerOptions struct{ client *http.Client }

// WithHTTPClient supplies the transport unless the typed configuration has one.
func WithHTTPClient(client *http.Client) Option {
	return func(options *providerOptions) { options.client = client }
}

// Provider creates models and discovers the Gateway's public catalog.
type Provider struct {
	baseURL      string
	client       *http.Client
	headers      http.Header
	limits       Limits
	exchanger    authn.TokenExchanger
	namespace    string
	audience     string
	exchangeGate chan struct{}
}

// NewWithAccessToken constructs a provider without exchanging or refreshing tokens.
func NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error) {
	if !validCredential(cfg.AccessToken) {
		return nil, errors.New("grafana: invalid access token")
	}
	p, err := newProvider(cfg.BaseURL, cfg.HTTPClient, cfg.Headers, cfg.Limits, opts)
	if err != nil {
		return nil, err
	}
	p.exchanger = authn.NewStaticTokenExchanger(cfg.AccessToken)
	return p, nil
}

// NewWithCloudAuth constructs an authlib token exchanger; omitted audience defaults to ai-sdk.
func NewWithCloudAuth(cfg CloudAuthConfig, opts ...Option) (*Provider, error) {
	if !validCredential(cfg.CAPToken) {
		return nil, errors.New("grafana: invalid CAP token")
	}
	if !validCredential(cfg.Namespace) {
		return nil, errors.New("grafana: invalid namespace")
	}
	if cfg.Audience == "" {
		cfg.Audience = "ai-sdk"
	}
	if !validCredential(cfg.Audience) {
		return nil, errors.New("grafana: invalid audience")
	}
	_, err := normalizeURL(cfg.TokenExchangeURL)
	if err != nil {
		return nil, errors.New("grafana: invalid token exchange URL")
	}
	p, err := newProvider(cfg.BaseURL, cfg.HTTPClient, cfg.Headers, cfg.Limits, opts)
	if err != nil {
		return nil, err
	}
	p.exchanger, err = authn.NewTokenExchangeClient(authn.TokenExchangeConfig{Token: cfg.CAPToken, TokenExchangeURL: cfg.TokenExchangeURL}, authn.WithHTTPClient(p.client))
	if err != nil {
		return nil, errors.New("grafana: cannot construct token exchanger")
	}
	p.namespace, p.audience = cfg.Namespace, cfg.Audience
	return p, nil
}

func newProvider(rawURL string, client *http.Client, headers http.Header, limits *Limits, opts []Option) (*Provider, error) {
	baseURL, err := normalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	options := providerOptions{}
	for _, option := range opts {
		if option == nil {
			return nil, errors.New("grafana: nil option")
		}
		option(&options)
	}
	if client == nil {
		client = options.client
	}
	if client == nil {
		client = http.DefaultClient
	}
	// A private client value keeps caller configuration immutable and prevents
	// redirects from forwarding Grafana credentials to another endpoint.
	ownedClient := *client
	ownedClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	selectedLimits := DefaultLimits()
	if limits != nil {
		selectedLimits = *limits
	}
	for _, limit := range []int64{selectedLimits.DiscoveryBytes, selectedLimits.UnaryBytes, selectedLimits.ErrorBytes, selectedLimits.StreamBytes, selectedLimits.StreamEventBytes, selectedLimits.StreamEvents} {
		if limit <= 0 || limit >= int64(math.MaxInt) {
			return nil, errors.New("grafana: invalid client limit")
		}
	}
	if selectedLimits.StreamEventBytes > selectedLimits.StreamBytes {
		return nil, errors.New("grafana: event byte limit exceeds stream limit")
	}
	canonical := make(http.Header, len(headers))
	for key, values := range headers {
		if !validHeaderName(key) {
			return nil, errors.New("grafana: invalid configured header name")
		}
		name := http.CanonicalHeaderKey(key)
		if _, duplicate := canonical[name]; duplicate {
			return nil, errors.New("grafana: duplicate configured header name")
		}
		canonical[name] = append([]string(nil), values...)
		for _, value := range values {
			if !validHeaderValue(value) {
				return nil, errors.New("grafana: invalid configured header value")
			}
		}
	}
	return &Provider{baseURL: baseURL, client: &ownedClient, headers: canonical, limits: selectedLimits, exchangeGate: make(chan struct{}, 1)}, nil
}

func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || !utf8.ValidString(raw) || strings.TrimSpace(raw) != raw || u == nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.Opaque != "" {
		return "", errors.New("grafana: invalid API-prefix URL")
	}
	if strings.ContainsAny(u.Path, "\r\n\x00") {
		return "", errors.New("grafana: invalid API-prefix URL")
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func validCredential(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) == -1
}

var publicModelID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,127}$`)

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for i := range len(value) {
		c := value[i]
		allowed := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c))
		if !allowed {
			return false
		}
	}
	return true
}

func validHeaderValue(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for i := range len(value) {
		if value[i] == 127 || value[i] < 32 && value[i] != '\t' {
			return false
		}
	}
	return true
}

type userIDTokenKey struct{}

// WithUserIDToken attaches an acting-user token to Gateway requests in ctx.
func WithUserIDToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, userIDTokenKey{}, token)
}

func (p *Provider) request(ctx context.Context, method, route string, callHeaders ...map[string]string) (*http.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	headers := p.headers.Clone()
	for _, entries := range callHeaders {
		seen := make(map[string]bool, len(entries))
		for name, value := range entries {
			canonical := http.CanonicalHeaderKey(name)
			if !validHeaderName(name) || !validHeaderValue(value) || seen[canonical] {
				return nil, errors.New("grafana: invalid call headers")
			}
			seen[canonical] = true
			headers.Set(canonical, value)
		}
	}
	user, _ := ctx.Value(userIDTokenKey{}).(string)
	if user != "" && !validCredential(user) {
		return nil, errors.New("grafana: invalid acting-user token")
	}
	select {
	case p.exchangeGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	token, err := p.exchanger.Exchange(ctx, authn.TokenExchangeRequest{Namespace: p.namespace, Audiences: []string{p.audience}})
	<-p.exchangeGate
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Authlib may include token-service response prose in its error. Do not
		// retain that arbitrary text in an inspectable error chain.
		return nil, protocolError("grafana: token exchange failed", 0, nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if token == nil || !validCredential(token.Token) {
		return nil, errors.New("grafana: invalid exchanged access token")
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+route, nil)
	if err != nil {
		return nil, errors.New("grafana: cannot construct Gateway request")
	}
	req.Header = headers
	req.Header.Set("X-Access-Token", token.Token)
	req.Header.Del("X-Grafana-Id")
	if user != "" {
		req.Header.Set("X-Grafana-Id", user)
	}
	return req, nil
}

// LanguageModel returns a model retaining the requested public identifier.
func (p *Provider) LanguageModel(id string) (provider.LanguageModel, error) {
	if !publicModelID.MatchString(id) {
		return nil, errors.New("grafana: invalid model ID")
	}
	return &model{provider: p, id: id}, nil
}

type model struct {
	provider *Provider
	id       string
}

func (m *model) SpecificationVersion() string               { return "v4" }
func (m *model) Provider() string                           { return "grafana" }
func (m *model) ModelID() string                            { return m.id }
func (m *model) SupportedURLs() map[string][]*regexp.Regexp { return nil }
