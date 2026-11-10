# Design

## Context

- #761 left one seam: `execution.Config.Compose` (`ModelComposer func(Selection) (provider.LanguageModel, error)`) runs in `selectModel` after validation; any error, panic or invalid model becomes `errInternal`. Production passes `service.ModelObservers.Compose`, which wraps `selection.Model` in the chain of `selection.Source`. `gateway.Options` is empty.
- Every logical model is `observers(identity(fallback(translate(native))))`; single-candidate models have no fallback wrapper.
- `Call.generate` and `Call.stream` call `attempts.enter()` before invoking the composed model. `attemptCapture.view` synthesizes one attempt for a single expected configured candidate when entered and no attempt was observed, failed unless the error is nil or `ErrInvalidOutput`.
- ProviderWire commits a stream as soon as `Call.Stream` returns, before reading the first part. A unary result is written only after `Generate` returns.
- The logger and Prometheus model middlewares (root modules, replaced locally in `ai-gateway/go.mod`) gain an optional `IsDenied` classifier in the `record-middleware-denials` change this one builds on. The Gateway log allowlist (`model_log_policy.go`) closes `ai_sdk.outcome` and `ai_sdk.error.type`.
- Request facts live in two places: `service` keeps the observation snapshot (correlation ID, namespace, caller service); `auth.Caller` keeps access and the identity actor, but not its type.
- `config` already parses one strict bounded YAML setting (`model_catalog`) with `apiKeyEnv` indirection; `configuredSecrets` feeds the summary redactor.
- #757 is built in parallel on the same base and appends `SourceStackCatalog`, a third observer chain and `CategoryCatalogConfiguration`.

## Goals / Non-Goals

**Goals:** a typed extension contract that cannot bypass the host; one pipeline for every selection; denials that are neither errors nor retries; operator configuration with the same safety rules as the model catalog.

