package evidence

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/fallback"
)

type Completion string

const (
	Completed  Completion = "completed"
	Incomplete Completion = "incomplete"
)

type Phase string

const (
	Unary     Phase = "unary"
	Setup     Phase = "setup"
	Committed Phase = "committed"
)

type ReplayRisk string

const (
	GenerationMayHaveRun ReplayRisk = "generationMayHaveRun"
	GenerationCompleted  ReplayRisk = "generationCompleted"
)

type DispositionState string

const (
	Available   DispositionState = "available"
	Unavailable DispositionState = "unavailable"
	Redacted    DispositionState = "redacted"
	Malformed   DispositionState = "malformed"
	OverLimit   DispositionState = "over-limit"
)

type Reason string

const (
	ProducerDoesNotExpose Reason = "producerDoesNotExpose"
	CredentialSource      Reason = "credentialSource"
	InvalidJSON           Reason = "invalidJSON"
	SourceLimit           Reason = "sourceBytes"
	EncodedLimit          Reason = "encodedBytes"
	AggregateLimit        Reason = "aggregateBytes"
	AttemptLimit          Reason = "attemptCount"
)

type Component struct {
	State    DispositionState `json:"state"`
	Value    json.RawMessage  `json:"value,omitempty"`
	Redacted bool             `json:"redacted,omitempty"`
	Reason   Reason           `json:"reason,omitempty"`
}

type NativeError struct {
	Message     string          `json:"message,omitempty"`
	Type        string          `json:"type,omitempty"`
	Code        json.RawMessage `json:"code,omitempty"`
	StatusCode  int             `json:"statusCode,omitempty"`
	IsRetryable bool            `json:"isRetryable"`
	Details     json.RawMessage `json:"details,omitempty"`
}

type Attempt struct {
	Index            int                     `json:"index"`
	Provider         string                  `json:"provider"`
	ModelID          string                  `json:"modelId"`
	ProviderInstance string                  `json:"providerInstance,omitempty"`
	Selection        fallback.AttemptOutcome `json:"selection"`
	Completion       Completion              `json:"completion,omitempty"`
	WillFallback     *bool                   `json:"willFallback,omitempty"`
	NativeError      *NativeError            `json:"nativeError,omitempty"`
}

type Identity struct {
	ID        string `json:"id,omitempty"`
	ModelID   string `json:"modelId,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type Classification string

const (
	ClassificationInvalidRequest   Classification = "invalid_request"
	ClassificationModelNotFound    Classification = "model_not_found"
	ClassificationRateLimit        Classification = "rate_limit_exceeded"
	ClassificationOverloaded       Classification = "overloaded"
	ClassificationFailedDependency Classification = "failed_dependency"
	ClassificationUpstream         Classification = "upstream_error"
	ClassificationTimeout          Classification = "timeout"
	ClassificationCanceled         Classification = "canceled"
	ClassificationInternal         Classification = "internal_error"
)

type Gateway struct {
	HTTPStatusCode int            `json:"httpStatusCode"`
	Classification Classification `json:"classification"`
	Phase          Phase          `json:"phase"`
	IsRetryable    bool           `json:"isRetryable"`
	ReplayRisk     ReplayRisk     `json:"replayRisk,omitempty"`
}
