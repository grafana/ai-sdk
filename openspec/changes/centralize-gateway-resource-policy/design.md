# Design

## Context

See proposal.md for motivation. This change builds on #428 (`nrbrd/app-platform`), where the Gateway is an App Platform plugin:
- `internal/gateway/routes.go` owns a `ServeMux` for the `config` and `language-model` routes, behind the access middleware. A fixed `routeHandlers` struct sends configured and BYOK callers to two separate ProviderWire handlers, and refusal always writes the ProviderWire permission document.
- `gateway.go` builds the handlers from ProviderWire types: `providerv4.Limits` in settings, `providerv4.RequestSelector`/`Selection` and the policy sentinels in `service/selection.go`, and `providerv4.HostErrorWriter` in discovery and access. Config validation constructs a throwaway ProviderWire handler just to check durations.
- The host delivers each request body as one route RPC message, limited by the plugin SDK's default 16 MiB gRPC receive size. Responses go back as one message per `Flush`; a unary response is one message. The platform also ends routes after 60s and applies front-door limits.
- There is no `http.Server`, listener, JWKS client or container image any more. Aggregate admission belongs to the plugin host; #100 is closed.

Limits on the request/response path today:

| Limit | Where | Default |
| --- | --- | --- |
| Front door, route RPC message size, 60s route deadline | plugin host | platform-owned, 16 MiB receive |
| `max_request_bytes` setting | ProviderWire `readBody` (`LimitReader`) | 1 MiB |
| unary response bytes (constant) | `unarySuccessPreflight`, final encode, error docs (`min(…, 64 KiB)`), summary `sourceBytes` | 8 MiB |
| stream frame bytes (constant) | `streamEventPreflight`, warning count, source/reasoning/custom sizing, fixed-frame construction check | 1 MiB |
| stream parts (constant) | per-request part counter in consume and drain | 100,000 |
| `maxRawUsageBytes`, `maxErrorResponseBytes` | ProviderWire constants | 1 MiB, 64 KiB |
| `ProviderResponseBytes`, `ProviderResponseHeaderTimeout` (constants) | shared outbound `boundedTransport` (all providers, BYOK) | 16 MiB, 10s |
| `max_model_duration` setting; idle and drain durations (constants) | ProviderWire runtime | 120s/30s/1s |
| BYOK `maxCredentials` | `byok.DecodeRequest` (all arrays) and `byok.New` | 8 |
| Go client `Limits` | `providers/grafana` | discovery 4 MiB, unary 16 MiB, error 64 KiB, stream 64 MiB, event 1 MiB, 100k events, raw 1 MiB |

Adapter-agnostic machinery that lives in `providerwire/v4` today:
- `selectModel` (selection worker under the deadline) and `invokeModel` (unary worker);
- in `serveStream`, the setup handoff and late-stream cleanup; `waitStreamPart` and `drainStream`;
- `safeErrorFromProvider`/`FromTransport`/`FromResolution` (classification);
- `attemptCapture` and `executionRequest.snapshot` (attempt views);
- `IsProtectedCallHeader`, `ReservedProviderOptionNamespace` and `gateway` control extraction (host request policy).

What a second adapter needs, from the evidence:
- **#248 (Chat Completions)** copied the limits, the body read, a `DoGenerate` worker without panic containment, a `drain` loop and an `errorStatus` classifier. It edited the router, process, catalog and telemetry code to add itself, and had no attempt capture.
- **#427 (Anthropic Messages)** names #394 as its preferred first step.
- **Upstream service docs** accept `providerOptions.gateway` (`byok`, `models`, `order`, `only`) on the Chat Completions and OpenResponses APIs. These are current service docs, not pinned evidence.
- **App-sdk manifests** allow nested custom route paths, so future `v1/...` routes can be declared.

Settings are `GF_PLUGIN_*` keys from Grafana's plugin section; only `max_request_bytes` and `max_model_duration` are configurable limits. Every Gateway-constructed model uses the shared outbound client from `gateway.go`. The Anthropic and OpenAI SDKs do not set `Accept-Encoding`, so `http.Transport` decompresses transparently, and `boundedBody` already counts decompressed bytes.

## Goals / Non-Goals

