package bedrock

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortableReasoningMax(t *testing.T) {
	for _, tc := range []struct {
		model string
		want  string
	}{
		{"anthropic.claude-sonnet-4-6", "max"},
		{"amazon.nova-2-lite-v1:0", "high"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var warnings []provider.Warning
			config := resolveReasoningConfig(tc.model, isAnthropicModel(tc.model), provider.ReasoningMax, nil, &warnings)
			require.NotNil(t, config)
			assert.Equal(t, tc.want, config.MaxReasoningEffort)
			if tc.want == "high" {
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnCompatibility, warnings[0].Type)
			}
		})
	}
	budget, ok := reasoningBudget("anthropic.claude-sonnet-4-5", provider.ReasoningMax)
	require.True(t, ok)
	assert.Equal(t, 60800, budget)
}
