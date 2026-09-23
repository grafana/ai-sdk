package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudCredentials_Requests(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		assert.Equal(t, "Bearer 27038:dummy-cap", r.Header.Get("Authorization"))
		assert.Len(t, r.Header.Values("Authorization"), 1)
		for _, name := range []string{"X-Access-Token", "X-Grafana-Id", "X-Scope-OrgID", "X-Cloud-Org-ID", "X-Access-Policy-ID"} {
			assert.Empty(t, r.Header.Values(name), name)
		}
		assert.Equal(t, "configured", r.Header.Get("X-Custom"))
		switch r.URL.Path {
		case "/api/v1/aisdk/config":
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, discoveryFixture)
		case "/api/v1/aisdk/language-model":
			assert.Equal(t, http.MethodPost, r.Method)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.NotContains(t, string(body), "dummy-cap")
			assert.NotContains(t, string(body), "27038")
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &fields))
			if r.Header.Get("Ai-Language-Model-Streaming") == "true" {
				assert.Empty(t, r.Header.Get("X-Request-Marker"))
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
			} else {
				assert.Equal(t, "accepted", r.Header.Get("X-Request-Marker"))
				assert.JSONEq(t, `{"x-request-marker":"accepted"}`, string(fields["headers"]))
				assert.Contains(t, string(fields["prompt"]), "Summarize this incident.")
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, unaryFixture)
			}
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	p, err := NewWithCloudCredentials(CloudCredentialsConfig{
		StackID: 27038, CAPToken: "dummy-cap", BaseURL: srv.URL + "/api/v1/aisdk", Headers: http.Header{"X-Custom": {"configured"}},
	})
	require.NoError(t, err)
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	maxTokens := 32
	result, err := m.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("Summarize this incident.")}, MaxOutputTokens: &maxTokens,
		Headers: map[string]string{"x-request-marker": "accepted"},
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Content[0].Text)
	assert.NotContains(t, string(result.Request.Body), "dummy-cap")
	stream, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}, MaxOutputTokens: &maxTokens})
	require.NoError(t, err)
	for range stream.Stream {
	}
	assert.NotContains(t, string(stream.Request.Body), "dummy-cap")
	assert.Equal(t, int32(3), requests.Load())
}

func TestCloudCredentials_Validation(t *testing.T) {
	valid := CloudCredentialsConfig{StackID: 1, CAPToken: "cap", BaseURL: "https://example.test/api/v1/aisdk"}
	for _, tc := range []struct {
		name   string
		change func(*CloudCredentialsConfig)
	}{
		{"zero stack", func(c *CloudCredentialsConfig) { c.StackID = 0 }},
		{"negative stack", func(c *CloudCredentialsConfig) { c.StackID = -1 }},
		{"empty token", func(c *CloudCredentialsConfig) { c.CAPToken = "" }},
		{"token whitespace", func(c *CloudCredentialsConfig) { c.CAPToken = "bad cap" }},
		{"token injection", func(c *CloudCredentialsConfig) { c.CAPToken = "bad\r\nsecret" }},
		{"token utf8", func(c *CloudCredentialsConfig) { c.CAPToken = string([]byte{0xff}) }},
		{"unsafe URL", func(c *CloudCredentialsConfig) { c.BaseURL = "https://secret@example.test" }},
		{"reserved config", func(c *CloudCredentialsConfig) { c.Headers = http.Header{"authorization": {"private-value"}} }},
		{"access token config", func(c *CloudCredentialsConfig) { c.Headers = http.Header{"x-ACCESS-token": {"private-value"}} }},
		{"user config", func(c *CloudCredentialsConfig) { c.Headers = http.Header{"X-Grafana-Id": {"private-value"}} }},
		{"spoofed stack", func(c *CloudCredentialsConfig) { c.Headers = http.Header{"x-scope-orgid": {"private-value"}} }},
		{"spoofed organization", func(c *CloudCredentialsConfig) { c.Headers = http.Header{"x-cloud-org-id": {"private-value"}} }},
		{"spoofed policy", func(c *CloudCredentialsConfig) { c.Headers = http.Header{"X-Access-Policy-ID": {"private-value"}} }},
		{"invalid limits", func(c *CloudCredentialsConfig) { c.Limits = &Limits{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.change(&cfg)
			p, err := NewWithCloudCredentials(cfg)
			require.Error(t, err)
			assert.Nil(t, p)
			assert.NotContains(t, err.Error(), "private-value")
			assert.NotContains(t, err.Error(), "secret")
		})
	}
}

func TestCloudCredentials_ConflictsAndCancellation(t *testing.T) {
	var requests atomic.Int32
	p, err := NewWithCloudCredentials(CloudCredentialsConfig{StackID: 42, CAPToken: "dummy-cap", BaseURL: "https://example.test/api/v1/aisdk", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(unaryFixture)), Request: r}, nil
	})}})
	require.NoError(t, err)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	for _, name := range []string{"Authorization", "AUTHORIZATION", "X-Access-Token", "x-grafana-id", "x-scope-orgid", "x-cloud-org-id", "x-access-policy-id"} {
		t.Run(name, func(t *testing.T) {
			for _, value := range []string{"private-value", ""} {
				for _, streaming := range []bool{false, true} {
					options := provider.CallOptions{Prompt: []provider.Message{}, Headers: map[string]string{name: value}}
					var err error
					if streaming {
						_, err = m.DoStream(context.Background(), options)
					} else {
						_, err = m.DoGenerate(context.Background(), options)
					}
					require.Error(t, err)
					assert.NotContains(t, err.Error(), "private-value")
					assert.Zero(t, requests.Load())
				}
			}
		})
	}
	_, err = p.ListModels(WithUserIDToken(context.Background(), "user-token"))
	require.Error(t, err)
	assert.Zero(t, requests.Load())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = m.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{}})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, requests.Load())
	_, err = m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	assert.Equal(t, int32(1), requests.Load())
}

func TestCloudCredentials_ConcurrentAndRedirect(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer 42:dummy-cap", r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	p, err := NewWithCloudCredentials(CloudCredentialsConfig{StackID: 42, CAPToken: "dummy-cap", BaseURL: origin.URL + "/api/v1/aisdk"})
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := p.ListModels(context.Background())
			assert.Error(t, err)
		})
	}
	wg.Wait()
	assert.Zero(t, reached.Load())
}
