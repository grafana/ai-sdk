package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
)

func listingResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCatalogHealthChecker_Check(t *testing.T) {
	stack := testStackCatalog()
	stack.Spec.Providers["unused"] = v1alpha1.Provider{Type: "openai"}
	stack.Keys["unused"] = "unused-catalog-key"
	model := stack.Spec.Models["balanced"]
	model.Fallback = append(model.Fallback, v1alpha1.Target{Provider: "claude", Model: "claude-opus"})
	stack.Spec.Models["balanced"] = model
	var calls atomic.Int32
	checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Nil(t, r.Body)
		switch r.URL.Host {
		case "api.anthropic.com":
			assert.Equal(t, "/v1/models", r.URL.Path)
			assert.Equal(t, "claude-catalog-key", r.Header.Get("X-Api-Key"))
			assert.Equal(t, "2023-06-01", r.Header.Get("Anthropic-Version"))
			assert.Empty(t, r.Header.Get("Authorization"))
			assert.Equal(t, "100", r.URL.Query().Get("limit"))
			if r.URL.Query().Get("after_id") == "" {
				return listingResponse(200, `{"data":[{"id":"claude-sonnet"}],"has_more":true,"last_id":"a&b"}`), nil
			}
			assert.Equal(t, "a&b", r.URL.Query().Get("after_id"))
			return listingResponse(200, `{"data":[{"id":"claude-opus"}],"has_more":false}`), nil
		case "api.openai.com":
			assert.Equal(t, "/v1/models", r.URL.Path)
			assert.Empty(t, r.URL.RawQuery)
			assert.Empty(t, r.Header.Get("X-Api-Key"))
			if r.Header.Get("Authorization") == "Bearer unused-catalog-key" {
				return listingResponse(401, "sentinel-provider-error"), nil
			}
			assert.Equal(t, "Bearer gpt-catalog-key", r.Header.Get("Authorization"))
			return listingResponse(200, `{"data":[{"id":"gpt-large"},{"id":"GPT-SMALL"},{"id":"unconfigured"}]}`), nil
		default:
			return nil, errors.New("unexpected destination")
		}
	}))
	got, err := checker.Check(t.Context(), "stack", stack)
	require.NoError(t, err)
	assert.EqualValues(t, 4, calls.Load())
	body, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"providers":{"claude":"ok","gpt":"ok","unused":"invalidKey"},"models":{"balanced":{"primary":"ok","fallback":["ok","ok"]},"same":{"primary":"ok","fallback":[]},"fast":{"primary":"unknownModel","fallback":[]}}}`, string(body))
}

func TestCatalogHealthChecker_Preflight(t *testing.T) {
	checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
		t.Error("invalid catalog attempted a provider request")
		return nil, errors.New("unexpected provider request")
	}))
	for _, tc := range []struct {
		name   string
		mutate func(StackCatalog)
	}{
		{"missing key", func(stack StackCatalog) { delete(stack.Keys, "claude") }},
		{"invalid shape", func(stack StackCatalog) { clear(stack.Spec.Models) }},
		{"endpoint override", func(stack StackCatalog) {
			stack.Spec.Providers["claude"] = v1alpha1.Provider{Type: "anthropic", BaseURL: new("https://example.invalid")}
		}},
		{"compatible type", func(stack StackCatalog) { stack.Spec.Providers["gpt"] = v1alpha1.Provider{Type: "openai-compatible"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stack := testStackCatalog()
			tc.mutate(stack)
			_, err := checker.Check(t.Context(), "stack", stack)
			require.ErrorIs(t, err, execution.ErrCatalogConfiguration)
		})
	}
}

func TestCatalogHealthChecker_ListingOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		replies   []*http.Response
		status    HealthStatus
		found     HealthStatus
		missing   HealthStatus
		anthropic bool
		custom    bool
	}{
		{name: "empty listing", replies: []*http.Response{listingResponse(200, `{"data":[]}`)}, status: HealthOK, missing: HealthUnknownModel},
		{name: "invalid key", replies: []*http.Response{listingResponse(401, "secret")}, status: HealthInvalidKey},
		{name: "forbidden", replies: []*http.Response{listingResponse(403, "secret")}, status: HealthRefused},
		{name: "rate limit", replies: []*http.Response{listingResponse(429, "secret")}, status: HealthRefused},
		{name: "upstream server", replies: []*http.Response{listingResponse(502, "secret")}, status: HealthUnreachable},
		{name: "transport", replies: []*http.Response{nil}, status: HealthUnreachable},
		{name: "not a listing", replies: []*http.Response{listingResponse(200, `{"data":null}`)}, status: HealthRefused},
		{name: "malformed ID", replies: []*http.Response{listingResponse(200, `{"data":[{}]}`)}, status: HealthRefused},
		{name: "oversized listing", replies: []*http.Response{listingResponse(200, strings.Repeat(" ", healthResponseBytes+1))}, status: HealthNotChecked},
		{name: "compatible listing", replies: []*http.Response{listingResponse(200, `{"data":[{"id":"listed"}]}`)}, status: HealthOK, found: HealthOK, custom: true},
		{name: "no compatible listing", replies: []*http.Response{listingResponse(501, "secret")}, status: HealthNotChecked, custom: true},
		{name: "later page fails", replies: []*http.Response{listingResponse(200, `{"data":[{"id":"listed"}],"has_more":true,"last_id":"1"}`), listingResponse(401, "secret")}, status: HealthOK, found: HealthOK, anthropic: true},
		{name: "repeated cursor", replies: []*http.Response{listingResponse(200, `{"data":[],"has_more":true,"last_id":"1"}`), listingResponse(200, `{"data":[{"id":"listed"}],"has_more":true,"last_id":"1"}`)}, status: HealthOK, found: HealthOK, anthropic: true},
		{name: "page cap", replies: []*http.Response{listingResponse(200, `{"data":[{"id":"listed"}],"has_more":true,"last_id":"1"}`), listingResponse(200, `{"data":[],"has_more":true,"last_id":"2"}`), listingResponse(200, `{"data":[],"has_more":true,"last_id":"3"}`)}, status: HealthOK, found: HealthOK, anthropic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := config.ResolvedProvider{Type: "openai", APIKey: "stored-key"}
			if tc.anthropic {
				p.Type = "anthropic"
			}
			if tc.custom {
				p.Type, p.BaseURL = "openai-compatible", "https://example.invalid/prefix/v1/"
			}
			calls := 0
			checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
				if tc.custom {
					assert.Equal(t, "https://example.invalid/prefix/v1/models", r.URL.String())
					assert.Equal(t, "Bearer stored-key", r.Header.Get("Authorization"))
				}
				require.Less(t, calls, len(tc.replies), "no extra requests")
				resp := tc.replies[calls]
				calls++
				if resp == nil {
					return nil, errors.New("secret transport error")
				}
				resp.Header = http.Header{"Content-Type": {"text/event-stream"}}
				return resp, nil
			}))
			got := checker.list(t.Context(), p, map[string]struct{}{"listed": {}, "missing": {}})
			assert.Equal(t, len(tc.replies), calls)
			assert.Equal(t, tc.status, got.status)
			if tc.missing == "" {
				tc.missing = HealthNotChecked
			}
			if tc.found == "" {
				tc.found = tc.missing
			}
			assert.Equal(t, tc.found, got.model("listed"))
			assert.Equal(t, tc.missing, got.model("missing"))
		})
	}
}

func TestCatalogHealthChecker_Bounds(t *testing.T) {
	t.Run("admission does not queue and releases on cancellation", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			for i := range healthMaxRuns {
				go func() {
					_, err := checker.Check(ctx, fmt.Sprint(i), testStackCatalog())
					assert.NoError(t, err)
				}()
				synctest.Wait()
				_, err := checker.Check(t.Context(), fmt.Sprint(i), testStackCatalog())
				require.ErrorIs(t, err, ErrCatalogHealthBusy)
			}
			_, err := checker.Check(t.Context(), "another", testStackCatalog())
			require.ErrorIs(t, err, ErrCatalogHealthBusy)
			cancel()
			synctest.Wait()
			_, err = checker.Check(ctx, "0", testStackCatalog())
			require.NoError(t, err)
		})
	})
	t.Run("fan-out and deadlines", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var peak, active, calls atomic.Int32
			checker := NewCatalogHealthChecker(selectionTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				n := active.Add(1)
				defer active.Add(-1)
				for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
				}
				<-r.Context().Done()
				return nil, r.Context().Err()
			}))
			stack := testStackCatalog()
			for i := range 10 {
				name := fmt.Sprintf("extra-%02d", i)
				stack.Spec.Providers[name] = v1alpha1.Provider{Type: "openai"}
				stack.Keys[name] = "key"
			}
			start := time.Now()
			got, err := checker.Check(t.Context(), "stack", stack)
			require.NoError(t, err)
			assert.Equal(t, 2*healthProviderTimeout, time.Since(start))
			assert.EqualValues(t, healthMaxProviders, calls.Load())
			assert.EqualValues(t, healthProviderWorkers, peak.Load())
			assert.Equal(t, HealthUnreachable, got.Providers["claude"])
			assert.Equal(t, HealthNotChecked, got.Providers["gpt"])
			assert.Equal(t, ModelHealth{Primary: HealthNotChecked, Fallback: []HealthStatus{HealthNotChecked}}, got.Models["balanced"])
		})
	})
}
