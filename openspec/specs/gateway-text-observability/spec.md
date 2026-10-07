# gateway-text-observability Specification

## Purpose

Define privacy-safe, bounded-cardinality, once-per-logical-call observability for Gateway text generation across logs, Prometheus metrics, and Agent Observability. Define the separate operator-owned guard boundary without changing metadata-only recording.

## Requirements
### Requirement: One startup-composed logical middleware chain
The Gateway SHALL wrap each configured canonical catalog entry's logical text model exactly once at startup with approved context enrichment, Agent Observability recording, structured logging, Prometheus model metrics, canonical public identity, and then the inner model in that request order. In WP8 the inner model SHALL be the direct provider; WP9 MAY replace it with fallback beneath the unchanged logical wrapper and SHALL NOT wrap physical candidates with WP8 observers. Response observation SHALL occur in reverse order. Alias and canonical resolution SHALL return the same composed model instance, and one invocation SHALL traverse each logical observer exactly once.

The observer chain SHALL be shared service behavior below public API adapters. It SHALL NOT be constructed in ProviderWire request handling and SHALL NOT change ProviderWire validation, public bytes, error mapping, commitment, stream ordering, timeout, or cancellation precedence. Separately enabled Gateway guards SHALL own policy admission and delayed commitment outside the logical model invocation; those changes SHALL NOT be attributed to recording middleware.

#### Scenario: Alias uses one canonical chain
- **WHEN** an authenticated text request resolves an alias for a configured canonical model
- **THEN** the catalog SHALL return the same startup-composed model used by the canonical ID
- **AND** logging, metrics, and Agent Observability SHALL each observe one logical call

#### Scenario: ProviderWire behavior remains unchanged
- **WHEN** equivalent unary or streaming text calls execute with and without the logical observers in a deterministic test
- **THEN** both calls SHALL invoke the direct model once with semantically identical call options
- **AND** SHALL produce the same ProviderWire status, response events, event order, and clean-EOF behavior

### Requirement: Canonical public identity on every logical surface
Logical model telemetry SHALL identify provider as `grafana` and model as the canonical public catalog ID from startup composition. Requested aliases, provider instance names, provider types, backend model IDs, provider response IDs, response-derived model identity, and routing topology SHALL NOT appear in logical logs, Prometheus labels, Agent Observability model fields, or Agent Observability metadata.

Provider response metadata SHALL remain unmodified for the protocol adapter and later private physical-attempt observation. Logical observation SHALL ignore that metadata rather than rewriting the provider result or stream part.

#### Scenario: Backend response identity differs
- **WHEN** canonical model `grafana/assistant` receives a result or stream metadata naming provider `anthropic`, a backend model ID, and a provider response ID
- **THEN** every logical log, metric, and Agent Observability record SHALL use only `grafana` and `grafana/assistant` for model identity
- **AND** none of the backend identity or response ID values SHALL occur on those surfaces
- **AND** the downstream ProviderWire handler SHALL receive the original result or stream parts unchanged

#### Scenario: Alias is requested
- **WHEN** a caller invokes alias `assistant` for canonical model `grafana/assistant`
- **THEN** no logical model telemetry SHALL use `assistant` as its model identity
- **AND** logical model telemetry SHALL use `grafana/assistant`

### Requirement: Trusted bounded request observation context
The Gateway SHALL create one private immutable observation view per accepted HTTP request. It SHALL contain only a bounded Gateway-generated opaque request correlation ID, normalized verified caller service and namespace from the authenticated caller context, and validated static region and application values when configured. Empty optional values SHALL be omitted.

The Gateway SHALL NOT enumerate arbitrary context values or derive these fields from unverified request headers, raw or parsed tokens, full auth claims, acting-user email, permissions, groups, prompts, tool data, provider options, provider metadata, or request/response bodies. The observation view SHALL NOT mutate provider call headers or provider options.

Correlation SHALL remain server-owned observation metadata. Neither Gateway client SHALL send or receive a new correlation field or header under this capability.

