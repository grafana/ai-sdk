package bedrock

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRequest_DisabledThinkingParity(t *testing.T) {
	const legacy = "anthropic.claude-3-5-sonnet-20241022-v2:0"
	const fable = "us.anthropic.claude-fable-5-v1:0"
	const opus = "us.anthropic.claude-opus-5-v1:0"
	const disabled = `{"reasoningConfig":{"type":"disabled"}}`
	const disabledWarning = "providerOptions.amazonBedrock.reasoningConfig"
	const typedEffortWarning = disabledWarning + ".maxReasoningEffort"
	const rawEffortWarning = "providerOptions.amazonBedrock.additionalModelRequestFields.output_config.effort"
	for _, tc := range []struct {
		name, model string
		reasoning   provider.ReasoningEffort
		options     string
		want        string
		warnings    []provider.Warning
	}{
		{"portable disabled", testAnthropicModel, provider.ReasoningNone, "", `{"thinking":{"type":"disabled"}}`, nil},
		{"explicit disabled", testAnthropicModel, provider.ReasoningProviderDefault, disabled, `{"thinking":{"type":"disabled"}}`, nil},
		{"portable legacy omission", legacy, provider.ReasoningNone, "", `null`, nil},
		{"explicit legacy omission", legacy, provider.ReasoningProviderDefault, disabled, `null`, nil},
		{"portable between tools", testSonnet55Model, provider.ReasoningNone, "", `{"thinking":{"type":"between_tools"}}`, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: "thinking cannot be disabled for " + testSonnet55Model + ". Using 'between_tools' thinking instead."}}},
		{"explicit between tools", testSonnet55Model, provider.ReasoningProviderDefault, disabled, `{"thinking":{"type":"between_tools"}}`, []provider.Warning{{Type: provider.WarnUnsupported, Feature: disabledWarning, Details: "thinking cannot be disabled for " + testSonnet55Model + ". Using 'between_tools' thinking instead."}}},
		{"bare base Fable", "anthropic.claude-fable-5", provider.ReasoningNone, "", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: "thinking cannot be disabled for anthropic.claude-fable-5. Using 'adaptive' thinking instead."}}},
		{"portable adaptive", fable, provider.ReasoningNone, "", `{"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}}`, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: "thinking cannot be disabled for " + fable + ". Using 'adaptive' thinking instead."}}},
		{"explicit adaptive", fable, provider.ReasoningProviderDefault, disabled, `{"thinking":{"type":"adaptive"}}`, []provider.Warning{{Type: provider.WarnUnsupported, Feature: disabledWarning, Details: "thinking cannot be disabled for " + fable + ". Using 'adaptive' thinking instead."}}},
		{"adaptive preserves raw effort", fable, provider.ReasoningNone, `{"additionalModelRequestFields":{"output_config":{"effort":"max"}}}`, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"max"}}`, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: "thinking cannot be disabled for " + fable + ". Using 'adaptive' thinking instead."}}},
		{"adaptive typed effort precedes raw", fable, provider.ReasoningNone, `{"reasoningConfig":{"maxReasoningEffort":"high"},"additionalModelRequestFields":{"output_config":{"effort":"max"}}}`, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: "thinking cannot be disabled for " + fable + ". Using 'adaptive' thinking instead."}}},
		{"disabled clamps typed xhigh", "anthropic.claude-haiku-5-5", provider.ReasoningProviderDefault, `{"reasoningConfig":{"type":"disabled","maxReasoningEffort":"xhigh","display":"summarized","budgetTokens":3000},"additionalModelRequestFields":{"thinking":{"type":"enabled","budget_tokens":2000},"output_config":{"effort":"low","custom":true}}}`, `{"thinking":{"type":"disabled"},"output_config":{"effort":"high","custom":true}}`, []provider.Warning{{Type: provider.WarnUnsupported, Feature: typedEffortWarning, Details: "effort 'xhigh' is not supported by anthropic.claude-haiku-5-5 with 'disabled' thinking. The effort has been lowered to 'high'."}}},
		{"base Opus caps max", opus, provider.ReasoningProviderDefault, `{"reasoningConfig":{"type":"disabled","maxReasoningEffort":"max"}}`, `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, []provider.Warning{{Type: provider.WarnUnsupported, Feature: typedEffortWarning, Details: "effort 'max' is not supported by " + opus + " with 'disabled' thinking. The effort has been lowered to 'high'."}}},
		{"between tools clamps raw max", testSonnet55Model, provider.ReasoningProviderDefault, `{"reasoningConfig":{"type":"disabled"},"additionalModelRequestFields":{"output_config":{"effort":"max","custom":true}}}`, `{"thinking":{"type":"between_tools"},"output_config":{"effort":"high","custom":true}}`, []provider.Warning{
			{Type: provider.WarnUnsupported, Feature: disabledWarning, Details: "thinking cannot be disabled for " + testSonnet55Model + ". Using 'between_tools' thinking instead."},
			{Type: provider.WarnUnsupported, Feature: rawEffortWarning, Details: "effort 'max' is not supported by " + testSonnet55Model + " with 'between_tools' thinking. The effort has been lowered to 'high'."},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokens := 1024
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.reasoning, MaxOutputTokens: &tokens}
			if tc.options != "" {
				opts.ProviderOptions = provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: json.RawMessage(tc.options)}}
			}
			req, warnings, _ := mustBuildRequest(t, tc.model, opts)
			fields, err := json.Marshal(req.AdditionalModelRequestFields)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(fields))
			assert.Equal(t, tc.warnings, warnings)
			require.NotNil(t, req.InferenceConfig)
			require.NotNil(t, req.InferenceConfig.MaxTokens)
			assert.Equal(t, tokens, *req.InferenceConfig.MaxTokens)
		})
	}
}

