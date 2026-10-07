package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readFacts(t *testing.T, s *State, gateway *Gateway) snapshot {
	t.Helper()
	metadata, err := Metadata(s.Snapshot(), nil, gateway, false)
	require.NoError(t, err)
	var value namespace
	require.NoError(t, json.Unmarshal(metadata["gateway"], &value))
	return value.Evidence
}

func TestState_AttemptBoundaries(t *testing.T) {
	for _, count := range []int{16, 17} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx, s := New(t.Context(), "alias", "canonical")
			for i := 1; i <= count; i++ {
				index := s.begin(catalog.ConfiguredCandidate{Provider: "openai", ModelID: fmt.Sprint(i), ProviderInstance: "configured"})
				require.Equal(t, i, index)
				outcome := fallback.AttemptFailed
				if i == count {
					outcome = fallback.AttemptSelected
				}
				ObserveDecision(ctx, fallback.Attempt{Index: i, Outcome: outcome, WillFallback: i < count})
			}
			s.ObservePart(provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "current event", StatusCode: 401})})
			require.NotNil(t, s.CurrentError())
			assert.Equal(t, "current event", s.CurrentError().Message)
			s.ObservePart(provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}})
			facts := readFacts(t, s, &Gateway{Phase: Committed})
			assert.Equal(t, count, facts.SelectedAttempt)
			assert.Equal(t, GenerationCompleted, facts.Gateway.ReplayRisk)
			encoded, err := json.Marshal(facts.Attempts)
			require.NoError(t, err)
			if count == 17 {
				assert.JSONEq(t, `{"state":"over-limit","reason":"attemptCount","count":17}`, string(encoded))
				assert.Nil(t, s.facts.Attempts)
			} else {
				var attempts []Attempt
				require.NoError(t, json.Unmarshal(encoded, &attempts))
				assert.Len(t, attempts, 16)
			}
		})
	}
}

func TestState_UnaryIdentityBeyondAttemptLimit(t *testing.T) {
	ctx, s := New(t.Context(), "alias", "canonical")
	for i := 1; i <= 17; i++ {
		s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: fmt.Sprint(i)})
	}
	s.returned(17, &provider.GenerateResult{Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "selected-id", ModelID: "reported-model"}}}, nil, false, true)
	ObserveDecision(ctx, fallback.Attempt{Index: 17, Outcome: fallback.AttemptSelected})
	facts := readFacts(t, s, &Gateway{Phase: Unary})
	require.NotNil(t, facts.Native)
	assert.Equal(t, "selected-id", facts.Native.ResponseIdentity.ID)
	assert.Equal(t, GenerationCompleted, facts.Gateway.ReplayRisk)
}

func TestState_SealAndCancellationFacts(t *testing.T) {
	ctx, s := New(t.Context(), "alias", "canonical")
	index := s.begin(catalog.ConfiguredCandidate{Provider: "openai", ModelID: "first"})
	retryable := true
	s.returned(index, nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Message: "rate limited", IsRetryable: &retryable}), false, true)
	ObserveDecision(ctx, fallback.Attempt{Index: 1, Outcome: fallback.AttemptFailed, WillFallback: true})
	before, err := Metadata(s.Snapshot(), nil, nil, false)
	require.NoError(t, err)
	s.Seal()
	assert.Zero(t, s.begin(catalog.ConfiguredCandidate{Provider: "anthropic", ModelID: "never invoked"}))
	s.returned(1, &provider.GenerateResult{}, nil, false, false)
	ObserveDecision(ctx, fallback.Attempt{Index: 1, Outcome: fallback.AttemptSelected})
	s.ObservePart(provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}})
	after, err := Metadata(s.Snapshot(), nil, nil, false)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Contains(t, string(after["gateway"]), `"willFallback":true`)
	assert.NotContains(t, string(after["gateway"]), "never invoked")
}

func TestState_MetadataImmutability(t *testing.T) {
	_, s := New(t.Context(), "alias", "canonical")
	s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: "configured"})
	s.returned(1, &provider.GenerateResult{}, nil, false, false)
	original := provider.ProviderMetadata{"gateway": json.RawMessage(`{"routing":{"spoof":true},"nativeMetadata":{"null":null},"false":false}`), "vendor": json.RawMessage(`{"nested":[null,false,0,"",[],{}]}`)}
	before, err := json.Marshal(original)
	require.NoError(t, err)
	first, err := Metadata(s.Snapshot(), original, nil, false)
	require.NoError(t, err)
	second, err := Metadata(s.Snapshot(), original, nil, true)
	require.NoError(t, err)
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.Equal(t, first, second)
	var value namespace
	require.NoError(t, json.Unmarshal(first["gateway"], &value))
	assert.JSONEq(t, string(original["gateway"]), string(value.NativeMetadata))
	assert.Equal(t, original["vendor"], first["vendor"])
}

func TestState_OptionalRetentionBudget(t *testing.T) {
	_, s := New(t.Context(), "alias", "canonical")
	for i := 1; i <= 16; i++ {
		index := s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: fmt.Sprint(i)})
		err := provider.NewAPICallError(provider.APICallErrorOptions{Message: "failed", Data: json.RawMessage(`{"detail":"` + strings.Repeat("x", 20000) + `"}`)})
		s.returned(index, nil, err, false, true)
	}
	retained := 0
	overLimit := 0
	for _, a := range s.facts.Attempts {
		require.NotNil(t, a.NativeError.Details)
		if readDetails(t, a.NativeError).State == Available {
			encoded, err := json.Marshal(a.NativeError.Details)
			require.NoError(t, err)
			retained += len(encoded)
		} else {
			overLimit++
			assert.Equal(t, AggregateLimit, readDetails(t, a.NativeError).Reason)
		}
	}
	assert.LessOrEqual(t, retained, maxSuccessDetailBytes)
	assert.Positive(t, overLimit)
	_ = readFacts(t, s, &Gateway{Phase: Unary})
}

