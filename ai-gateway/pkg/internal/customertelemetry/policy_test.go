package customertelemetry

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func policyInput(namespace string) PolicyInput {
	eligibility := make(map[Signal]Eligibility)
	service := make(map[Signal]bool)
	for _, signal := range []Signal{SignalGenerations, SignalTraces, SignalMetrics, SignalLogs} {
		eligibility[signal], service[signal] = EligibilityEligible, true
	}
	return PolicyInput{Namespace: namespace, Revision: "policy-v1", ValidUntil: testNow.Add(time.Minute),
		Eligibility: eligibility, ServiceAllowed: service, Preference: PreferenceAbsent}
}

func TestPolicy_DefaultsAndRestrictions(t *testing.T) {
	stack := testStack(t, "A")
	for _, tc := range []struct {
		name        string
		mutate      func(*PolicyInput)
		generations bool
		metrics     bool
	}{
		{name: "absent preference", mutate: func(*PolicyInput) {}, generations: true, metrics: true},
		{name: "disabled", mutate: func(in *PolicyInput) { in.Preference = PreferenceDisabled }},
		{name: "unknown generation eligibility", mutate: func(in *PolicyInput) { in.Eligibility[SignalGenerations] = EligibilityUnknown }, metrics: true},
		{name: "ineligible generations", mutate: func(in *PolicyInput) { in.Eligibility[SignalGenerations] = EligibilityIneligible }, metrics: true},
		{name: "missing eligibility", mutate: func(in *PolicyInput) { in.Eligibility = nil }},
		{name: "service denies generations", mutate: func(in *PolicyInput) { in.ServiceAllowed[SignalGenerations] = false }, metrics: true},
		{name: "missing service constraints", mutate: func(in *PolicyInput) { in.ServiceAllowed = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := policyInput("A")
			tc.mutate(&input)
			policy, err := NewPolicy(stack, input, testNow)
			require.NoError(t, err)
			assert.Equal(t, tc.generations, policy.Allows(stack, SignalGenerations, testNow))
			assert.Equal(t, tc.metrics, policy.Allows(stack, SignalMetrics, testNow))
			assert.False(t, policy.Allows(stack, SignalLogs, testNow))
			assert.False(t, policy.Allows(stack, Signal("future"), testNow))
			assert.False(t, policy.Allows(Stack{}, SignalGenerations, testNow))
			assert.False(t, policy.Allows(testStack(t, "B"), SignalGenerations, testNow))
			assert.False(t, policy.Allows(stack, SignalGenerations, input.ValidUntil))
			assert.Equal(t, input.ValidUntil, policy.ValidUntil())
		})
	}
}

func TestPolicy_InvalidInput(t *testing.T) {
	stack := testStack(t, "A")
	for _, tc := range []struct {
		name   string
		mutate func(*PolicyInput)
		want   error
	}{
		{name: "foreign stack", mutate: func(in *PolicyInput) { in.Namespace = "B" }, want: ErrStack},
		{name: "missing preference is not absence", mutate: func(in *PolicyInput) { in.Preference = "" }, want: ErrPolicy},
		{name: "failed preference", mutate: func(in *PolicyInput) { in.Preference = PreferenceFailed }, want: ErrPolicyUnavailable},
		{name: "invalid preference", mutate: func(in *PolicyInput) { in.Preference = PreferenceState("invalid") }, want: ErrPolicy},
		{name: "invalid eligibility", mutate: func(in *PolicyInput) { in.Eligibility[SignalMetrics] = Eligibility("invalid") }, want: ErrPolicy},
		{name: "unknown eligibility signal", mutate: func(in *PolicyInput) { in.Eligibility[Signal("future")] = EligibilityEligible }, want: ErrPolicy},
		{name: "unknown constraint signal", mutate: func(in *PolicyInput) { in.ServiceAllowed[Signal("future")] = true }, want: ErrPolicy},
		{name: "missing revision", mutate: func(in *PolicyInput) { in.Revision = "" }, want: ErrSnapshot},
		{name: "expired", mutate: func(in *PolicyInput) { in.ValidUntil = testNow }, want: ErrSnapshot},
		{name: "zero expiry", mutate: func(in *PolicyInput) { in.ValidUntil = time.Time{} }, want: ErrSnapshot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := policyInput("A")
			tc.mutate(&input)
			policy, err := NewPolicy(stack, input, testNow)
			require.ErrorIs(t, err, tc.want)
			assert.False(t, policy.Allows(stack, SignalMetrics, testNow))
			assert.Equal(t, CaptureNone, policy.Capture(stack, SignalTraces, testNow))
			assert.Empty(t, policy.MetricNames(stack, testNow))
		})
	}
}

