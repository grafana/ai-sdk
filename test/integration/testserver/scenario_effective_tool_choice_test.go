package main

import (
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectiveToolChoiceScenarios_LocalEffects(t *testing.T) {
	for _, scenario := range effectiveToolChoiceScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			effects := 0
			result := streamEffectiveToolChoice(t.Context(), scenario.choice, scenario.callName, func() { effects++ })
			for part := range result.FullStream() {
				switch part.(type) {
				case aisdk.StreamToolApprovalRequest, aisdk.StreamToolApprovalResponse, aisdk.StreamToolOutputDenied:
					assert.Fail(t, "unexpected approval effect")
				}
			}
			if scenario.callName == "lookup" {
				require.NoError(t, result.Err())
				assert.Equal(t, 1, effects)
				assert.Len(t, result.ToolResults(), 1)
			} else {
				require.ErrorContains(t, result.Err(), "tool choice")
				assert.Zero(t, effects)
				assert.Empty(t, result.ToolResults())
			}
		})
	}
}
