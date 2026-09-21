package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_VertexRequestCapabilities(t *testing.T) {
	for _, modelCase := range []struct {
		id    string
		limit int
	}{
		{"claude-sonnet-4@20250514", 64000},
		{"claude-opus-4@20250514", 32000},
	} {
		t.Run(modelCase.id, func(t *testing.T) {
			for _, tc := range []struct {
				name       string
				maxTokens  int
				budget     int
				sampling   bool
				topP       bool
				wantTokens int
				wantClamp  bool
			}{
				{name: "default", wantTokens: modelCase.limit},
				{name: "explicit", maxTokens: 2048, wantTokens: 2048},
				{name: "budget under limit", maxTokens: 2048, budget: 1024, wantTokens: 3072},
				{name: "explicit clamp", maxTokens: modelCase.limit, budget: 1024, wantTokens: modelCase.limit, wantClamp: true},
				{name: "default budget clamp", budget: 1024, wantTokens: modelCase.limit},
				{name: "sampling", sampling: true, wantTokens: modelCase.limit},
				{name: "topP", topP: true, wantTokens: modelCase.limit},
			} {
				for _, stream := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
						requests := make(chan map[string]any, 1)
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							var body map[string]any
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Error(err)
								w.WriteHeader(http.StatusBadRequest)
								return
							}
							requests <- body
							if stream {
								w.Header().Set("Content-Type", "text/event-stream")
								_, err := fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_test\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"test\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":0}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
								assert.NoError(t, err)
								return
							}
							w.Header().Set("Content-Type", "application/json")
							_, err := fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","model":"test","content":[],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":0}}`)
							assert.NoError(t, err)
						}))
						defer server.Close()
						m := &model{
							client:  anthropicsdk.NewClient(option.WithoutEnvironmentDefaults(), option.WithAPIKey("test"), option.WithBaseURL(server.URL), option.WithMaxRetries(0), option.WithRequestTimeout(10*time.Second)),
							modelID: modelCase.id, providerName: "anthropic.vertex", resolveModel: ResolveVertexModelID,
							generateID: defaultGenerateID, capabilities: vertexProviderCapabilities,
						}
						opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hello")}}
						if tc.maxTokens != 0 {
							opts.MaxOutputTokens = &tc.maxTokens
						}
						if tc.budget != 0 {
							opts.ProviderOptions = provider.BuildProviderOptions(AnthropicOptions{Thinking: &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: tc.budget}})
						}
						if tc.sampling {
							temperature, topK := 0.5, 20
							opts.Temperature, opts.TopK = &temperature, &topK
						}
						if tc.topP {
							topP := 0.9
							opts.TopP = &topP
						}
						var warnings []provider.Warning
						if stream {
							result, err := m.DoStream(context.Background(), opts)
							require.NoError(t, err)
							for part := range result.Stream {
								assert.NotEqual(t, provider.PartError, part.Type)
								warnings = append(warnings, part.Warnings...)
							}
						} else {
							result, err := m.DoGenerate(context.Background(), opts)
							require.NoError(t, err)
							warnings = result.Warnings
						}
						body := <-requests
						assert.Equal(t, float64(tc.wantTokens), body["max_tokens"])
						assert.Equal(t, modelCase.id, body["model"])
						assert.Equal(t, modelCase.id, m.ModelID())
						if tc.wantClamp {
							require.Len(t, warnings, 1)
							assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
							assert.Equal(t, "maxOutputTokens", warnings[0].Feature)
						} else {
							assert.Empty(t, warnings)
						}
						if tc.budget != 0 {
							assert.Equal(t, map[string]any{"type": "enabled", "budget_tokens": float64(tc.budget)}, body["thinking"])
						}
						if tc.sampling {
							assert.Equal(t, 0.5, body["temperature"])
							assert.Equal(t, float64(20), body["top_k"])
						}
						if tc.topP {
							assert.Equal(t, 0.9, body["top_p"])
						}
					})
				}
			}
		})
	}
}
