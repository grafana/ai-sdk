package customertelemetry

import (
	"errors"
	"time"
)

// Eligibility is an authoritative product/signal decision, not URL availability.
type Eligibility string

const (
	// EligibilityUnknown cannot authorize recording.
	EligibilityUnknown Eligibility = "unknown"
	// EligibilityEligible allows policy evaluation for this signal.
	EligibilityEligible Eligibility = "eligible"
	// EligibilityIneligible prohibits recording for this signal.
	EligibilityIneligible Eligibility = "ineligible"
)

// PreferenceState distinguishes optional configuration absence from read failure.
// It models a future configuration source; no such source is installed here.
type PreferenceState string

const (
	// PreferenceAbsent selects defaults subject to eligibility and service limits.
	PreferenceAbsent PreferenceState = "absent"
	// PreferenceDisabled explicitly disables customer recording.
	PreferenceDisabled PreferenceState = "disabled"
	// PreferenceFailed indicates a failed read, never an absent preference.
	PreferenceFailed PreferenceState = "failed"
)

// CaptureMode identifies the permitted generation/span content profile.
type CaptureMode string

const (
	// CaptureNone allows no generation or span recording.
	CaptureNone CaptureMode = "none"
	// CaptureMetadataOnly excludes content, raw errors, and arbitrary attributes.
	CaptureMetadataOnly CaptureMode = "metadata-only"
)

// DefaultProfileVersion identifies the reviewed content and metric profile.
// Changing content or metric dimensions requires an explicit compatibility review.
const DefaultProfileVersion = "metadata-only-v1"

var (
	// ErrPolicy indicates an invalid policy input rather than configuration absence.
	ErrPolicy = errors.New("customertelemetry: invalid recording policy")
	// ErrPolicyUnavailable indicates a failed preference lookup.
	ErrPolicyUnavailable = errors.New("customertelemetry: recording policy unavailable")
)

// PolicyInput separates authoritative eligibility, optional preferences, and
// mandatory service constraints. Missing eligibility is unknown; missing
// ServiceAllowed entries deny that signal. Revision covers all source decisions.
type PolicyInput struct {
	Namespace      string
	Revision       string
	ValidUntil     time.Time
	Eligibility    map[Signal]Eligibility
	ServiceAllowed map[Signal]bool
	Preference     PreferenceState
}

// Policy is an immutable effective policy snapshot. Its zero value denies all
// recording. It contains neither destinations nor credential material.
type Policy struct {
	stack      Stack
	revision   string
	validUntil time.Time
	allowed    map[Signal]bool
}

// NewPolicy evaluates the default profile without I/O. A failed lookup returns
// no policy; it cannot restore defaults or replace a known disabled policy.
// Cache owners must independently enforce freshness and update ordering.
func NewPolicy(stack Stack, input PolicyInput, now time.Time) (Policy, error) {
	if stack.namespace == "" || input.Namespace != stack.namespace {
		return Policy{}, ErrStack
	}
	if !validMetadata(input.Revision, input.ValidUntil, now) {
		return Policy{}, ErrSnapshot
	}
	switch input.Preference {
	case PreferenceAbsent, PreferenceDisabled:
	case PreferenceFailed:
		return Policy{}, ErrPolicyUnavailable
	default:
		return Policy{}, ErrPolicy
	}
	for signal, eligibility := range input.Eligibility {
		if !validSignal(signal) {
			return Policy{}, ErrPolicy
		}
		switch eligibility {
		case "", EligibilityUnknown, EligibilityEligible, EligibilityIneligible:
		default:
			return Policy{}, ErrPolicy
		}
	}
	for signal := range input.ServiceAllowed {
		if !validSignal(signal) {
			return Policy{}, ErrPolicy
		}
	}
	allowed := make(map[Signal]bool, 3)
	for _, signal := range []Signal{SignalGenerations, SignalTraces, SignalMetrics} {
		allowed[signal] = input.Preference == PreferenceAbsent &&
			input.Eligibility[signal] == EligibilityEligible && input.ServiceAllowed[signal]
	}
	return Policy{stack, input.Revision, input.ValidUntil, allowed}, nil
}

// Revision identifies the source policy independently of destinations and secrets.
// Consumers must also include DefaultProfileVersion when comparing profiles.
func (p Policy) Revision() string { return p.revision }

// ValidUntil returns the exclusive validity deadline.
func (p Policy) ValidUntil() time.Time { return p.validUntil }

// Allows checks signal permission for the matching stack at the supplied time.
func (p Policy) Allows(stack Stack, signal Signal, now time.Time) bool {
	return stack.namespace != "" && p.stack == stack &&
		validMetadata(p.revision, p.validUntil, now) && p.allowed[signal]
}

// Capture returns the allowed generation/span profile, or CaptureNone.
func (p Policy) Capture(stack Stack, signal Signal, now time.Time) CaptureMode {
	if (signal == SignalGenerations || signal == SignalTraces) && p.Allows(stack, signal, now) {
		return CaptureMetadataOnly
	}
	return CaptureNone
}

// MetricNames returns the fixed allowed model instruments in a fresh slice.
// Instrument availability depends on reported usage and observed stream timing.
func (p Policy) MetricNames(stack Stack, now time.Time) []string {
	if !p.Allows(stack, SignalMetrics, now) {
		return nil
	}
	return []string{
		"gen_ai.client.operation.duration",
		"gen_ai.client.token.usage",
		"gen_ai.client.time_to_first_token",
	}
}

// MetricAttributes returns permitted dimensions for an allowed instrument.
// Values must be closed, normalized classifications. Customer/model identities,
// and arbitrary attributes are not authorized by this profile.
func (p Policy) MetricAttributes(stack Stack, instrument string, now time.Time) []string {
	if !p.Allows(stack, SignalMetrics, now) {
		return nil
	}
	switch instrument {
	case "gen_ai.client.operation.duration":
		return []string{"gen_ai.operation.name", "error.type"}
	case "gen_ai.client.token.usage":
		return []string{"gen_ai.operation.name", "gen_ai.token.type"}
	case "gen_ai.client.time_to_first_token":
		return []string{"gen_ai.operation.name"}
	default:
		return nil
	}
}

// RecordingDestination combines independently resolved policy and destination
// snapshots. False means omit this signal, never use an operator/default tenant.
// The returned target is still subject to later ingestion credential resolution.
func RecordingDestination(stack Stack, destinations Destinations, policy Policy, signal Signal, now time.Time) (Destination, bool) {
	if !policy.Allows(stack, signal, now) {
		return Destination{}, false
	}
	return destinations.Lookup(stack, signal, now)
}