**Goals:**
- Shared bounds that every adapter inherits: the route request-body bound and the outbound response bound. Host-owned bounds are documented.
- One execution runtime that owns selection, deadlines, invocation, stream lifecycle, classification, attempt views and host request policy for every adapter.
- One host composition that registers adapters as routes, without per-adapter edits to dispatch, access, telemetry or error documents.
- Adapters own only mapping, encoding, error documents and their protocol state machine. They keep correctness validation, encode once and hold no size accounting.
- The server and the Go client accept the same outputs by default.
- ProviderWire's observable behavior changes only where size limits are removed.

**Non-Goals:**
- Host admission, capacity or plugin gRPC settings.
- Request structural validation, which #395 owns. The MCP option caps in `nativeoptions` stay unchanged.
- Decoder shape simplification in `providers/grafana`, which #375 owns. This change only touches limit plumbing there.
- JSON helpers (#396), raw output (#116), telemetry sink caps, configuration-file and discovery-catalog limits, and the `fallback` package buffer limits.
- New or renamed settings. The existing `max_request_bytes` and `max_model_duration` keep their names; fixed constants stay fixed.
- **New adapters, routes, `/v1/models` or `/v1/messages/count_tokens`.** This change delivers the contract that #248 and the #427 adapter rework onto; the test adapter proves it.
- **Inbound credential translation for native SDKs** (#248's `AdapterAuthenticator`, the Anthropic SDK's `x-api-key`). The platform authenticates before the plugin runs, so this is per-adapter client setup, plus #460 if the platform should accept more.
- **Changing the lifecycle rules for sequential native responses (#393).** The shared validator (D18) keeps today's rules and becomes the single place that decision lands.
- **Publishing BYOK attempt overviews (#420).** The runtime captures BYOK attempts (D19); encoding them stays with #420.
- **Shared warning, native-identity and diagnostics encoding.** Where these go on the wire differs by protocol; the runtime supplies the values.
- **Changing the selector input** beyond naming the raw `gateway` value "controls". The upstream service carries `providerOptions.gateway` on every API, so `(modelID, options, controls)` serves all adapters.

## Decisions

### D1. Route request-body bound on the mux
`gateway.newHandler` wraps the route mux once with `http.MaxBytesHandler`, using the existing `max_request_bytes` setting (1 MiB default). It moves from `Settings.ProviderWire.RequestBytes` to a route-level settings field.
- **One wrapper for every route:** `MaxBytesHandler` only replaces the request body with a bounded reader and reads nothing itself. Wrapping the mux once therefore leaves access refusal unchanged (refused callers' bodies are never read), and every route registered later, such as a Chat Completions adapter, inherits the bound without per-route code.
- **ProviderWire:** `readBody` becomes `io.ReadAll` plus close. A `*http.MaxBytesError` maps to the fixed invalid request (400), keeping today's classification; any other read error maps to internal.
- **Test server:** every ProviderWire handler the testserver constructs (`providerwire_v4.go`, `recorded_usage.go`, `structured_output.go`) is served behind the same wrapper.
- **Godoc:** the package godoc states that hosts own the body bound.

The check is cheap because the platform has already buffered the body, so it doesn't bound memory. It still matters: it bounds UTF-8 scanning, schema validation and mapping work for every adapter, below the 16 MiB route message size.

- *Alternative:* no Gateway-side limit, relying on the 16 MiB host message size. Rejected: that would let JSON of up to 16 MiB reach schema validation and mapping, and a future adapter served elsewhere would have no bound at all.
- *Alternative:* keep `RequestBytes` in each adapter. Rejected, because every new adapter would duplicate it.
- *Alternative:* raise or lower the plugin gRPC receive size. Rejected, because that belongs to the host.

### D2. Outbound transport stays per-response with fixed bounds
`outbound.NewAnthropicClient` becomes `outbound.NewModelClient`, since it serves every provider type. `config.ProviderResponseBytes` (16 MiB, below the Anthropic SDK's 32 MiB scanner limit) and `config.ProviderResponseHeaderTimeout` (10s) stay fixed constants. A gzip test pins the decompressed-bytes behavior.

- *Alternative:* an aggregate byte budget per logical call, across attempts. Rejected: attempts are already bounded by candidate count (16 configured, 8 BYOK) and the shared deadline, and it would need context plumbing through the fallback package.

### D3. Remove adapter output budgets and the part counter
ProviderWire `Limits` goes away; the remaining `ModelDuration`, `StreamIdleDuration` and `StreamDrainDuration` become `execution.Limits` (D11). This change removes:
- `RequestBytes`, `UnaryResponseBytes`, `StreamParts`, `StreamFrameBytes` and their settings;
- the canonical-frame construction check;
- `unarySuccessPreflight`, `streamEventPreflight`, `warningsPreflight`, `streamWarningCountFits`, `sourcePreflight`, `reasoningFileFits`, `responseIdentityFits`/`responseStringsFit`, the custom-part sizing and `minimum*Bytes`;
- `maxRawUsageBytes` and `maxErrorResponseBytes`;
- the `streamPartCounter`.

The drain loop is bounded by `StreamDrainDuration` alone, and the initial stream-state map capacity becomes a constant.

- *Alternative:* keep one final unary/frame cap. Rejected: it cannot prevent the allocation it measures, it can only fail a committed stream, and it duplicates the upstream bound.
- *Alternative:* keep `StreamParts`. Rejected: part volume derives from upstream bytes, which are bounded, and time is bounded by the total and idle deadlines (and the 60s platform deadline). Drain does not need a count once it has a deadline.

### D4. Validation is separated from sizing
The preflight functions are replaced by small validity helpers, called in place on every mapping path:
- UTF-8 strings;
- metadata namespaces: a valid UTF-8 name and a valid object (`{`-prefixed and `utf8.Valid`);
- raw usage: one valid UTF-8 JSON object;
- custom kinds;
- timestamps, token ranges and discriminators.

Provider lifecycle and correlation checks move to the runtime instead (D18).

These checks must stay explicit because `encoding/json` passes invalid UTF-8 inside `RawMessage` through unchanged. Behavior differs only where size used to be the reason for failure.

### D5. Encode once; optional data falls back only on encoding failure
- **Unary:** validate and map, attach `execution.Metadata` when an overview exists, then `json.Marshal` once. If marshalling fails, return the fixed internal error.
- **Finish:** the same, using one `encodeStreamFrame`.
- **HTTP and SSE errors:** build the base response plus optional metadata and `nativeError`, then marshal once. Only if that fails, emit the base document, which always encodes.

`execution.Metadata` always relocates a native `gateway` value. It returns nil only when the overview is absent or cannot be encoded.

- *Alternative:* keep the two-pass original-then-enriched approach. Rejected: the issue removes fit-based omission, and the second pass only existed to measure fit.

### D6. Execution projection drops `sourceBytes`
`execution.Project` and `execution.Summary` lose their `sourceBytes` parameter, along with `boundedText` and the oversized-body branches. Native bodies are bounded by the outbound transport. The request view that replaces `executionRequest`/`executionView` in the runtime (D11) has no `sourceBytes`, and the `sourceBytes == 0` disable branch goes too, so summaries are always eligible.

### D7. BYOK account bound is execution policy
`byok.DecodeRequest` keeps rejecting empty arrays and drops the `> maxCredentials` check. `byok.New` keeps the check, which only sees selected-provider accounts. The selector maps a `byok.ErrInvalidRequest` from `New` to `execution.ErrInvalidBYOK`, so the response remains the fixed BYOK credentials invalid-request error.

- *Alternative:* truncate to the first eight. Rejected, because it silently ignores caller credentials.

### D8. Go client limits: `DiscoveryBytes` + `ResponseBytes`
`Limits{DiscoveryBytes int64; ResponseBytes int64}`, with defaults 4 MiB and 64 MiB:
- discovery success and error bodies use `DiscoveryBytes`, which keeps parity with the TS helper;
- model-route unary success, non-2xx error bodies and the cumulative SSE stream use `ResponseBytes`.

`eventReader` drops `eventBytes`, `count` and their checks, and keeps the cumulative total. `wireUsage` drops `maxRawUsageBytes`. Pre-decode UTF-8/JSON validation stays. The 64 MiB default is four times the default outbound bound, which covers JSON re-encoding expansion in normal output.

- *Alternative:* no client limits, matching Vercel's fetch. Rejected: Go `io.ReadAll` on an untrusted endpoint needs a memory bound.
- *Alternative:* keep the per-event and count limits. Rejected: they reject server-accepted output and duplicate the cumulative bound.

These limit changes touch the same functions that #375 will simplify, so the two need reconciling when they meet.

### D9. Limit ledger in `ai-gateway/README.md`
The "Limits" section gets the ledger, next to the existing platform notes and the `max_request_bytes`/`max_model_duration` settings rows. Each group of work in tasks.md updates its own ledger rows:

| Retained limit | Owner |
| --- | --- |
| front door, route RPC message size (16 MiB receive), 60s route deadline, termination grace | plugin host |
| `max_request_bytes` | route handler |
| provider response bytes (16 MiB) and header timeout (10s), no-redirect, connection pool | outbound transport |
| `max_model_duration`, stream idle (30s) and drain (1s) durations | execution runtime |
| model file size (1 MiB), YAML string cap | configuration loader |
| catalog route ceilings (1,024 IDs, 16 candidates, 128 aliases, 2,048-byte strings) | discovery configuration |
| BYOK eight accounts (selected provider) | execution attempt policy |
| `agento11y.*` queue/batch/payload, physical-record and diagnostic caps | telemetry sinks |
| MCP option caps | request validation (#395) |
| Go client `DiscoveryBytes`, `ResponseBytes` | independent client |

### D10. Runtime package: extend `ai-gateway/internal/execution`
The runtime joins the existing `internal/execution` package, which already owns overviews, summaries and redaction. Its files map to its concerns: `limits.go`, `selection.go`, `policy.go`, `call.go`, `stream.go`, `classify.go`, `capture.go`.
- **ProviderWire's constructor** takes `*execution.Runtime`. `providerwire/v4` has no consumer outside this module (the command, testserver and tests), so it does not re-export runtime types or constructors; supporting outside-module construction is out of scope.
- **Host code** (`cmd/.../service`, `gateway`, `config`) imports `execution` directly. Sentinels move there without `providerwire v4:` prefixes: `ErrAccountAccess`, `ErrInvalidBYOK`, `ErrInvalidBYOKSelector`, `ErrUnsupportedGatewayControl`, `ErrReservedProviderOptions`.

- *Alternative:* a new public `ai-gateway/runtime` package. Rejected: there is no external consumer, and a public package would freeze an API before a second adapter uses it.
- *Alternative:* a separate `internal/runtime` package. Rejected: it would split the attempt capture from the overview projection it feeds, and `runtime` shadows the standard library in imports.

### D11. Runtime API: `Runtime` → `Call` → `Stream`
`execution.New(Config{Selector, Limits, ConfiguredAPIKeys})` returns an immutable `*Runtime`, built once by the host and shared by every adapter:
- **`Runtime.Begin(ctx, Request{ModelID, Options, Translate})`** applies host request policy (D12) and reconstructs tool history (D18). It installs the attempt translator (D17) and request-local attempt capture (D19), starts the execution deadline, and runs the selector on a contained worker, validating the selection. It returns a `*Call` or a classified failure.
- **`Call.Generate()`** is the unary worker: panic and `nil, nil` containment, prompt return on deadline or cancellation, late results discarded, and the result validated (D18).
- **`Call.Stream()`** runs `DoStream` with setup-to-adapter handoff, late-stream cleanup by the setup owner, and setup-failure drain. The returned `*Stream` exposes:
  - `Next()`, which returns a validated part (D18) or a finished, closed, invalid-output, canceled, total-timeout or idle-timeout outcome;
  - `Accept()`, which restarts the idle timer;
  - `Close()`, which cancels and starts one duration-bounded drain.
- **`Call.View(err)`** returns the overview and current-summary view with configured-key protection. `Call.Close()` seals capture and releases the deadline.
- **`Classify(err)`** returns a `Failure{Category, Reason}` from the closed category set and closed policy reasons. ProviderWire maps categories to its fixed documents.

Adapters own commitment (when to write headers), encoding and their state machine. The runtime never touches an `http.ResponseWriter`.

- *Alternative:* a runtime that takes encoder callbacks and writes responses. Rejected: commitment, framing and terminal-error encoding differ per protocol, so the callbacks would carry most of the adapter.

### D12. Host request policy in `Begin`
`Begin` takes the mapped `provider.CallOptions`:
- removes `ProviderOptions["gateway"]` and passes its raw value to the selector as controls;
- refuses the `grafana` and `grafana-ai-sdk` namespaces (`ReservedProviderOptionNamespace` moves here) and any `IsProtectedCallHeader` name, with closed reasons.

ProviderWire's `mapWireProviderOptions` stops special-casing these names, and `mapWireHeaders` keeps only its protocol rules: duplicate case-insensitive names and dropping `ai-language-model-*`/`ai-o11y-*`. `config/file.go` reads the reserved namespace from `execution`. Request structure stays with #395.

- *Alternative:* keep the policy in each adapter's mapper. Rejected: it is credential and host-namespace protection, and any adapter that forgot it would forward credentials.

### D13. One access-aware selector
`service.NewSelector(catalog, byokClient, byokFactory)` replaces `NewConfiguredSelector`/`NewBYOKSelector`. It reads the caller's access from the context:
- configured access uses catalog resolution, refusing non-nil controls as today;
- BYOK access decodes accounts from the controls;
- anything else returns `ErrAccountAccess`.

The host builds one ProviderWire handler instead of two, and `byAccess` remains only for discovery (D15).

- *Alternative:* keep one handler per population. Rejected: every adapter would double its construction and the routes would need dispatch wrappers.

### D14. Adapter registration table and host-error writers
`gateway/routes.go` replaces `routeHandlers` with a slice of `route{method, path, label, handler, errors}`. The host builds the mux from the table:
- the mux sits behind one `http.MaxBytesHandler` (D1);
- each entry sits behind access middleware whose refusal calls `entry.errors.WriteHostError(w, execution.HostPermission)`;
- labels come from the table.

`execution.HostErrorWriter` is a one-method interface with the closed host categories permission, internal and BYOK-discovery-unsupported. ProviderWire's `HostErrorWriter` implements it; the unused authentication category is removed, since the platform authenticates. A test asserts the table and the committed manifest paths match. The existing two-route requirement is unchanged.

- *Alternative:* an `Adapter` interface that registers its own routes. Rejected: routes must match the static manifest, and a table in one file is simpler to check against it.

### D15. Host-owned catalog listing, adapter-owned discovery projection
The ProviderWire discovery document moves from `cmd/.../internal/discovery` into `providerwire/v4` as its projection over `[]catalog.ModelInfo`. A host wrapper owns the rest:
- panic-safe listing;
- the access decision: configured access gets the projection, BYOK access gets the adapter's unsupported-discovery error, anything else gets permission;
- internal failures.

Future adapters such as `/v1/models` reuse the wrapper with their own projection.

### D16. Shared settings
`config.Settings` gets `Execution execution.Limits` and a route-level `MaxRequestBytes` in place of `ProviderWire providerv4.Limits`. Validation calls `Limits.Validate`, together with the existing minimum-duration check against the provider header timeout; it no longer constructs a ProviderWire handler.

### D17. Per-attempt native option translation
Some API semantics map to different native options per backend. #248 shows the need: Chat `store`, strict JSON schema and `parallel_tool_calls` map to:
- `openai.store`/`strictJsonSchema`/`parallelToolCalls` for OpenAI Responses;
- `anthropic.disableParallelToolUse` for Anthropic;
- `<namespace>.parallel_tool_calls` for OpenAI-compatible.

Fallback picks the backend inside the logical model, so the adapter cannot know it while mapping.
- **The translator:** the adapter passes `Translate func(execution.Target, provider.CallOptions) provider.CallOptions` in `Begin`. `Target{Provider, Namespace}` is the attempt's provider type and native option namespace (the compatible `providerName`, or the type for Anthropic and OpenAI).
- **Where it runs:** the host wraps every configured candidate and every BYOK account model once, at construction, with `execution.AttemptModel(model, target)`. It reads the request's translator from a runtime-owned context key, applies it to a copy of the options, re-checks host policy, and contains panics as non-retryable internal failures. Without a translator it is a pass-through.
- **Placement:** the wrapper sits below the observability middleware and inside fallback, so each physical attempt is translated and observed as it is sent.

- *Alternative:* adapter-specific model wrappers at catalog build, as in #248. Rejected: it couples the catalog to every adapter and leaks a private context key per adapter.
- *Alternative:* translate once before selection, for all candidates. Rejected: candidates in one route can differ in provider type.

### D18. Shared provider output lifecycle validation
The provider lifecycle checks in ProviderWire move into the runtime: `processStreamPart`'s text, metadata, reasoning and finish rules, `stream_tools.go`'s tool phases, `mapUnarySuccess`'s tool call and result correlation, and `unresolvedProviderCalls`.
- **Stream:** `Stream.Next` validates every part before returning it. Any violation ends the stream with an invalid-output outcome, cancellation and drain. Parts after a valid finish are suppressed, and `Next` reports finished.
- **Unary:** `Call.Generate` validates the result's content lifecycle.
- **History:** `Begin` reconstructs pending provider-owned calls from the prompt and seeds both validators.
- **What adapters keep:** only checks specific to their protocol. ProviderWire keeps UTF-8 and metadata shape for its encoding and the JavaScript safe-integer usage bound; Chat would keep its parallel and tool-choice rules.

The rules stay exactly as ProviderWire enforces them today, and its existing lifecycle tests are the regression contract. #393 can later change a rule once for every adapter.

- *Alternative:* each adapter validates in its own state machine. Rejected: #248 already re-implements the same text-ID and tool-phase rules, and protocol drift on provider correctness would be silent.

### D19. Attempt capture for every selection
`Begin` installs `fallback.WithAttemptObserver` for every selection. BYOK account models are already `fallback.New` chains, so they report attempts too. `Call.View` projects a configured overview only when the selection carries catalog candidates, exactly as today; BYOK attempts are captured but not published until #420.

### D20. One PR with ordered commits
Everything lands in one PR on #428. Commits follow the task groups in tasks.md, and each leaves `mise run test-ai-gateway` and `mise run test-grafana` passing:
1. route bound;
2. outbound;
3. BYOK;
4. client limits;
5. runtime extraction: selection, limits, invoke, stream, drain, classification, capture for every selection. The part counter and `sourceBytes` go here.
6. provider output lifecycle validation;
7. host request policy and per-attempt translation;
8. host composition: selector, route table, host errors, discovery, settings;
9. unary budgets;
10. streaming budgets;
11. integration.

The client commit comes before the server stops limiting output, so intermediate states stay coherent. Groups 5–8 keep ProviderWire's handler, race and command tests passing unchanged, except where they assert the removed part counter.

## Risks / Trade-offs

- **Per-request memory grows with the outbound bound, encoding expansion and concurrency.** Standard `json.Marshal` HTML-escapes `<>&`, up to 6×. The plugin route recorder also buffers each response body until `Flush` and clones it into one gRPC message, so a unary response is held about twice more. Route responses have no gRPC message cap (go-plugin uses `MaxInt32`), so no host limit fails them first. → Documented. Admission belongs to the host, and the outbound bound is a single shared constant to tune if needed.
- **ProviderWire library users with providers outside the shared transport lose adapter-level bounds.** → Package godoc and the ledger state that hosts own request, upstream and deadline bounds.
- **An in-process provider that floods parts without I/O.** → The total and idle deadlines and the platform route deadline still bound processing, and drain is time-bounded. No Gateway-constructed provider behaves this way.
- **Pathological escaping can exceed the client's 64 MiB default.** → Documented and configurable. The TS client has no limit.
- **Breaking API.** ProviderWire `Limits` and `providers/grafana.Limits` change; no plugin setting changes. → The PR title carries `!`; release automation is still inactive (#245).
- **Large single PR.** → Ordered commits per concern, and a PR description that maps commits to spec deltas.
- **Lifecycle regressions while moving the runtime.** The stream ownership, lifecycle and race tests are subtle. → Groups 5–8 move code with ProviderWire's handler, race and command tests unchanged as the regression contract. The test adapter adds cross-adapter evidence.
- **Translator misuse.** An adapter could use the translator to inject host-owned options. → Translated options pass the same host policy, and violations fail before provider contact.
- **Validator too strict for some native streams.** #393 shows sequential native responses that today's rules reject. → This change keeps the rules unchanged; #393 changes them once in the validator.
- **The runtime API fits one adapter's needs.** → The test adapter exercises it without ProviderWire. #248's handler maps onto `Begin`/`Generate`/`Stream` without needing response writing, which is the shape D11 rules out of the runtime.
- **Internal type in a public package's constructor.** Code outside the module can no longer construct a ProviderWire handler. → Accepted: there is no such consumer, and AGENTS.md does not require compatibility. Revisit with a public runtime package if one appears.
- **Test churn.** Existing preflight and over-limit tests and TS cases assert size failures. → Convert them to large-success or validity tests. Recorded provider fixtures stay untouched.
- **Coordination with #375, #395, #248 and #427.** → #375 and #395 touch the client decoders and request mapping. #248 and the #427 adapter must rebase onto the runtime and the route table. State the contract in the PR description so those owners can plan.

## Migration Plan

1. Open one draft PR against `nrbrd/app-platform` (#428). Retarget it to main after #428 merges.
2. Operators change nothing: `max_request_bytes` and `max_model_duration` keep their names and meaning.
3. Go client users replace the removed `Limits` fields with `ResponseBytes`. ProviderWire has no outside-module users to migrate.
4. Rollback means reverting the PR. No persisted state changes.

## Open Questions

- Whether the 1 MiB default request-body bound suits file-input workloads can be tuned through configuration later, without changing the approach.
