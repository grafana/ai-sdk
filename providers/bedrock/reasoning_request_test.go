package bedrock

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureBedrockRequest(t *testing.T, modelID string, opts provider.CallOptions, streaming bool) (map[string]any, []provider.Warning) {
	t.Helper()
	frames := encodeFixtures(t,
		`{"messageStart":{"role":"assistant"}}`,
		`{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"ok"}}}`,
		`{"contentBlockStop":{"contentBlockIndex":0}}`,
		`{"messageStop":{"stopReason":"end_turn"}}`,
		`{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`,
	)
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		path := "/model/" + encodeModelIDPathSegment(modelID) + "/converse"
		if streaming {
			assert.Equal(t, path+"-stream", r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
			_, err := w.Write(frames)
			assert.NoError(t, err)
		} else {
			assert.Equal(t, path, r.URL.EscapedPath())
			w.Header().Set("Content-Type", "application/json")
			_, err := w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
			assert.NoError(t, err)
		}
	}))
	defer server.Close()
	model := New(modelID, WithRegion("us-east-1"), WithBaseURL(server.URL), WithBearerToken("test"))
	assert.Equal(t, modelID, model.ModelID())
	if streaming {
		result, err := model.DoStream(context.Background(), opts)
		require.NoError(t, err)
		var warnings []provider.Warning
		for part := range result.Stream {
			require.NotEqual(t, provider.PartError, part.Type, "%v", part.APICallError)
			if part.Type == provider.PartStreamStart {
				warnings = append(warnings, part.Warnings...)
			}
		}
		return request, warnings
	}
	result, err := model.DoGenerate(context.Background(), opts)
	require.NoError(t, err)
	return request, result.Warnings
}

func TestModel_OpenAIReasoningRequest(t *testing.T) {
	for _, tc := range []struct {
		modelID string
		want    string
	}{
		{"openai.gpt-oss-120b-1:0", `{"reasoning_effort":"high"}`},
		{"us.openai.gpt-oss-120b-1:0", `{"reasoning_effort":"high"}`},
		{"openai.gpt-5.4", `{"reasoning":{"effort":"high"}}`},
		{"eu.openai.gpt-5.4", `{"reasoning":{"effort":"high"}}`},
		{"custom-openai.gpt-5.4", `{"reasoningConfig":{"maxReasoningEffort":"high"}}`},
		{"foo.bar.openai.gpt-5.4", `{"reasoningConfig":{"maxReasoningEffort":"high"}}`},
		{"openai.", `{"reasoningConfig":{"maxReasoningEffort":"high"}}`},
		{"amazon.nova-pro-v1:0", `{"reasoningConfig":{"maxReasoningEffort":"high"}}`},
		{"anthropic.claude-sonnet-4-6", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`},
	} {
		t.Run(tc.modelID, func(t *testing.T) {
			for _, source := range []string{"root", "provider", "default", "nested fields"} {
				for _, streaming := range []bool{false, true} {
					t.Run(source+map[bool]string{false: "/generate", true: "/stream"}[streaming], func(t *testing.T) {
						opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}}
						bo := BedrockOptions{}
						switch source {
						case "root":
							opts.Reasoning = provider.ReasoningHigh
						case "provider", "nested fields":
							bo.ReasoningConfig = &ReasoningConfig{MaxReasoningEffort: "high"}
							if source == "nested fields" {
								bo.AdditionalModelRequestFields = map[string]any{"reasoning": map[string]any{"effort": "low", "summary": "auto"}, "custom": "value"}
							}
						}
						opts.ProviderOptions = provider.BuildProviderOptions(bo)
						before, err := json.Marshal(opts)
						require.NoError(t, err)
						body, warnings := captureBedrockRequest(t, tc.modelID, opts, streaming)
						assert.Empty(t, warnings)
						var want map[string]any
						require.NoError(t, json.Unmarshal([]byte(tc.want), &want))
						if source != "root" {
							delete(want, "thinking")
						}
						if source == "default" {
							assert.NotContains(t, body, "additionalModelRequestFields")
						} else {
							if source == "nested fields" {
								nested, ok := want["reasoning"].(map[string]any)
								if ok {
									nested["summary"] = "auto"
								} else {
									want["reasoning"] = map[string]any{"effort": "low", "summary": "auto"}
								}
								want["custom"] = "value"
							}
							assert.Equal(t, want, body["additionalModelRequestFields"])
						}
						after, err := json.Marshal(opts)
						require.NoError(t, err)
						assert.JSONEq(t, string(before), string(after))
					})
				}
			}
		})
	}
}
