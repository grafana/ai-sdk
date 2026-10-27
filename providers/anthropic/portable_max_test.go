package anthropic

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPortableReasoningMax(t *testing.T) {
	var warnings []provider.Warning
	adaptive := resolveReasoningConfig(provider.ReasoningMax, getModelCapabilities("claude-sonnet-4-6"), &warnings)
	require.NotNil(t, adaptive)
	assert.Equal(t, "max", adaptive.effort)
	assert.Empty(t, warnings)
	budget := resolveReasoningConfig(provider.ReasoningMax, getModelCapabilities("claude-sonnet-4-5"), &warnings)
	require.NotNil(t, budget)
	require.NotNil(t, budget.thinking)
	assert.EqualValues(t, 60800, budget.thinking.BudgetTokens)
}