#### Scenario: Authenticated request is enriched
- **WHEN** authentication stores a normalized caller and static region/application are configured
- **THEN** logical logs and Agent Observability metadata SHALL contain the same request correlation ID, caller service, namespace, region, and application under fixed keys
- **AND** the direct provider's call headers and provider options SHALL contain none of those values unless an independent future policy explicitly approves them

#### Scenario: Untrusted headers attempt enrichment
- **WHEN** an authenticated request supplies arbitrary request, region, application, user, or tenant headers
- **THEN** those header values SHALL NOT replace or extend the trusted observation view
- **AND** none SHALL become logical metadata or provider-bound options through this capability

#### Scenario: Optional deployment context is absent
- **WHEN** region or application has no configured trusted value
- **THEN** the field SHALL be absent from logical telemetry rather than populated from request input or a placeholder

#### Scenario: Existing clients require no change
- **WHEN** the registered Vercel client or WP7 Go client performs a text call
- **THEN** no new request field, request header, response field, or response header SHALL be required or returned for observability correlation

### Requirement: Privacy-safe structured model logs
The Gateway SHALL emit fixed structured start and one terminal record for unary and streaming text using the reusable logger middleware. It SHALL keep all capture flags and per-stream-part logging disabled and SHALL apply a Gateway-owned attribute allowlist followed by default secret-key redaction.

Allowed logical values SHALL be limited to fixed event/schema names, a bounded generated call/correlation ID, call type, canonical public identity, closed outcome and error classifications, normalized status/retryability, duration, usage counters, unified finish reason, warning type/count, bounded stream part type/count, time to first output, and the trusted observation context. Logs SHALL omit prompt/output/reasoning text, tools, files, raw parts, request/response bodies, headers, provider options/metadata, raw finish reasons, arbitrary warning feature/message/detail strings, arbitrary error text, provider response metadata, credentials, backend identity, and topology.

#### Scenario: Unary provider failure contains secrets
- **WHEN** a unary provider error contains a credential, provider URL, backend model ID, response body, and arbitrary message
- **THEN** the logical terminal log SHALL retain only its approved closed error classification and timing
- **AND** no secret or provider-originated value SHALL be emitted

#### Scenario: Stream contains payload and provider metadata
- **WHEN** a stream contains text, response metadata, provider metadata, a provider error part, usage, and finish
- **THEN** logical logs SHALL include only approved lifecycle summaries and counters
- **AND** no per-part log or payload/provider detail SHALL be emitted

### Requirement: Bounded-cardinality logical model metrics
The Gateway SHALL register the reusable Prometheus model middleware exactly once against the existing service-owned registry and expose its collectors on the existing unauthenticated `/metrics` route. Model metrics SHALL use requested identity mode.

Labels SHALL be limited to operation, canonical configured public model identity, closed status and error classes, normalized status code (`100`–`599`, `none`, or `other`), unified finish reason, token type, and closed stream-part type. Caller, namespace, correlation ID, alias, region, application, provider instance, backend model, response ID, arbitrary error, and arbitrary stream values SHALL NOT be metric labels. Duplicate registration SHALL fail startup before readiness.

#### Scenario: Unary and streaming text are scraped
- **WHEN** authenticated unary and streaming calls complete and `/metrics` is scraped
- **THEN** the scrape SHALL expose request count, in-flight calls, duration, token usage, and applicable stream timing/count metrics for the canonical public model
- **AND** no forbidden label value SHALL occur

#### Scenario: Collector registration collides
- **WHEN** model collectors cannot be registered exactly once in the service registry
- **THEN** startup SHALL fail before listener binding and readiness

### Requirement: Metadata-only Agent Observability recording
When Agent Observability export is enabled, the Gateway SHALL use one process-wide client configured for metadata-only content capture, recording middleware only, requested canonical identity, and an allowlisted context provider. Hooks on that export client, experimental SDK features, and request-controlled capture changes SHALL remain disabled. Optional Gateway guards SHALL use an independent strict HTTP client; guard enablement SHALL NOT enable content recording.

