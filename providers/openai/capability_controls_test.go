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

func TestBuildParams_GPT6ReasoningEffortCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		model    string
		effort   string
		fallback provider.ReasoningEffort
		want     string
		warn     bool
		force    bool
	}{
		{name: "supported option", model: "gpt-6-astra", effort: "max", want: "max"},
		{name: "unsupported option", model: "gpt-6-astra", effort: "none", warn: true},
		{name: "unsupported fallback", model: "gpt-7", fallback: provider.ReasoningMinimal, warn: true},
		{name: "supported fallback", model: "gpt-6", fallback: provider.ReasoningHigh, want: "high"},
		{name: "option precedes fallback", model: "gpt-6", effort: "minimal", fallback: provider.ReasoningHigh, warn: true},
		{name: "earlier model remains unchanged", model: "gpt-5.6", effort: "none", want: "none"},
		{name: "unrecognized ID forced reasoning", model: "gpt-6chat", effort: "minimal", want: "minimal", force: true},
		{name: "mantle override", model: "openai.gpt-6-astra", effort: "none", want: "none"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			options := OpenAIResponsesOptions{ReasoningEffort: tc.effort}
			if tc.force {
				force := true
				options.ForceReasoning = &force
			}
			body, warnings := buildBody(t, tc.model, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.fallback,
				ProviderOptions: withOpenAIOptions(options),
			})
			assert.Equal(t, tc.effort, options.ReasoningEffort)
			if tc.want == "" {
				if reasoning, ok := body["reasoning"].(map[string]any); ok {
					assert.NotContains(t, reasoning, "effort")
				}
			} else {
				reasoning, ok := body["reasoning"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.want, reasoning["effort"])
			}
			if tc.warn {
				require.Contains(t, warningFeatures(warnings), "reasoningEffort")
				for _, warning := range warnings {
					if warning.Feature == "reasoningEffort" {
						assert.Equal(t, provider.WarnUnsupported, warning.Type)
						assert.Contains(t, warning.Details, "low, medium, high, xhigh, max")
						break
					}
				}
			} else {
				assert.NotContains(t, warningFeatures(warnings), "reasoningEffort")
			}
		})
	}
}

func TestBuildParams_ConfigurationUpdateAndCompactionTrigger(t *testing.T) {
	truth := true
	for _, tc := range []struct {
		name        string
		model       string
		options     OpenAIResponsesOptions
		wantUpdate  bool
		wantTrigger bool
		wantWarning bool
	}{
		{name: "supported update and trigger", model: "gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffort: "low", ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &truth, PreviousResponseID: "resp_1"}, wantUpdate: true, wantTrigger: true},
		{name: "old model still triggers", model: "gpt-5.6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &truth}, wantTrigger: true, wantWarning: true},
		{name: "pro rejects update", model: "gpt-6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, ReasoningMode: "pro"}, wantWarning: true},
		{name: "empty context management rejects update", model: "gpt-6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, ContextManagement: []ContextManagementEntry{}}, wantWarning: true},
		{name: "auto truncation rejects update", model: "gpt-6", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, Truncation: "auto"}, wantWarning: true},
		{name: "mantle requires endpoint verification", model: "openai.gpt-6-astra", options: OpenAIResponsesOptions{ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh}, wantWarning: true},
		{name: "default request unchanged", model: "gpt-6", options: OpenAIResponsesOptions{}},
		{name: "explicit false has no trigger", model: "gpt-5.6", options: OpenAIResponsesOptions{CompactionTrigger: new(bool)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prompt := []provider.Message{provider.UserText("hi")}
			before, err := json.Marshal(prompt)
			require.NoError(t, err)
			body, warnings := buildBody(t, tc.model, provider.CallOptions{Prompt: prompt, ProviderOptions: withOpenAIOptions(tc.options)})
			after, err := json.Marshal(prompt)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			input := body["input"].([]any)
			if tc.wantUpdate {
				assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
				assert.Equal(t, "high", input[0].(map[string]any)["reasoning"].(map[string]any)["effort"])
				input = input[1:]
			}
			assert.Equal(t, "hi", input[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
			if tc.wantTrigger {
				assert.Equal(t, "compaction_trigger", input[len(input)-1].(map[string]any)["type"])
				input = input[:len(input)-1]
			}
			require.Len(t, input, 1)
			if tc.wantWarning {
				assert.Contains(t, warningFeatures(warnings), "reasoningEffortUpdate")
			} else {
				assert.NotContains(t, warningFeatures(warnings), "reasoningEffortUpdate")
			}
			if tc.options.PreviousResponseID != "" {
				assert.Equal(t, tc.options.PreviousResponseID, body["previous_response_id"])
				assert.Equal(t, "low", body["reasoning"].(map[string]any)["effort"])
			}
		})
	}
}

func TestBuildParams_AzureUsesOpenAIModelCapabilities(t *testing.T) {
	body, warnings, _, err := buildParamsForProvider("gpt-6-astra", provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: withAzureOptions(t, map[string]any{"reasoningEffortUpdate": "high"}),
	}, "azure")
	require.NoError(t, err)
	assert.Empty(t, warnings)
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, json.Unmarshal(encoded, &request))
	input := request["input"].([]any)
	assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
}

func TestBuildParams_InvalidReasoningEffortUpdate(t *testing.T) {
	_, _, _, err := buildParams("gpt-6", provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{ReasoningEffortUpdate: "minimal"}),
	})
	require.ErrorContains(t, err, "invalid reasoningEffortUpdate")
}

func TestBuildParams_CompactionTriggerFollowsHistory(t *testing.T) {
	trigger := true
	encrypted := "encrypted"
	for _, store := range []bool{true, false} {
		t.Run(map[bool]string{true: "stored", false: "stateless"}[store], func(t *testing.T) {
			compaction := provider.CustomPart("openai.compaction")
			compaction.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "cmp_1", EncryptedContent: &encrypted})
			body, warnings := buildBody(t, "gpt-6", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(compaction), provider.UserText("continue")},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
					Store: &store, ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &trigger,
				}),
			})
			assert.Empty(t, warnings)
			input := body["input"].([]any)
			require.Len(t, input, 4)
			assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
			if store {
				assert.Equal(t, "item_reference", input[1].(map[string]any)["type"])
				assert.Equal(t, "cmp_1", input[1].(map[string]any)["id"])
			} else {
				assert.Equal(t, "compaction", input[1].(map[string]any)["type"])
				assert.Equal(t, "cmp_1", input[1].(map[string]any)["id"])
				assert.Equal(t, encrypted, input[1].(map[string]any)["encrypted_content"])
			}
			assert.Equal(t, "continue", input[2].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
			assert.Equal(t, "compaction_trigger", input[3].(map[string]any)["type"])
		})
	}
}

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
