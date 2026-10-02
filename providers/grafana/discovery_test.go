package grafana

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModels_AtomicValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, media string
		valid             bool
	}{
		{"valid", discoveryFixture, "application/json", true},
		{"nullable description", strings.Replace(discoveryFixture, `"description":"public description"`, `"description":null`, 1), "application/json", true},
		{"wrong property casing", strings.Replace(discoveryFixture, `"models"`, `"Models"`, 1), "application/json", false},
		{"escaped lone surrogate", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"name":"\ud800"`, 1), "application/json", false},
		{"empty", `{"models":[]}`, "application/json", true},
		{"additive", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"private":{"backend":"do-not-expose"},"name":"Assistant"`, 1), "application/json", true},
		{"missing models", `{}`, "application/json", false},
		{"null models", `{"models":null}`, "application/json", false},
		{"null row", `{"models":[null]}`, "application/json", false},
		{"malformed", `{"models":[`, "application/json", false},
		{"trailing", discoveryFixture + ` {}`, "application/json", false},
		{"duplicate", strings.ReplaceAll(discoveryFixture, "grafana/assistant", "assistant"), "application/json", false},
		{"empty name", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"name":""`, 1), "application/json", false},
		{"missing name", strings.Replace(discoveryFixture, `"name":"Assistant",`, "", 1), "application/json", false},
		{"null name", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"name":null`, 1), "application/json", false},
		{"invalid ID", strings.ReplaceAll(discoveryFixture, "grafana/assistant", "has space"), "application/json", false},
		{"wrong spec", strings.Replace(discoveryFixture, `"v4"`, `"v3"`, 1), "application/json", false},
		{"wrong provider", strings.Replace(discoveryFixture, `"provider":"grafana"`, `"provider":"private-backend"`, 1), "application/json", false},
		{"mismatched ID", strings.Replace(discoveryFixture, `"modelId":"assistant"`, `"modelId":"private-backend"`, 1), "application/json", false},
		{"wrong description", strings.Replace(discoveryFixture, `"description":"public description"`, `"description":2`, 1), "application/json", false},
		{"invalid UTF8", strings.Replace(discoveryFixture, "Assistant", string([]byte{255}), 1), "application/json", false},
		{"wrong media", discoveryFixture, "text/html", false},
		{"missing media", discoveryFixture, "", false},
		{"suffix media", discoveryFixture, "application/problem+json", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Type"] = []string{tc.media}
				_, _ = io.WriteString(w, tc.body)
			}, nil)
			rows, err := p.ListModels(context.Background())
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, rows)
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
				assert.NotContains(t, err.Error(), "private-backend")
				var api *provider.APICallError
				require.ErrorAs(t, err, &api)
				assert.False(t, api.IsRetryable)
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestListModels_BoundsAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int64
		body   string
		status int
		valid  bool
	}{
		{"exact", int64(len(discoveryFixture)), discoveryFixture, 200, true},
		{"one over", int64(len(discoveryFixture)) - 1, discoveryFixture, 200, false},
		{"large", 16, strings.Repeat("x", 100000), 200, false},
		{"error bound", 16, strings.Repeat("private-error", 10000), 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tc.body)}
			limits := DefaultLimits()
			limits.DiscoveryBytes = tc.limit
			limits.ErrorBytes = tc.limit
			p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", Limits: &limits, HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
			})}})
			require.NoError(t, err)
			rows, err := p.ListModels(context.Background())
			if tc.valid {
				require.NoError(t, err)
				assert.Len(t, rows, 2)
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
			}
			assert.True(t, body.closed)
			assert.LessOrEqual(t, body.read, int(tc.limit+1))
		})
	}
	t.Run("read failure closes body", func(t *testing.T) {
		cause := errors.New("read failure")
		body := &trackedBody{Reader: errorReader{cause}}
		p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
		})}})
		require.NoError(t, err)
		_, err = p.ListModels(context.Background())
		assert.ErrorIs(t, err, cause)
		assert.True(t, body.closed)
	})
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
