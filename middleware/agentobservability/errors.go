package agentobservability

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrHookDenied is the sentinel error wrapped by every *HookDenialError. Use
	// errors.Is(err, agentobservability.ErrHookDenied) for generic deny detection.
	ErrHookDenied = errors.New("agent observability: hook denied request")
	// ErrHookTransformFailed indicates that a hook-provided replacement could
	// not be applied without losing or restoring request content.
	ErrHookTransformFailed = errors.New("agent observability: hook transform failed")
)

// HookDenialError is returned by HooksMiddleware when the Agent Observability
// preflight hook responds with action "deny". Consumers can use errors.As to
// recover Reason and RuleID; errors.Is(err, agentobservability.ErrHookDenied)
// detects the deny case generically.
type HookDenialError struct {
	// Reason is the server-reported deny reason. May be empty.
	Reason string
	// RuleID identifies the rule that triggered the denial. May be empty.
	RuleID string
	// Cause carries any wrapped error. Deny responses usually have no cause.
	Cause error
}

// Error formats the deny reason and the rule that triggered it.
func (e *HookDenialError) Error() string {
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "request blocked by an Agent Observability hook rule"
	}
	if id := strings.TrimSpace(e.RuleID); id != "" {
		return fmt.Sprintf("Agent Observability hook denied by rule %s: %s", id, reason)
	}
	return fmt.Sprintf("Agent Observability hook denied: %s", reason)
}

// Unwrap exposes ErrHookDenied for errors.Is matching, plus the optional Cause
// chain when present.
func (e *HookDenialError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrHookDenied}
	}
	return []error{ErrHookDenied, e.Cause}
}