Each logical model invocation for an authenticated unary or streaming text call SHALL produce one generation containing canonical public model identity, generation mode, approved trusted metadata, normalized usage marked as cache-inclusive without adding cache buckets again, unified finish reason, timing including streaming first output when available, and a closed error classification when applicable. It SHALL omit input/output content, system prompts, detailed errors, response/provider IDs, backend identity, provider options/metadata, raw artifacts, credentials, and topology.

The Agent Observability client MAY add only its fixed SDK provenance/content-capture metadata markers and a mirrored closed error category after the Gateway filter. These fixed client-owned fields SHALL NOT carry request input, provider detail, exporter configuration, or arbitrary error text.

#### Scenario: Successful text generation is recorded
- **WHEN** an authenticated unary or streaming text call succeeds with Agent Observability enabled
- **THEN** exactly one metadata-only generation SHALL be finalized with canonical public identity, approved context, usage, finish, and timing
- **AND** its exported record and span SHALL contain no text payload or backend identity

#### Scenario: Provider error is recorded safely
- **WHEN** a provider call or stream part carries an error containing private provider details
- **THEN** the Agent Observability generation SHALL retain only the metadata-only closed error state supported by the exporter
- **AND** provider error text and details SHALL not be exported

#### Scenario: Export is disabled explicitly
- **WHEN** Agent Observability export is explicitly disabled in a development or test configuration
- **THEN** logging and Prometheus observation SHALL continue
- **AND** the model call SHALL not start an Agent Observability generation

### Requirement: Strict bounded Agent Observability exporter configuration
Gateway-owned settings SHALL explicitly select and validate Agent Observability enablement, protocol, endpoint, transport security, authentication secret reference, finite batch and queue sizes, finite payload bytes, finite retry/backoff behavior, a positive per-attempt HTTP/gRPC export timeout, and finite flush/shutdown durations before client construction. The export-attempt timeout SHALL default to 10 seconds and SHALL be no greater than 5 minutes, independently of flush/shutdown deadlines. Ambient SDK timeout, retry, queue, and experimental-feature environment settings SHALL be rejected under their supported current and legacy spellings before client construction or secret resolution, without exposing their values in diagnostics. Literal credentials SHALL not be representable in a YAML file or command argument. Production SHALL reject cleartext export and missing required credentials.

The exporter SHALL be asynchronous and fail open for model traffic. Queue saturation, serialization failure, transport failure, and rejected exports SHALL produce only bounded counters and fixed diagnostic classes; they SHALL NOT expose credentials, endpoints, payloads, generation content, or raw exporter errors. The process SHALL flush and shut down the client after request serving stops using an independent bounded context.

#### Scenario: Exporter configuration is unsafe
- **WHEN** production configuration enables an insecure endpoint, names an unset secret, or provides a non-positive resource bound
- **THEN** startup SHALL fail before constructing models, binding, or readiness
- **AND** no secret value SHALL appear in the error or log

#### Scenario: Export destination is unavailable
- **WHEN** export queueing or delivery fails after startup
- **THEN** model calls and ProviderWire responses SHALL continue unchanged
- **AND** only a fixed export-failure diagnostic and bounded counter SHALL be emitted

#### Scenario: Process shuts down with queued records
- **WHEN** shutdown begins with Agent Observability records queued
- **THEN** request work SHALL stop according to the existing process order
- **AND** the process SHALL boundedly wait for already-started recorders before flushing, while refusing new recorder acquisition after close begins
- **AND** export flush and client shutdown SHALL finish or be abandoned within the configured independent deadline

### Requirement: Logical text lifecycle semantics
Unary observation SHALL start immediately before the shared model invocation and finalize once after its result or pre-result error. Streaming observation SHALL start immediately before stream setup and finalize once after normal channel close, provider error observation, premature close, downstream cancellation, or timeout as represented at the model boundary. When upstream closure and downstream context cancellation are both observable at finalization, cancellation or timeout SHALL take precedence consistently across every logical observer. Every observer SHALL pass through the original result, error, request metadata, response headers, and stream parts without mutation.

