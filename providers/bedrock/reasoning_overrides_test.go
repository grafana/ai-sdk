package bedrock

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_ExplicitReasoningOverrides(t *testing.T) {
	for _, namespace := range []string{"amazonBedrock", "bedrock"} {
		for _, reasoning := range []provider.ReasoningEffort{provider.ReasoningMinimal, provider.ReasoningHigh, provider.ReasoningXHigh, provider.ReasoningMax} {
			for _, tc := range []struct {
				name      string
				modelID   string
				config    string
				want      string
				maxTokens int
			}{
				{"adaptive effort", "anthropic.claude-sonnet-4-6-v1:0", `{"maxReasoningEffort":"low"}`, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`, 1024},
				{"budget only", testAnthropicModel, `{"budgetTokens":5000}`, `{"thinking":{"type":"enabled","budget_tokens":5000}}`, 6024},
				{"zero budget", testAnthropicModel, `{"budgetTokens":0}`, `{"thinking":{"type":"enabled","budget_tokens":0}}`, 1024},
				{"Nova effort", "us.amazon.nova-2-lite-v1:0", `{"maxReasoningEffort":"low"}`, `{"reasoningConfig":{"type":"enabled","maxReasoningEffort":"low"}}`, 1024},
				{"OpenAI OSS effort", testOpenAIModel, `{"maxReasoningEffort":"low"}`, `{"reasoning_effort":"low"}`, 1024},
				{"OpenAI effort", "openai.gpt-5", `{"maxReasoningEffort":"low"}`, `{"reasoning":{"effort":"low"}}`, 1024},
				{"unknown explicit effort", testMistralModel, `{"maxReasoningEffort":"low"}`, `{"reasoningConfig":{"maxReasoningEffort":"low"}}`, 1024},
			} {
				t.Run(fmt.Sprintf("%s/%s/%s", namespace, reasoning, tc.name), func(t *testing.T) {
					maxTokens := 1024
					opts := provider.CallOptions{
						Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: reasoning, MaxOutputTokens: &maxTokens,
						ProviderOptions: provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(`{"reasoningConfig":` + tc.config + `}`)}},
					}
					req, warnings, _ := mustBuildRequest(t, tc.modelID, opts)
					fields, err := json.Marshal(req.AdditionalModelRequestFields)
					require.NoError(t, err)
					assert.JSONEq(t, tc.want, string(fields))
					require.NotNil(t, req.InferenceConfig)
					assert.Equal(t, tc.maxTokens, *req.InferenceConfig.MaxTokens)
					assert.Empty(t, warnings)
					assert.Equal(t, 1024, maxTokens)
				})
			}
		}
	}
}

func TestBuildRequest_ExplicitDisabledReasoning(t *testing.T) {
	for _, modelID := range []string{testAnthropicModel, "anthropic.claude-sonnet-4-6-v1:0", "global.anthropic.claude-sonnet-5-5", "amazon.nova-2-lite-v1:0", testOpenAIModel, testMistralModel} {
		for _, reasoning := range []provider.ReasoningEffort{provider.ReasoningNone, provider.ReasoningMinimal, provider.ReasoningHigh, provider.ReasoningXHigh, provider.ReasoningMax} {
			for _, namespace := range []string{"amazonBedrock", "bedrock"} {
				t.Run(fmt.Sprintf("%s/%s/%s", modelID, reasoning, namespace), func(t *testing.T) {
					config := ReasoningConfig{Type: "disabled", BudgetTokens: 5000, MaxReasoningEffort: "low"}
					before := config
					maxTokens := 1024
					req, warnings, _ := mustBuildRequest(t, modelID, provider.CallOptions{
						Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: reasoning, MaxOutputTokens: &maxTokens,
						ProviderOptions: provider.ProviderOptions{namespace: BedrockOptions{ReasoningConfig: &config}},
					})
					assert.Empty(t, req.AdditionalModelRequestFields)
					require.NotNil(t, req.InferenceConfig)
					assert.Equal(t, 1024, *req.InferenceConfig.MaxTokens)
					assert.Empty(t, warnings)
					assert.Equal(t, before, config)
				})
			}
		}
	}
}

func TestBuildRequest_ReasoningOverrideNamespacePrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modern provider.ProviderOption
		want   string
	}{
		{"absent", nil, "low"},
		{"null", provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`null`)}, "low"},
		{"modern explicit", BedrockOptions{ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "medium"}}, "medium"},
		{"modern empty prevents merging", BedrockOptions{}, "high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, testOpenAIModel, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningHigh,
				ProviderOptions: provider.ProviderOptions{
					"amazonBedrock": tc.modern,
					"bedrock":       BedrockOptions{ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "low"}},
				},
			})
			assert.Equal(t, map[string]any{"reasoning_effort": tc.want}, req.AdditionalModelRequestFields)
			assert.Empty(t, warnings)
		})
	}
}

func TestBuildRequest_ReasoningConfigReuse(t *testing.T) {
	for _, namespace := range []string{"amazonBedrock", "bedrock"} {
		for _, form := range []string{"value", "pointer", "raw"} {
			for _, reasoning := range []provider.ReasoningEffort{provider.ReasoningProviderDefault, provider.ReasoningMax} {
				t.Run(fmt.Sprintf("%s/%s/%s", namespace, form, reasoning), func(t *testing.T) {
					config := ReasoningConfig{Type: reasoningTypeBetweenTools, MaxReasoningEffort: "max"}
					before := config
					bo := BedrockOptions{ReasoningConfig: &config}
					var option provider.ProviderOption = bo
					switch form {
					case "pointer":
						option = &bo
					case "raw":
						option = provider.RawProviderOption{Key: namespace, Raw: jsonRawOrZero(bo)}
					}
					optionBefore := jsonRawOrZero(option)
					opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: reasoning, ProviderOptions: provider.ProviderOptions{namespace: option}}
					check := func(t *testing.T) {
						req, warnings, _ := mustBuildRequest(t, "global.anthropic.claude-sonnet-5-5", opts)
						assert.Equal(t, map[string]any{"thinking": map[string]any{"type": reasoningTypeBetweenTools}, "output_config": map[string]any{"effort": "high"}}, req.AdditionalModelRequestFields)
						require.Len(t, warnings, 1)
						assert.Equal(t, "providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort", warnings[0].Feature)
					}
					check(t)
					check(t)
					assert.Equal(t, before, config)
					t.Run("concurrent", func(t *testing.T) {
						for i := range 8 {
							t.Run(fmt.Sprint(i), func(t *testing.T) { t.Parallel(); check(t) })
						}
					})
					assert.Equal(t, before, config)
					assert.Equal(t, optionBefore, jsonRawOrZero(option))
				})
			}
		}
	}
}

func TestBuildRequest_ReasoningDefaultControls(t *testing.T) {
	for _, tc := range []struct {
		name      string
		reasoning provider.ReasoningEffort
		config    *ReasoningConfig
		want      string
	}{
		{"omitted", provider.ReasoningProviderDefault, nil, `null`},
		{"default preserves explicit effort", provider.ReasoningProviderDefault, &ReasoningConfig{MaxReasoningEffort: "low"}, `{"output_config":{"effort":"low"}}`},
		{"default disabled leaves explicit effort", provider.ReasoningProviderDefault, &ReasoningConfig{Type: "disabled", BudgetTokens: 5000, MaxReasoningEffort: "low"}, `{"output_config":{"effort":"low"}}`},
		{"none removes partial controls", provider.ReasoningNone, &ReasoningConfig{BudgetTokens: 5000, MaxReasoningEffort: "low"}, `null`},
		{"typed zero derives budget", provider.ReasoningHigh, &ReasoningConfig{}, `{"thinking":{"type":"enabled","budget_tokens":38400}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := cloneReasoningConfig(tc.config)
			req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.reasoning,
				ProviderOptions: provider.BuildProviderOptions(BedrockOptions{ReasoningConfig: tc.config}),
			})
			fields, err := json.Marshal(req.AdditionalModelRequestFields)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(fields))
			assert.Empty(t, warnings)
			assert.Equal(t, before, tc.config)
		})
	}
}
