package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ RequestAuthenticator = cloudGatewayAuthenticator{}
	_ RequestAuthenticator = cloudProviderWireAuthenticator{}
)

func TestCloudGatewayAuthenticator_Assertions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(http.Header)
		valid  bool
	}{
		{name: "stack only", valid: true},
		{name: "parser leaves credentials to adapter", mutate: func(h http.Header) { h.Set("Authorization", "native-provider-credential") }, valid: true},
		{name: "leading zero digits", mutate: func(h http.Header) { h.Set("X-Scope-OrgID", "00123") }, valid: true},
		{name: "ignored policy headers", mutate: func(h http.Header) {
			h[http.CanonicalHeaderKey("X-Cloud-Org-ID")] = []string{"malformed", "duplicated"}
			h.Set("X-Access-Policy-ID", "")
		}, valid: true},
		{name: "maximum integer", mutate: func(h http.Header) { h.Set("X-Scope-OrgID", "9223372036854775807") }, valid: true},
		{name: "lowercase keys", mutate: func(h http.Header) {
			for key, values := range h {
				delete(h, key)
				h[strings.ToLower(key)] = values
			}
		}, valid: true},
	}
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{name: "missing"}, {name: "no values", values: []string{}}, {name: "empty", values: []string{""}},
		{name: "duplicate", values: []string{"123", "123"}},
		{name: "coalesced", values: []string{"123,456"}},
		{name: "leading space", values: []string{" 123"}},
		{name: "trailing space", values: []string{"123 "}},
		{name: "embedded space", values: []string{"12 3"}},
		{name: "tab", values: []string{"12\t3"}},
		{name: "newline", values: []string{"12\n3"}},
		{name: "control", values: []string{"12\x003"}},
		{name: "delete", values: []string{"12\x7f3"}},
		{name: "unicode space", values: []string{"12\u00a03"}},
		{name: "invalid UTF8", values: []string{"12\xff3"}},
	} {
		tests = append(tests, struct {
			name   string
			mutate func(http.Header)
			valid  bool
		}{
			name: "X-Scope-OrgID/" + tc.name, mutate: func(h http.Header) {
				if tc.values == nil {
					h.Del("X-Scope-OrgID")
					return
				}
				h[http.CanonicalHeaderKey("X-Scope-OrgID")] = tc.values
			},
		})
	}
	tests = append(tests, struct {
		name   string
		mutate func(http.Header)
		valid  bool
	}{
		name: "X-Scope-OrgID/case collision", mutate: func(h http.Header) { h["x-scope-orgid"] = []string{"123"} },
	})
	for _, value := range []string{"0", "-1", "+1", "1.0", "1e2", "abc", "9223372036854775808"} {
		tests = append(tests, struct {
			name   string
			mutate func(http.Header)
			valid  bool
		}{
			name: "X-Scope-OrgID/" + value, mutate: func(h http.Header) { h.Set("X-Scope-OrgID", value) },
		})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := cloudHeaders()
			if tc.mutate != nil {
				tc.mutate(headers)
			}
			caller, err := NewCloudGatewayAuthenticator().Authenticate(context.Background(), headers)
			if !tc.valid {
				require.Error(t, err)
				assert.Empty(t, caller)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, SourceCloudGateway, caller.Source)
			assert.Equal(t, "stacks-"+strconv.FormatInt(caller.stackID, 10), caller.Namespace)
			assert.Positive(t, caller.stackID)
			assert.Empty(t, caller.Service)
			assert.Nil(t, caller.ActingUser)
		})
	}
}

func TestCloudProviderWireAuthenticator_AuthenticationBeforeBody(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(http.Header)
		valid  bool
	}{
		{name: "trusted assertions", valid: true},
		{name: "missing stack", mutate: func(h http.Header) { h.Del("X-Scope-OrgID") }},
	}
	for _, header := range []string{"Authorization", "X-Access-Token", "X-Grafana-Id"} {
		for _, values := range [][]string{nil, {""}, {"private-credential"}, {"one", "two"}} {
			tests = append(tests, struct {
				name   string
				mutate func(http.Header)
				valid  bool
			}{
				name: header + "/" + strings.Join(values, ","), mutate: func(h http.Header) { h[strings.ToLower(header)] = values },
			})
		}
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headers := cloudHeaders()
			if tc.mutate != nil {
				tc.mutate(headers)
			}
			body := &unreadBody{}
			request := httptest.NewRequest(http.MethodPost, "/protected", body)
			request.Header = headers
			errors := providerv4.NewHostErrorWriter()
			calls := 0
			handler := Middleware(NewCloudProviderWireAuthenticator(), func(w http.ResponseWriter) { errors.Write(w, providerv4.HostErrorAuthentication) }, func(context.Context, Observation) {}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				caller, ok := CallerFromContext(r.Context())
				require.True(t, ok)
				assert.Equal(t, int64(123), caller.stackID)
				w.WriteHeader(http.StatusNoContent)
			}))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Zero(t, body.reads)
			if tc.valid {
				assert.Equal(t, 1, calls)
				assert.Equal(t, http.StatusNoContent, response.Code)
				return
			}
			assert.Zero(t, calls)
			assert.Equal(t, http.StatusUnauthorized, response.Code)
			assert.Equal(t, `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`, response.Body.String())
		})
	}
}

func cloudHeaders() http.Header {
	return http.Header{"X-Scope-Orgid": {"123"}}
}
