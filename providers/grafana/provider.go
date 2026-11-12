package grafana

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/authlib/authn"
	"golang.org/x/sync/singleflight"
)

const maxBufferedResponseBytes int64 = 2 << 30

// TokenExchangeConfig configures CAP exchange for an internally provisioned service.
//
// BaseURL is the Gateway API root, such as
// https://grafana.example/apis/aigateway.ext.grafana.app/v1alpha1, and the
// client appends /namespaces/<namespace> for each call. This differs from the
// other constructors, which take the namespaced URL shown on the Gateway's
// overview page; a BaseURL with a namespaces path segment is refused.
//
// Namespace is optional. A stack namespace, such as stacks-123, is the
// namespace of every call that does not select one with WithNamespace, and a
// call that selects another fails. Leave it empty to serve several stacks, so
// that every call selects its namespace. The wildcard * exchanges one token for
// every namespace, and every call still selects the namespace it routes to.
// Exchanged tokens are cached per namespace and subject.
//
// Audience is required. In Grafana Cloud it is the Gateway's API group,
// aigateway.ext.grafana.app, which the caller's access policy must allow; in
// self-hosted Grafana it is grafana.
//
// HTTPClient is used for token exchanges and Gateway requests alike, so a
// transport that trusts a private certificate authority covers both.
type TokenExchangeConfig struct {
	CAPToken         string
	TokenExchangeURL string
	Namespace        string
	Audience         string
	BaseURL          string
	HTTPClient       *http.Client
	Headers          http.Header
}

// ServiceAccountConfig configures a Grafana service account token.
type ServiceAccountConfig struct {
	Token      string
	BaseURL    string
	HTTPClient *http.Client
	Headers    http.Header
}

// AccessTokenConfig configures a caller-managed access token.
type AccessTokenConfig struct {
	AccessToken string
	BaseURL     string
	HTTPClient  *http.Client
	Headers     http.Header
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
	baseURL           string
	client            *http.Client
	headers           http.Header
	bufferedReadLimit int64
	exchanger         authn.TokenExchanger
	tokenExchange     bool
	namespace         string
	audience          string
	bearer            string
	exchanges         singleflight.Group
}

const wildcardNamespace = "*"

var namespacePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validNamespace(namespace string) bool { return namespacePattern.MatchString(namespace) }

// NewWithServiceAccountToken constructs a provider that sends a Grafana
// service account token as its only credential, an Authorization bearer.
func NewWithServiceAccountToken(cfg ServiceAccountConfig, opts ...Option) (*Provider, error) {
	if !validCredential(cfg.Token) {
		return nil, errors.New("grafana: invalid service account token")
	}
	p, err := newProvider(cfg.BaseURL, cfg.HTTPClient, cfg.Headers, opts)
	if err != nil {
		return nil, err
	}
	p.bearer = "Bearer " + cfg.Token
	return p, nil
}

func reservedAuthenticationHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "x-access-token", "x-grafana-id":
		return true
	}
	return false
}

func validateCallAuthenticationHeaders(headers map[string]string) error {
	for name := range headers {
		if reservedAuthenticationHeader(name) {
			return errors.New("grafana: reserved authentication header")
		}
	}
	return nil
}

// NewWithAccessToken constructs a provider without exchanging or refreshing tokens.
func NewWithAccessToken(cfg AccessTokenConfig, opts ...Option) (*Provider, error) {
	if !validCredential(cfg.AccessToken) {
		return nil, errors.New("grafana: invalid access token")
	}
	p, err := newProvider(cfg.BaseURL, cfg.HTTPClient, cfg.Headers, opts)
	if err != nil {
		return nil, err
	}
	p.exchanger = authn.NewStaticTokenExchanger(cfg.AccessToken)
	return p, nil
}

// NewWithTokenExchange constructs an authlib token exchanger for an explicit audience.
func NewWithTokenExchange(cfg TokenExchangeConfig, opts ...Option) (*Provider, error) {
	if !validCredential(cfg.CAPToken) {
		return nil, errors.New("grafana: invalid CAP token")
	}
	if cfg.Namespace != "" && cfg.Namespace != wildcardNamespace && !validNamespace(cfg.Namespace) {
		return nil, errors.New("grafana: invalid namespace")
	}
	if !validCredential(cfg.Audience) {
		return nil, errors.New("grafana: invalid audience")
	}
	_, err := normalizeURL(cfg.TokenExchangeURL)
	if err != nil {
		return nil, errors.New("grafana: invalid token exchange URL")
	}
	if namespacedURL(cfg.BaseURL) {
		return nil, errors.New("grafana: token exchange base URL must be the Gateway API root, without /namespaces/<namespace>")
	}
	p, err := newProvider(cfg.BaseURL, cfg.HTTPClient, cfg.Headers, opts)
	if err != nil {
		return nil, err
	}
	p.exchanger, err = authn.NewTokenExchangeClient(authn.TokenExchangeConfig{Token: cfg.CAPToken, TokenExchangeURL: cfg.TokenExchangeURL}, authn.WithHTTPClient(p.client))
	if err != nil {
		return nil, errors.New("grafana: cannot construct token exchanger")
	}
	p.tokenExchange, p.namespace, p.audience = true, cfg.Namespace, cfg.Audience
	return p, nil
}

func namespacedURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for segment := range strings.SplitSeq(u.Path, "/") {
		if segment == "namespaces" {
			return true
		}
	}
	return false
}

func newProvider(rawURL string, client *http.Client, headers http.Header, opts []Option) (*Provider, error) {
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
	canonical := make(http.Header, len(headers))
	for key, values := range headers {
		if reservedAuthenticationHeader(key) {
			return nil, errors.New("grafana: reserved authentication header")
		}
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
	return &Provider{baseURL: baseURL, client: &ownedClient, headers: canonical, bufferedReadLimit: maxBufferedResponseBytes}, nil
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

const maxModelIDBytes = 2048

// tokenExchangeTimeout bounds a shared token exchange, including authlib's
// retries, independently of the callers waiting for it.
const tokenExchangeTimeout = 30 * time.Second

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

type onBehalfOfKey struct{}

// WithOnBehalfOf makes token-exchange calls in ctx act on behalf of the
// identity in subjectToken, a Grafana user ID token or a delegated access
// token. The exchange uses it as its subject, so the Gateway receives the
// caller and that identity in one access token. Other flows, and exchanges
// for every namespace, refuse such calls before any network request.
func WithOnBehalfOf(ctx context.Context, subjectToken string) context.Context {
	return context.WithValue(ctx, onBehalfOfKey{}, subjectToken)
}

type namespaceKey struct{}

// WithNamespace selects the stack namespace, such as stacks-123, of
// token-exchange calls in ctx: the call is routed to that namespace and its
// token is exchanged for it, unless the provider exchanges for every
// namespace. A call fails before any network request when its namespace is not
// a DNS label, differs from the provider's configured namespace, or reaches a
// service account or access token provider, whose credential fixes the stack.
func WithNamespace(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKey{}, namespace)
}

// resolveNamespaces returns the route and exchange namespaces of a call. Flows
// other than token exchange route to their configured URL and return none.
func (p *Provider) resolveNamespaces(ctx context.Context) (route, exchange string, err error) {
	call, selected := ctx.Value(namespaceKey{}).(string)
	switch {
	case !selected && p.tokenExchange && (p.namespace == "" || p.namespace == wildcardNamespace):
		return "", "", errors.New("grafana: call namespace required")
	case !selected:
		return p.namespace, p.namespace, nil
	case !p.tokenExchange:
		return "", "", errors.New("grafana: call namespace requires token exchange")
	case !validNamespace(call):
		return "", "", errors.New("grafana: invalid call namespace")
	case p.namespace == "":
		return call, call, nil
	case p.namespace == wildcardNamespace:
		return call, wildcardNamespace, nil
	case call != p.namespace:
		return "", "", errors.New("grafana: call namespace conflicts with the configured namespace")
	}
	return call, call, nil
}

func (p *Provider) request(ctx context.Context, method, route string, callHeaders ...map[string]string) (*http.Request, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, entries := range callHeaders {
		if err := validateCallAuthenticationHeaders(entries); err != nil {
			return nil, err
		}
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
	routeNamespace, exchangeNamespace, err := p.resolveNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	subject, onBehalfOf := ctx.Value(onBehalfOfKey{}).(string)
	if onBehalfOf {
		switch {
		case !validCredential(subject):
			return nil, errors.New("grafana: invalid on-behalf-of token")
		case !p.tokenExchange:
			return nil, errors.New("grafana: on-behalf-of calls require token exchange")
		case exchangeNamespace == wildcardNamespace:
			return nil, errors.New("grafana: on-behalf-of calls require a concrete namespace")
		}
	}
	credentialHeader, credential := "Authorization", p.bearer
	if p.bearer == "" {
		token, err := p.exchange(ctx, exchangeNamespace, subject)
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
		credentialHeader, credential = "X-Access-Token", token.Token
	}
	prefix := p.baseURL
	if p.tokenExchange {
		prefix += "/namespaces/" + routeNamespace
	}
	req, err := http.NewRequestWithContext(ctx, method, prefix+route, nil)
	if err != nil {
		return nil, errors.New("grafana: cannot construct Gateway request")
	}
	req.Header = headers
	req.Header.Set(credentialHeader, credential)
	return req, nil
}

// exchange returns an access token for subject in namespace. Concurrent calls
// for one namespace and subject share a single exchange that no caller can
// cancel, bounded by tokenExchangeTimeout, while each caller still returns
// when its own ctx ends. An exchange that outlives its callers still fills the
// token cache.
func (p *Provider) exchange(ctx context.Context, namespace, subject string) (*authn.TokenExchangeResponse, error) {
	result := p.exchanges.DoChan(namespace+"\x00"+subject, func() (any, error) {
		exchangeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tokenExchangeTimeout)
		defer cancel()
		return p.exchanger.Exchange(exchangeCtx, authn.TokenExchangeRequest{Namespace: namespace, Audiences: []string{p.audience}, SubjectToken: subject})
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case shared := <-result:
		if shared.Err != nil {
			return nil, shared.Err
		}
		return shared.Val.(*authn.TokenExchangeResponse), nil
	}
}

// LanguageModel returns a model retaining the requested identifier.
func (p *Provider) LanguageModel(id string) (provider.LanguageModel, error) {
	if len(id) > maxModelIDBytes || !validCredential(id) {
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
