// Package foundry configures explicit endpoint and authentication settings for
// Claude on Microsoft Foundry. Its request options work with anthropic-sdk-go
// directly as well as the ai-sdk Anthropic provider.
package foundry

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// Credential contains exactly one API key or Microsoft Entra bearer token.
type Credential struct {
	APIKey    string
	AuthToken string
}

// Config supplies explicit Foundry settings without reading environment defaults.
// Set exactly one of APIKey, AuthToken, or Credential.
type Config struct {
	BaseURL   string
	APIKey    string
	AuthToken string
	// Credential is called for every HTTP attempt, including retries. It must be
	// safe for concurrent use. Token caching and refresh belong to the callback.
	// Callback errors and invalid credentials fail without an SDK retry.
	Credential func(context.Context) (Credential, error)
}

// NormalizeBaseURL accepts a resource host or its /anthropic service root.
// HTTPS is required except for HTTP loopback endpoints used by local tests.
func NormalizeBaseURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", fmt.Errorf("foundry: invalid base URL")
	}
	loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	secure := u.Scheme == "https" || (u.Scheme == "http" && loopback)
	if u.Host == "" || u.User != nil || !secure {
		return "", fmt.Errorf("foundry: base URL must use HTTPS (HTTP allowed only for loopback tests)")
	}
	if u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf("foundry: base URL must not contain query parameters or a fragment")
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path != "" && path != "/anthropic" {
		return "", fmt.Errorf("foundry: base URL must be a resource host or /anthropic service root")
	}
	u.Path, u.RawPath = "/anthropic/", ""
	return u.String(), nil
}

// Validate checks the endpoint and credential configuration without invoking the
// credential callback or making a network request.
func (c Config) Validate() error {
	if _, err := NormalizeBaseURL(c.BaseURL); err != nil {
		return err
	}
	count := 0
	if strings.TrimSpace(c.APIKey) != "" {
		count++
	}
	if strings.TrimSpace(c.AuthToken) != "" {
		count++
	}
	if c.Credential != nil {
		count++
	}
	if count != 1 {
		return fmt.Errorf("foundry: requires exactly one API key, auth token, or credential callback")
	}
	return nil
}

// RequestOptions returns reusable anthropic-sdk-go options. The endpoint and credentials
// override SDK environment defaults, redirects are rejected by the default HTTP client, and each
// attempt uses only the configured credential. Callers can append transport
// options to override the HTTP client or add application policy.
func (c Config) RequestOptions() ([]option.RequestOption, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := NormalizeBaseURL(c.BaseURL)
	if err != nil {
		return nil, err
	}
	apiKey, authToken := strings.TrimSpace(c.APIKey), strings.TrimSpace(c.AuthToken)
	if c.Credential != nil {
		// Older Anthropic adapters resolve ambient auth before request middleware.
		// An explicit non-secret value preempts that chain; middleware replaces it
		// with the callback credential or aborts before any HTTP request is sent.
		apiKey = "foundry-credential-callback"
	}
	return []option.RequestOption{
		option.WithoutEnvironmentDefaults(),
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
		option.WithAuthToken(authToken),
		option.WithHTTPClient(&http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}),
		option.WithMiddleware(func(r *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			credential := Credential{APIKey: c.APIKey, AuthToken: c.AuthToken}
			if c.Credential != nil {
				var err error
				credential, err = c.Credential(r.Context())
				if err != nil {
					return credentialFailure(r, fmt.Errorf("foundry: resolving credential: %w", err))
				}
			}
			credential.APIKey = strings.TrimSpace(credential.APIKey)
			credential.AuthToken = strings.TrimSpace(credential.AuthToken)
			if (credential.APIKey == "") == (credential.AuthToken == "") {
				return credentialFailure(r, fmt.Errorf("foundry: credential must contain exactly one API key or auth token"))
			}
			r.Header.Del("Authorization")
			r.Header.Del("x-api-key")
			r.Header.Del("api-key")
			if credential.AuthToken != "" {
				r.Header.Set("Authorization", "Bearer "+credential.AuthToken)
			} else {
				r.Header.Set("x-api-key", credential.APIKey)
			}
			return next(r)
		}),
	}, nil
}

func credentialFailure(r *http.Request, err error) (*http.Response, error) {
	// The native SDK retries transport errors without a response. A local
	// non-retryable response prevents that retry while preserving the cause.
	return &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"X-Should-Retry": {"false"}},
		Body:       http.NoBody,
		Request:    r,
	}, err
}

// LogValue omits credentials when Config is logged with slog.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Bool("has_base_url", c.BaseURL != ""),
		slog.Bool("has_api_key", c.APIKey != ""),
		slog.Bool("has_auth_token", c.AuthToken != ""),
		slog.Bool("has_credential_callback", c.Credential != nil),
	)
}

// LogValue omits credential values when Credential is logged with slog.
func (c Credential) LogValue() slog.Value {
	return slog.GroupValue(slog.Bool("has_api_key", c.APIKey != ""), slog.Bool("has_auth_token", c.AuthToken != ""))
}
