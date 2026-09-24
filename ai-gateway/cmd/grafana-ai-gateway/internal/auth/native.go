package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type nativeAuthenticator struct {
	delegate RequestAuthenticator
	source   Source
}

// NativeAuthenticator accepts SDK Bearer credentials in access-token mode. It does
// not change the existing ProviderWire header contract or cloud trust boundary.
func NativeAuthenticator(delegate RequestAuthenticator, source Source) RequestAuthenticator {
	return nativeAuthenticator{delegate, source}
}

func (a nativeAuthenticator) Authenticate(ctx context.Context, headers http.Header) (Caller, error) {
	if a.source == SourceCloudGateway {
		return a.delegate.Authenticate(ctx, headers)
	}
	for key := range headers {
		if strings.EqualFold(key, "X-Access-Token") || strings.EqualFold(key, "X-Grafana-Id") || strings.HasPrefix(strings.ToLower(key), "x-scope-") {
			return Caller{}, errors.New("gateway auth: conflicting native credentials")
		}
	}
	value, err := exactlyOneHeader(headers, "Authorization", true)
	if err != nil {
		return Caller{}, err
	}
	scheme, token, ok := strings.Cut(value, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t\r\n,") {
		return Caller{}, errors.New("gateway auth: invalid native bearer")
	}
	normalized := make(http.Header)
	normalized.Set("X-Access-Token", token)
	return a.delegate.Authenticate(ctx, normalized)
}
