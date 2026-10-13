package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	logmiddleware "github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_BYOKLoggerCapture(t *testing.T) {
	const rawOptions = `{"byok":{"openai":[{"apiKey":"dummy-first","baseURL":"https://dummy-endpoint.example/v1","organization":"dummy-organization","project":"dummy-project","future":{"unfamiliar":"dummy-unfamiliar"}},{"apiKey":"dummy-second"}]},"ordinary":true}`
	for _, operation := range []string{"generate", "stream", "error"} {
		for _, limit := range []int{1, 8192} {
			t.Run(operation+"/"+strconv.Itoa(limit), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					for _, key := range []string{"dummy-first", "dummy-second", "dummy-unfamiliar", "dummy-endpoint", "dummy-organization", "dummy-project"} {
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
						_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"hello"},{"type":"text","text":""}],"finishReason":{"unified":"stop","raw":"end_turn"},"usage":{"inputTokens":{"total":2,"noCache":2,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":1,"text":1,"reasoning":0}}}`)
					}
				}))
				defer srv.Close()
				p, err := grafana.NewWithCloudCredentials(grafana.CloudCredentialsConfig{StackID: 123, CAPToken: "dummy-cap", BaseURL: srv.URL + "/api/v1/aisdk"})
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
				for _, key := range []string{"dummy-first", "dummy-second", "dummy-cap"} {
					assert.NotContains(t, logs.String(), key)
				}
				if limit > 1 {
					assert.Contains(t, logs.String(), "ordinary")
					for _, value := range []string{"dummy-unfamiliar", "dummy-endpoint", "dummy-organization", "dummy-project"} {
						assert.Contains(t, logs.String(), value)
					}
					assert.Contains(t, logs.String(), "[REDACTED]")
				}
			})
		}
	}
}
