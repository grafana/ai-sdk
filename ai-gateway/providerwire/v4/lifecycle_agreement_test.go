package v4

import (
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamVerdict is the index of the rejected part, or -1, and the finished
// phase after each accepted part.
type streamVerdict struct {
	rejectAt int
	finished []bool
}

func runtimeVerdict(tc lifecycleStreamCase) streamVerdict {
	verdict := streamVerdict{rejectAt: -1}
	validator := execution.NewStreamValidator(tc.history)
	for i, part := range tc.parts {
		if validator.Check(part) != nil {
			verdict.rejectAt = i
			return verdict
		}
		verdict.finished = append(verdict.finished, validator.SegmentFinished())
	}
	return verdict
}

// providerWireVerdict runs the parts through ProviderWire's own state the way
// runStream does: a real first stream-start is checked before the loop.
func providerWireVerdict(t *testing.T, tc lifecycleStreamCase) streamVerdict {
	t.Helper()
	h := &handler{}
	recorder := httptest.NewRecorder()
	state := newStreamState(tc.history)
	verdict := streamVerdict{rejectAt: -1}
	parts := tc.parts
	offset := 0
	if len(parts) > 0 && parts[0].Type == provider.PartStreamStart {
		require.NoError(t, state.validator.Check(parts[0]), "runStream accepts a real first stream-start")
		verdict.finished = append(verdict.finished, false)
		parts, offset = parts[1:], 1
	}
	for i, part := range parts {
		switch h.processStreamPart(recorder, state, part) {
		case streamPartAdapterFailure:
			verdict.rejectAt = i + offset
			return verdict
		case streamPartWriterFailure:
			require.Fail(t, "writer failure")
		}
		verdict.finished = append(verdict.finished, state.phase == segmentFinished)
	}
	return verdict
}

func TestLifecycleAgreement_Stream(t *testing.T) {
	for _, tc := range lifecycleStreamCases() {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, runtimeVerdict(tc), providerWireVerdict(t, tc))
		})
	}
}

func TestLifecycleAgreement_Unary(t *testing.T) {
	for _, tc := range lifecycleUnaryCases() {
		t.Run(tc.name, func(t *testing.T) {
			runtimeErr := execution.ValidateGenerateResult(tc.result, tc.history)
			_, mapErr := mapUnarySuccess(tc.result, unaryMappingContext{history: tc.history})
			assert.Equal(t, runtimeErr != nil, mapErr != nil)
		})
	}
}
