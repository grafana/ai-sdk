package bedrock

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAnthropicModel = "anthropic.claude-sonnet-4-5-20250929-v1:0"
const testMistralModel = "mistral.mistral-large-2407-v1:0"
const testNovaModel = "amazon.nova-lite-v1:0"
const testOpenAIModel = "openai.gpt-oss-20251101-v1:0"

func mustBuildRequest(t *testing.T, modelID string, opts provider.CallOptions) (*converseInput, []provider.Warning, requestMeta) {
	t.Helper()
	req, warnings, meta, err := buildRequest(modelID, opts)
	require.NoError(t, err)
	return req, warnings, meta
}

func warningFeatures(warnings []provider.Warning) []string {
	features := make([]string, 0, len(warnings))
	for _, w := range warnings {
		features = append(features, w.Feature)
	}
	return features
}

func marshalRequestBody(t *testing.T, req *converseInput) map[string]json.RawMessage {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields))
	return fields
}

func TestBuildRequest_SystemMessage(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("you are a helpful assistant"),
			provider.UserText("hi"),
		},
	})
	require.Len(t, req.System, 1)
	assert.Equal(t, "you are a helpful assistant", req.System[0].Text)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	require.Len(t, req.Messages[0].Content, 1)
	assert.Equal(t, "hi", req.Messages[0].Content[0].Text)
}

func TestBuildRequest_InferenceConfig(t *testing.T) {
	temp := 0.5
	topP := 0.9
	topK := 20
	maxTok := 512
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		MaxOutputTokens: &maxTok,
		Temperature:     &temp,
		TopP:            &topP,
		TopK:            &topK,
		StopSequences:   []string{"END"},
	})
	require.NotNil(t, req.InferenceConfig)
	assert.Equal(t, 512, *req.InferenceConfig.MaxTokens)
	assert.Equal(t, 0.5, *req.InferenceConfig.Temperature)
	assert.Equal(t, 0.9, *req.InferenceConfig.TopP)
	assert.Equal(t, 20, *req.InferenceConfig.TopK)
	assert.Equal(t, []string{"END"}, req.InferenceConfig.StopSequences)
	assert.Empty(t, warnings)
}

func TestBuildRequest_TemperatureClamping(t *testing.T) {
	tooHigh := 2.0
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:      []provider.Message{provider.UserText("x")},
		Temperature: &tooHigh,
	})
	require.NotNil(t, req.InferenceConfig)
	assert.Equal(t, 1.0, *req.InferenceConfig.Temperature)
	require.Len(t, warnings, 1)
	assert.Equal(t, "temperature", warnings[0].Feature)
	assert.Contains(t, warnings[0].Details, "clamped to 1.0")

	tooLow := -0.5
	req2, warnings2, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:      []provider.Message{provider.UserText("x")},
		Temperature: &tooLow,
	})
	assert.Equal(t, 0.0, *req2.InferenceConfig.Temperature)
	require.Len(t, warnings2, 1)
	assert.Contains(t, warnings2[0].Details, "clamped to 0")
}

func TestBuildRequest_UnsupportedParams(t *testing.T) {
	freq := 0.5
	pres := 0.5
	seed := 42
	_, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:           []provider.Message{provider.UserText("x")},
		FrequencyPenalty: &freq,
		PresencePenalty:  &pres,
		Seed:             &seed,
	})
	features := map[string]bool{}
	for _, w := range warnings {
		features[w.Feature] = true
	}
	assert.True(t, features["frequencyPenalty"])
	assert.True(t, features["presencePenalty"])
	assert.True(t, features["seed"])
}

func TestBuildRequest_ToolsAndToolChoice(t *testing.T) {
	cases := []struct {
		name         string
		toolChoice   provider.ToolChoice
		assertChoice func(t *testing.T, c *toolChoiceUnion)
	}{
		{
			name:       "auto",
			toolChoice: provider.ToolChoice{Type: provider.ToolChoiceAuto},
			assertChoice: func(t *testing.T, c *toolChoiceUnion) {
				require.NotNil(t, c)
				assert.NotNil(t, c.Auto)
				assert.Nil(t, c.Any)
			},
		},
		{
			name:       "required",
			toolChoice: provider.ToolChoice{Type: provider.ToolChoiceRequired},
			assertChoice: func(t *testing.T, c *toolChoiceUnion) {
				require.NotNil(t, c)
				assert.NotNil(t, c.Any)
				assert.Nil(t, c.Auto)
			},
		},
		{
			name:       "specific tool",
			toolChoice: provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "weather"},
			assertChoice: func(t *testing.T, c *toolChoiceUnion) {
				require.NotNil(t, c)
				require.NotNil(t, c.Tool)
				assert.Equal(t, "weather", c.Tool.Name)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("x")},
				Tools: []provider.Tool{
					{Type: provider.ToolTypeFunction, Name: "weather", Description: "get weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
				},
				ToolChoice: &tc.toolChoice,
			})
			require.NotNil(t, req.ToolConfig)
			require.Len(t, req.ToolConfig.Tools, 1)
			require.NotNil(t, req.ToolConfig.Tools[0].ToolSpec)
			assert.Equal(t, "weather", req.ToolConfig.Tools[0].ToolSpec.Name)
			tc.assertChoice(t, req.ToolConfig.ToolChoice)
		})
	}
}

func TestBuildRequest_FunctionToolGuards(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather", Description: "   ", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})
	require.NotNil(t, req.ToolConfig)
	require.Len(t, req.ToolConfig.Tools, 1)
	spec := req.ToolConfig.Tools[0].ToolSpec
	require.NotNil(t, spec)
	assert.Empty(t, spec.Description)
}

func TestBuildRequest_FunctionToolStrict(t *testing.T) {
	strictTrue := true
	strictFalse := false
	tests := []struct {
		name        string
		modelID     string
		strict      *bool
		want        *bool
		wantWarning bool
	}{
		{name: "absent", modelID: testAnthropicModel},
		{name: "true", modelID: testAnthropicModel, strict: &strictTrue, want: &strictTrue},
		{name: "false", modelID: testAnthropicModel, strict: &strictFalse, want: &strictFalse},
		{name: "unsupported opus 4.7 true", modelID: "anthropic.claude-opus-4-7", strict: &strictTrue, wantWarning: true},
		{name: "unsupported opus 4.8 false", modelID: "anthropic.claude-opus-4-8", strict: &strictFalse, wantWarning: true},
		{name: "unsupported regional opus 5 true", modelID: "us.anthropic.claude-opus-5", strict: &strictTrue, wantWarning: true},
		{name: "unsupported fable 5 false", modelID: "eu.anthropic.claude-fable-5", strict: &strictFalse, wantWarning: true},
		{name: "unsupported sonnet 5 true", modelID: "anthropic.claude-sonnet-5", strict: &strictTrue, wantWarning: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, tc.modelID, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("x")},
				Tools: []provider.Tool{{
					Type:        provider.ToolTypeFunction,
					Name:        "weather",
					Strict:      tc.strict,
					InputSchema: json.RawMessage(`{"type":"object"}`),
				}},
			})
			require.NotNil(t, req.ToolConfig)
			require.Len(t, req.ToolConfig.Tools, 1)
			spec := req.ToolConfig.Tools[0].ToolSpec
			require.NotNil(t, spec)
			assert.Equal(t, tc.want, spec.Strict)
			if tc.wantWarning {
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
				assert.Equal(t, "strict", warnings[0].Feature)
				assert.Contains(t, warnings[0].Details, "strict mode is not supported")
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestBuildRequest_AnthropicThinkingEnabled(t *testing.T) {
	bo := BedrockOptions{
		ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 2048},
	}
	temp := 0.7
	maxTok := 1024
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		Temperature:     &temp,
		MaxOutputTokens: &maxTok,
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	require.NotNil(t, req.AdditionalModelRequestFields)
	thinking, ok := req.AdditionalModelRequestFields["thinking"].(map[string]any)
	require.True(t, ok, "thinking field present")
	assert.Equal(t, "enabled", thinking["type"])
	assert.Equal(t, 2048, thinking["budget_tokens"])
	// MaxTokens increased by the budget.
	require.NotNil(t, req.InferenceConfig)
	assert.Equal(t, 1024+2048, *req.InferenceConfig.MaxTokens)
	// Temperature dropped + warning emitted.
	assert.Nil(t, req.InferenceConfig.Temperature)
	hasTempWarn := false
	for _, w := range warnings {
		if w.Feature == "temperature" {
			hasTempWarn = true
			assert.Contains(t, w.Details, "thinking is enabled")
		}
	}
	assert.True(t, hasTempWarn, "temperature warning should be emitted")
}

