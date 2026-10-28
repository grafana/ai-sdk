package customertelemetry

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/authlib/authn"
	"github.com/grafana/authlib/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
)

var testNow = time.Date(2026, time.October, 28, 12, 0, 0, 0, time.UTC)

func authorizedCaller(namespace string, mode auth.AccessMode) auth.Caller {
	var claims authn.Claims[authn.AccessTokenClaims]
	claims.Rest.Namespace = namespace
	claims.Rest.Actor = &authn.ActorClaims{IDTokenClaims: authn.IDTokenClaims{Type: types.TypeUser, Identifier: "test-user"}}
	policy := auth.AccessPolicy{Mode: mode}
	return policy.Decide(authn.NewAccessTokenAuthInfo(claims), namespace)
}

func testStack(t *testing.T, namespace string) Stack {
	t.Helper()
	stack, err := StackFromCaller(authorizedCaller(namespace, auth.AccessModeCloud))
	require.NoError(t, err)
	return stack
}

func destinationInput(namespace string) DestinationInput {
	signals := make(map[Signal]Destination)
	for _, signal := range []Signal{SignalGenerations, SignalTraces, SignalMetrics, SignalLogs} {
		signals[signal] = Destination{
			Endpoint:          "https://ingest.example.test/" + string(signal),
			IngestionIdentity: namespace + "-" + string(signal),
			Credential:        CredentialRef{Name: "deployment-writer", Version: "credential-v1"},
		}
	}
	return DestinationInput{Namespace: namespace, Revision: "destination-v1", ValidUntil: testNow.Add(time.Minute), Signals: signals}
}

func TestStackFromCaller_Authorization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caller auth.Caller
		want   string
	}{
		{name: "byok", caller: authorizedCaller("Stack-A", auth.AccessModeCloud), want: "Stack-A"},
		{name: "configured", caller: authorizedCaller("Stack-B", auth.AccessModeSelfHosted), want: "Stack-B"},
		{name: "missing", caller: auth.Caller{}},
		{name: "fabricated namespace without authorization", caller: auth.Caller{Namespace: "Stack-A"}},
		{name: "refused", caller: (auth.AccessPolicy{Mode: auth.AccessModeCloud}).Decide(nil, "Stack-A")},
		{name: "empty namespace", caller: authorizedCaller("", auth.AccessModeCloud)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stack, err := StackFromCaller(tc.caller)
			if tc.want == "" {
				require.ErrorIs(t, err, ErrStack)
				assert.Zero(t, stack)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, stack.Namespace())
		})
	}
}

func TestDestinations_IsolationAndValidity(t *testing.T) {
	a, b := testStack(t, "A"), testStack(t, "B")
	for _, stack := range []Stack{a, b} {
		input := destinationInput(stack.Namespace())
		snapshot, err := NewDestinations(stack, input, testNow)
		require.NoError(t, err)
		assert.Equal(t, input.Revision, snapshot.Revision())
		assert.Equal(t, input.ValidUntil, snapshot.ValidUntil())
		for signal, expected := range input.Signals {
			actual, ok := snapshot.Lookup(stack, signal, testNow)
			require.True(t, ok)
			assert.Equal(t, expected, actual)
		}
		other := a
		if stack == a {
			other = b
		}
		for _, tc := range []struct {
			name  string
			stack Stack
			now   time.Time
		}{
			{name: "foreign", stack: other, now: testNow},
			{name: "zero", now: testNow},
			{name: "expired", stack: stack, now: input.ValidUntil},
			{name: "missing clock", stack: stack},
		} {
			t.Run(stack.Namespace()+"/"+tc.name, func(t *testing.T) {
				value, ok := snapshot.Lookup(tc.stack, SignalMetrics, tc.now)
				assert.False(t, ok)
				assert.Zero(t, value)
			})
		}
	}
	t.Run("mismatched discovery", func(t *testing.T) {
		_, err := NewDestinations(a, destinationInput("B"), testNow)
		assert.ErrorIs(t, err, ErrStack)
	})
	t.Run("zero stack", func(t *testing.T) {
		_, err := NewDestinations(Stack{}, destinationInput(""), testNow)
		assert.ErrorIs(t, err, ErrStack)
	})
	t.Run("partial availability and copies", func(t *testing.T) {
		input := destinationInput("A")
		delete(input.Signals, SignalGenerations)
		expected := input.Signals[SignalMetrics]
		snapshot, err := NewDestinations(a, input, testNow)
		require.NoError(t, err)
		clear(input.Signals)
		value, ok := snapshot.Lookup(a, SignalMetrics, testNow)
		require.True(t, ok)
		assert.Equal(t, expected, value)
		value.Credential.Version = "mutated"
		again, ok := snapshot.Lookup(a, SignalMetrics, testNow)
		require.True(t, ok)
		assert.Equal(t, expected, again)
		_, ok = snapshot.Lookup(a, SignalGenerations, testNow)
		assert.False(t, ok)
	})
}

