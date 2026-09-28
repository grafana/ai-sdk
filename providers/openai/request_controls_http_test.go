package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesCapabilityControls_GenerateAndStreamRequests(t *testing.T) {
	trigger := true
	for _, tc := range []struct {
		name, model string
		options     OpenAIResponsesOptions
		wantUpdate  bool
		wantWarning string
	}{
		{name: "supported", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "high", ReasoningEffortUpdate: OpenAIReasoningEffortUpdateLow, CompactionTrigger: &trigger}, wantUpdate: true},
		{name: "unsupported effort and update", model: "gpt-5.6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateLow}, wantWarning: "reasoningEffortUpdate"},
		{name: "invalid gpt6 effort", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "none"}, wantWarning: "reasoningEffort"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var bodies []map[string]any
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				data, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				var body map[string]any
				require.NoError(t, json.Unmarshal(data, &body))
				bodies = append(bodies, body)
				contentType, payload := "application/json", `{"id":"resp_1","status":"completed","output":[]}`
				if body["stream"] == true {
					contentType = "text/event-stream"
					payload = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_1","status":"completed","output":[]}}` + "\n\n"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
			})}
			model := NewResponses("test-key", tc.model, WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
			options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, ProviderOptions: withOpenAIOptions(tc.options)}
			generated, err := model.DoGenerate(t.Context(), options)
			require.NoError(t, err)
			streamed, err := model.DoStream(t.Context(), options)
			require.NoError(t, err)
			var streamWarnings []provider.Warning
			for part := range streamed.Stream {
				if part.Type == provider.PartStreamStart {
					streamWarnings = part.Warnings
				}
			}
			require.Len(t, bodies, 2)
			assert.Equal(t, generated.Warnings, streamWarnings)
			for _, body := range bodies {
				input := body["input"].([]any)
				if tc.wantUpdate {
					assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
					assert.Equal(t, "compaction_trigger", input[len(input)-1].(map[string]any)["type"])
					assert.Equal(t, "high", body["reasoning"].(map[string]any)["effort"])
				} else {
					assert.Contains(t, input[0].(map[string]any), "content")
					if tc.wantWarning == "reasoningEffort" {
						if reasoning, ok := body["reasoning"].(map[string]any); ok {
							assert.NotContains(t, reasoning, "effort")
						}
					}
				}
			}
			if tc.wantWarning != "" {
				assert.Contains(t, warningFeatures(generated.Warnings), tc.wantWarning)
			} else {
				assert.NotContains(t, warningFeatures(generated.Warnings), "reasoningEffortUpdate")
			}
		})
	}
}