func TestBuildRequest_TopLevelReasoningAnthropicThinking(t *testing.T) {
	tests := []struct {
		name          string
		modelID       string
		wantType      string
		wantBudget    int
		wantEffort    string
		wantMaxTokens int
	}{
		{
			name:          "older model uses budget tokens",
			modelID:       "anthropic.claude-sonnet-4-5-20250929-v1:0",
			wantType:      "enabled",
			wantBudget:    38400,
			wantMaxTokens: 38400 + 4096,
		},
		{
			name:       "adaptive model uses effort",
			modelID:    "anthropic.claude-sonnet-4-6-v1:0",
			wantType:   "adaptive",
			wantEffort: "high",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reasoning := provider.ReasoningHigh
			req, warnings, _ := mustBuildRequest(t, tc.modelID, provider.CallOptions{
				Prompt:    []provider.Message{provider.UserText("x")},
				Reasoning: &reasoning,
			})
			assert.Empty(t, warnings)
			require.NotNil(t, req.AdditionalModelRequestFields)
			thinking, ok := req.AdditionalModelRequestFields["thinking"].(map[string]any)
			require.True(t, ok, "thinking field present")
			assert.Equal(t, tc.wantType, thinking["type"])
			if tc.wantBudget > 0 {
				assert.Equal(t, tc.wantBudget, thinking["budget_tokens"])
				require.NotNil(t, req.InferenceConfig)
				require.NotNil(t, req.InferenceConfig.MaxTokens)
				assert.Equal(t, tc.wantMaxTokens, *req.InferenceConfig.MaxTokens)
			} else {
				assert.NotContains(t, thinking, "budget_tokens")
				assert.Nil(t, req.InferenceConfig)
			}
			if tc.wantEffort != "" {
				outputConfig, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
				require.True(t, ok, "output_config field present")
				assert.Equal(t, tc.wantEffort, outputConfig["effort"])
			}
		})
	}
}

func TestBuildRequest_TopLevelReasoningMergesProviderConfig(t *testing.T) {
	t.Run("partial display preserves adaptive derivation", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, warnings, _ := mustBuildRequest(t, "anthropic.claude-sonnet-4-6-v1:0", provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Display: "summarized"},
			}),
		})
		assert.Empty(t, warnings)
		thinking, ok := req.AdditionalModelRequestFields["thinking"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"type": "adaptive", "display": "summarized"}, thinking)
		outputConfig, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "high", outputConfig["effort"])
	})

	t.Run("none overrides partial config", func(t *testing.T) {
		reasoning := provider.ReasoningNone
		req, warnings, _ := mustBuildRequest(t, "anthropic.claude-sonnet-4-6-v1:0", provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Display: "summarized"},
			}),
		})
		assert.Empty(t, warnings)
		assert.NotContains(t, req.AdditionalModelRequestFields, "thinking")
		assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
	})

	t.Run("explicit enabled type retains derived effort", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, warnings, _ := mustBuildRequest(t, "anthropic.claude-sonnet-4-6-v1:0", provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 3000},
			}),
		})
		assert.Empty(t, warnings)
		thinking, ok := req.AdditionalModelRequestFields["thinking"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"type": "enabled", "budget_tokens": 3000}, thinking)
		outputConfig, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "high", outputConfig["effort"])
		require.NotNil(t, req.InferenceConfig)
		require.NotNil(t, req.InferenceConfig.MaxTokens)
		assert.Equal(t, 7096, *req.InferenceConfig.MaxTokens)
	})

	t.Run("explicit disabled type clears derived values", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, warnings, _ := mustBuildRequest(t, "anthropic.claude-sonnet-4-6-v1:0", provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Type: "disabled", BudgetTokens: 3000},
			}),
		})
		assert.Empty(t, warnings)
		assert.NotContains(t, req.AdditionalModelRequestFields, "thinking")
		assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
		assert.Nil(t, req.InferenceConfig)
	})

	t.Run("explicit effort overrides non-Anthropic derivation", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, warnings, _ := mustBuildRequest(t, testNovaModel, provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "low"},
			}),
		})
		assert.Empty(t, warnings)
		reasoningConfig, ok := req.AdditionalModelRequestFields["reasoningConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "low", reasoningConfig["maxReasoningEffort"])
	})

	t.Run("explicit non-Anthropic fields are serialized with derived effort", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, warnings, _ := mustBuildRequest(t, testNovaModel, provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 3000},
			}),
		})
		require.Len(t, warnings, 1)
		assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
		assert.Equal(t, "budgetTokens", warnings[0].Feature)
		reasoningConfig, ok := req.AdditionalModelRequestFields["reasoningConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{
			"type":               "enabled",
			"budgetTokens":       3000,
			"maxReasoningEffort": "high",
		}, reasoningConfig)
	})

	t.Run("non-Anthropic budget is omitted when type is adaptive", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, warnings, _ := mustBuildRequest(t, testNovaModel, provider.CallOptions{
			Prompt:    []provider.Message{provider.UserText("x")},
			Reasoning: &reasoning,
			ProviderOptions: provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Type: "adaptive", BudgetTokens: 3000},
			}),
		})
		require.Len(t, warnings, 2)
		assert.Equal(t, []string{"budgetTokens", "adaptive thinking"}, []string{warnings[0].Feature, warnings[1].Feature})
		reasoningConfig, ok := req.AdditionalModelRequestFields["reasoningConfig"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, map[string]any{"maxReasoningEffort": "high"}, reasoningConfig)
	})
}

