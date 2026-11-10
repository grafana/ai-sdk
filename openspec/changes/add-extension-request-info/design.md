# Design

## Context

- `extension.Call` carries the namespace, key ownership, source, caller type, correlation ID and one `Model`: `Selection.ID`, which is the canonical ID for catalog selections and the requested selector for BYOK. The requested model as sent, the planned candidates and the caller's UID are not exposed.
- Request facts come from two places. `service.ExtensionRequest(ctx)` reads the observation snapshot (correlation ID) and `auth.Caller` (namespace, type). `Host.Compose(selected)` adds the source and `Selection.ID`. `Compose` runs once per call, after selection, so the pipeline is per call.
- `auth.Caller.Identity` is the identity actor's typed UID from `AuthInfo.GetUID()` (`user:<uid>`, `service-account:<uid>`), set for users, service accounts, and internal services that call on behalf of one. `gateway-caller-access` forbids emitting it.
- The attempt capture (`execution.attemptCapture`) is created in `Runtime.selectModel`, installed with `fallback.WithAttemptObserver` on the call context, and marked entered by `enteredModel` beneath every composed layer. `view(err)` seals it, synthesizes the one attempt of a single configured candidate, and projects the overview. `seal()` nils the attempts, so a second view projects none.
- ProviderWire seals at the first stream part (`runStream` calls `View` after the first `Next`); the extension pipeline dispatches a stream's record when its forwarding goroutine ends, later. Unary records are dispatched inside `DoGenerate`, before ProviderWire's `View`.
- Fallback decides every stream attempt before `DoStream` returns, so a stream's observed attempts are final at establishment. Only the synthesized single-candidate attempt depends on the outcome.
- `fallback.WithAttemptObserver` stores one context value; a later call replaces the earlier one. Extensions receive a context but never return one, and never receive the model.
- Logs: `observationLogAttrs` feeds model call logs (through the `model_log_policy.go` allowlist) and the `http request completed` line. Agent Observability: `observationMetadata` feeds generations, filtered by `allowedAgentMetadataKeys`.
- #757 (parallel, not on this base) adds `Selection.Catalog`, `StackCatalogSelection` (attributed like configured), the `catalog` log key and `gateway.catalog` metadata key.

## Goals / Non-Goals

**Goals:** complete request information for every source; physical attempts in the observation record, for unary and stream, from the runtime's own capture; the caller UID for extensions, logs and Agent Observability, never in metrics or provider requests.

