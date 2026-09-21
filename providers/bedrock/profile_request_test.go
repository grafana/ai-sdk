package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testApplicationProfile = "arn:aws:bedrock:us-east-1:123456789012:application-inference-profile/opaque"

func rawBedrockOptions(raw string) provider.ProviderOptions {
	return provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(raw)}}
}

func TestModel_ProfileReasoningRequest(t *testing.T) {
	legacy := provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{"reasoningConfig":{"type":"enabled","budgetTokens":1024}}`)}
	for _, tc := range []struct {
		name      string
		options   provider.ProviderOptions
		reasoning provider.ReasoningEffort
		anthropic bool
		thinking  bool
		budget    int
		fields    string
	}{
		{name: "typed positive", options: provider.BuildProviderOptions(BedrockOptions{ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 1024}}), anthropic: true, thinking: true, budget: 1024},
		{name: "typed zero omitted", options: provider.BuildProviderOptions(BedrockOptions{ReasoningConfig: &ReasoningConfig{Type: "enabled"}})},
		{name: "raw zero", options: rawBedrockOptions(`{"reasoningConfig":{"type":"enabled","budgetTokens":0}}`), anthropic: true, thinking: true},
		{name: "raw absent", options: rawBedrockOptions(`{"reasoningConfig":{"type":"enabled"}}`)},
		{name: "budget without type", options: rawBedrockOptions(`{"reasoningConfig":{"budgetTokens":1024}}`), anthropic: true},
		{name: "legacy", options: provider.ProviderOptions{"bedrock": legacy}, anthropic: true, thinking: true, budget: 1024},
		{name: "modern shadows legacy", options: provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{}`)}, "bedrock": legacy}},
		{name: "modern null falls back", options: provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`null`)}, "bedrock": legacy}, anthropic: true, thinking: true, budget: 1024},
		{name: "modern zero overrides legacy", options: provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{"reasoningConfig":{"type":"enabled","budgetTokens":0}}`)}, "bedrock": legacy}, anthropic: true, thinking: true},
		{name: "root high alone", reasoning: provider.ReasoningHigh, fields: `{"reasoningConfig":{"maxReasoningEffort":"high"}}`},
		{name: "root high explicit zero", reasoning: provider.ReasoningHigh, options: rawBedrockOptions(`{"reasoningConfig":{"budgetTokens":0}}`), anthropic: true, thinking: true},
		{name: "root high explicit budget", reasoning: provider.ReasoningHigh, options: rawBedrockOptions(`{"reasoningConfig":{"budgetTokens":1024}}`), anthropic: true, thinking: true, budget: 1024},
		{name: "root none removes budget", reasoning: provider.ReasoningNone, options: rawBedrockOptions(`{"reasoningConfig":{"type":"enabled","budgetTokens":1024,"maxReasoningEffort":"high"}}`), anthropic: true},
		{name: "disabled override", reasoning: provider.ReasoningHigh, options: rawBedrockOptions(`{"reasoningConfig":{"type":"disabled","budgetTokens":0,"maxReasoningEffort":"high"}}`), anthropic: true},
		{name: "adaptive explicit budget", options: rawBedrockOptions(`{"reasoningConfig":{"type":"adaptive","budgetTokens":0,"maxReasoningEffort":"high"}}`), anthropic: true, thinking: true, budget: -1, fields: `{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`},
		{name: "explicit effort", options: rawBedrockOptions(`{"reasoningConfig":{"budgetTokens":0,"maxReasoningEffort":"high"}}`), anthropic: true, fields: `{"output_config":{"effort":"high"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, streaming := range []bool{false, true} {
				t.Run(map[bool]string{false: "generate", true: "stream"}[streaming], func(t *testing.T) {
					maxTokens, temperature, topP, topK := 128, 0.5, 0.9, 10
					opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}, ProviderOptions: tc.options, Reasoning: tc.reasoning, MaxOutputTokens: &maxTokens, Temperature: &temperature, TopP: &topP, TopK: &topK}
					before, err := json.Marshal(opts)
					require.NoError(t, err)
					body, warnings := captureBedrockRequest(t, testApplicationProfile, opts, streaming)
					if tc.anthropic {
						assert.Equal(t, []any{"/delta/stop_sequence"}, body["additionalModelResponseFieldPaths"])
					} else {
						assert.NotContains(t, body, "additionalModelResponseFieldPaths")
					}
					inference := body["inferenceConfig"].(map[string]any)
					wantMax := maxTokens
					if tc.thinking {
						assert.NotContains(t, inference, "temperature")
						assert.NotContains(t, inference, "topP")
						assert.NotContains(t, inference, "topK")
						require.Len(t, warnings, 3)
						for i, feature := range []string{"temperature", "topP", "topK"} {
							assert.Equal(t, feature, warnings[i].Feature)
						}
						if tc.budget >= 0 {
							wantMax += tc.budget
						}
					} else {
						assert.Equal(t, temperature, inference["temperature"])
						assert.Equal(t, topP, inference["topP"])
						assert.Equal(t, float64(topK), inference["topK"])
						assert.Empty(t, warnings)
					}
					assert.Equal(t, float64(wantMax), inference["maxTokens"])
					fields := tc.fields
					if fields == "" && tc.thinking {
						raw, err := json.Marshal(map[string]any{"thinking": map[string]any{"type": "enabled", "budget_tokens": tc.budget}})
						require.NoError(t, err)
						fields = string(raw)
					}
					if fields == "" {
						assert.NotContains(t, body, "additionalModelRequestFields")
					} else {
						raw, err := json.Marshal(body["additionalModelRequestFields"])
						require.NoError(t, err)
						assert.JSONEq(t, fields, string(raw))
					}
					after, err := json.Marshal(opts)
					require.NoError(t, err)
					assert.JSONEq(t, string(before), string(after))
				})
			}
		})
	}
}