func TestReasoningBudget_UsesModelCapabilities(t *testing.T) {
	tests := []struct {
		modelID string
		want    int
	}{
		{"anthropic.claude-sonnet-4-5-20250929-v1:0", 38400},
		{"anthropic.claude-opus-4-1-v1:0", 19200},
		{"anthropic.claude-sonnet-4-0-v1:0", 38400},
		{"anthropic.claude-opus-4-0-v1:0", 19200},
		{"anthropic.claude-3-haiku-20240307-v1:0", 2458},
		{"anthropic.unknown-model-v1:0", 2458},
	}
	for _, tc := range tests {
		t.Run(tc.modelID, func(t *testing.T) {
			budget, ok := reasoningBudget(tc.modelID, provider.ReasoningHigh)
			require.True(t, ok)
			assert.Equal(t, tc.want, budget)
		})
	}
}

func TestBuildRequest_TopLevelReasoningNonAnthropicEffort(t *testing.T) {
	reasoning := provider.ReasoningMedium
	req, _, _ := mustBuildRequest(t, testOpenAIModel, provider.CallOptions{
		Prompt:    []provider.Message{provider.UserText("x")},
		Reasoning: &reasoning,
	})
	assert.Equal(t, "medium", req.AdditionalModelRequestFields["reasoning_effort"])

	req, _, _ = mustBuildRequest(t, testNovaModel, provider.CallOptions{
		Prompt:    []provider.Message{provider.UserText("x")},
		Reasoning: &reasoning,
	})
	rc, ok := req.AdditionalModelRequestFields["reasoningConfig"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "medium", rc["maxReasoningEffort"])
}

func TestBuildRequest_TopLevelReasoningNonAnthropicCompatibilityWarnings(t *testing.T) {
	tests := []struct {
		name          string
		reasoning     provider.ReasoningEffort
		wantEffort    string
		wantWarning   bool
		wantInDetails string
	}{
		{
			name:          "minimal maps to low with warning",
			reasoning:     provider.ReasoningMinimal,
			wantEffort:    "low",
			wantWarning:   true,
			wantInDetails: `reasoning "minimal" is not directly supported by this model. mapped to effort "low".`,
		},
		{
			name:       "low maps to low",
			reasoning:  provider.ReasoningLow,
			wantEffort: "low",
		},
		{
			name:       "medium maps to medium",
			reasoning:  provider.ReasoningMedium,
			wantEffort: "medium",
		},
		{
			name:       "high maps to high",
			reasoning:  provider.ReasoningHigh,
			wantEffort: "high",
		},
		{
			name:          "xhigh maps to max with warning",
			reasoning:     provider.ReasoningXHigh,
			wantEffort:    "max",
			wantWarning:   true,
			wantInDetails: `reasoning "xhigh" is not directly supported by this model. mapped to effort "max".`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, testOpenAIModel, provider.CallOptions{
				Prompt:    []provider.Message{provider.UserText("x")},
				Reasoning: &tc.reasoning,
			})
			assert.Equal(t, tc.wantEffort, req.AdditionalModelRequestFields["reasoning_effort"])
			if tc.wantWarning {
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnCompatibility, warnings[0].Type)
				assert.Equal(t, "reasoning", warnings[0].Feature)
				assert.Equal(t, tc.wantInDetails, warnings[0].Details)
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestBuildRequest_AnthropicOnlyOptionsOnNonAnthropicModel(t *testing.T) {
	bo := BedrockOptions{
		ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 2048},
	}
	req, warnings, _ := mustBuildRequest(t, testMistralModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	_, has := req.AdditionalModelRequestFields["thinking"]
	assert.False(t, has, "thinking must not be set for non-Anthropic models")
	found := false
	for _, w := range warnings {
		if w.Feature == "budgetTokens" {
			found = true
			assert.Contains(t, w.Details, "Anthropic models on Bedrock")
		}
	}
	assert.True(t, found, "budgetTokens warning required")
}

func TestBuildRequest_AnthropicEffortLevel(t *testing.T) {
	bo := BedrockOptions{
		ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "high"},
	}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	oc, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", oc["effort"])
}

func TestBuildRequest_OpenAIEffort(t *testing.T) {
	bo := BedrockOptions{
		ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "high"},
	}
	req, _, _ := mustBuildRequest(t, testOpenAIModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	assert.Equal(t, "high", req.AdditionalModelRequestFields["reasoning_effort"])
}

func TestBuildRequest_NovaEffort(t *testing.T) {
	bo := BedrockOptions{
		ReasoningConfig: &ReasoningConfig{MaxReasoningEffort: "high"},
	}
	req, _, _ := mustBuildRequest(t, testNovaModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	rc, ok := req.AdditionalModelRequestFields["reasoningConfig"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "high", rc["maxReasoningEffort"])
}

func TestBuildRequest_JSONResponseToolFallback(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"foo":{"type":"string"}}}`)
	req, _, meta := mustBuildRequest(t, "anthropic.claude-3-haiku-20240307-v1:0", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("give me JSON")},
		ResponseFormat: &provider.ResponseFormat{
			Type:   provider.ResponseFormatJSON,
			Schema: schema,
		},
	})
	require.True(t, meta.usesJSONResponseTool, "older Anthropic models fall back to json tool")
	require.NotNil(t, req.ToolConfig)
	require.Len(t, req.ToolConfig.Tools, 1)
	assert.Equal(t, "json", req.ToolConfig.Tools[0].ToolSpec.Name)
	require.NotNil(t, req.ToolConfig.ToolChoice)
	assert.NotNil(t, req.ToolConfig.ToolChoice.Any)
}

func TestBuildRequest_NativeStructuredOutputForSupportedAnthropic(t *testing.T) {
	schema := json.RawMessage(`{"type":"object"}`)
	req, _, meta := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		ResponseFormat: &provider.ResponseFormat{
			Type:   provider.ResponseFormatJSON,
			Schema: schema,
		},
	})
	assert.False(t, meta.usesJSONResponseTool, "native structured output should be used")
	oc, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
	require.True(t, ok)
	fmt2, _ := oc["format"].(map[string]any)
	require.NotNil(t, fmt2)
	assert.Equal(t, "json_schema", fmt2["type"])
}

func TestBuildRequest_NativeStructuredOutputSanitizesUnsupportedConstraints(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"labels":{"type":"array","maxItems":3,"items":{"type":"string"}}},"required":["labels"],"additionalProperties":true}`)
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:         []provider.Message{provider.UserText("x")},
		ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
	})

	outputConfig, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
	require.True(t, ok)
	format, ok := outputConfig["format"].(map[string]any)
	require.True(t, ok)
	sanitized, ok := format["schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, sanitized["additionalProperties"])
	properties, ok := sanitized["properties"].(map[string]any)
	require.True(t, ok)
	labels, ok := properties["labels"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, labels, "maxItems")
	assert.Equal(t, "max items: 3.", labels["description"])
}