**Non-Goals:** tenant configuration (#764, #765); `AroundModel` and `TransformResponse`; full request info such as UID, canonical ID, candidates and attempts (#763); per-namespace caching (#765); Agent Observability changes.

## Decisions

### Package layout

| Package | Role |
|---|---|
| `ai-gateway/pkg/extension` | Public v0 contract. Leaf: imports only stdlib, `provider`, `schema` and the Prometheus client. |
| `ai-gateway/pkg/gateway` | `Options.Extensions`, `Options.Mandatory`, `DefaultExtensions()`. |
| `ai-gateway/pkg/internal/extensionhost` | Registration checks, operator config, instance construction and close, ordered lists, the pipeline model. |
| `ai-gateway/internal/execution` | Uses `extension.Source`, classifies the sentinels, owns model entry and exports the policy check. |

Alternative: put the sentinels in `execution` and re-export them. Rejected because the public package would then import an internal one and stop being a leaf.

### Go shapes

```go
package extension

type Source string
const (
	SourceOperatorCatalog Source = "operator-catalog"
	SourceRequestBYOK     Source = "request-byok"
	SourceStackCatalog    Source = "stack-catalog"
)
// Keys is KeysOperator for SourceOperatorCatalog and KeysCustomer otherwise.
func (s Source) Keys() KeyOwnership

type KeyOwnership string
const (KeysOperator KeyOwnership = "operator"; KeysCustomer KeyOwnership = "customer")

type Scope string
const (ScopeOperator Scope = "operator"; ScopeTenant Scope = "tenant")

type FailurePolicy string
const (FailClosed FailurePolicy = "fail-closed"; FailSkip FailurePolicy = "skip")

type Definition struct {
	Name             string
	Scope            Scope
	Keys             []KeyOwnership // nil: both
	Sources          []Source       // nil: every source whose ownership is in Keys
	Priority         int            // ascending, transforms only
	FailurePolicy    FailurePolicy
	ConfigSchema     schema.Schema  // zero: only an empty configuration
	Secrets          []string       // operator entry supplies <name>Env
	EnabledByDefault bool
	New              func(config json.RawMessage, deps Dependencies) (any, error)
}

type Dependencies struct {
	Logger     *slog.Logger          // scoped with extension=<name>
	Registerer prometheus.Registerer // const label extension=<name>
	Clock      func() time.Time
	Secrets    map[string]string
}

type CallerType string
const (
	CallerUser            CallerType = "user"
	CallerServiceAccount  CallerType = "service-account"
	CallerInternalService CallerType = "internal-service"
)

// Call is one extension's view of one call. Each extension gets its own.
type Call struct {
	Namespace     string
	Keys          KeyOwnership
	Source        Source
	Caller        CallerType
	CorrelationID string
	Model         string // selection ID: canonical ID, or the BYOK selector
	State         any    // owned by this extension for this call
}

type Admitter interface{ Admit(ctx context.Context, call *Call) error }
type RequestTransformer interface {
	TransformRequest(ctx context.Context, call *Call, params provider.CallOptions) (provider.CallOptions, error)
}
type RequestInspector interface {
	InspectRequest(ctx context.Context, call *Call, params provider.CallOptions) error
}
type ResponseInspector interface{ InspectResponse(ctx context.Context, call *Call, part Part) error }
type Observer interface{ Observe(ctx context.Context, call *Call, record Record) error }
type Closer interface{ Close(ctx context.Context) error }

type PartType string
const (PartText PartType = "text"; PartReasoning PartType = "reasoning"; PartToolCall PartType = "tool-call"; PartFinish PartType = "finish")

type Part struct {
	Type     PartType
	ID       string // stream block or tool call ID; may be empty for unary text
	Text     string // stream delta or unary full text
	ToolName string
	Input    string // complete tool input
}

type Phase string
const (
	PhaseAdmit            Phase = "admit"
	PhaseTransformRequest Phase = "transform-request"
	PhaseInspectRequest   Phase = "inspect-request"
	PhaseInspectResponse  Phase = "inspect-response"
	PhaseObserve          Phase = "observe"
)

type Outcome string
const (
	OutcomeSuccess       Outcome = "success"
	OutcomeDenied        Outcome = "denied"
	OutcomeError         Outcome = "error"
	OutcomeCancelled     Outcome = "cancelled"
	OutcomeInvalidOutput Outcome = "invalid-output"
)

type Denial struct {
	Extension string
	Phase     Phase
}

type Record struct {
	Outcome  Outcome
	DeniedBy *Denial
	Usage    provider.Usage
	Started  time.Time
	Duration time.Duration
}

var (
	ErrDenied      = errors.New("extension: denied")
	ErrUnavailable = errors.New("extension: unavailable")
)
```

```go
package gateway

type Options struct {
	Extensions []extension.Definition
	Mandatory  []string
}

func DefaultExtensions() []extension.Definition
```

- The factory returns `any` because a definition is data and an instance's phases are discovered by type assertion; the host refuses an instance with no phase interface. A named empty interface would add no checking.
- Only admit and the two inspectors may deny. A transform that returns `ErrDenied` is a failure; this keeps "change params" and "deny" in separate phases.
- `ErrUnavailable` is the call's error after a `fail-closed` failure. An extension may return it, which is equivalent to any other non-denial error. The host never surfaces extension error text: it returns the bare sentinel upward.
- `ConfigSchema` uses the root `schema.Schema`, so authors can write `schema.SchemaFor[Config]()`, and #764 can validate `Middleware.spec.config` against the same schema.
- Names follow the Kubernetes DNS label rule, because #764 requires `metadata.name == spec.type`.

### Source vocabulary: alias in `execution`

`execution` keeps `type Source = extension.Source` and re-declares `SourceOperatorCatalog` and `SourceRequestBYOK` as the extension constants. The extension host uses `extension.SourceStackCatalog`. #757 adds `execution.SourceStackCatalog Source = "stack-catalog"` in the same const block: after merging, it is a text conflict in that block, both constants have the same type and value, and every existing `execution.Source*` reference keeps compiling. A full move would turn #757's references into compile errors. A follow-up can drop the alias.

### Placement: a layer between the observers and the selected model

The production composer becomes `observers.Compose(selection with Model = pipeline(selection))`, so every call is `observers(pipeline(entry(identity(fallback(...)))))`:

- Observers are outermost, so they see every denial, abort and unavailable outcome as the call's result and record `denied` without new plumbing.
- The pipeline is above fallback for every source, so fallback never sees an extension outcome and retries nothing, and routed and BYOK chains run the phases once per call.
- All phases run inside the pipeline's `DoGenerate` and `DoStream`. Request info comes from the call context there, so the pipeline needs nothing more from the composer.

**Deviation from the #761 follow-ups.** `ModelComposer` keeps its signature, and composer errors stay internal. Follow-ups 1 and 2 assumed admit would run in the composer, but then built-in observers would never see an admit denial, denials would take two paths (a `Runtime.Call` error and an invocation error), and #757's `Compose` hotspot would change. The goals behind both follow-ups are met differently: request info is read from the invocation context before the selected model is entered, and the sentinels pass through classification without a synthesized attempt (next section). A composer context can be added when a consumer needs it, for example #765's namespace config, which the pipeline can also read at invocation.

### Model entry

`Runtime.Call` builds the attempt capture right after selection, wraps `selection.Model` in an unexported `entered` model that calls `attempts.enter()` and then delegates, and composes around it. `Call.generate` and `Call.stream` no longer call `enter`. With no composer the order is unchanged. A pipeline refusal before the selected model is invoked leaves the capture unentered: no synthesized attempt and no overview. In `view`, `ErrDenied` and `ErrUnavailable` join `ErrInvalidOutput` as outcomes whose synthesized attempt is `selected`: after entry they can only come from response inspection, after the provider succeeded.

### Classification and documents

- `ClassifyProvider` and `ClassifySelection` check `extension.ErrDenied` (permission) and `extension.ErrUnavailable` (new `CategoryUnavailable`) first. `ClassifyStreamPart` checks them before the status-less provider-stream rule, because in-band outcomes travel as a status-less `*provider.APICallError` whose cause is the sentinel.
- ProviderWire reuses `safePermission` and adds `safeUnavailable` (503, `internal_server_error`, `unavailable`, "service unavailable", retryable). The stream error event for an extension outcome carries no native `Current` summary.
- `CategoryUnavailable` and `safeUnavailable` are appended at the end of their lists. #757 appends `CategoryCatalogConfiguration` at the same place, so the merge is a text conflict in those lists only.

### Ordering, filtering and the cached lists

At startup the host builds, for each of the three sources, the applicable instances in three precomputed orders: gates, inspectors and observers by name; transforms by scope (tenant first), priority and name. Applicability is ownership (`source.Keys()` in `Keys`) and, when set, `Sources`. A definition whose `Sources` include a source outside its `Keys` fails registration. Per call, the pipeline picks the lists for `selection.Source`; an unknown source is a composer error, as for the observers. #765 adds the config revision to this key.

### Request phases and the policy re-check

- Admit runs with a `Call` built from the context: namespace and correlation ID from the service observation snapshot, and the caller type from `auth.Caller`, which gains a type set in `Decide` (internal service when attributed, else the identity actor's type).
- Before each transform, the host clones the top-level `Headers`, `ProviderOptions` and `Prompt` containers, so a transform cannot mutate the runtime's or the caller's maps. Nested values are documented as read-only.
- After each transform, `execution.CheckPolicy` (today's unexported `allowedTranslation`, exported and shared with translation) checks the result. A refusal is that extension's failure, of class policy.
- The inspectors see exactly the options the pipeline passes to the inner model.

### Response phases

- **Unary:** after `DoGenerate` succeeds, the pipeline runs inspectors over the parts normalized from `Content` (text, reasoning, tool calls) and one finish. An abort returns `ErrDenied` (or `ErrUnavailable`) and drops the result.
- **Stream:** the pipeline returns its own channel and forwards from a goroutine. Text and reasoning deltas, tool calls and finishes are inspected before forwarding; every other part is forwarded as is. On abort it withholds the part, sends one `PartError` with a fixed message and the sentinel as cause, cancels the inner context, drains the inner stream bounded by the drain duration, and closes. Sends select on the model context, so a consumer that stops reading never blocks the goroutine.
- **Invalid output:** the pipeline seeds `execution.NewStreamValidator` or `ValidateGenerateResult` from `RebuildToolHistory` of the options it received, which are the runtime's own. Inspectors therefore see only parts the runtime will accept. On the first violation, the pipeline stops inspecting, forwards the part unchanged for the runtime to refuse, and records `invalid-output`.

### Observe

- Observe runs once per call, off the response's critical path. The record is built when the outcome is known: when `DoGenerate` returns, or when the forwarding goroutine ends a stream (close, abort, cancellation). For a unary call, the dispatch happens after the result or error is returned to the caller's layer. For a stream, it happens after the pipeline has closed its output channel.
- The host's `observations` dispatcher starts one goroutine per call that runs every applicable observer in name order, including those after a denier, each with `context.WithoutCancel(ctx)` bounded by a named per-call `observeTimeout` of 1 second. It counts work with a `sync.WaitGroup` under a mutex-guarded `closing` flag, like `AgentObservabilityRuntime`. After `Close(ctx)` begins, new observations are dropped and counted (`grafana_ai_gateway_extension_observations_dropped_total`), and `Close` waits for in-flight ones until `ctx` ends.
- Usage comes from the result, or from the shared streaming usage aggregation the observers use. Outcome precedence matches the observers: cancellation, then denial, then invalid output, then error, then success.
- Alternative: run observers synchronously on the call. Rejected: a slow billing or audit exporter would add its latency to every unary response.

### Failure policies and panics

Every call into extension code goes through one `invoke` helper with recover-to-error. A failure is a non-denial error, a panic or a policy refusal. Under `fail-closed`, the call ends with `ErrUnavailable`: from `DoGenerate`/`DoStream` before entry, as an unary error after entry, or as an in-band part for streams. Under `skip`, the phase's effect is discarded: transforms keep their input, admit and inspectors count as passed. Every failure, including a discarded `skip` policy violation, increments `extension_events_total{event="failed"}` and logs one warning. Observe and close failures are counted and logged only. Factory failures and panics fail startup.

### Denied outcome in the observers

- The root middlewares' `IsDenied` (from `record-middleware-denials`): Prometheus status `denied` with `error_type` `none`; logger outcome and class `denied` at `Level`. A generic denial hook keeps both middlewares free of Gateway types.
- `NewModelObservers` sets both to `errors.Is(err, extension.ErrDenied)`, and the Gateway log allowlist accepts `denied` for `ai_sdk.outcome` and `ai_sdk.error.type`.
- Agent Observability is unchanged: a denied call records a generation without a finish, like any failed call, and its `CallError` is already cleared by the Gateway filter.
- Extension events: `grafana_ai_gateway_extension_events_total{extension, phase, event}`, where event is `denied` or `failed` whatever the failure policy, and `grafana_ai_gateway_extension_observations_dropped_total`, both registered once in `service.Telemetry`.

### Operator config

```yaml
# GF_PLUGIN_EXTENSIONS
quota:
  config:
    limit: 1000
  apiKeyEnv: GF_PLUGIN_QUOTA_KEY
audit:
  enabled: false
```

- Parsed in `config` with the model catalog's rules: at most 64 KiB, string keys, no aliases, merge keys or trailing documents. Unknown names, tenant names, and keys other than `enabled`, `config` and declared `<secret>Env` fail. An entry defaults to `enabled: true`.
- `config` is converted to JSON and validated with the compiled schema. A failure names the extension and the instance location, never the schema library's message, which can echo values.
- Order in `New`: registration checks before the idle check; settings; catalog; extension config shape and schemas; then secrets (provider keys, Agent Observability token, extension secrets). Extension secret values join `configuredSecrets`.
- Mandatory checks run on the parsed setting: no entry, `enabled: false`, an invalid config or a missing secret fails startup.

### Lifecycle

Enabled instances are built in `New` after secrets are resolved, before the handler. `sinks.close` adds one goroutine that first waits for in-flight observations, then closes every `Closer` instance concurrently, all under the same 1 second context. Close failures and panics are counted, and a close still running at the deadline is abandoned. A factory may run more than once per process, once per configuration it is given; the godoc says so.

### Tenant extensions in this change

A tenant definition enabled by default is built once at startup with an empty configuration and applies everywhere its ownership and sources allow. #765 replaces this with per-namespace resolution.

## Test plan

| Acceptance | Evidence |
|---|---|
| Phase order; instance without phases fails | `extensionhost` table tests with a recording extension per phase combination, unary and stream; startup test with a phase-less instance. |
| Transform order; gates and inspectors stop at the first deny | Ordering tests over mixed scopes, priorities and names; denial at each position asserts which phases ran. |
| `InspectRequest` sees the final params | Transform adds a system message and a header; the inspector's params equal the fake model's received options. |
| `InspectResponse` abort before and after commitment, unary and stream | Pipeline tests, plus ProviderWire handler tests: unary permission document with no result; committed stream with two deltas, one permission error event, end. |
| Shared per-call state | Admit stores a value, observe reads it; concurrent calls under `-race` see only their own. |
| `Observe` once for success, denial, provider error and cancellation | Counting observer per outcome; denial names extension and phase; failing and panicking observers leave the result unchanged. A blocking observer does not delay the unary return or the stream close; shutdown waits for it within the budget and drops and counts later observations. |
| Protected headers or `gateway` options refused before any provider call | Transforms injecting each, under both policies; fake provider records zero calls carrying them. |
| `ErrDenied` returns the permission document, no fallback | Gateway handler test over a two-candidate configured model: zero provider calls; output denial after a failed primary: two calls. Overview assertions for both. |
| Panic containment | Panics in factory, each phase and close; startup fails or the call follows the policy; other concurrent calls succeed. |
| Mandatory and secret-safe startup | `config` and `gateway.New` tables: disabled, unconfigured, invalid config, missing secret, unknown or tenant mandatory; messages contain no values; an idle host still fails invalid registration. |
| Ownership and source filtering | Operator-owned extension on BYOK runs nothing; undeclared runs on configured, routed and BYOK; `Sources` narrows, including with `SourceStackCatalog`. |
| Routed and BYOK get the pipeline | Production-composition test (as in #761's identity test) with one extension: phases run once per call for configured, alias, routed and BYOK, unary and stream. |
| Denied outcome in observers | Root middleware tests for `IsDenied` and nil; Gateway observer test: `status="denied"` series, no `error` series, log `ai_sdk.outcome="denied"`. |
| Extension metrics and logs | Counter and log tests per event; no error text or values. |
| Final flush | `gateway` test with a blocking closer: `Close` returns within the budget, other sinks flushed. |
| Client classification | `test/providerwire-v4/client-consumption.test.ts` gains the unavailable document and an in-band forbidden event; Go client equivalent. |
| External distribution | The #761 generated-module build test also compiles a `main` that defines an extension. |

Plugin-process tests stay unchanged. CI provides the e2e evidence.

## Risks / Trade-offs

- [A second lifecycle validator per call when extensions apply] → It is only built when an extension implements response inspection or observe; it keeps inspectors on valid parts and makes `invalid-output` exact.
- [Observers see the caller's params, not the transformed ones] → Intended: observers describe the caller's call. Logs capture no params anyway.
- [Extension logs bypass the Gateway allowlist] → Extensions are compiled-in trusted code; the guide states their privacy obligations.
- [Root middleware release] → Two new optional fields, nil by default; both modules get a minor alpha release.
- **#763:** `Call` grows (UID, canonical ID, candidates, catalog identity). Attempts for `Observe` need the runtime's capture, which sits below the pipeline; the `entered` model is the natural fan-out point, and #763 must keep the fallback observer unreplaceable.
- **#764:** the admission validator needs the registered tenant definitions, which `Run` already owns alongside `managedKinds`; it reuses `ConfigSchema` and the name rule.
- **#765:** see Follow-ups for #765.
- **#757 merge:** text conflicts only, in the `Source` const block (`selection.go`), the category lists (`classify.go`, `errors.go`), `NewModelObservers` options next to the third chain (`model_observability.go`), and `Runtime.Call` near the capture construction (`runtime.go`). `ModelComposer` is unchanged. No shared spec requirement is modified by both changes.

## Migration Plan

None: without extensions, behavior and bytes are unchanged, and the shipped build registers none. Rollback is a revert.

## Recorded decisions

- **Pre-model refusal overview:** no overview. The issue's wording ("the overview reports not attempted") is met by the absent `gateway.execution` metadata on the error document, not by an empty-attempt overview, which the owned schema and the routing summary do not express. The PR description records this.
- **Agent Observability:** unchanged for denials.
- **Composer signature:** unchanged; see Placement. The PR description records the deviation from the #761 follow-ups.
- **Policy re-check:** after each transform, with the extension's failure policy applied.

## Follow-ups for #765

- **Collector registration.** Factories will run once per namespace configuration revision. A factory that registers collectors through `Dependencies.Registerer` collides on its second run, so #765 needs either idempotent registration (handling `prometheus.AlreadyRegisteredError`) or a process-level setup step separate from per-configuration construction.
- **Close on eviction.** Instances built per namespace configuration must be closed when their cache entry is evicted or replaced, not only at shutdown, and close must not race in-flight calls or observations still holding the instance.
- **Cache key.** The ordered lists add the configuration revision to the source key.
