package auth

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/grafana/authlib/types"
)

type cloudGatewayAuthenticator struct{}

// NewCloudGatewayAuthenticator returns an authenticator that validates identity assertions from a trusted proxy.
func NewCloudGatewayAuthenticator() RequestAuthenticator { return cloudGatewayAuthenticator{} }

func (cloudGatewayAuthenticator) Authenticate(_ context.Context, headers http.Header) (Caller, error) {
	stack, err := cloudAssertion(headers, "X-Scope-OrgID")
	if err != nil {
		return Caller{}, err
	}
	stackID, err := positiveDecimal(stack)
	if err != nil {
		return Caller{}, err
	}
	return Caller{Source: SourceCloudGateway, Namespace: types.CloudNamespaceFormatter(stackID), stackID: stackID}, nil
}

type cloudProviderWireAuthenticator struct{}

// NewCloudProviderWireAuthenticator returns an authenticator that rejects credentials the proxy must remove.
func NewCloudProviderWireAuthenticator() RequestAuthenticator {
	return cloudProviderWireAuthenticator{}
}

func (cloudProviderWireAuthenticator) Authenticate(ctx context.Context, headers http.Header) (Caller, error) {
	for key := range headers {
		if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "X-Access-Token") || strings.EqualFold(key, "X-Grafana-Id") {
			return Caller{}, fmt.Errorf("gateway auth: request contains a credential header that the trusted proxy must remove")
		}
	}
	return cloudGatewayAuthenticator{}.Authenticate(ctx, headers)
}

func cloudAssertion(headers http.Header, name string) (string, error) {
	value, err := exactlyOneHeader(headers, name, true)
	if err != nil {
		return "", err
	}
	if !utf8.ValidString(value) || strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", fmt.Errorf("gateway auth: invalid %s header", name)
	}
	return value, nil
}

func positiveDecimal(value string) (int64, error) {
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("gateway auth: stack ID must be a positive decimal integer")
		}
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("gateway auth: stack ID must be a positive decimal integer")
	}
	return number, nil
}
