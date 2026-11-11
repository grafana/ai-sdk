package execution

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/provider"
)

// ErrInvalidOutput reports provider output that breaks an output lifecycle
// rule. [ClassifyProvider] classifies it as internal, and the call's [View]
// for it is the view of a successful call: the provider call succeeded and
// the Gateway refused its output.
var ErrInvalidOutput = errors.New("execution: invalid provider output")

func registeredFinishReason(reason provider.UnifiedFinishReason) bool {
	switch reason {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonInterrupted,
		provider.FinishReasonOther:
		return true
	default:
		return false
	}
}

func validToolResult(result json.RawMessage) bool {
	return json.Valid(result) && !bytes.Equal(bytes.TrimSpace(result), []byte("null"))
}

func isTrue(value *bool) bool { return value != nil && *value }