func TestBuildRequest_Opus47And48StructuredOutputFallback(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"foo":{"type":"string"}}}`)
	models := []string{
		"anthropic.claude-opus-4-7-v1:0",
		"us.anthropic.claude-opus-4-8-v1:0",
		"eu.anthropic.claude-opus-4-8-v1:0",
	}

	for _, modelID := range models {
		t.Run(modelID+" without tools", func(t *testing.T) {
			req, _, meta := mustBuildRequest(t, modelID, provider.CallOptions{
				Prompt:         []provider.Message{provider.UserText("give me JSON")},
				ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
			})
			require.True(t, meta.usesJSONResponseTool)
			require.NotNil(t, req.ToolConfig)
			require.Len(t, req.ToolConfig.Tools, 1)
			assert.Equal(t, "json", req.ToolConfig.Tools[0].ToolSpec.Name)
			assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
		})
	}

	t.Run("with user tool injects instruction", func(t *testing.T) {
		for _, modelID := range []string{
			"anthropic.claude-opus-4-7-v1:0",
			"anthropic.claude-opus-4-8-v1:0",
			"us.anthropic.claude-opus-5",
			"anthropic.claude-fable-5-v1:0",
			"anthropic.claude-sonnet-5-v1:0",
		} {
			t.Run(modelID, func(t *testing.T) {
				req, _, meta := mustBuildRequest(t, modelID, provider.CallOptions{
					Prompt: []provider.Message{
						provider.NewSystemMessage("existing system"),
						provider.UserText("give me JSON"),
					},
					Tools:          []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}},
					ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
				})
				assert.False(t, meta.usesJSONResponseTool)
				require.NotNil(t, req.ToolConfig)
				require.Len(t, req.ToolConfig.Tools, 1)
				assert.Equal(t, "weather", req.ToolConfig.Tools[0].ToolSpec.Name)
				require.Len(t, req.System, 1)
				assert.Contains(t, req.System[0].Text, "existing system\n\nJSON schema:")
				assert.Contains(t, req.System[0].Text, string(schema))
				assert.Contains(t, req.System[0].Text, "Do not wrap it in markdown fences")
				assert.NotContains(t, req.AdditionalModelRequestFields, "output_config")
			})
		}
	})

	t.Run("empty system message is replaced by instruction", func(t *testing.T) {
		req, _, _ := mustBuildRequest(t, "anthropic.claude-opus-4-8-v1:0", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewSystemMessage(""),
				provider.UserText("give me JSON"),
			},
			Tools:          []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
		})
		require.Len(t, req.System, 1)
		assert.Contains(t, req.System[0].Text, "JSON schema:")
	})

	t.Run("other rejected models use json tool", func(t *testing.T) {
		for _, modelID := range []string{"us.anthropic.claude-opus-5", "anthropic.claude-fable-5-v1:0", "anthropic.claude-sonnet-5-v1:0"} {
			req, _, meta := mustBuildRequest(t, modelID, provider.CallOptions{
				Prompt:         []provider.Message{provider.UserText("give me JSON")},
				ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
			})
			require.True(t, meta.usesJSONResponseTool)
			require.NotNil(t, req.ToolConfig)
			assert.Equal(t, "json", req.ToolConfig.Tools[0].ToolSpec.Name)
			outputConfig, _ := req.AdditionalModelRequestFields["output_config"].(map[string]any)
			assert.NotContains(t, outputConfig, "format")
		}
	})

	t.Run("adaptive thinking does not enable rejected native format", func(t *testing.T) {
		reasoning := provider.ReasoningHigh
		req, _, meta := mustBuildRequest(t, "anthropic.claude-opus-4-8-v1:0", provider.CallOptions{
			Prompt:         []provider.Message{provider.UserText("give me JSON")},
			Reasoning:      &reasoning,
			ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: schema},
		})
		require.True(t, meta.usesJSONResponseTool)
		thinking, ok := req.AdditionalModelRequestFields["thinking"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "adaptive", thinking["type"])
		outputConfig, _ := req.AdditionalModelRequestFields["output_config"].(map[string]any)
		assert.NotContains(t, outputConfig, "format")
	})
}

func TestBuildRequest_ToolResultMessageCollapsedToUserRole(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("call the tool"),
			provider.NewAssistantMessage(provider.ToolCallPart("call-1", "weather", json.RawMessage(`{"city":"Berlin"}`))),
			provider.NewToolMessage(provider.ToolResultPart("call-1", "weather", &provider.ToolResultOutput{
				Type: provider.ToolOutputText, Text: "Sunny, 22C",
			})),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})
	require.Len(t, req.Messages, 3)
	// 0: original user; 1: assistant (tool call); 2: user (tool result).
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Equal(t, "assistant", req.Messages[1].Role)
	require.NotNil(t, req.Messages[1].Content[0].ToolUse)
	assert.Equal(t, "weather", req.Messages[1].Content[0].ToolUse.Name)
	// Tool message collapses to user role with toolResult content.
	assert.Equal(t, "user", req.Messages[2].Role)
	require.NotNil(t, req.Messages[2].Content[0].ToolResult)
	assert.Equal(t, "call-1", req.Messages[2].Content[0].ToolResult.ToolUseID)
	assert.Equal(t, "Sunny, 22C", req.Messages[2].Content[0].ToolResult.Content[0].Text)
}

func toolResultFileCallOptions(file provider.ToolResultContentValue) provider.CallOptions {
	return provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ToolResultPart("call-123", "document-reader", &provider.ToolResultOutput{
				Type:    provider.ToolOutputContent,
				Content: []provider.ToolResultContentValue{file},
			})),
		},
		Tools: []provider.Tool{{
			Type:        provider.ToolTypeFunction,
			Name:        "document-reader",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	}
}

func TestBuildRequest_ToolResultDocument(t *testing.T) {
	tests := []struct {
		name            string
		mediaType       string
		filename        string
		providerOptions provider.ProviderOptions
		expectedFormat  string
		expectedName    string
		expectCitations bool
	}{
		{
			name:           "PDF",
			mediaType:      "application/pdf",
			filename:       "tool-result.v1.pdf",
			expectedFormat: "pdf",
			expectedName:   "tool-result",
		},
		{
			name:      "citations",
			mediaType: "text/markdown",
			providerOptions: provider.BuildProviderOptions(FilePartOptions{
				Citations: &FilePartCitations{Enabled: true},
			}),
			expectedFormat:  "md",
			expectedName:    "document-1",
			expectCitations: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
				Type:            provider.ToolContentFile,
				Data:            &provider.DataContent{Base64: "base64data"},
				MediaType:       tt.mediaType,
				Filename:        tt.filename,
				ProviderOptions: tt.providerOptions,
			}))

			require.Len(t, req.Messages, 1)
			require.Len(t, req.Messages[0].Content, 1)
			result := req.Messages[0].Content[0].ToolResult
			require.NotNil(t, result)
			require.Len(t, result.Content, 1)
			document := result.Content[0].Document
			require.NotNil(t, document)
			assert.Equal(t, tt.expectedFormat, document.Format)
			assert.Equal(t, tt.expectedName, document.Name)
			assert.Equal(t, "base64data", document.Source.Bytes)
			if tt.expectCitations {
				require.NotNil(t, document.Citations)
				assert.True(t, document.Citations.Enabled)
			} else {
				assert.Nil(t, document.Citations)
			}
			assert.Empty(t, warnings)
		})
	}
}

func TestBuildRequest_ToolResultImage(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
		Type:      provider.ToolContentFile,
		Data:      &provider.DataContent{Base64: "base64data"},
		MediaType: "image/jpeg",
	}))

	require.Len(t, req.Messages, 1)
	result := req.Messages[0].Content[0].ToolResult
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	image := result.Content[0].Image
	require.NotNil(t, image)
	assert.Equal(t, "jpeg", image.Format)
	assert.Equal(t, "base64data", image.Source.Bytes)
	assert.Empty(t, warnings)
}

func TestBuildRequest_ToolResultEmptyData(t *testing.T) {
	emptyData := provider.Base64DataContent("")
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
		Type:      provider.ToolContentFile,
		Data:      &emptyData,
		MediaType: "image/jpeg",
	}))

	require.Len(t, req.Messages, 1)
	result := req.Messages[0].Content[0].ToolResult
	require.NotNil(t, result)
	require.Len(t, result.Content, 1)
	require.NotNil(t, result.Content[0].Image)
	assert.Empty(t, result.Content[0].Image.Source.Bytes)
	assert.Empty(t, warnings)
}

func TestBuildRequest_UnsupportedToolResultFileMediaType(t *testing.T) {
	tests := []struct {
		mediaType     string
		expectedError string
	}{
		{
			mediaType:     "image/avif",
			expectedError: `bedrock: image media type "image/avif" is not supported`,
		},
		{
			mediaType:     "unsupported/mime-type",
			expectedError: `bedrock: file media type "unsupported/mime-type" is not supported`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			_, _, _, err := buildRequest(testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
				Type:      provider.ToolContentFile,
				Data:      &provider.DataContent{Base64: "base64data"},
				MediaType: tt.mediaType,
			}))

			require.EqualError(t, err, tt.expectedError)
		})
	}
}

func TestBuildRequest_DocumentNamesShareCounterWithToolResults(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.FilePart("application/pdf", provider.DataContent{Base64: "AAECAw=="})),
			provider.NewAssistantMessage(provider.ToolCallPart("call-123", "document-reader", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("call-123", "document-reader", &provider.ToolResultOutput{
				Type: provider.ToolOutputContent,
				Content: []provider.ToolResultContentValue{{
					Type:      provider.ToolContentFile,
					Data:      &provider.DataContent{Base64: "base64data"},
					MediaType: "application/pdf",
				}},
			})),
		},
		Tools: []provider.Tool{{
			Type:        provider.ToolTypeFunction,
			Name:        "document-reader",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}},
	})

	require.Len(t, req.Messages, 3)
	assert.Equal(t, "document-1", req.Messages[0].Content[0].Document.Name)
	assert.Equal(t, "document-2", req.Messages[2].Content[0].ToolResult.Content[0].Document.Name)
	assert.Empty(t, warnings)
}

func TestBuildRequest_InvalidToolCallInput(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(provider.ToolCallPart("call-1", "cityAttractions", json.RawMessage(`{ "city": "San Francisco", }`))),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "cityAttractions", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})

	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	block := req.Messages[0].Content[0].ToolUse
	require.NotNil(t, block)
	assert.Equal(t, "call-1", block.ToolUseID)
	assert.Equal(t, "cityAttractions", block.Name)
	assert.JSONEq(t, `{"rawInvalidInput":"{ \"city\": \"San Francisco\", }"}`, string(block.Input))
}

func TestBuildRequest_MistralNormalizesToolCallId(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testMistralModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("call the tool"),
			provider.NewAssistantMessage(provider.ToolCallPart("tooluse_bpe71yCfRu2b5i-nKGDr5g", "weather", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("tooluse_bpe71yCfRu2b5i-nKGDr5g", "weather", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})),
		},
		Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	assert.Equal(t, "toolusebp", req.Messages[1].Content[0].ToolUse.ToolUseID)
	assert.Equal(t, "toolusebp", req.Messages[2].Content[0].ToolResult.ToolUseID)
}

func TestBuildRequest_ImageMessage(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.FilePart("image/png", provider.DataContent{Base64: "iVBOR..."})),
		},
	})
	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	require.NotNil(t, req.Messages[0].Content[0].Image)
	assert.Equal(t, "png", req.Messages[0].Content[0].Image.Format)
	assert.Equal(t, "iVBOR...", req.Messages[0].Content[0].Image.Source.Bytes)
	assert.Empty(t, warnings)
}

func TestBuildRequest_VideoMessage(t *testing.T) {
	t.Run("inline", func(t *testing.T) {
		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.FilePart("video/mp4", provider.DataContent{Base64: "AAECAw=="})),
			},
		})
		require.Len(t, req.Messages, 1)
		require.Len(t, req.Messages[0].Content, 1)
		video := req.Messages[0].Content[0].Video
		require.NotNil(t, video)
		assert.Equal(t, "mp4", video.Format)
		assert.Equal(t, "AAECAw==", video.Source.Bytes)
		assert.Empty(t, warnings)
	})

	t.Run("S3", func(t *testing.T) {
		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.FilePart("video/webm", provider.DataContent{URL: "s3://bucket/video.webm"})),
			},
		})
		video := req.Messages[0].Content[0].Video
		require.NotNil(t, video)
		assert.Equal(t, "webm", video.Format)
		require.NotNil(t, video.Source.S3Location)
		assert.Equal(t, "s3://bucket/video.webm", video.Source.S3Location.URI)
		assert.Empty(t, warnings)
	})

	t.Run("unsupported media type", func(t *testing.T) {
		_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(provider.FilePart("video/unsupported", provider.DataContent{Base64: "AAECAw=="})),
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `video media type "video/unsupported" is not supported`)
	})
}

func TestBuildRequest_ToolResultVideo(t *testing.T) {
	t.Run("inline", func(t *testing.T) {
		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
			Type:      provider.ToolContentFileData,
			Data:      "AAECAw==",
			MediaType: "video/mp4",
		}))
		result := req.Messages[0].Content[0].ToolResult
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		video := result.Content[0].Video
		require.NotNil(t, video)
		assert.Equal(t, "mp4", video.Format)
		assert.Equal(t, "AAECAw==", video.Source.Bytes)
		assert.Empty(t, warnings)
	})

	t.Run("S3", func(t *testing.T) {
		req, warnings, _ := mustBuildRequest(t, testAnthropicModel, toolResultFileCallOptions(provider.ToolResultContentValue{
			Type:      provider.ToolContentFileURL,
			URL:       "s3://bucket/video.mov",
			MediaType: "video/quicktime",
		}))
		result := req.Messages[0].Content[0].ToolResult
		require.NotNil(t, result)
		require.Len(t, result.Content, 1)
		video := result.Content[0].Video
		require.NotNil(t, video)
		assert.Equal(t, "mov", video.Format)
		require.NotNil(t, video.Source.S3Location)
		assert.Equal(t, "s3://bucket/video.mov", video.Source.S3Location.URI)
		assert.Empty(t, warnings)
	})
}

func TestBuildRequest_TopLevelMediaTypes(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		base64    string
		check     func(t *testing.T, block contentBlock)
	}{
		{
			name:      "image",
			mediaType: "image",
			base64:    "iVBORw0KGgo=",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Image)
				assert.Equal(t, "png", block.Image.Format)
			},
		},
		{
			name:      "URL-safe unpadded image",
			mediaType: "image",
			base64:    "_9g",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Image)
				assert.Equal(t, "jpeg", block.Image.Format)
			},
		},
		{
			name:      "mixed-alphabet image",
			mediaType: "image",
			base64:    "_9g+",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Image)
				assert.Equal(t, "jpeg", block.Image.Format)
			},
		},
		{
			name:      "image with whitespace",
			mediaType: "image",
			base64:    " /9\tg= \n",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Image)
				assert.Equal(t, "jpeg", block.Image.Format)
			},
		},
		{
			name:      "valid signature with invalid suffix",
			mediaType: "image",
			base64:    "iVBORw0KGgoAAAAAAAAAAAAA%%%",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Image)
				assert.Equal(t, "png", block.Image.Format)
			},
		},
		{
			name:      "video",
			mediaType: "video",
			base64:    "AAAAGGZ0eXA=",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Video)
				assert.Equal(t, "mp4", block.Video.Format)
			},
		},
		{
			name:      "application",
			mediaType: "application",
			base64:    "JVBERi0xLjQ=",
			check: func(t *testing.T, block contentBlock) {
				require.NotNil(t, block.Document)
				assert.Equal(t, "pdf", block.Document.Format)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{
					provider.NewUserMessage(provider.FilePart(tt.mediaType, provider.DataContent{Base64: tt.base64})),
				},
			})

			require.Len(t, req.Messages, 1)
			require.Len(t, req.Messages[0].Content, 1)
			tt.check(t, req.Messages[0].Content[0])
			assert.Empty(t, warnings)
		})
	}
}

func TestBuildRequest_DocumentMediaTypes(t *testing.T) {
	tests := []struct {
		mediaType string
		format    string
	}{
		{mediaType: "application/pdf", format: "pdf"},
		{mediaType: "text/csv", format: "csv"},
		{mediaType: "application/msword", format: "doc"},
		{mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", format: "docx"},
		{mediaType: "application/vnd.ms-excel", format: "xls"},
		{mediaType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", format: "xlsx"},
		{mediaType: "text/html", format: "html"},
		{mediaType: "text/plain", format: "txt"},
		{mediaType: "text/markdown", format: "md"},
	}
	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{
					provider.NewUserMessage(provider.FilePart(tt.mediaType, provider.DataContent{Base64: "AAECAw=="})),
				},
			})

			require.Len(t, req.Messages, 1)
			require.Len(t, req.Messages[0].Content, 1)
			document := req.Messages[0].Content[0].Document
			require.NotNil(t, document)
			assert.Equal(t, tt.format, document.Format)
			assert.Equal(t, "document-1", document.Name)
			assert.Equal(t, "AAECAw==", document.Source.Bytes)
			assert.Empty(t, warnings)
		})
	}
}

func TestBuildRequest_NamedDocumentDoesNotAdvanceGeneratedName(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.ContentPart{
					Type:      provider.ContentPartTypeFile,
					MediaType: "application/pdf",
					Filename:  "named.pdf",
					Data:      &provider.DataContent{Base64: "AAECAw=="},
				},
				provider.FilePart("application/pdf", provider.DataContent{Base64: "AAECAw=="}),
			),
		},
	})

	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 2)
	assert.Equal(t, "named", req.Messages[0].Content[0].Document.Name)
	assert.Equal(t, "document-1", req.Messages[0].Content[1].Document.Name)
	assert.Empty(t, warnings)
}

func TestBuildRequest_TextDocumentData(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{
				Type:      provider.ContentPartTypeFile,
				MediaType: "text",
				Filename:  "notes.txt",
				Data:      &provider.DataContent{Text: "hello"},
			}),
		},
	})

	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	document := req.Messages[0].Content[0].Document
	require.NotNil(t, document)
	assert.Equal(t, "txt", document.Format)
	assert.Equal(t, "notes", document.Name)
	assert.Equal(t, "aGVsbG8=", document.Source.Bytes)
	assert.Empty(t, warnings)
}

func TestBuildRequest_UnsupportedDocumentMediaType(t *testing.T) {
	_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.FilePart("application/octet-stream", provider.DataContent{Base64: "AAECAw=="})),
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `media type "application/octet-stream"`)
}

func TestBuildRequest_MalformedFileOptionsTakePrecedenceOverUnsupportedMediaType(t *testing.T) {
	part := provider.FilePart("application/octet-stream", provider.DataContent{Base64: "AAECAw=="})
	part.ProviderOptions = provider.ProviderOptions{
		"amazonBedrock": provider.RawProviderOption{
			Key: "amazonBedrock",
			Raw: json.RawMessage(`{"citations":"invalid"}`),
		},
	}

	_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.NewUserMessage(part)},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid provider options")
	assert.NotContains(t, err.Error(), `media type "application/octet-stream"`)
}

func TestBuildRequest_UnsupportedTextDocumentMediaType(t *testing.T) {
	_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.ContentPart{
				Type:      provider.ContentPartTypeFile,
				MediaType: "application/octet-stream",
				Data:      &provider.DataContent{Text: "hello"},
			}),
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `media type "application/octet-stream"`)
}

func TestBuildRequest_UnsupportedImageMediaTypes(t *testing.T) {
	for _, mediaType := range []string{"image/jpg", "image/avif"} {
		t.Run(mediaType, func(t *testing.T) {
			_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{
					provider.NewUserMessage(provider.FilePart(mediaType, provider.DataContent{Base64: "AAECAw=="})),
				},
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf(`media type %q`, mediaType))
		})
	}
}

func TestBuildRequest_TopLevelMediaTypeCannotBeDetected(t *testing.T) {
	_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(provider.FilePart("image", provider.DataContent{Base64: "AAEC"})),
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), `media type "image"`)
	assert.Contains(t, err.Error(), "could not be auto-detected")
}

func TestBuildRequest_UnsupportedFileDataVariants(t *testing.T) {
	tests := []struct {
		name string
		data provider.DataContent
	}{
		{name: "URL", data: provider.DataContent{URL: "https://example.com/x.png"}},
		{name: "provider reference", data: provider.DataContent{Reference: json.RawMessage(`{"bedrock":"file-ref-123"}`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{
					provider.NewUserMessage(provider.FilePart("image/png", tt.data)),
				},
			})

			require.Error(t, err)
			assert.Contains(t, err.Error(), "not supported")
		})
	}
}

func TestBuildRequest_AdditionalModelRequestFieldsPreservesDerived(t *testing.T) {
	bo := BedrockOptions{
		ReasoningConfig:              &ReasoningConfig{Type: "enabled", BudgetTokens: 2048},
		AdditionalModelRequestFields: map[string]any{"thinking": map[string]any{"type": "custom", "display": "visible"}},
	}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	v, _ := req.AdditionalModelRequestFields["thinking"].(map[string]any)
	assert.Equal(t, "enabled", v["type"])
	assert.Equal(t, 2048, v["budget_tokens"])
	assert.Equal(t, "visible", v["display"])
}

func TestBuildRequest_AdditionalModelRequestFieldsMergesOutputConfig(t *testing.T) {
	bo := BedrockOptions{
		AdditionalModelRequestFields: map[string]any{
			"output_config": map[string]any{
				"format": map[string]any{"type": "caller"},
				"extra":  true,
			},
		},
	}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		ResponseFormat: &provider.ResponseFormat{
			Type:   provider.ResponseFormatJSON,
			Schema: json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`),
		},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	oc, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
	require.True(t, ok)
	format, ok := oc["format"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "json_schema", format["type"])
	assert.Equal(t, true, oc["extra"])
}