Usage SHALL be recorded from the unary result or independently aggregated across every usage-bearing stream part using the established strongest-value semantics. Streaming time to first output SHALL start at the model call and stop at the first payload-bearing shared stream part. A provider error part SHALL be observed as an error without being made terminal by middleware; later parts and finish SHALL remain ordered and visible to ProviderWire.

#### Scenario: Stream error precedes later finish
- **WHEN** a stream emits usage, a provider error part, later text, and finish before closing
- **THEN** every part SHALL pass through unchanged and in order
- **AND** logical observers SHALL finalize once with the established error classification and strongest usage
- **AND** middleware SHALL not terminate the stream or trigger fallback

#### Scenario: Usage is split across stream parts
- **WHEN** different stream parts report independently stronger usage counters
- **THEN** the terminal logical log, metrics, and Agent Observability generation SHALL preserve the strongest normalized counters

### Requirement: Bounded stream observation cleanup
Every logical stream observer SHALL own only the tee channel it introduces. On downstream cancellation it SHALL stop blocking on output, close its output exactly once, finalize its logical observation exactly once, and drain only its immediate upstream until channel close or an absolute configured deadline. A continuously ready or non-cooperative upstream SHALL not retain a Gateway-owned observer or drain goroutine beyond that deadline.

The Gateway SHALL configure every observer with a finite drain duration no greater than the validated ProviderWire stream-drain duration. ProviderWire SHALL remain owner of protocol termination and its immediate input; observer cleanup SHALL not write protocol events or extend handler latency.

#### Scenario: Silent stream is canceled
- **WHEN** a committed stream is canceled while an observer tee is waiting
- **THEN** cancellation SHALL propagate through every logical layer
- **AND** each layer SHALL finalize and release its Gateway-owned work within its drain deadline

#### Scenario: Continuously ready upstream ignores cancellation
- **WHEN** an upstream keeps producing parts after cancellation
- **THEN** each observer SHALL stop draining at its absolute deadline even if receives remain continuously ready
- **AND** the HTTP handler SHALL not wait for observer drain completion

### Requirement: Work-package boundaries remain explicit
This capability SHALL end at one logical text-call observation. Work package 9 SHALL place fallback below this chain and SHALL exclusively own candidate attempt hooks, private provider/backend identity, retry decisions, winner selection, and physical outcomes. WP8 SHALL NOT wrap candidates individually or define physical attempt records.

Work package 6 SHALL own image/capacity integration, work package 7 the Go client, work package 10 production activation and smoke, work package 27 per-request Agent Observability controls, and each later content capability its own new observation mapping. Unsupported future content SHALL not be inferred from raw provider values in this change.

#### Scenario: Fallback is added later
- **WHEN** work package 9 composes ordered fallback beneath the WP8 logical chain
- **THEN** one client call SHALL still produce one WP8 logical record per surface
- **AND** only WP9 private records SHALL describe selected, failed, or canceled physical outcomes plus the actual fallback decision, with retry represented by `failed` and `willFallback=true`

#### Scenario: Later capability is unsupported
- **WHEN** a request activates a content family not owned by text runtime
- **THEN** WP8 SHALL not inspect raw input or provider data to synthesize telemetry for that family
- **AND** its owning capability package SHALL add explicit normalized observation later

### Requirement: Operator capture is independent from returned native values

Gateway metadata-only logger, Prometheus, enrichment and Agent Observability observations SHALL retain their existing payload exclusions and canonical logical model identity when native warning/source/response identity values become caller-visible. Arbitrary warning strings, source ID/URL/title/filename and response ID/modelId SHALL NOT become operator attributes, metric labels or metadata-only exported payloads. The observer chain SHALL pass original results/parts unchanged to protocol encoding; operator capture restrictions SHALL NOT censor returned values. Known credentials and another tenant's state SHALL remain protected independently of ordinary application scalar/display values.

