package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayAnthropicStream_FinishOnMessageStop(t *testing.T) {
	const verdict = `[{"type":"dangerous_tool_use","status":{"type":"available"}}]`
	frame := func(event, data string) string { return fmt.Sprintf("event: %s\ndata: %s\n\n", event, data) }
	start := frame("message_start", `{"type":"message_start","message":{"id":"msg","type":"message","role":"assistant","model":"backend","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`)
	delta := func(safeguard string) string {
		return frame("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","safeguard_results":`+safeguard+`},"usage":{"output_tokens":1}}`)
	}
	stop := frame("message_stop", `{"type":"message_stop"}`)

	for _, tc := range []struct {
		name        string
		body        string
		wantFinish  bool
		wantVerdict bool
	}{
		{
			name:        "finish carries the verdict from a later delta",
			body:        start + delta("null") + delta(verdict) + delta("null") + stop,
			wantFinish:  true,
			wantVerdict: true,
		},
		{
			name: "stream ending after a delta is a premature close",
			body: start + delta(verdict),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer backend.Close()

			catalog, err := BuildCatalog(config.File{Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "native", Model: "backend"}}}},
				map[string]config.ResolvedProvider{"native": {Type: "anthropic", APIKey: "native-key", BaseURL: backend.URL, ProviderName: "anthropic"}},
				backend.Client(), identityModelFactory)
			require.NoError(t, err)
			handler, err := providerv4.New(providerv4.Config{Resolver: catalog, Limits: serviceTestLimits()})
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[]}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
			request.Header.Set(providerv4.HeaderModelID, "public")
			request.Header.Set(providerv4.HeaderStreaming, "true")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)

			var finishes []struct {
				ProviderMetadata map[string]map[string]json.RawMessage `json:"providerMetadata"`
			}
			var types []provider.StreamPartType
			for _, line := range strings.Split(response.Body.String(), "\n") {
				data, ok := strings.CutPrefix(line, "data: ")
				if !ok {
					continue
				}
				var part struct {
					Type             provider.StreamPartType               `json:"type"`
					ProviderMetadata map[string]map[string]json.RawMessage `json:"providerMetadata"`
				}
				require.NoError(t, json.Unmarshal([]byte(data), &part))
				types = append(types, part.Type)
				if part.Type == provider.PartFinish {
					finishes = append(finishes, struct {
						ProviderMetadata map[string]map[string]json.RawMessage `json:"providerMetadata"`
					}{part.ProviderMetadata})
				}
			}

			if !tc.wantFinish {
				assert.Empty(t, finishes)
				assert.Equal(t, 1, strings.Count(response.Body.String(), `"code":"internal_error"`))
				return
			}
			require.Len(t, finishes, 1)
			assert.Equal(t, provider.PartFinish, types[len(types)-1])
			if tc.wantVerdict {
				assert.JSONEq(t, verdict, string(finishes[0].ProviderMetadata["anthropic"]["safeguardResults"]))
			}
		})
	}
}