func TestBuildRequest_AdditionalModelResponseFieldPathsAnthropicOnly(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
	})
	assert.Equal(t, []string{"/delta/stop_sequence"}, req.AdditionalModelResponseFieldPaths)

	req2, _, _ := mustBuildRequest(t, testNovaModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
	})
	assert.Empty(t, req2.AdditionalModelResponseFieldPaths)
}

func TestBuildRequest_FilterToolContentWhenNoTools(t *testing.T) {
	// No tools but the prompt carries tool-call/result parts. They must be
	// stripped and a warning emitted.
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("hi"),
			provider.NewAssistantMessage(provider.ToolCallPart("c1", "weather", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("c1", "weather", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})),
		},
	})
	// Only the user message survives.
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	found := false
	for _, w := range warnings {
		if w.Feature == "toolContent" {
			found = true
		}
	}
	assert.True(t, found, "toolContent warning must be emitted")
}

func TestBuildRequest_FilterToolContentAfterToolChoiceNone(t *testing.T) {
	none := provider.ToolChoice{Type: provider.ToolChoiceNone}
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("hi"),
			provider.NewAssistantMessage(provider.ToolCallPart("c1", "weather", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("c1", "weather", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		ToolChoice: &none,
	})
	assert.Nil(t, req.ToolConfig)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Contains(t, warningFeatures(warnings), "toolContent")
}

func TestBuildRequest_TrimsOnlyFinalAssistantPrefillText(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("initial user  \n"),
			provider.NewAssistantMessage(provider.TextPart("earlier assistant  \n")),
			provider.UserText("follow-up user  \n"),
			provider.NewAssistantMessage(provider.TextPart("earlier in final assistant block  \n")),
			provider.NewAssistantMessage(
				provider.TextPart("preserved assistant part  \n"),
				provider.TextPart("  final prefill  \n\n"),
			),
		},
	})

	require.Len(t, req.Messages, 4)
	assert.Equal(t, "initial user  \n", req.Messages[0].Content[0].Text)
	assert.Equal(t, "earlier assistant  \n", req.Messages[1].Content[0].Text)
	assert.Equal(t, "follow-up user  \n", req.Messages[2].Content[0].Text)
	require.Len(t, req.Messages[3].Content, 3)
	assert.Equal(t, "earlier in final assistant block  \n", req.Messages[3].Content[0].Text)
	assert.Equal(t, "preserved assistant part  \n", req.Messages[3].Content[1].Text)
	assert.Equal(t, "final prefill", req.Messages[3].Content[2].Text)
}