Consumer applications SHALL be able to configure existing reusable middleware independently around providers/grafana without enabling server capture. Tests SHALL use middleware.WrapLanguageModel with Middleware.WrapGenerate to inspect actual unary warnings/content/Response.Body and WrapStream to observe forwarded PartStreamStart/PartSource/PartResponseMeta through a context-aware test tee. These hooks SHALL receive contracted values without mutating output; hook access SHALL NOT be equated with automatic built-in capture/export.

A separately configured consumer logger.Middleware with CaptureOptions.ResponseBody and a consumer-owned slog destination SHALL prove actual opt-in logging of the bounded Gateway unary body, including native nested identity/warnings/sources within its configured capture budget. Raw-body accessibility alone SHALL NOT be called capture proof. Tests/docs SHALL distinguish that logged Gateway body from native transport diagnostics and from typed native Response identity overwritten by both clients. This capability SHALL NOT add middleware/API surface, universal stream-warning/source export, an unsupported Agent Observability source recording representation or a diagnostic carrier.

#### Scenario: Native identity differs from canonical route
- **WHEN** an alias resolves a canonical route whose provider supplies a different native response ID/modelId
- **THEN** returned streaming identity and raw unary response identity SHALL retain native values
- **AND** logical operator metrics/logs/exports SHALL retain canonical identity and exclude native payload values

#### Scenario: Native warnings and display are returned without server capture
- **WHEN** a metadata-only configured service returns native warnings and URL/document sources with native IDs and file-path display
- **THEN** callers SHALL receive those values unchanged within protocol bounds
- **AND** server logger/metrics/metadata-only Agent Observability exports SHALL omit their arbitrary strings

#### Scenario: Consumer capture is separately configured
- **WHEN** consumer-owned middleware wraps providers/grafana with independent capture settings/destinations while server capture remains metadata-only
- **THEN** WrapGenerate/WrapStream tests SHALL observe contracted fields and a separate opt-in consumer logger test SHALL assert actual Gateway response-body capture at its own destination
- **AND** tests SHALL prove typed unary identity remains replaced while native identity is present in the Gateway response body
- **AND** neither observation location SHALL mutate responses to satisfy the other's capture settings
- **AND** hook access SHALL NOT imply automatic source/warning capture by every built-in middleware

### Requirement: Operator-owned policy is separate from caller input

Guards SHALL default to disabled. When enabled, Gateway SHALL run both preflight and postflight for unary and streaming inference. Authentication, request validation, and canonical public model resolution SHALL precede evaluation. Caller identity, headers, provider options, and bypass values SHALL NOT select the endpoint, policy tenant, credentials, or enablement. Disabled guards SHALL preserve existing unguarded behavior and SHALL send no hook requests.

The guard runtime SHALL use an explicit operator endpoint base and preserve its path prefix when joining `api/v1/hooks:evaluate`. Production SHALL require HTTPS. Gateway SHALL require explicit `basic` or `bearer` authentication, without a default mode. Basic SHALL require `guards.auth-username`; bearer SHALL reject a username. `guards.auth-secret-env` SHALL reference a runtime environment secret resolved before listener binding. Gateway SHALL refuse redirects and SHALL NOT retry hook RPCs internally.

Both phases SHALL identify provider as `grafana`, model as the resolved public catalog ID, and agent as `grafana-ai-gateway`. Gateway SHALL NOT reevaluate policy per fallback candidate or derive a conversation ID from request correlation.

#### Scenario: Caller attempts to replace operator policy
- **WHEN** an authenticated caller supplies tenant, authorization, endpoint, or bypass values
- **THEN** those values SHALL NOT change operator policy configuration
- **AND** protected or unsupported request values SHALL retain their validation rejection

#### Scenario: Alias request is denied before fallback
- **WHEN** preflight denies a request using a model alias
- **THEN** Sigil SHALL receive the resolved public model identity
- **AND** no provider candidate SHALL receive an inference call
- **AND** no provider-generation observation SHALL start