func TestDestinations_InvalidInput(t *testing.T) {
	stack := testStack(t, "A")
	for _, endpoint := range []string{
		"", "/relative", "http://ingest.example.test", "https:///missing-host",
		"https://user:secret@ingest.example.test", "https://ingest.example.test?token=secret",
		"https://ingest.example.test?", "https://ingest.example.test#secret", "https://ingest.example.test#",
		"https://bad host", "https://host:invalid", "https:opaque",
	} {
		t.Run(endpoint, func(t *testing.T) {
			input := destinationInput("A")
			destination := input.Signals[SignalMetrics]
			destination.Endpoint = endpoint
			input.Signals[SignalMetrics] = destination
			_, err := NewDestinations(stack, input, testNow)
			require.Equal(t, ErrDestination, err)
			assert.NotContains(t, err.Error(), "secret")
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*DestinationInput)
		want   error
	}{
		{name: "revision", mutate: func(in *DestinationInput) { in.Revision = "" }, want: ErrSnapshot},
		{name: "zero expiry", mutate: func(in *DestinationInput) { in.ValidUntil = time.Time{} }, want: ErrSnapshot},
		{name: "expired", mutate: func(in *DestinationInput) { in.ValidUntil = testNow }, want: ErrSnapshot},
		{name: "unknown signal", mutate: func(in *DestinationInput) { in.Signals[Signal("future")] = in.Signals[SignalMetrics] }, want: ErrDestination},
		{name: "missing identity", mutate: func(in *DestinationInput) {
			d := in.Signals[SignalMetrics]
			d.IngestionIdentity = ""
			in.Signals[SignalMetrics] = d
		}, want: ErrDestination},
		{name: "missing authority", mutate: func(in *DestinationInput) {
			d := in.Signals[SignalMetrics]
			d.Credential.Name = ""
			in.Signals[SignalMetrics] = d
		}, want: ErrDestination},
		{name: "missing rotation version", mutate: func(in *DestinationInput) {
			d := in.Signals[SignalMetrics]
			d.Credential.Version = ""
			in.Signals[SignalMetrics] = d
		}, want: ErrDestination},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := destinationInput("A")
			tc.mutate(&input)
			snapshot, err := NewDestinations(stack, input, testNow)
			assert.ErrorIs(t, err, tc.want)
			assert.Zero(t, snapshot)
		})
	}
}

func TestDestinations_NoImplicitSerialization(t *testing.T) {
	stack := testStack(t, "A")
	destinations, err := NewDestinations(stack, destinationInput("A"), testNow)
	require.NoError(t, err)
	data, err := json.Marshal(map[string]any{"destinations": destinations})
	require.NoError(t, err)
	assert.JSONEq(t, `{"destinations":{}}`, string(data))
}

func TestDestinations_NoAmbientCredentialFallback(t *testing.T) {
	t.Setenv("AGENTO11Y_API_KEY", "operator-secret")
	t.Setenv("OPENAI_API_KEY", "provider-secret")
	stack := testStack(t, "A")
	input := destinationInput("A")
	d := input.Signals[SignalMetrics]
	d.Credential = CredentialRef{}
	input.Signals[SignalMetrics] = d
	snapshot, err := NewDestinations(stack, input, testNow)
	require.ErrorIs(t, err, ErrDestination)
	assert.Zero(t, snapshot)
	assert.NotContains(t, err.Error(), "secret")
}