func TestBuildRequest_AssistantWhitespaceMatchesECMAScript(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "byte order mark is trimmed", text: "\ufeff  prefill  \ufeff", want: "prefill"},
		{name: "next line is preserved", text: "\u0085", want: "\u0085"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(provider.TextPart(tc.text))},
			})

			require.Len(t, req.Messages, 1)
			require.Len(t, req.Messages[0].Content, 1)
			assert.Equal(t, tc.want, req.Messages[0].Content[0].Text)
		})
	}
}

func TestBuildRequest_PreservesSignedReasoningWhitespace(t *testing.T) {
	reasoning := provider.ReasoningPart("signed reasoning  \n")
	reasoning.ProviderOptions = provider.BuildProviderOptions(ReasoningMetadata{Signature: "sig-1"})
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.NewAssistantMessage(reasoning)},
	})

	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	require.NotNil(t, req.Messages[0].Content[0].ReasoningContent)
	require.NotNil(t, req.Messages[0].Content[0].ReasoningContent.ReasoningText)
	assert.Equal(t, "signed reasoning  \n", req.Messages[0].Content[0].ReasoningContent.ReasoningText.Text)
	assert.Equal(t, "sig-1", req.Messages[0].Content[0].ReasoningContent.ReasoningText.Signature)
}

