package grafana

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceAccount_ConflictsAndCancellation(t *testing.T) {
	var requests atomic.Int32
	p, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_dummy", BaseURL: "https://example.test/api/v1/aisdk", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(unaryFixture)), Request: r}, nil
	})}})
	require.NoError(t, err)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	for _, name := range []string{"Authorization", "AUTHORIZATION", "X-Access-Token", "x-grafana-id"} {
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
	_, err = p.ListModels(WithOnBehalfOf(context.Background(), "user-token"))
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

func TestServiceAccount_ConcurrentAndRedirect(t *testing.T) {
	var reached atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached.Add(1) }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer glsa_dummy", r.Header.Get("Authorization"))
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	p, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_dummy", BaseURL: origin.URL + "/api/v1/aisdk"})
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
	const gatewayOptions = `{"byok":{"anthropic":[{"apiKey":"dummy-anthropic","baseURL":"https://approved.example/anthropic"}],"openai":[{"apiKey":"dummy-openai-first","baseURL":"https://approved.example/v1","organization":"org-customer","project":"proj-customer"},{"apiKey":"dummy-openai-second"}]}}`
	const expected = `{"prompt":[],"providerOptions":{"gateway":` + gatewayOptions + `,"openai":{"store":false}}}`
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/aisdk/language-model", r.URL.Path)
				assert.Equal(t, []string{"Bearer glsa_dummy"}, r.Header.Values("Authorization"))
				assert.Equal(t, "openai/model-not-in-catalog", r.Header.Get("Ai-Language-Model-Id"))
				assert.Equal(t, strconv.FormatBool(streaming), r.Header.Get("Ai-Language-Model-Streaming"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.JSONEq(t, expected, string(body))
				assert.NotContains(t, string(body), "glsa_dummy")
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"type\":\"stream-start\"}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, unaryFixture)
				}
			}))
			t.Cleanup(srv.Close)
			p, err := NewWithServiceAccountToken(ServiceAccountConfig{Token: "glsa_dummy", BaseURL: srv.URL + "/api/v1/aisdk"})
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
