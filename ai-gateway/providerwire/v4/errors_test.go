package v4

import (
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/stretchr/testify/assert"
)

func TestSafeErrorFor_Routing(t *testing.T) {
	for _, tc := range []struct {
		name     string
		category execution.Category
		reason   execution.Reason
		want     safeErrorCategory
	}{
		{"routing controls", execution.CategoryInvalidRouting, execution.ReasonRoutingControls, safeRoutingControls},
		{"provider timeouts", execution.CategoryInvalidRouting, execution.ReasonProviderTimeouts, safeProviderTimeouts},
		{"no candidates", execution.CategoryInvalidRouting, execution.ReasonNoCandidates, safeNoCandidates},
		{"attempt bound", execution.CategoryInvalidRouting, execution.ReasonAttemptBound, safeAttemptBound},
		{"fallback model", execution.CategoryModelNotFound, execution.ReasonFallbackModel, safeFallbackModelNotFound},
		{"requested model", execution.CategoryModelNotFound, execution.ReasonNone, safeModelNotFound},
		{"routing without reason", execution.CategoryInvalidRouting, execution.ReasonNone, safeInternal},
		{"routing with policy reason", execution.CategoryInvalidRouting, execution.ReasonProtectedCallHeader, safeInternal},
		{"routing reason on another category", execution.CategoryInvalidRequest, execution.ReasonNoCandidates, safeInvalidRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, safeErrorFor(tc.category, tc.reason).category)
		})
	}
}
