package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

type anthropicMessagesAuthenticator struct{ next RequestAuthenticator }

// NewAnthropicMessagesAuthenticator adapts Anthropic SDK credential headers to
// the configured Gateway authenticator. The credential is never retained in
// the authenticated caller or passed to provider execution.
func NewAnthropicMessagesAuthenticator(next RequestAuthenticator) RequestAuthenticator {
	return anthropicMessagesAuthenticator{next: next}
}

func (a anthropicMessagesAuthenticator) Authenticate(ctx context.Context, headers http.Header) (Caller, error) {
	apiKey, err := exactOptionalHeader(headers, "x-api-key")
	if err != nil {
		return Caller{}, err
	}
	authorization, err := exactOptionalHeader(headers, "Authorization")
	if err != nil {
		return Caller{}, err
	}
	accessToken, err := exactOptionalHeader(headers, "X-Access-Token")
	if err != nil {
		return Caller{}, err
	}
	count := 0
	for _, value := range []string{apiKey, authorization, accessToken} {
		if value != "" {
			count++
		}
	}
	if count > 1 {
		return Caller{}, fmt.Errorf("gateway auth: conflicting Anthropic credentials")
	}
	cloned := headers.Clone()
	if accessToken == "" && apiKey != "" {
		cloned.Set("X-Access-Token", apiKey)
		cloned.Del("x-api-key")
	}
	return a.next.Authenticate(ctx, cloned)
}

func exactOptionalHeader(headers http.Header, name string) (string, error) {
	var values []string
	matches := 0
	for key, candidates := range headers {
		if strings.EqualFold(key, name) {
			matches++
			values = append(values, candidates...)
		}
	}
	if matches == 0 {
		return "", nil
	}
	if matches != 1 || len(values) != 1 || values[0] == "" || strings.Contains(values[0], ",") {
		return "", fmt.Errorf("gateway auth: invalid %s header", name)
	}
	return values[0], nil
}