func TestBuildRequest_DisabledThinkingTypedPassThrough(t *testing.T) {
	const fable = "us.anthropic.claude-fable-5-v1:0"
	const opus = "us.anthropic.claude-opus-5-v1:0"
	for _, tc := range []struct {
		name, model string
		reasoning   provider.ReasoningEffort
		config      *ReasoningConfig
		output      any
		want        string
		warning     provider.Warning
	}{
		{"opaque fallback", fable, provider.ReasoningNone, nil,
			json.RawMessage(`{"effort":"max","custom":true}`),
			`{"thinking":{"type":"adaptive"},"output_config":{"effort":"max","custom":true}}`,
			provider.Warning{Type: provider.WarnCompatibility, Feature: "reasoning", Details: "thinking cannot be disabled for " + fable + ". Using 'adaptive' thinking instead."}},
		{"typed map cap", opus, provider.ReasoningProviderDefault, &ReasoningConfig{Type: "disabled"},
			map[string]string{"effort": "max", "custom": "kept"},
			`{"thinking":{"type":"disabled"},"output_config":{"effort":"high","custom":"kept"}}`,
			provider.Warning{Type: provider.WarnUnsupported, Feature: "providerOptions.amazonBedrock.additionalModelRequestFields.output_config.effort", Details: "effort 'max' is not supported by " + opus + " with 'disabled' thinking. The effort has been lowered to 'high'."}},
		{"opaque object cap", opus, provider.ReasoningNone, nil,
			json.RawMessage(`{"effort":"max","custom":9007199254740993}`),
			`{"thinking":{"type":"disabled"},"output_config":{"effort":"high","custom":9007199254740993}}`,
			provider.Warning{Type: provider.WarnUnsupported, Feature: "providerOptions.amazonBedrock.additionalModelRequestFields.output_config.effort", Details: "effort 'max' is not supported by " + opus + " with 'disabled' thinking. The effort has been lowered to 'high'."}},
		{"opaque member cap", opus, provider.ReasoningNone, nil,
			map[string]any{"effort": json.RawMessage(`"xhigh"`), "custom": true},
			`{"thinking":{"type":"disabled"},"output_config":{"effort":"high","custom":true}}`,
			provider.Warning{Type: provider.WarnUnsupported, Feature: "providerOptions.amazonBedrock.additionalModelRequestFields.output_config.effort", Details: "effort 'xhigh' is not supported by " + opus + " with 'disabled' thinking. The effort has been lowered to 'high'."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bo := BedrockOptions{ReasoningConfig: tc.config, AdditionalModelRequestFields: map[string]any{"output_config": tc.output}}
			before, err := json.Marshal(bo)
			require.NoError(t, err)
			opts := provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.reasoning,
				ProviderOptions: provider.BuildProviderOptions(bo),
			}
			for range 2 {
				req, warnings, _ := mustBuildRequest(t, tc.model, opts)
				fields, err := json.Marshal(req.AdditionalModelRequestFields)
				require.NoError(t, err)
				assert.JSONEq(t, tc.want, string(fields))
				if tc.name == "opaque object cap" {
					assert.Contains(t, string(fields), `"custom":9007199254740993`)
				}
				assert.Equal(t, []provider.Warning{tc.warning}, warnings)
			}
			after, err := json.Marshal(bo)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}

func TestBuildRequest_DisabledThinkingTypedPassThroughJSONTool(t *testing.T) {
	bo := BedrockOptions{
		StructuredOutputMode: StructuredOutputModeJSONTool,
		AdditionalModelRequestFields: map[string]any{
			"output_config": json.RawMessage(`{"effort":"max","custom":true,"format":{"type":"json_schema"}}`),
		},
	}
	req, warnings, _ := mustBuildRequest(t, "us.anthropic.claude-fable-5-v1:0", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningNone,
		ProviderOptions: provider.BuildProviderOptions(bo),
	})
	fields, err := json.Marshal(req.AdditionalModelRequestFields)
	require.NoError(t, err)
	assert.JSONEq(t, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"max","custom":true}}`, string(fields))
	require.Len(t, warnings, 1)
	assert.Equal(t, provider.WarnCompatibility, warnings[0].Type)
	assert.Equal(t, "reasoning", warnings[0].Feature)
	assert.JSONEq(t, `{"effort":"max","custom":true,"format":{"type":"json_schema"}}`, string(bo.AdditionalModelRequestFields["output_config"].(json.RawMessage)))
}

func TestBuildRequest_PortableNoneWithOtherFamilyConfig(t *testing.T) {
	const modelID = "amazon.nova-micro-v1:0"
	for _, tc := range []struct {
		name, config, want string
		derived            bool
		budgetWarning      bool
	}{
		{"empty config opts in", `{}`, `{"reasoningConfig":{"maxReasoningEffort":"low"}}`, true, false},
		{"explicit zero is preserved", `{"type":"enabled","budgetTokens":0}`, `{"reasoningConfig":{"type":"enabled","budgetTokens":0,"maxReasoningEffort":"low"}}`, true, true},
		{"budget without enabled type is omitted", `{"budgetTokens":0}`, `{"reasoningConfig":{"maxReasoningEffort":"low"}}`, true, true},
		{"explicit effort still takes precedence", `{"type":"enabled","maxReasoningEffort":"medium"}`, `{"reasoningConfig":{"type":"enabled","budgetTokens":5000,"maxReasoningEffort":"medium","stale":true}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := json.RawMessage(`{"reasoningConfig":` + tc.config + `,"additionalModelRequestFields":{"reasoningConfig":{"type":"disabled","budgetTokens":5000,"stale":true}}}`)
			req, warnings, _ := mustBuildRequest(t, modelID, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningNone,
				ProviderOptions: provider.ProviderOptions{"amazonBedrock": provider.RawProviderOption{Key: "amazonBedrock", Raw: raw}},
			})
			fields, err := json.Marshal(req.AdditionalModelRequestFields)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(fields))
			var wantWarnings []provider.Warning
			if tc.derived {
				wantWarnings = append(wantWarnings, provider.Warning{Type: provider.WarnCompatibility, Feature: "reasoning", Details: `reasoning "none" is not directly supported by this model. mapped to effort "low".`})
			}
			if tc.budgetWarning {
				wantWarnings = append(wantWarnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "budgetTokens", Details: "budgetTokens applies only to Anthropic models on Bedrock and will be ignored for this model."})
			}
			assert.Equal(t, wantWarnings, warnings)
		})
	}
}

