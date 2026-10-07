package evidence

import (
	"encoding/json"
	"errors"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const (
	maxEssentialBytes     = 32 << 10
	maxErrorDetailBytes   = 24 << 10
	maxSuccessDetailBytes = 256 << 10
)

type snapshot struct {
	RequestedModelID string   `json:"requestedModelId"`
	CanonicalModelID string   `json:"canonicalModelId"`
	SelectedAttempt  int      `json:"selectedAttempt,omitempty"`
	Attempts         any      `json:"attempts"`
	Native           *native  `json:"native,omitempty"`
	Gateway          *Gateway `json:"gateway,omitempty"`
}

type native struct {
	ResponseIdentity Identity `json:"responseIdentity"`
}

type routing struct {
	OriginalModelID string `json:"originalModelId"`
	CanonicalSlug   string `json:"canonicalSlug"`
	FinalProvider   string `json:"finalProvider,omitempty"`
}

type namespace struct {
	Routing        routing         `json:"routing"`
	Evidence       snapshot        `json:"evidence"`
	NativeMetadata json.RawMessage `json:"nativeMetadata,omitempty"`
}

type attemptDisposition struct {
	State  DispositionState `json:"state"`
	Reason Reason           `json:"reason"`
	Count  int              `json:"count"`
}

func Metadata(facts Facts, original provider.ProviderMetadata, gateway *Gateway, minimal bool) (provider.ProviderMetadata, error) {
	if facts.AttemptCount == 0 {
		return original, nil
	}
	if facts.err != nil {
		return nil, facts.err
	}
	attempts := append([]Attempt(nil), facts.Attempts...)
	for i := range attempts {
		attempts[i].NativeError = summary(attempts[i].NativeError)
	}
	if gateway != nil && gateway.Classification != "" && gateway.Classification != ClassificationCanceled && gateway.Classification != ClassificationTimeout && facts.SelectedAttempt == 0 && len(attempts) != 0 {
		last := &attempts[len(attempts)-1]
		if last.Selection == fallback.AttemptCanceled && last.WillFallback == nil {
			last.Selection = fallback.AttemptFailed
		}
	}
	evidence := snapshot{RequestedModelID: facts.RequestedModelID, CanonicalModelID: facts.CanonicalModelID, SelectedAttempt: facts.SelectedAttempt, Attempts: attempts}
	if facts.SelectedAttempt != 0 && facts.Identity != nil {
		evidence.Native = &native{ResponseIdentity: *facts.Identity}
	}
	if facts.AttemptCount > maxAttempts {
		evidence.Attempts = attemptDisposition{State: OverLimit, Reason: AttemptLimit, Count: facts.AttemptCount}
	}
	if gateway != nil {
		value := *gateway
		value.ReplayRisk = GenerationMayHaveRun
		if facts.Completed {
			value.ReplayRisk = GenerationCompleted
		}
		evidence.Gateway = &value
	}
	value := namespace{Routing: routing{OriginalModelID: facts.RequestedModelID, CanonicalSlug: facts.CanonicalModelID, FinalProvider: facts.SelectedProvider}, Evidence: evidence}
	essential, err := json.Marshal(struct {
		Metadata     namespace    `json:"metadata"`
		CurrentError *NativeError `json:"nativeError,omitempty"`
	}{value, summary(facts.CurrentError)})
	if err != nil {
		return nil, err
	}
	if len(essential) > maxEssentialBytes {
		return nil, errors.New("gateway evidence: essential encoded allocation exceeded")
	}
	remaining := maxSuccessDetailBytes
	if gateway != nil {
		remaining = maxErrorDetailBytes
	}
	for i := range attempts {
		if attempts[i].NativeError == nil {
			continue
		}
		details := facts.Attempts[i].NativeError.Details
		if len(details) == 0 {
			continue
		}
		var component struct {
			State DispositionState `json:"state"`
		}
		if err := json.Unmarshal(details, &component); err != nil {
			return nil, err
		}
		if (minimal && component.State == Available) || len(details) > remaining {
			details = disposition(OverLimit, AggregateLimit)
		} else {
			remaining -= len(details)
		}
		attempts[i].NativeError.Details = details
	}
	value.NativeMetadata = original["gateway"]
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	metadata := make(provider.ProviderMetadata, len(original)+1)
	for key, raw := range original {
		metadata[key] = raw
	}
	metadata["gateway"] = encoded
	return metadata, nil
}