func TestState_ExactOptionalAllocations(t *testing.T) {
	base, err := json.Marshal(Component{State: Available, Value: json.RawMessage(`""`)})
	require.NoError(t, err)
	for _, scope := range []struct {
		name         string
		bytes, count int
		gateway      *Gateway
	}{
		{"error", maxErrorDetailBytes, 1, &Gateway{Phase: Unary}},
		{"success", maxSuccessDetailBytes / 8, 8, nil},
	} {
		for _, over := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/over=%t", scope.name, over), func(t *testing.T) {
				_, s := New(t.Context(), "alias", "canonical")
				for i := 1; i <= scope.count; i++ {
					bytes := scope.bytes - len(base)
					if over && i == scope.count {
						bytes++
					}
					index := s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: fmt.Sprint(i)})
					s.returned(index, nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "safe", Data: json.RawMessage(`"` + strings.Repeat("x", bytes) + `"`)}), false, true)
				}
				facts := readFacts(t, s, scope.gateway)
				attempts := facts.Attempts.([]any)
				details := attempts[len(attempts)-1].(map[string]any)["nativeError"].(map[string]any)["details"].(map[string]any)
				if over {
					assert.Equal(t, string(OverLimit), details["state"])
					assert.Equal(t, string(AggregateLimit), details["reason"])
				} else {
					assert.Equal(t, string(Available), details["state"])
				}
			})
		}
	}
}

func TestState_UnrunDecision(t *testing.T) {
	ctx, s := New(t.Context(), "alias", "canonical")
	s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: "configured"})
	before := s.Snapshot()
	for _, index := range []int{0, 2} {
		ObserveDecision(ctx, fallback.Attempt{Index: index, Outcome: fallback.AttemptSelected})
		assert.Equal(t, before, s.Snapshot())
	}
}

func TestState_SnapshotIsolation(t *testing.T) {
	ctx, s := New(t.Context(), "alias", "canonical")
	index := s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: "configured"})
	s.returned(index, nil, provider.NewAPICallError(provider.APICallErrorOptions{Data: json.RawMessage(`{"message":"safe","code":"original","detail":"original"}`)}), false, true)
	ObserveDecision(ctx, fallback.Attempt{Index: index, Outcome: fallback.AttemptFailed, WillFallback: true})
	before := s.Snapshot()
	first := s.Snapshot()
	*first.Attempts[0].WillFallback = false
	first.Attempts[0].NativeError.Code[1] = 'X'
	first.Attempts[0].NativeError.Details[1] = 'X'
	first.Attempts[0].Provider = "changed"
	assert.Equal(t, before, s.Snapshot())
	full, err := Metadata(before, nil, nil, false)
	require.NoError(t, err)
	minimal, err := Metadata(before, nil, nil, true)
	require.NoError(t, err)
	assert.Contains(t, string(full["gateway"]), "original")
	assert.NotContains(t, string(minimal["gateway"]), `"detail":"original"`)
	assert.Equal(t, before, s.Snapshot())
}

func TestState_ErrorReplacement(t *testing.T) {
	_, s := New(t.Context(), "alias", "canonical")
	s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: "configured"})
	for _, size := range []int{10000, 20000, 10000} {
		s.ObservePart(provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "current", Data: json.RawMessage(`{"detail":"` + strings.Repeat("x", size) + `"}`)})})
		facts := s.Snapshot()
		require.Len(t, facts.Attempts, 1)
		assert.Equal(t, len(facts.Attempts[0].NativeError.Details), s.detailBytes)
		assert.Empty(t, facts.CurrentError.Details)
	}
	s.ObservePart(provider.StreamPart{Type: provider.PartError})
	assert.Nil(t, s.CurrentError())
	assert.NotNil(t, s.Snapshot().Attempts[0].NativeError)
}

func TestMetadata_EssentialAllocation(t *testing.T) {
	_, s := New(t.Context(), "alias", "canonical")
	for i := 1; i <= maxAttempts; i++ {
		index := s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: strings.Repeat("<", 1000)})
		s.returned(index, nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: strings.Repeat("x", 1000)}), false, true)
	}
	facts := s.Snapshot()
	require.Len(t, facts.Attempts, maxAttempts)
	for _, minimal := range []bool{false, true} {
		metadata, err := Metadata(facts, nil, &Gateway{Phase: Unary}, minimal)
		require.ErrorContains(t, err, "essential encoded allocation")
		assert.Nil(t, metadata)
	}
}

func TestState_ConcurrentSnapshots(t *testing.T) {
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Go(func() {
			ctx, s := New(context.Background(), fmt.Sprint(i), "canonical")
			s.begin(catalog.ConfiguredCandidate{Provider: "native", ModelID: fmt.Sprint(i)})
			ObserveDecision(ctx, fallback.Attempt{Index: 1, Outcome: fallback.AttemptSelected})
			for n := 0; n < 32; n++ {
				metadata, err := Metadata(s.Snapshot(), nil, nil, false)
				require.NoError(t, err)
				assert.Contains(t, string(metadata["gateway"]), fmt.Sprintf(`"modelId":"%d"`, i))
			}
			s.Seal()
		})
	}
	group.Wait()
}
