package agentobservability

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHookDenialError_IsUnwrap(t *testing.T) {
	assert.Equal(t, "agent observability: hook denied request", ErrHookDenied.Error())

	tests := []struct {
		name string
		err  *HookDenialError
		want string
	}{
		{
			name: "with reason and rule",
			err:  &HookDenialError{Reason: "policy violation", RuleID: "rule-42"},
			want: "Agent Observability hook denied by rule rule-42: policy violation",
		},
		{
			name: "with reason only",
			err:  &HookDenialError{Reason: "policy violation"},
			want: "Agent Observability hook denied: policy violation",
		},
		{
			name: "empty reason",
			err:  &HookDenialError{RuleID: "rule-42"},
			want: "Agent Observability hook denied by rule rule-42: request blocked by an Agent Observability hook rule",
		},
		{
			name: "zero value",
			err:  &HookDenialError{},
			want: "Agent Observability hook denied: request blocked by an Agent Observability hook rule",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.err.Error())
			assert.True(t, errors.Is(tc.err, ErrHookDenied), "errors.Is should detect ErrHookDenied")

			var asTarget *HookDenialError
			assert.True(t, errors.As(tc.err, &asTarget), "errors.As should extract *HookDenialError")
			assert.Equal(t, tc.err.Reason, asTarget.Reason)
			assert.Equal(t, tc.err.RuleID, asTarget.RuleID)
		})
	}
}

func TestHookDenialError_WithCause(t *testing.T) {
	cause := fmt.Errorf("upstream rpc: deadline exceeded")
	denial := &HookDenialError{Reason: "policy", RuleID: "rule-1", Cause: cause}

	assert.True(t, errors.Is(denial, ErrHookDenied))
	assert.True(t, errors.Is(denial, cause), "Cause should be in the unwrap chain")

	var asTarget *HookDenialError
	assert.True(t, errors.As(denial, &asTarget))
	assert.Same(t, cause, asTarget.Cause)
}

func TestHookDenialError_WrappedByFmtErrorf(t *testing.T) {
	denial := &HookDenialError{Reason: "policy", RuleID: "rule-1"}
	wrapped := fmt.Errorf("middleware: %w", denial)

	assert.True(t, errors.Is(wrapped, ErrHookDenied))

	var asTarget *HookDenialError
	assert.True(t, errors.As(wrapped, &asTarget))
	assert.Equal(t, "policy", asTarget.Reason)
	assert.Equal(t, "rule-1", asTarget.RuleID)
}