func TestBuildRequest_DeferredClaudeReasoningUnchanged(t *testing.T) {
	for _, modelID := range []string{
		"global.anthropic.claude-fable-5-1",
		"us.anthropic.claude-fable-5-2-v1:0",
		"arn:aws:bedrock:us-east-1:123456789012:inference-profile/us.anthropic.claude-opus-5-5-v1:0",
	} {
		t.Run(modelID, func(t *testing.T) {
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningNone}
			req, warnings, _ := mustBuildRequest(t, modelID, opts)
			assert.Empty(t, req.AdditionalModelRequestFields)
			assert.Empty(t, warnings)

			opts.Reasoning = provider.ReasoningProviderDefault
			opts.ProviderOptions = provider.BuildProviderOptions(BedrockOptions{
				ReasoningConfig: &ReasoningConfig{Type: "disabled", MaxReasoningEffort: "max"},
			})
			req, warnings, _ = mustBuildRequest(t, modelID, opts)
			assert.Equal(t, map[string]any{"output_config": map[string]any{"effort": "max"}}, req.AdditionalModelRequestFields)
			assert.Empty(t, warnings)

			opts.Reasoning = provider.ReasoningXHigh
			opts.ProviderOptions = nil
			req, warnings, _ = mustBuildRequest(t, modelID, opts)
			assert.Equal(t, map[string]any{
				"thinking":      map[string]any{"type": "adaptive"},
				"output_config": map[string]any{"effort": "max"},
			}, req.AdditionalModelRequestFields)
			assert.Equal(t, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: `reasoning "xhigh" is not directly supported by this model. mapped to effort "max".`}}, warnings)
		})
	}
}

