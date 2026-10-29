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
		want     safeError
	}{
		{"routing controls", execution.CategoryInvalidRequest, execution.ReasonRoutingControls, safeError{category: safeInvalidRequest, reason: routingControls}},
		{"no candidates", execution.CategoryInvalidRequest, execution.ReasonNoCandidates, safeError{category: safeInvalidRequest, reason: routingNoCandidates}},
		{"attempt bound", execution.CategoryInvalidRequest, execution.ReasonAttemptBound, safeError{category: safeInvalidRequest, reason: routingAttemptBound}},
		{"fallback model", execution.CategoryModelNotFound, execution.ReasonFallbackModel, safeError{category: safeModelNotFound, reason: routingFallbackModel}},
		{"requested model", execution.CategoryModelNotFound, execution.ReasonNone, safeError{category: safeModelNotFound}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, safeErrorFor(tc.category, tc.reason))
		})
	}
}
