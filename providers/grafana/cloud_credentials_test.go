package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	logmiddleware "github.com/grafana/ai-sdk/middleware/logger"
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

func TestModel_BYOKProjection(t *testing.T) {
	const gatewayOptions = `{"byok":{"anthropic":[{"apiKey":"dummy-anthropic"}],"openai":[{"apiKey":"dummy-openai-first"},{"apiKey":"dummy-openai-second"}]}}`
	const expected = `{"prompt":[],"providerOptions":{"gateway":` + gatewayOptions + `,"openai":{"store":false}}}`
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/aisdk/language-model", r.URL.Path)
				assert.Equal(t, []string{"Bearer 123:dummy-cap"}, r.Header.Values("Authorization"))
				assert.Equal(t, "openai/model-not-in-catalog", r.Header.Get("Ai-Language-Model-Id"))
				assert.Equal(t, strconv.FormatBool(streaming), r.Header.Get("Ai-Language-Model-Streaming"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, expected, string(body))
				assert.NotContains(t, string(body), "dummy-cap")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, unaryFixture)
				}
			}))
			t.Cleanup(srv.Close)
			p, err := NewWithCloudCredentials(CloudCredentialsConfig{StackID: 123, CAPToken: "dummy-cap", BaseURL: srv.URL + "/api/v1/aisdk"})
			require.NoError(t, err)
			model, err := p.LanguageModel("openai/model-not-in-catalog")
			require.NoError(t, err)
			opts := provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{
				"gateway": provider.RawProviderOption{Raw: json.RawMessage(gatewayOptions)},
				"openai":  provider.RawProviderOption{Raw: json.RawMessage(`{"store":false}`)},
			}}
			var requestBody []byte
			if streaming {
				result, err := model.DoStream(context.Background(), opts)
				require.NoError(t, err)
				for range result.Stream {
				}
				require.NotNil(t, result.Request)
				requestBody = result.Request.Body
			} else {
				result, err := model.DoGenerate(context.Background(), opts)
				require.NoError(t, err)
				require.NotNil(t, result.Request)
				requestBody = result.Request.Body
			}
			assert.JSONEq(t, expected, string(requestBody))
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_BYOKLoggerCapture(t *testing.T) {
	const rawOptions = `{"byok":{"openai":[{"apiKey":"dummy-first","future":{"unfamiliar":"dummy-unfamiliar"}},{"apiKey":"dummy-second"}]},"ordinary":true}`
	for _, operation := range []string{"generate", "stream", "error"} {
		for _, limit := range []int{1, 8192} {
			t.Run(operation+"/"+strconv.Itoa(limit), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					for _, key := range []string{"dummy-first", "dummy-second", "dummy-unfamiliar"} {
						assert.Contains(t, string(body), key)
					}
					switch operation {
					case "error":
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = io.WriteString(w, `{"error":{"message":"invalid request","type":"invalid_request_error","code":"invalid_request","param":null}}`)
					case "stream":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
					default:
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, unaryFixture)
					}
				}))
				defer srv.Close()
				p, err := NewWithCloudCredentials(CloudCredentialsConfig{StackID: 123, CAPToken: "dummy-cap", BaseURL: srv.URL + "/api/v1/aisdk"})
				require.NoError(t, err)
				base, err := p.LanguageModel("openai/native")
				require.NoError(t, err)
				var logs bytes.Buffer
				model := logmiddleware.Wrap(base, logmiddleware.Options{Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Capture: logmiddleware.CaptureOptions{ProviderOptions: true, RequestBody: true, MaxJSONBytes: limit}})
				opts := provider.CallOptions{Prompt: []provider.Message{}, ProviderOptions: provider.ProviderOptions{"gateway": provider.RawProviderOption{Raw: json.RawMessage(rawOptions)}}}
				if operation == "stream" {
					result, err := model.DoStream(context.Background(), opts)
					require.NoError(t, err)
					for range result.Stream {
					}
					assert.Contains(t, string(result.Request.Body), "dummy-unfamiliar")
				} else {
					result, err := model.DoGenerate(context.Background(), opts)
					if operation == "error" {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
						assert.Contains(t, string(result.Request.Body), "dummy-unfamiliar")
					}
				}
				for _, key := range []string{"dummy-first", "dummy-second", "dummy-unfamiliar", "dummy-cap"} {
					assert.NotContains(t, logs.String(), key)
				}
				if limit > 1 {
					assert.Contains(t, logs.String(), "ordinary")
					assert.Contains(t, logs.String(), "[REDACTED]")
				}
			})
		}
	}
}