func TestBuildRequest_PortableReasoningFamilyRounding(t *testing.T) {
	const oss = "openai.gpt-oss-120b-1:0"
	const gpt = "us.openai.gpt-5.5"
	const nova = "us.amazon.nova-2-lite-v1:0"
	for _, tc := range []struct {
		model     string
		reasoning provider.ReasoningEffort
		want      string
		mapped    string
	}{
		{"anthropic.claude-opus-4-7", provider.ReasoningXHigh, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"xhigh"}}`, ""},
		{"anthropic.claude-sonnet-4-6", provider.ReasoningXHigh, `{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`, "high"},
		{oss, provider.ReasoningNone, `{"reasoning_effort":"low"}`, "low"},
		{oss, provider.ReasoningMinimal, `{"reasoning_effort":"low"}`, "low"},
		{oss, provider.ReasoningXHigh, `{"reasoning_effort":"high"}`, "high"},
		{oss, provider.ReasoningMax, `{"reasoning_effort":"high"}`, "high"},
		{gpt, provider.ReasoningNone, `{"reasoning":{"effort":"none"}}`, ""},
		{gpt, provider.ReasoningMinimal, `{"reasoning":{"effort":"minimal"}}`, ""},
		{gpt, provider.ReasoningXHigh, `{"reasoning":{"effort":"xhigh"}}`, ""},
		{gpt, provider.ReasoningMax, `{"reasoning":{"effort":"max"}}`, ""},
		{nova, provider.ReasoningNone, `null`, ""},
	} {
		t.Run(tc.model+"/"+string(tc.reasoning), func(t *testing.T) {
			req, warnings, _ := mustBuildRequest(t, tc.model, provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: tc.reasoning})
			fields, err := json.Marshal(req.AdditionalModelRequestFields)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(fields))
			if tc.mapped == "" {
				assert.Empty(t, warnings)
			} else {
				assert.Equal(t, []provider.Warning{{Type: provider.WarnCompatibility, Feature: "reasoning", Details: fmt.Sprintf("reasoning %q is not directly supported by this model. mapped to effort %q.", tc.reasoning, tc.mapped)}}, warnings)
			}
		})
	}

	t.Run("unsupported none", func(t *testing.T) {
		opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Reasoning: provider.ReasoningNone}
		req, warnings, _ := mustBuildRequest(t, "amazon.nova-micro-v1:0", opts)
		assert.Empty(t, req.AdditionalModelRequestFields)
		require.Len(t, warnings, 1)
		assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
		assert.Equal(t, "reasoning", warnings[0].Feature)
		opts.Reasoning = provider.ReasoningLow
		_, lowWarnings, _ := mustBuildRequest(t, "amazon.nova-micro-v1:0", opts)
		assert.Equal(t, lowWarnings, warnings)
	})
}