func TestBuildRequest_ProfileClassificationStages(t *testing.T) {
	for _, reasoning := range []provider.ReasoningEffort{provider.ReasoningProviderDefault, provider.ReasoningNone} {
		opts := provider.CallOptions{
			Prompt: []provider.Message{provider.UserText("Hi")}, Reasoning: reasoning,
			ProviderOptions: rawBedrockOptions(`{"reasoningConfig":{"type":"enabled","budgetTokens":0}}`),
			ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceRequired},
			Tools: []provider.Tool{
				{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)},
				{Type: provider.ToolTypeProvider, Name: "terminal", ID: "anthropic.bash_20250124"},
			},
		}
		req, warnings, _ := mustBuildRequest(t, testApplicationProfile, opts)
		assert.Equal(t, []string{"/delta/stop_sequence"}, req.AdditionalModelResponseFieldPaths)
		require.NotNil(t, req.ToolConfig)
		if reasoning == provider.ReasoningNone {
			require.Len(t, req.ToolConfig.Tools, 1)
			require.Len(t, warnings, 1)
			assert.Equal(t, "tool anthropic.bash_20250124", warnings[0].Feature)
			assert.NotContains(t, req.AdditionalModelRequestFields, "tool_choice")
			require.NotNil(t, req.ToolConfig.ToolChoice)
		} else {
			require.Len(t, req.ToolConfig.Tools, 2)
			assert.Empty(t, warnings)
			assert.Nil(t, req.ToolConfig.ToolChoice)
			assert.Equal(t, map[string]any{"type": "any"}, req.AdditionalModelRequestFields["tool_choice"])
		}
	}
}

func TestModel_ProfileStructuredOutput(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       string
		reasoning provider.ReasoningEffort
		native    bool
	}{
		{"enabled zero", `{"reasoningConfig":{"type":"enabled","budgetTokens":0}}`, provider.ReasoningProviderDefault, true},
		{"budget without thinking", `{"reasoningConfig":{"budgetTokens":1024}}`, provider.ReasoningProviderDefault, false},
		{"root none", `{"reasoningConfig":{"type":"enabled","budgetTokens":1024}}`, provider.ReasoningNone, false},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/generate", true: "/stream"}[streaming], func(t *testing.T) {
				opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}, ProviderOptions: rawBedrockOptions(tc.raw), Reasoning: tc.reasoning, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}}
				body, warnings := captureBedrockRequest(t, testApplicationProfile, opts, streaming)
				assert.Empty(t, warnings)
				if tc.native {
					assert.NotContains(t, body, "toolConfig")
					fields := body["additionalModelRequestFields"].(map[string]any)
					assert.Contains(t, fields["output_config"], "format")
				} else {
					require.Contains(t, body, "toolConfig")
					tools := body["toolConfig"].(map[string]any)
					assert.Equal(t, map[string]any{"any": map[string]any{}}, tools["toolChoice"])
					assert.Equal(t, "json", tools["tools"].([]any)[0].(map[string]any)["toolSpec"].(map[string]any)["name"])
				}
			})
		}
	}
}

func TestBuildRequest_ProfileNullBudget(t *testing.T) {
	_, _, _, err := buildRequest(testApplicationProfile, provider.CallOptions{ProviderOptions: rawBedrockOptions(`{"reasoningConfig":{"type":"enabled","budgetTokens":null}}`)})
	require.ErrorContains(t, err, "budgetTokens")
}
