package extension

import (
	"context"
	"errors"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

var (
	// ErrDenied is returned by admit, request inspection or response
	// inspection to deny the call. It may be wrapped; the caller receives only
	// the Gateway's fixed permission error.
	ErrDenied = errors.New("extension: denied")
	// ErrUnavailable is the error of a call that an extension failure ended
	// under [FailClosed]. An extension may return it to report that it cannot
	// decide, which is equivalent to returning any other error.
	ErrUnavailable = errors.New("extension: unavailable")
)

// CallerType names the kind of verified caller.
type CallerType string

const (
	// CallerUser is a Grafana user.
	CallerUser CallerType = "user"
	// CallerServiceAccount is a Grafana service account.
	CallerServiceAccount CallerType = "service-account"
	// CallerInternalService is an allowlisted internal service calling with
	// configured access.
	CallerInternalService CallerType = "internal-service"
)

// Call is one extension's view of one call. Each applicable extension gets its
// own Call for each call, shared across that extension's phases of the call.
type Call struct {
	// Namespace is the route namespace the platform authorized.
	Namespace string
	Keys      KeyOwnership
	Source    Source
	Caller    CallerType
	// CallerIdentity is the verified identity actor's typed UID,
	// user:<uid> or service-account:<uid>, also for an internal service
	// calling on behalf of one. It is empty when the caller has none.
	CallerIdentity string
	// CorrelationID identifies the call in Gateway logs.
	CorrelationID string
	// RequestedModel is the model ID the call requested, as sent: an alias or
	// canonical ID for catalog selections, or the provider/model selector for
	// request BYOK.
	RequestedModel string
	// CanonicalModel is the requested model's canonical ID, empty for request
	// BYOK.
	CanonicalModel string
	// Candidates are the call's planned attempts, in the order the call may
	// try them.
	Candidates []Candidate
	// State belongs to the extension for this call, for example a reservation
	// made in admit and committed in observe.
	State any
}

// Candidate is one planned attempt of a call.
type Candidate struct {
	// Model is the requested model ID or gateway models entry, as sent, whose
	// chain the candidate belongs to.
	Model string
	// CanonicalModel is that model's canonical ID, empty for request BYOK.
	CanonicalModel string
	Provider       string
	// ProviderInstance is the configured provider instance, empty for request
	// BYOK.
	ProviderInstance string
	// ProviderModel is the model ID the provider receives.
	ProviderModel string
}

// AttemptOutcome is the selection decision for one physical attempt.
type AttemptOutcome string

const (
	// AttemptSelected is the attempt whose response the call used.
	AttemptSelected AttemptOutcome = "selected"
	// AttemptFailed is an attempt that failed before selection.
	AttemptFailed AttemptOutcome = "failed"
	// AttemptCanceled is an attempt the call ended before selection.
	AttemptCanceled AttemptOutcome = "canceled"
)

// Attempt is one physical attempt of a call. The i-th attempt is the i-th of
// the call's [Call.Candidates].
type Attempt struct {
	Provider string
	// ProviderInstance is the configured provider instance, empty for request
	// BYOK.
	ProviderInstance string
	ProviderModel    string
	Outcome          AttemptOutcome
	// StatusCode is the provider's HTTP status for a failed or canceled
	// attempt, or zero when unknown.
	StatusCode int
}

// Admitter decides whether a call may proceed, before its prompt is read.
type Admitter interface {
	Admit(ctx context.Context, call *Call) error
}

// RequestTransformer changes the call's parameters. It must return changed
// copies rather than mutate values reachable from params; the host clones only
// the top-level header, provider-option and prompt containers.
type RequestTransformer interface {
	TransformRequest(ctx context.Context, call *Call, params provider.CallOptions) (provider.CallOptions, error)
}

// RequestInspector checks the parameters the model will receive. It must not
// modify them.
type RequestInspector interface {
	InspectRequest(ctx context.Context, call *Call, params provider.CallOptions) error
}

// ResponseInspector checks each normalized part of the model's output before
// it is delivered.
type ResponseInspector interface {
	InspectResponse(ctx context.Context, call *Call, part Part) error
}

// Observer receives one record per call after its outcome is decided. It runs
// asynchronously, under a context that keeps the call's values but not its
// cancellation, bounded by the Gateway. Its error is counted and otherwise
// ignored.
type Observer interface {
	Observe(ctx context.Context, call *Call, record Record) error
}

// Closer releases an instance's resources at shutdown, within the Gateway's
// shared final-flush deadline.
type Closer interface {
	Close(ctx context.Context) error
}

// PartType is the type of a normalized response part.
type PartType string

const (
	// PartText is generated text.
	PartText PartType = "text"
	// PartReasoning is reasoning text.
	PartReasoning PartType = "reasoning"
	// PartToolCall is a complete tool call.
	PartToolCall PartType = "tool-call"
	// PartFinish ends a response segment.
	PartFinish PartType = "finish"
)

// Part is one normalized piece of a model's output, the same for unary and
// streaming calls. Text and reasoning carry a stream's delta, or a unary
// call's complete text. Other output is not inspected.
type Part struct {
	Type PartType
	// ID is the stream block or tool call ID. It may be empty for unary text.
	ID       string
	Text     string
	ToolName string
	// Input is a tool call's complete input.
	Input string
}

// Phase names an extension phase.
type Phase string

const (
	// PhaseAdmit is [Admitter.Admit].
	PhaseAdmit Phase = "admit"
	// PhaseTransformRequest is [RequestTransformer.TransformRequest].
	PhaseTransformRequest Phase = "transform-request"
	// PhaseInspectRequest is [RequestInspector.InspectRequest].
	PhaseInspectRequest Phase = "inspect-request"
	// PhaseInspectResponse is [ResponseInspector.InspectResponse].
	PhaseInspectResponse Phase = "inspect-response"
	// PhaseObserve is [Observer.Observe].
	PhaseObserve Phase = "observe"
)

// Outcome is the final outcome of a call.
type Outcome string

const (
	// OutcomeSuccess is a call that completed.
	OutcomeSuccess Outcome = "success"
	// OutcomeDenied is a call an extension denied.
	OutcomeDenied Outcome = "denied"
	// OutcomeError is a call that failed: a provider error, an extension
	// failure under [FailClosed] or an internal error.
	OutcomeError Outcome = "error"
	// OutcomeCancelled is a call whose context was canceled or expired.
	OutcomeCancelled Outcome = "cancelled"
	// OutcomeInvalidOutput is a call whose model output broke the provider
	// output lifecycle.
	OutcomeInvalidOutput Outcome = "invalid-output"
)

// Denial names the extension and phase that denied a call.
type Denial struct {
	Extension string
	Phase     Phase
}

// Record is the final record of one call.
type Record struct {
	Outcome Outcome
	// DeniedBy is set when Outcome is [OutcomeDenied].
	DeniedBy *Denial
	// Usage is the call's usage as the Gateway's logical observers report it,
	// zero when the model was not invoked.
	Usage provider.Usage
	// Attempts are the call's physical attempts, in order. A call refused
	// before the model was called has none; a call whose attempts cannot be
	// attributed to its candidates has none either.
	Attempts []Attempt
	Started  time.Time
	Duration time.Duration
}