**Non-Goals:** stack-catalog name and version (with #757); attempt timing or error text; Agent Observability `UserID`; per-namespace configuration (#765).

## Decisions

### Request information

| Field | Operator catalog | Routed | Request BYOK | Stack catalog |
|---|---|---|---|---|
| Namespace, keys, source, correlation ID | yes | yes | yes | yes |
| Caller type, caller identity UID | yes | yes | yes | yes |
| `RequestedModel` | as sent (alias or canonical) | as sent | selector as sent | catalog model ID as sent |
| `CanonicalModel` | canonical ID | requested model's canonical ID | empty | catalog model ID |
| `Candidates` | configured candidates | composed chain | one per request account | catalog chain |
| Catalog name and version | — | — | — | with #757, not here |

The stack-catalog column follows from `StackCatalogSelection` being attributed like configured; only the catalog fields need the merge.

### Go shapes

```go
package extension

type Call struct {
	Namespace      string
	Keys           KeyOwnership
	Source         Source
	Caller         CallerType
	CallerIdentity string // user:<uid> or service-account:<uid>; empty when none
	CorrelationID  string
	RequestedModel string // as sent: alias, canonical ID or BYOK selector
	CanonicalModel string // empty for request BYOK
	Candidates     []Candidate
	State          any
}

// Candidate is one planned attempt of the call's chain, in order.
type Candidate struct {
	Model            string // requested model or models entry, as sent
	CanonicalModel   string // that model's canonical ID; empty for request BYOK
	Provider         string
	ProviderInstance string // empty for request BYOK
	ProviderModel    string
}

type AttemptOutcome string
const (
	AttemptSelected AttemptOutcome = "selected"
	AttemptFailed   AttemptOutcome = "failed"
	AttemptCanceled AttemptOutcome = "canceled"
)

// Attempt is one physical attempt; the i-th is Candidates[i].
type Attempt struct {
	Provider         string
	ProviderInstance string
	ProviderModel    string
	Outcome          AttemptOutcome
	StatusCode       int // 0 when unknown
}

type Record struct {
	Outcome  Outcome
	DeniedBy *Denial
	Usage    provider.Usage
	Attempts []Attempt
	Started  time.Time
	Duration time.Duration
}
```

- `Model` is replaced, not kept: the package is v0 and unreleased, and the two IDs match the overview's `requestedModelId` and `canonicalModelId`.
- `Candidate` mirrors `execution.ExpectedAttempt`; `Attempt` mirrors the overview attempt without its error summary. Attempt outcomes keep fallback's spelling (`canceled`), distinct from the call outcome `cancelled`, as the overview does.
- Each extension's `Call` gets its own `Candidates` copy, and each observer its own `Attempts` copy.

### Request information sources

- `service.ExtensionRequest(ctx)` adds `CallerIdentity` from `auth.Caller.Identity`, the verified value the access decision holds, not the bounded observation value. Logs omit a value outside the observation bound; extensions still receive it.
- `Host.Compose(selected)` derives the rest from the selection, once per call: `RequestedModel` from `selected.RequestedID()`, `CanonicalModel` from `selected.CanonicalID()`, `Candidates` from `selected.Expected()`.

### Execution: per-call selection accessors

`selectModel` sets the call's capture on an unexported `Selection` field before composing; the capture already holds the requested model ID. New exported methods:

| Method | Returns |
|---|---|
| `RequestedID()` | the model ID as sent; empty outside a composer |
| `CanonicalID()` | `ID`, or empty for a request-account selection |
| `Expected()` | a copy of the expected attempts |
| `Attempts(err error)` | the call's attempts so far, projected and redacted like the overview for outcome `err`; nil before entry, when the overview would be omitted, or outside a composer |

`ModelComposer` keeps its signature. Alternative: a context accessor for the capture. Rejected: any code holding the call context, including extension code compiled into the Gateway, could read it, and the composer already runs per call.

### Capture: non-sealing read and the seal

- `seal()` stops discarding observed attempts. The first seal returns them; later seals return none, so a second `View` still projects nothing and late attempts are still ignored.
- `observed()` returns the entered flag and a copy of the attempts under the lock, sealed or not. `view` and `Attempts` share one projection, including single-candidate synthesis and redaction.
- So a stream record read after ProviderWire sealed at the first part still sees the fallback attempts.

### Attempts reach Observe

The pipeline reads `Attempts(err)` when it builds the record, with `err` the selected model's outcome as the pipeline saw it:

- **Unary:** the inner `DoGenerate` error, or after a result the context's error. Output refused by an inspector, or invalid output, after a successful inner call projects as selected, like the overview.
- **Stream, setup failed:** the inner `DoStream` error.
- **Stream, established:** decided when the forwarding goroutine ends, before it closes its output, because the consumer cancels the context when it sees the close: nil once a part, including an abort part, was delivered; the context error when the context ended first; otherwise a fixed premature-end error (failed). This matches what ProviderWire's first `View` projects for the same stream, whichever reads first.
- **Refused before the model:** not entered, so no attempts.
- **No attribution, or an attempt that does not match its expected destination:** no overview, so no attempts.

A unary call cancelled after its provider answered still reports the answered attempt, as `canceled`: fallback decides an answer that arrives after cancellation that way, and a single candidate's record follows. The runtime discards the observed attempts of such a call, so a fallback chain publishes no overview while its record keeps the attempt; a single candidate's overview synthesizes the same `canceled` attempt. The record describes attempts made; the overview describes the response.

### Unreplaceable capture

Structural, with no new guard: phases receive a context but cannot return one, never receive the model, and the pipeline invokes the selected model with the context it received, which carries the runtime's observer. Records carry copies, so an observer cannot change what another observer or the overview sees.

### Caller identity in logs and Agent Observability

- `requestObservation` gains `callerIdentity`, set in `Telemetry.ObserveAccess` from `caller.Identity` through `boundedObservationValue`, for every accepted caller.
- `observationLogAttrs` emits `caller_identity`; the model log allowlist accepts it as a bounded string. The `http request completed` line gets it too, since it shares those attributes.
- `observationMetadata` emits `gateway.caller_identity`; `allowedAgentMetadataKeys` accepts it. `Generation.UserID` stays cleared.
- Metrics read no observation value. Provider requests are unchanged: the observation bridge only adds a context value.

**Deviation from the issue:** the log key is `caller_identity`, not `gateway.caller_identity`, matching `namespace`/`gateway.namespace` and #757's `catalog`/`gateway.catalog`.

## Test plan

| Acceptance | Evidence |
|---|---|
| Request info matches access and selection | Extend `TestExtensionPipeline_EverySelectionGetsThePipelineOnce` (operator catalog, alias, routed, BYOK; unary and stream) with requested, canonical, candidates and caller identity. `TestExtensionRequest` table over `Decide`: user, service account, internal service alone and on behalf of a user, `self-hosted`. `TestPipeline_RequestInfo` for per-extension candidate copies. |
| Attempt shapes; stream at stream end; capture not replaceable | `execution`: non-sealing read before entry, single candidate, fallback, after `View` and `Close`, view-once kept, late attempts ignored, no attribution. Service pipeline tests: single candidate (one selected), fallback with 503 (failed 503, selected), admit denial (none), stream fallback whose consumer calls `View` at the first part while the fake provider holds the stream open (both attempts in the stream-end record), and single-candidate streams closing before and after their first part (record and overview agree), BYOK accounts. An extension that derives a context with its own attempt observer gets no attempt; an observer that mutates its attempts changes neither another observer's record nor the overview. |
| Operator and tenant extensions see the UID | Service test with an operator and a default-enabled tenant definition. |
| Logs and metadata carry it for every source | Observer tests for configured and BYOK: model logs carry `caller_identity`, generations `gateway.caller_identity`; internal service without a user: absent. Allowlist test: accepted string, refused oversized and non-string. Update `TestRequestObservation_AuthenticatedTrustBoundary`, which asserts today that the UID is absent. |
| No metric series gains a label | Gather the registry after calls with a UID: label names per family unchanged, no label value contains it. |
| Provider request snapshots unchanged | No fixture regenerated; `mise run parity-check`; extend the observation bridge test so provider headers and options carry no UID; a BYOK outbound test asserts the captured request bytes don't contain it. |

## Risks / Trade-offs

- [Breaking `Call.Model`] → v0, unreleased; the guide and godoc change in the same PR.
- [The pipeline's stream outcome rule could drift from ProviderWire's first view] → Both derive from whether a part reached the consumer, and the pipeline decides before closing its output; a test asserts the record and overview agree for a single-candidate stream that closes before, and after, its first part. A future adapter that cancels before observing the close would turn such a stream's attempt from failed to canceled.
- [UID oversize] → Logs omit it; extensions receive it. Grafana UIDs are short ASCII.
- [Tenant extensions see provider instances and models of operator catalogs] → The caller already receives them in `gateway.execution`; no new exposure.
- **#764:** contract change only; the validator doesn't use `Call`.
- **#765:** compatible. Selection facts are bound at `Compose`, request facts at invocation, so per-namespace list selection can move to invocation. Candidate copies scale with tenant extensions per call, bounded by the attempt bound.
- **#757 merge:** text conflicts in `Selection` fields, the capture construction in `runtime.go` (already conflicting with #762), `observation.go`, the log allowlist case, `allowedAgentMetadataKeys`, `text-observability.md` and `observation_test.go`. After the merge, add `Call.Catalog` from `Selection.Catalog`, and the version once a selection carries it. Attempts carry no messages, so request-scoped redaction doesn't affect them.

## Migration Plan

None. Without extensions, only logs and Agent Observability metadata change (one key each). Rollback is a revert.

## Recorded decisions

- Log key `caller_identity`, metadata key `gateway.caller_identity`.
- The UID goes to extensions, logs and Agent Observability metadata; never metric labels or provider requests; other user claims stay private.
- Attempts by fan-out from the capture; a single candidate reports one attempt, a pre-model refusal none; no credential-bearing fields.

## Follow-ups

- **#757 merge:** `Call.Catalog` and catalog version, and the stack-catalog column's evidence.
- **#765:** document that tenant extensions see the caller UID, and that tenant observers see the provider instances and provider models of operator-catalog candidates and attempts.