func TestBuildRequest_SkipsUnsignedReasoning(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(provider.ReasoningPart("private chain"), provider.TextPart("answer")),
		},
	})
	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	assert.Equal(t, "answer", req.Messages[0].Content[0].Text)
}

func TestBuildRequest_ToolResultErrorsDoNotSetStatus(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewToolMessage(provider.ToolResultPart("c1", "weather", &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: "boom"})),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
	})
	require.Len(t, req.Messages, 1)
	require.Len(t, req.Messages[0].Content, 1)
	require.NotNil(t, req.Messages[0].Content[0].ToolResult)
	assert.Empty(t, req.Messages[0].Content[0].ToolResult.Status)
}

func TestBuildRequest_TopLevelProviderOptionsPassthrough(t *testing.T) {
	optionJSON := json.RawMessage(`{
		"guardrailConfig": {
			"guardrailIdentifier": "guardrail-id",
			"guardrailVersion": "1",
			"trace": "enabled",
			"streamProcessingMode": "async"
		},
		"performanceConfig": {"latency": "optimized"},
		"anthropicBeta": ["computer-use-2025-01-24"],
		"cachePoint": {"type": "default"},
		"serviceTier": "priority",
		"additionalModelRequestFields": {"custom": true}
	}`)

	for _, key := range providerOptionKeys {
		t.Run(key, func(t *testing.T) {
			req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("x")},
				ProviderOptions: provider.ProviderOptions{
					key: provider.RawProviderOption{Key: key, Raw: optionJSON},
				},
			})
			fields := marshalRequestBody(t, req)
			assert.JSONEq(t, `{
				"guardrailIdentifier": "guardrail-id",
				"guardrailVersion": "1",
				"trace": "enabled",
				"streamProcessingMode": "async"
			}`, string(fields["guardrailConfig"]))
			assert.JSONEq(t, `{"latency":"optimized"}`, string(fields["performanceConfig"]))
			assert.JSONEq(t, `["computer-use-2025-01-24"]`, string(fields["anthropicBeta"]))
			assert.JSONEq(t, `{"type":"default"}`, string(fields["cachePoint"]))
			assert.JSONEq(t, `{"type":"priority"}`, string(fields["serviceTier"]))
			assert.JSONEq(t, `{
				"custom": true,
				"anthropic_beta": ["computer-use-2025-01-24"]
			}`, string(fields["additionalModelRequestFields"]))
		})
	}
}

func TestBuildRequest_TopLevelProviderOptionsTypedMatchesRaw(t *testing.T) {
	tests := []struct {
		name    string
		beta    []string
		pointer bool
		raw     json.RawMessage
	}{
		{
			name: "non-empty beta list",
			beta: []string{"computer-use-2025-01-24"},
			raw:  json.RawMessage(`{"anthropicBeta":["computer-use-2025-01-24"],"cachePoint":{"type":"default"}}`),
		},
		{
			name:    "non-empty beta list pointer",
			beta:    []string{"computer-use-2025-01-24"},
			pointer: true,
			raw:     json.RawMessage(`{"anthropicBeta":["computer-use-2025-01-24"],"cachePoint":{"type":"default"}}`),
		},
		{
			name: "empty beta list",
			beta: []string{},
			raw:  json.RawMessage(`{"anthropicBeta":[],"cachePoint":{"type":"default"}}`),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typed := BedrockOptions{
				AnthropicBeta: tc.beta,
				CachePoint:    &CachePoint{Type: "default"},
			}
			var typedOption provider.ProviderOption = typed
			if tc.pointer {
				typedOption = &typed
			}
			typedReq, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt:          []provider.Message{provider.UserText("x")},
				ProviderOptions: provider.BuildProviderOptions(typedOption),
			})
			rawReq, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("x")},
				ProviderOptions: provider.ProviderOptions{
					"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: tc.raw},
				},
			})
			typedBody, err := json.Marshal(typedReq)
			require.NoError(t, err)
			rawBody, err := json.Marshal(rawReq)
			require.NoError(t, err)
			assert.JSONEq(t, string(rawBody), string(typedBody))
		})
	}
}

