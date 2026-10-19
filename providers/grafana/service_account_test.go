package grafana

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const serviceAccountRoutePrefix = "/apis/aigateway.ext.grafana.app/v1alpha1/namespaces/stacks-1"

func TestServiceAccount_Requests(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, []string{"Bearer glsa_dummy-token"}, r.Header.Values("Authorization"))
		assert.Empty(t, r.Header.Values("X-Access-Token"))
		assert.Empty(t, r.Header.Values("X-Grafana-Id"))
		assert.Equal(t, "configured", r.Header.Get("X-Custom"))
		switch r.URL.Path {
		case serviceAccountRoutePrefix + "/config":
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, discoveryFixture)
		case serviceAccountRoutePrefix + "/language-model":
			assert.Equal(t, http.MethodPost, r.Method)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.NotContains(t, string(body), "glsa_dummy-token")
			if r.Header.Get("Ai-Language-Model-Streaming") == "true" {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, unaryFixture)
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, err := NewWithServiceAccountToken(ServiceAccountConfig{
		Token: "glsa_dummy-token", BaseURL: srv.URL + serviceAccountRoutePrefix, Headers: http.Header{"X-Custom": {"configured"}},
	})
	require.NoError(t, err)
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Content[0].Text)
	stream, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	for range stream.Stream {
	}
	assert.Equal(t, int32(3), requests.Load())
}

func TestServiceAccount_Validation(t *testing.T) {
	valid := ServiceAccountConfig{Token: "glsa_token", BaseURL: "https://example.test" + serviceAccountRoutePrefix}
	for _, tc := range []struct {
		name   string
		change func(*ServiceAccountConfig)
	}{
		{"empty token", func(c *ServiceAccountConfig) { c.Token = "" }},
		{"token whitespace", func(c *ServiceAccountConfig) { c.Token = "glsa_bad token" }},
		{"token injection", func(c *ServiceAccountConfig) { c.Token = "glsa_bad\r\nsecret" }},
		{"token utf8", func(c *ServiceAccountConfig) { c.Token = string([]byte{0xff}) }},
		{"unsafe URL", func(c *ServiceAccountConfig) { c.BaseURL = "https://secret@example.test" }},
		{"reserved config", func(c *ServiceAccountConfig) { c.Headers = http.Header{"authorization": {"private-value"}} }},
		{"invalid limits", func(c *ServiceAccountConfig) { c.Limits = &Limits{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.change(&cfg)
			p, err := NewWithServiceAccountToken(cfg)
			require.Error(t, err)
			assert.Nil(t, p)
			assert.NotContains(t, err.Error(), "private-value")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestServiceAccount_RedirectDoesNotLeakToken(t *testing.T) {
	var received atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { received.Add(1) }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer srv.Close()
	p, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_token", BaseURL: srv.URL})
	require.NoError(t, err)
	_, err = p.ListModels(context.Background())
	require.Error(t, err)
	assert.Zero(t, received.Load())
}