### Requirement: Guard disclosure does not enable recording

Preflight SHALL disclose the full supported prompt, system messages, history, tool definitions, arguments/results, and represented thinking to the configured Sigil service. Postflight SHALL disclose the effective request plus complete supported output in `input.output`. Gateway documentation SHALL state this disclosure even when recording is metadata-only. Guard evaluation SHALL NOT add payloads or raw rule diagnostics to Gateway logs, metrics, traces, or generation exports.

Guard metrics SHALL expose evaluation count and duration with only fixed phase and outcome labels. Phase SHALL be `preflight` or `postflight`. Outcome SHALL be `allow`, `deny`, `fail_open`, `service_failure`, `transform_failure`, `unsupported`, `canceled`, or `resource_failure`. HTTP duration SHALL include guard work. Logical provider observation SHALL remain around actual inference; postflight denial SHALL NOT erase incurred usage.

#### Scenario: Guarded request contains a prompt canary
- **WHEN** an allowed guarded request contains a unique prompt canary and generation export is enabled
- **THEN** the guard payload SHALL contain the supported content
- **AND** Gateway logs, metrics, traces, and generation exports SHALL omit the canary and raw guard diagnostics

### Requirement: Guarded input has a strict supported boundary

Guarded input SHALL support only text, represented reasoning, and function-tool payloads under a strict option allowlist. Gateway SHALL reject media, nested media, tool input examples, provider-defined tools, stored-history references, and provider options introducing hidden input with HTTP 400. Other options outside the guarded allowlist SHALL NOT pass through merely because the unguarded runtime supports opaque options.

Supported preflight transforms SHALL update the provider request atomically. Gateway SHALL reject unusable transforms with fixed HTTP 424 under both failure policies, without replaying original input. Required roles, part kinds, tool identity, unredacted numeric values, and reasoning integrity SHALL be preserved. Signed reasoning SHALL NOT change while retaining its signature.

#### Scenario: Hidden input is unsupported
- **WHEN** a guarded request uses media, stored response/item references, hidden instructions, or unsupported request options
- **THEN** Gateway SHALL return HTTP 400 without calling a provider
- **AND** it SHALL NOT silently omit that input from policy evaluation

#### Scenario: Safe redaction reaches every fallback candidate
- **WHEN** preflight transforms supported text and the logical model later selects fallback
- **THEN** preflight SHALL execute once
- **AND** each attempted candidate SHALL receive the effective transformed request

#### Scenario: Transform changes reasoning or rounds JSON numbers
- **WHEN** a returned preflight snapshot changes reasoning, required identity, or numeric values through canonicalization
- **THEN** Gateway SHALL return fixed HTTP 424 even with fail-open enabled
- **AND** no provider SHALL receive original input as a substitute

### Requirement: Strict verdicts and explicit failure policy

Gateway SHALL accept only explicit `allow` and `deny` from a successful hook response. Missing, null, unknown, duplicate, or malformed verdict fields SHALL be response failures, not normalized allow. A valid explicit deny SHALL remain authoritative despite malformed optional diagnostics and SHALL return fixed HTTP 403 `forbidden`. Closed guard failures SHALL return fixed HTTP 424 `failed_dependency`. Both envelopes SHALL keep `param:null` and omit rule IDs, reasons, endpoint details, and raw errors.

`guards.fail-open` SHALL default to `false`. Enabled fail-open MAY continue eligible guard-service transport or response failures and SHALL record fixed outcome `fail_open`. It SHALL NOT override explicit deny, unsafe transforms, unsupported input, caller cancellation, or local resource exhaustion. Admission and guard body/buffer exhaustion SHALL return local HTTP 424 under both policies.

#### Scenario: Guard service response is unavailable or invalid
- **WHEN** a hook times out, returns non-200, or returns an invalid verdict while the parent operation remains active
- **THEN** fail-closed SHALL return HTTP 424
- **AND** fail-open MAY continue eligible work with outcome `fail_open`