func TestBuildRequest_TopLevelProviderOptionsPreferModernNamespace(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.ProviderOptions{
			"bedrock": provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(`{
				"guardrailConfig": {"guardrailIdentifier": "legacy"},
				"legacyOnly": true
			}`)},
			"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`{
				"guardrailConfig": {"guardrailIdentifier": "modern"}
			}`)},
		},
	})
	fields := marshalRequestBody(t, req)
	assert.JSONEq(t, `{"guardrailIdentifier":"modern"}`, string(fields["guardrailConfig"]))
	assert.NotContains(t, fields, "legacyOnly")
}

func TestBuildRequest_TopLevelProviderOptionsFallBackFromNullModernNamespace(t *testing.T) {
	tests := []struct {
		name   string
		modern provider.ProviderOption
	}{
		{name: "raw null", modern: provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(`null`)}},
		{name: "empty raw value", modern: provider.RawProviderOption{Key: "amazonBedrock"}},
		{name: "nil value"},
		{name: "nil typed pointer", modern: (*BedrockOptions)(nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("x")},
				ProviderOptions: provider.ProviderOptions{
					"amazonBedrock": tc.modern,
					"bedrock": provider.RawProviderOption{
						Key: "bedrock",
						Raw: json.RawMessage(`{"guardrailConfig":{"guardrailIdentifier":"legacy"}}`),
					},
				},
			})
			fields := marshalRequestBody(t, req)
			assert.JSONEq(t, `{"guardrailIdentifier":"legacy"}`, string(fields["guardrailConfig"]))
		})
	}
}

func TestBuildRequest_TopLevelProviderOptionsDoNotOverrideActiveToolConfig(t *testing.T) {
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: json.RawMessage(`{"type":"object"}`)},
		},
		ProviderOptions: provider.ProviderOptions{
			"amazonBedrock": provider.RawProviderOption{
				Key: "amazonBedrock",
				Raw: json.RawMessage(`{"toolConfig":{"tools":[]}}`),
			},
		},
	})
	fields := marshalRequestBody(t, req)
	var config toolConfig
	require.NoError(t, json.Unmarshal(fields["toolConfig"], &config))
	require.Len(t, config.Tools, 1)
	assert.Equal(t, "weather", config.Tools[0].ToolSpec.Name)
}

func TestBuildRequest_ServiceTierPassthrough(t *testing.T) {
	bo := BedrockOptions{ServiceTier: "priority"}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	require.NotNil(t, req.ServiceTier)
	assert.Equal(t, "priority", req.ServiceTier.Type)
}

func TestBuildRequest_AnthropicWebSearchToolUnsupported(t *testing.T) {
	_, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeProvider, Name: "web_search", ID: "anthropic.web_search_20250305"},
		},
	})
	hasUnsupportedWarning := false
	for _, w := range warnings {
		if w.Feature == "web_search_20250305 tool" {
			hasUnsupportedWarning = true
		}
	}
	assert.True(t, hasUnsupportedWarning)
}

func TestBuildRequest_CachePointAfterSystem(t *testing.T) {
	cp := CachePoint{Type: "default", TTL: "5m"}
	bo := BedrockOptions{CachePoint: &cp}
	req, _, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			{
				Role:            provider.RoleSystem,
				Content:         []provider.ContentPart{provider.TextPart("system text")},
				ProviderOptions: provider.BuildProviderOptions(bo),
			},
			provider.UserText("x"),
		},
	})
	require.Len(t, req.System, 2)
	assert.Equal(t, "system text", req.System[0].Text)
	require.NotNil(t, req.System[1].CachePoint)
	assert.Equal(t, "default", req.System[1].CachePoint.Type)
	assert.Equal(t, "5m", req.System[1].CachePoint.TTL)
}

func TestBuildRequest_MultipleSystemMessagesSeparatedWarns(t *testing.T) {
	req, warnings, _ := mustBuildRequest(t, testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("first system"),
			provider.UserText("hi"),
			provider.NewSystemMessage("second system"),
		},
	})
	// Both system texts are still forwarded.
	require.Len(t, req.System, 2)
	assert.Equal(t, "first system", req.System[0].Text)
	assert.Equal(t, "second system", req.System[1].Text)

	found := false
	for _, w := range warnings {
		if w.Feature == "systemMessage" {
			found = true
			assert.Contains(t, w.Details, "separated by user/assistant")
		}
	}
	assert.True(t, found, "expected a systemMessage warning for separated system messages")
}

func TestBuildRequest_NativeStructuredOutputWhenThinkingEnabled(t *testing.T) {
	// An older Anthropic model that does NOT match supportsNativeStructuredOutput
	// markers, but with thinking enabled, should still use native structured
	// output (matching upstream's modelSupportsStructuredOutput || isThinkingEnabled).
	schema := json.RawMessage(`{"type":"object"}`)
	bo := BedrockOptions{ReasoningConfig: &ReasoningConfig{Type: "enabled", BudgetTokens: 1024}}
	req, _, meta := mustBuildRequest(t, "anthropic.claude-3-haiku-20240307-v1:0", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("x")},
		ResponseFormat: &provider.ResponseFormat{
			Type:   provider.ResponseFormatJSON,
			Schema: schema,
		},
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	assert.False(t, meta.usesJSONResponseTool, "thinking-enabled Anthropic should use native structured output")
	oc, ok := req.AdditionalModelRequestFields["output_config"].(map[string]any)
	require.True(t, ok)
	fmtObj, _ := oc["format"].(map[string]any)
	require.NotNil(t, fmtObj)
	assert.Equal(t, "json_schema", fmtObj["type"])
}

func TestBuildRequest_MalformedProviderOptionIsError(t *testing.T) {
	// A RawProviderOption with invalid JSON under the amazonBedrock key must
	// surface as a hard error rather than being silently ignored.
	opts := provider.ProviderOptions{
		"amazonBedrock": provider.RawProviderOption{
			Key: "amazonBedrock",
			Raw: json.RawMessage(`{"reasoningConfig": "not-an-object"}`),
		},
	}
	_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("x")},
		ProviderOptions: opts,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid provider options")
}

func TestBuildRequest_MalformedFilePartOptionIsError(t *testing.T) {
	// Invalid per-part provider option JSON must fail the build, not be
	// swallowed by a best-effort decode.
	badPart := provider.ContentPart{
		Type:      provider.ContentPartTypeFile,
		MediaType: "application/pdf",
		Data:      &provider.DataContent{Base64: "Zm9v"},
		ProviderOptions: provider.ProviderOptions{
			"amazonBedrock": provider.RawProviderOption{
				Key: "amazonBedrock",
				Raw: json.RawMessage(`{"citations": "nope"}`),
			},
		},
	}
	_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{
		Prompt: []provider.Message{
			{Role: provider.RoleUser, Content: []provider.ContentPart{badPart}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid provider options")
}