func TestPolicy_ProfileAndImmutableSnapshots(t *testing.T) {
	stack := testStack(t, "A")
	input := policyInput("A")
	policy, err := NewPolicy(stack, input, testNow)
	require.NoError(t, err)
	clear(input.Eligibility)
	clear(input.ServiceAllowed)
	input.Preference = PreferenceDisabled
	assert.Equal(t, "metadata-only-v1", DefaultProfileVersion)
	assert.Equal(t, CaptureMetadataOnly, policy.Capture(stack, SignalGenerations, testNow))
	assert.Equal(t, CaptureMetadataOnly, policy.Capture(stack, SignalTraces, testNow))
	assert.Equal(t, CaptureNone, policy.Capture(stack, SignalLogs, testNow))
	want := []string{"gen_ai.client.operation.duration", "gen_ai.client.token.usage", "gen_ai.client.time_to_first_token"}
	assert.Equal(t, want, policy.MetricNames(stack, testNow))
	names := policy.MetricNames(stack, testNow)
	names[0] = "arbitrary"
	assert.Equal(t, want, policy.MetricNames(stack, testNow))
	for _, tc := range []struct {
		instrument string
		attributes []string
	}{
		{instrument: want[0], attributes: []string{"gen_ai.operation.name", "error.type"}},
		{instrument: want[1], attributes: []string{"gen_ai.operation.name", "gen_ai.token.type"}},
		{instrument: want[2], attributes: []string{"gen_ai.operation.name"}},
		{instrument: "gen_ai.client.tool_calls_per_operation"},
		{instrument: "arbitrary"},
	} {
		attributes := policy.MetricAttributes(stack, tc.instrument, testNow)
		assert.Equal(t, tc.attributes, attributes)
		if len(attributes) > 0 {
			attributes[0] = "arbitrary"
		}
		assert.Equal(t, tc.attributes, policy.MetricAttributes(stack, tc.instrument, testNow))
		assert.Empty(t, policy.MetricAttributes(stack, tc.instrument, input.ValidUntil))
	}
	data, err := json.Marshal(map[string]any{"policy": policy})
	require.NoError(t, err)
	assert.JSONEq(t, `{"policy":{}}`, string(data))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				assert.True(t, policy.Allows(stack, SignalMetrics, testNow))
				policy.MetricNames(stack, testNow)[0] = "local-copy"
			}
		})
	}
	wg.Wait()
}