#### Scenario: Valid deny contains malformed optional diagnostics
- **WHEN** Sigil returns a valid explicit deny with malformed optional fields
- **THEN** Gateway SHALL return HTTP 403 under either failure policy
- **AND** no successful inference body SHALL be released

### Requirement: Complete output precedes successful release

Gateway SHALL validate and withhold successful unary output and streaming headers/frames until a complete selected output passes postflight. Streaming SHALL require a valid finish; EOF alone SHALL NOT approve partial output. Valid finish SHALL permit postflight without waiting for provider channel close. Canceling completed model work SHALL NOT cancel the active postflight context. Postflight denial or failure SHALL NOT restart fallback.

For an active operation, an explicit postflight deny SHALL return fixed HTTP 403, even with `transformed_input` or malformed optional diagnostics. With explicit allow, every present postflight `transformed_input` SHALL return fixed HTTP 424 under both failure policies, even when unchanged. Neither SHALL release original output or tool-argument deltas. Unsupported generated content SHALL fail closed instead of disappearing from the evaluated output.

#### Scenario: Completed tool call is denied
- **WHEN** postflight denies completed function-tool output
- **THEN** Gateway SHALL return HTTP 403 before committing SSE success
- **AND** a consumer-owned tool loop SHALL receive no executable call

#### Scenario: Unchanged postflight snapshot is present
- **WHEN** an allow response includes `transformed_input` identical to the effective request and output
- **THEN** Gateway SHALL return HTTP 424 under either failure policy
- **AND** no original text or tool delta SHALL reach the caller

#### Scenario: Stream finishes without channel closure
- **WHEN** validated provider output contains a valid finish but the provider channel remains open
- **THEN** Gateway SHALL proceed to postflight under the active operation context
- **AND** it SHALL release validated frames only after approval without a transform

### Requirement: Guard resources and deployment limits remain explicit

One `providerwire.model-duration` deadline SHALL bound both guard phases and inference, defaulting to 120 seconds. Each hook SHALL have a separate maximum timeout, defaulting to five seconds within the remaining operation budget. Existing stream idle, part, frame, cancellation, and finite drain bounds SHALL still apply. Guard timeout headers SHALL use the remaining phase budget within Sigil's accepted 1–119999 millisecond range.

Guard defaults SHALL limit request/decompressed-response bodies to 4 MiB, additional retained guard data to 8 MiB per operation, and active guarded operations to eight. Body configuration SHALL NOT exceed 4 MiB. Retained data accounting SHALL include projections, serialization, transforms, and buffered frames; conservative charges MAY reject content below nominal limits. The guard budget SHALL NOT be described as a heap cap. Documentation SHALL explain that metadata-only recorders retain content before export filtering and require production memory/latency measurement with recording enabled.

Documentation SHALL state that a strict verdict client cannot prove rule execution. No matching rules, disabled evaluators/policy services, and server-side transform-failure continuation can produce allow. Evaluator coverage of thinking and tool results, input versus output targets, and judge truncation below HTTP limits SHALL remain deployment checks. Gateway SHALL NOT claim that postflight undoes provider-executed side effects.

Guard judges SHALL use a direct or separately isolated inference route until recursion prevention is designed. Untrusted bypass headers SHALL NOT solve recursion. Source-checked codecs and synthetic command/handler/frontend tests SHALL NOT be described as live Sigil/provider parity or deployed ingress authorization proof. Cloud Basic instance-ID and `sigil:write` token guidance SHALL NOT imply that the configured hook ingress is deployed or accepts those credentials.

#### Scenario: Local guard capacity is exhausted
- **WHEN** admission or retained-output capacity is exhausted with fail-open enabled
- **THEN** Gateway SHALL return local HTTP 424 and release no successful content
- **AND** it SHALL NOT select another fallback candidate

#### Scenario: Caller cancels during evaluation
- **WHEN** the caller cancels during preflight or postflight with fail-open enabled
- **THEN** Gateway SHALL cancel the hook request and stop the operation
- **AND** it SHALL NOT start provider work or release buffered output after cancellation
