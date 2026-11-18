package anthropic

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_PortableAnthropicReasoning(t *testing.T) {
	for _, tc := range []struct {
		modelID    string
		reasoning  provider.ReasoningEffort
		wantEffort string
		wantDetail string
	}{
		{"claude-sonnet-4-6", provider.ReasoningXHigh, "high", `reasoning "xhigh" is not directly supported by this model. mapped to effort "high".`},
		{"claude-opus-4-6", provider.ReasoningXHigh, "high", `reasoning "xhigh" is not directly supported by this model. mapped to effort "high".`},
		{"claude-opus-4-7", provider.ReasoningXHigh, "xhigh", ""},
		{"claude-opus-4-6", provider.ReasoningMinimal, "low", `reasoning "minimal" is not directly supported by this model. mapped to effort "low".`},
	} {
		t.Run(tc.modelID+"/"+string(tc.reasoning), func(t *testing.T) {
			p, _, warnings, _, err := buildParams(tc.modelID, provider.CallOptions{Reasoning: tc.reasoning}, false)
			require.NoError(t, err)
			assertThinkingJSON(t, p.Thinking, `{"type":"adaptive","display":"summarized"}`)
			assert.Equal(t, tc.wantEffort, string(p.OutputConfig.Effort))
			if tc.wantDetail != "" {
				assert.Equal(t, []provider.Warning{{
					Type: provider.WarnCompatibility, Feature: "reasoning", Details: tc.wantDetail,
				}}, warnings)
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func TestBuildParams_Haiku55AndBaseFable5Thinking(t *testing.T) {
	budget := &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 4000}
	disabled := &ThinkingConfig{Type: ThinkingDisabled}
	const haiku, fable = "claude-haiku-5-5", "claude-fable-5"
	tests := []struct {
		name         string
		modelID      string
		reasoning    provider.ReasoningEffort
		thinking     *ThinkingConfig
		effort       string
		wantThinking string
		wantEffort   string
		warnType     provider.WarningType
		warnFeature  string
		warnDetail   string
	}{
		{name: "haiku default", modelID: haiku},
		{
			name: "haiku budget", modelID: haiku, thinking: budget,
			wantThinking: `{"type":"adaptive"}`,
			warnType:     provider.WarnUnsupported, warnFeature: "providerOptions.anthropic.thinking",
			warnDetail: "budget-based thinking is not supported by %s. Using adaptive thinking instead. Use 'effort' to control how much the model thinks.",
		},
		{
			name: "haiku none", modelID: haiku, reasoning: provider.ReasoningNone,
			wantThinking: `{"type":"disabled"}`,
		},
		{
			name: "haiku explicit disabled", modelID: haiku, thinking: disabled, effort: "high",
			wantThinking: `{"type":"disabled"}`, wantEffort: "high",
		},
		{
			name: "haiku explicit disabled at xhigh", modelID: haiku, thinking: disabled, effort: "xhigh",
			wantThinking: `{"type":"disabled"}`, wantEffort: "high",
			warnType: provider.WarnUnsupported, warnFeature: "providerOptions.anthropic.effort",
			warnDetail: "effort 'xhigh' is not supported by %s when thinking is disabled. The effort has been lowered to 'high'.",
		},
		{
			name: "fable budget", modelID: fable, thinking: budget,
			wantThinking: `{"type":"adaptive"}`,
			warnType:     provider.WarnUnsupported, warnFeature: "providerOptions.anthropic.thinking",
			warnDetail: "budget-based thinking is not supported by %s; it always uses adaptive thinking. Using adaptive thinking instead. Use 'effort' to control how much the model thinks.",
		},
		{
			name: "fable explicit disabled", modelID: fable, thinking: disabled,
			warnType: provider.WarnUnsupported, warnFeature: "providerOptions.anthropic.thinking",
			warnDetail: "thinking cannot be disabled for %s; it always uses adaptive thinking. The thinking setting has been removed. Lower 'effort' to reduce thinking.",
		},
		{
			name: "fable none", modelID: fable, reasoning: provider.ReasoningNone,
			wantEffort: "low", warnType: provider.WarnCompatibility, warnFeature: "reasoning",
			warnDetail: "reasoning 'none' is not supported by %s; it always uses adaptive thinking. Using effort 'low' to minimize thinking instead.",
		},
		{
			name: "fable explicit effort takes precedence", modelID: fable, reasoning: provider.ReasoningNone,
			effort: "high", wantEffort: "high",
		},
	}
	for _, vertex := range []bool{false, true} {
		for _, tc := range tests {
			t.Run(fmt.Sprintf("%s/vertex=%t", tc.name, vertex), func(t *testing.T) {
				modelID, caps := tc.modelID, directProviderCapabilities
				if vertex {
					modelID, caps = ResolveVertexModelID(modelID), vertexProviderCapabilities
				}
				opts := provider.CallOptions{
					Reasoning: tc.reasoning,
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{
						Thinking: tc.thinking, Effort: tc.effort,
					}),
				}
				p, _, warnings, _, err := buildParamsWithCapabilities(modelID, opts, false, caps)
				require.NoError(t, err)
				data, err := json.Marshal(p)
				require.NoError(t, err)
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &body))
				assert.Equal(t, "128000", string(body["max_tokens"]))
				if tc.wantThinking != "" {
					assert.JSONEq(t, tc.wantThinking, string(body["thinking"]))
				} else {
					assert.NotContains(t, body, "thinking")
				}
				if tc.wantEffort != "" {
					assert.JSONEq(t, fmt.Sprintf(`{"effort":%q}`, tc.wantEffort), string(body["output_config"]))
				} else {
					assert.NotContains(t, body, "output_config")
				}
				if tc.warnDetail != "" {
					assert.Equal(t, []provider.Warning{{
						Type: tc.warnType, Feature: tc.warnFeature, Details: fmt.Sprintf(tc.warnDetail, modelID),
					}}, warnings)
				} else {
					assert.Empty(t, warnings)
				}
			})
		}
	}
}