func TestRecordingDestination_IsolationRotationAndFailure(t *testing.T) {
	a, b := testStack(t, "A"), testStack(t, "B")
	destinations, err := NewDestinations(a, destinationInput("A"), testNow)
	require.NoError(t, err)
	policy, err := NewPolicy(a, policyInput("A"), testNow)
	require.NoError(t, err)
	foreign, err := NewPolicy(b, policyInput("B"), testNow)
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		stack   Stack
		policy  Policy
		now     time.Time
		allowed bool
	}{
		{name: "matching", stack: a, policy: policy, now: testNow, allowed: true},
		{name: "foreign policy", stack: a, policy: foreign, now: testNow},
		{name: "foreign destination", stack: b, policy: foreign, now: testNow},
		{name: "missing policy", stack: a, now: testNow},
		{name: "missing attribution", policy: policy, now: testNow},
		{name: "expired", stack: a, policy: policy, now: testNow.Add(time.Minute)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destination, ok := RecordingDestination(tc.stack, destinations, tc.policy, SignalMetrics, tc.now)
			assert.Equal(t, tc.allowed, ok)
			if !tc.allowed {
				assert.Zero(t, destination)
			}
		})
	}
	before, ok := RecordingDestination(a, destinations, policy, SignalMetrics, testNow)
	require.True(t, ok)
	rotatedInput := destinationInput("A")
	changed := rotatedInput.Signals[SignalMetrics]
	changed.Credential.Version = "credential-v2"
	rotatedInput.Signals[SignalMetrics] = changed
	rotated, err := NewDestinations(a, rotatedInput, testNow)
	require.NoError(t, err)
	after, ok := RecordingDestination(a, rotated, policy, SignalMetrics, testNow)
	require.True(t, ok)
	assert.Equal(t, before.Endpoint, after.Endpoint)
	assert.Equal(t, before.IngestionIdentity, after.IngestionIdentity)
	assert.NotEqual(t, before.Credential.Version, after.Credential.Version)
	assert.Equal(t, "policy-v1", policy.Revision())
	assert.Equal(t, destinations.Revision(), rotated.Revision())

	disabledInput := policyInput("A")
	disabledInput.Preference, disabledInput.Revision = PreferenceDisabled, "policy-v2"
	disabled, err := NewPolicy(a, disabledInput, testNow)
	require.NoError(t, err)
	assert.NotEqual(t, policy.Revision(), disabled.Revision())
	disabledInput.Preference = PreferenceFailed
	failed, err := NewPolicy(a, disabledInput, testNow)
	require.ErrorIs(t, err, ErrPolicyUnavailable)
	for _, snapshot := range []Policy{disabled, failed} {
		_, ok := RecordingDestination(a, rotated, snapshot, SignalMetrics, testNow)
		assert.False(t, ok)
	}
	assert.False(t, disabled.Allows(a, SignalMetrics, disabled.ValidUntil()))
}

type fakeResolver struct {
	destination DestinationInput
	policy      PolicyInput
	err         error
}

func (f fakeResolver) ResolveDestinations(_ context.Context, stack Stack, now time.Time) (Destinations, error) {
	if f.err != nil {
		return Destinations{}, f.err
	}
	return NewDestinations(stack, f.destination, now)
}

func (f fakeResolver) ResolvePolicy(_ context.Context, stack Stack, now time.Time) (Policy, error) {
	if f.err != nil {
		return Policy{}, f.err
	}
	return NewPolicy(stack, f.policy, now)
}

var (
	_ DestinationResolver = fakeResolver{}
	_ PolicyResolver      = fakeResolver{}
)

func TestResolvers_NoFallback(t *testing.T) {
	stack := testStack(t, "A")
	for _, tc := range []struct {
		name     string
		resolver fakeResolver
		allowed  bool
	}{
		{name: "authorized", resolver: fakeResolver{destination: destinationInput("A"), policy: policyInput("A")}, allowed: true},
		{name: "other customer", resolver: fakeResolver{destination: destinationInput("B"), policy: policyInput("B")}},
		{name: "lookup failure", resolver: fakeResolver{err: errors.New("unavailable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			destinations, destinationErr := tc.resolver.ResolveDestinations(context.Background(), stack, testNow)
			policy, policyErr := tc.resolver.ResolvePolicy(context.Background(), stack, testNow)
			if tc.allowed {
				require.NoError(t, destinationErr)
				require.NoError(t, policyErr)
			} else {
				require.Error(t, destinationErr)
				require.Error(t, policyErr)
			}
			_, ok := RecordingDestination(stack, destinations, policy, SignalMetrics, testNow)
			assert.Equal(t, tc.allowed, ok)
		})
	}
}
