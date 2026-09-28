package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
