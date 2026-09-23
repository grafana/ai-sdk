# Upstream Parity Coverage

This document describes what the conformance baseline verifies against the
registered upstream target in `upstream.yaml`. It is a coverage map, not a claim
of total parity.

## Record Ownership

This file records stable coverage classifications, confidence sources, supported
boundaries and accepted deviations. Update it when one of those facts changes.

GitHub issues labeled `upstream-sync` are authoritative for actionable deferred
work, including its upstream evidence, Go difference, impact, scope, dependencies
and acceptance tests. Upgrade PRs own their target, validation and run-specific
issue list. Do not add dated target assessments, issue catalogs, upgrade-run ledgers
or copies of issue contents here, and do not mirror issue state in this file.

## Status Values

- `automated`: covered by committed tests or validation scripts.
- `manual`: requires source or test comparison during implementation or review.
- `mixed`: only the stated subset has automated or manual evidence.
- `documented-deviation`: intentionally different from upstream behavior or CI policy.
- `gap`: known missing coverage or metadata.

## Confidence Model

Conformance tests are both a parity checker and the repository's behavioral
confidence suite. Prefer them for any bug fix or feature whose behavior can be
expressed as recorded provider input plus expected upstream output. Use
hand-written Go tests for local invariants, error paths, and small helpers that
do not cross a provider or UI wire boundary.

When a reported bug can be reproduced by replaying provider chunks or asserting
provider requests, add or update the conformance fixture first, observe the Go
failure, and then fix the implementation. When adding a new parity-sensitive
feature, record or import upstream behavior alongside the implementation so the
fixture becomes the executable contract for future upgrades.

## Layered Coverage Map

### Core ai-sdk Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| `StreamText` orchestration lifecycle | mixed | `expected.jsonl` covers start, step, finish, and selected multi-step flows; root and provider-independent tests cover provider-ordered step content, locally generated tool-part reconciliation, abort/incomplete streams, per-step model call-setting overrides and validation, explicit empty active-tool disabling, deep per-step provider-option merging, runtime-context carry-forward, invalid/error `StepResult.Content`, per-step first-content timeouts, and semantic inter-content timeouts. | Intentional Go deviation: upstream `ai@7.0.65` emits locally generated approval requests adjacent to their tool calls during stream transformation, while Go batches local approval handling after provider streaming and appends those requests after recorded provider content. Both `StepResult.Content` and response messages preserve their SDK's respective order. The conformance harness cannot configure `PrepareStep`, so per-step call-setting behavior remains unit-test covered. Go does not expose upstream `onLanguageModelCallStart` / `onLanguageModelCallEnd` callbacks; provider metadata remains available through step and result surfaces instead. Add fixtures for newly supported step options or stop conditions. |
| `TextStreamPart` to `UIMessageChunk` conversion | automated | `expected.jsonl` compares Go output with upstream `toUIMessageStream()` output, including provider-independent goldens for file, reasoning-file, and metadata-bearing empty text delta chunks. Root unit tests cover `CreateUIMessageStream` framing against `ai@7.0.65` `createUIMessageStream`: no added `start` or `finish` chunk, response-ID injection into a `start` chunk that lacks one, preservation of a written `start` message ID, last-assistant continuation with `OnFinish` replacing the continued message, `OnFinish` delivery without original messages, a generated response ID when no original messages are supplied, and abort and finish-reason capture. | New chunk types require fixture coverage before being treated as complete. Intentional Go deviation: `ToUIMessageStream` defaults the ID generator when original messages are present, while upstream `toUIMessageStream` injects a generated ID only when `generateMessageId` is supplied, except for last-assistant continuation ID reuse. `CreateUIMessageStream` always resolves a response ID, which matches upstream `createUIMessageStream`. Known gap: `CreateUIMessageStream` closes its writer when `Execute` returns, so a stream merged inside `Execute` that delivers chunks later loses them, while upstream keeps the stream open until every merged stream drains. |
| UI message wire format | automated | `expected.jsonl` verifies chunk type names, JSON fields, ordering, and deterministic IDs. | Frontend-specific hook behavior still requires source review. Adjacent locally-executed `tool-output-available` and `tool-output-error` chunks within a step are compared without regard to their order, since concurrent tools can complete in either order; see the `concurrent-tool-emission-nondeterminism` deviation. Provider-executed outputs and outputs of rejected tool calls remain exactly ordered. |
| UI message stream readers | mixed | Go unit tests cover `StreamUIMessage` progressive snapshots, `AssembleUIMessage` final assembly, partial tool input JSON repair, snapshot cloning, lazy IDs, metadata merging, finish-state callbacks, repeated tool-call IDs across steps, reasoning files and IDs, source document filenames, loose known-chunk decoding, unknown discriminator rejection, and default `ChunkError` handling compared against `ai@7.0.65` source. | Reader options intentionally do not yet expose upstream `message`, `onError`, or `terminateOnError`; `StreamUIMessage` follows upstream's default `terminateOnError=false` for error chunks. Custom chunks and parts are modeled, emitted, and assembled. Upstream-only tool UI fields such as `title`, `toolMetadata`, `rawInput`, and `preliminary` remain documented representation gaps rather than hidden reader guarantees. |
| SSE framing and parsing | automated | Conformance replay and framing tests verify fixture framing and stream parsing behavior. | Browser transport behavior is outside this suite. |
| Tool orchestration | mixed | Tool call, no-arg tool, approvals, parallel tools, selected multi-step fixtures, `toolChoice`, `activeTools`, tool provider options, and tool error simulation are configurable in conformance. Root tests cover upstream-compatible injective tool-approval signatures and guarded legacy verification, plus tools-independent auto defaults and explicit/per-step choice preservation across StreamText, GenerateText, and Agent. | Upstream's experimental tool-caller routing, including model-visible versus execution-only tools, local code-mode binding, and automatic provider `allowedCallers` preparation, has no Go orchestration API. Provider-specific `allowedCallers` can be configured manually, but this does not cover the core routing semantics. Add fixtures for newly reported tool-loop behavior. |
| Agent / ToolLoopAgent core orchestration | mixed | Root unit tests cover Agent identity, settings/per-call merge, `StepCountIs(20)` Agent default, direct `StreamText` one-step default preservation, callback merging, runtime context propagation, provider call-header marker insertion, approval/external/provider-executed tool inheritance, structured output, and stream error propagation. | No dedicated conformance fixture yet because `ToolLoopAgent` delegates to the existing `StreamText`/`GenerateText` path. Upstream `prepareCall`, `allowSystemInMessages`, `include`, `_internal` ID generators, tools-context/call-options-template behavior, telemetry, stream transforms, sandbox sessions, repair/refine hooks, `toolOrder`, TypeScript generic/schema inference, and `callOptionsSchema` are documented gaps or Go adaptations, not silent options. Provider network User-Agent/header behavior is a gap beyond the root `provider.CallOptions.Headers` boundary. |
| Structured output | automated | Fixtures with `expected-object.json` validate object, array, choice, and raw JSON output paths. The OpenAI `structured-json-output-length` fixture asserts the parsed `OutputValue` for a non-`stop` finish reason. Root unit tests cover ordered partial snapshots and array element streams, including delayed consumption beyond the channel buffer. | Parse and validation failures remain unit-tested because fixtures currently model successful output expectations only. `PartialOutputStream` and `ElementStream` are Go result APIs rather than provider or UI wire boundaries, so their delivery semantics remain unit-test covered. Upstream's stable `repairText` callback on the legacy object APIs is not exposed by the Go `Output` abstraction. |
| Error and warning behavior | mixed | Provider-independent abort fixtures cover upstream-visible incomplete stream behavior; provider errors and warning paths use focused Go tests plus upstream source comparison when no real recorded or pinned provider fixture exists. | Intentional Go deviation: upstream logs `GenerateText` and Agent streaming-only timeout warnings before model invocation; Go has no global warning sink and returns them through successful `GenerateTextResult.Warnings` instead, so failed or blocked calls cannot expose them. Add provider conformance fixtures only when authentic live or pinned provider input is available. |
| Usage, finish reason, and metadata propagation | mixed | Existing fixtures validate covered chunk fields and request snapshots; opt-in `expected-usage.json` snapshots compare per-step provider usage. | Add targeted fixtures for newly exposed metadata. |
| Cancellation and abort behavior | automated | Provider-independent `ui/stream-abort` fixtures compare actual `StreamText` cancellation UI chunks for no-output, partial-output, and pending-output streams against abort behavior re-attested from `ai@7.0.65` source and tests; focused Go tests cover `GenerateText` cancellation during and immediately after tool execution. | Provider transport cancellation remains covered by provider-specific unit tests rather than timed replay fixtures. |
| Validated URL downloads | gap | Upstream source review is required today. | The Go port does not expose the core `download` / `experimental_download` path used by `StreamText`, `GenerateText`, object APIs, and agents. Consequently it also does not implement provider-utils DNS pinning and redirect validation against DNS aliases or rebinding. |
| Realtime transport | gap | Upstream source review is required today. | The Go port does not expose upstream realtime model or browser transport APIs, including raw string and binary WebSocket event serialization or OpenAI realtime speech translation. |

### Provider Contract Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| `provider.LanguageModel` vs upstream `LanguageModelV4` | mixed | `mise run parity-provider-shape` reports discriminator drift; semantic interface changes still require source comparison. | Exact method/field-shape equivalence remains manual. |
| Language model batch contract | gap | Source review against provider v4 batch types and core `batch` orchestration is required today. | The Go contract and providers do not expose the registered baseline's optional batch submission APIs. |
| Speech translation model contract | gap | Source review against provider v4 and core `streamTranslate` is required today. | The Go port intentionally scopes its provider boundary to LanguageModel and does not expose the experimental speech-translation model, core streaming orchestration, OpenAI realtime translation factory, or conformance coverage. |
| `provider.CallOptions` mapping | mixed | Request snapshots validate behavior-affecting fields that current fixtures exercise. | Intentional Go adaptation: `CallOptions.Reasoning` uses a zero-valued provider-default enum, so omission and explicit ProviderWire `"provider-default"` normalize to one provider-domain value; unlike upstream's optional string, direct provider-domain JSON cannot preserve that presence distinction. Provider behavior is unchanged, and strict wire tests cover explicit normalization. New option fields require either fixture coverage or documented gaps. |
| Message and content part taxonomy | mixed | Provider request snapshots cover current message/content variants. The Anthropic `ui-tool-model-output` fixture verifies that persisted UI tool results apply `Tool.ToModelOutput` and reach the provider as multipart text/file content. | New content parts must include provider request assertions. Known Go conversion gaps versus `ai@7.0.65` include upstream `convertDataPart` support and some tool conversion semantics (`input-streaming` filtering default and output-denied default text); preserve existing Go behavior unless a focused parity change updates fixtures and docs. |
| Tool definitions and tool choice | mixed | Tool fixtures assert declared tool schemas and selected tool request behavior. Focused direct-provider tests reject inactive function/provider fields and malformed provider args before native I/O; nil Go provider args normalize to `{}` on Gateway request projection. | Forced choices and active tool subsets are known harness gaps. The Go nil-args default matches the registered provider-tool factories' empty-object default; strict HTTP still requires an explicit args object. |
| Provider metadata and options passthrough | mixed | Provider options used by current fixtures are covered by request snapshots. | Provider-specific option expansion requires new fixtures. |
| Warning model and unsupported feature handling | manual | Source review and Go tests are required today. | Add request or stream fixtures when warning behavior affects upstream-visible output. |
| Stream part taxonomy | mixed | Provider fixture replay covers stream parts present in committed chunks. | New stream parts require upstream fixture import or recording. |

### ProviderWire V4 Contract Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| Registered LanguageModelV4 request surface | automated | `ai-gateway/test/providerwire-v4/surface.ts` typechecks exhaustive finite request/response witnesses, while `ai-gateway/providerwire/v4/schema/request.json` and focused positive/negative cases define the complete serialized request projection. Production Go golden replay proves the expected runtime outcome reached by every committed request. | The registered public client is authoritative for observable request emission. This evidence makes no compatibility claim for Vercel's private Gateway service. |
| Unary Go runtime | automated | `ai-gateway/providerwire/v4` tests cover envelope validation, bounded UTF-8 body reads, complete-schema-before-mapping order, one case per unsupported family, catalog/model sequencing, fixed safe errors, fixed host-composition documents, minimal unary output, privacy, and response bounds. The committed request goldens replay through the production handler, including a pinned-client provider-tool/MCP golden. Both HTTP modes test choice preservation without tools and schema/unsupported-family rejection before resolution. Unary WP13 tests cover provider-owned calls/results, deferred history, allowlisted metadata, precommit bounds, privacy and native fake-provider acceptance. | Runtime support is narrower than schema acceptance: text generation, scalar controls, function tools and WP13 provider-defined tools/results execute on direct routes; files, approvals, structured output, root provider options other than `anthropic.mcpServers`, body headers, reasoning/custom content, and raw output fail safely. Namespaced function-tool options belong to the supported unary subset. Requests are expected to be serialized by SDK clients, so malformed JSON and structural limits use the bounded standard Go/schema path; duplicate members use the last value and escaped lone surrogates normalize to U+FFFD. Multi-capability unsupported precedence and raw unary response-body details outside content/finish/usage are not contracts. Host HTTP server timeouts remain responsible for slow body availability. |
| Gateway HTTP request projection | automated | Semantic goldens captured through `@ai-sdk/gateway@4.0.52` verify method, route, final normalized protocol headers, unary/streaming mode, presence, header composition, provider definitions/MCP continuation, URL serialization, and byte-to-base64 conversion. Production replay verifies the same records against Go without rewriting them. | The registered public client is authoritative for observable request emission. This evidence makes no compatibility claim for Vercel's private Gateway service. |
| Gateway client response consumption | automated | Focused registered-client probes cover unary overwrite behavior, all seven recognized error classes, clean-EOF SSE, `[DONE]`, raw filtering, and timestamp conversion. The AGPL ProviderWire contract workspace calls the real Go handler through the pinned client for minimal unary success, strict streaming text, ordered stream errors, timeout, established-stream abort, representative unary errors, cancellation and unary/streaming hosted tool continuation; the independent Go client is compared on the same request/output families. | The registered public client is authoritative for observable response consumption, including permissive acceptance and overwritten fields. Private protocol DTOs and test-time schemas own unobserved server shape; raw HTTP, privacy, state, and bounds tests own standard JSON normalization, the minimal unary document, fixed errors, overflow-safe unary preflight and final encoded-byte bounds, plus strict stream bytes and early-stopping stream frame bounds. Standard unary JSON encoding may allocate a bounded constant multiple of the configured limit for escaping. |
| ProviderWire streaming runtime | automated | `ai-gateway/providerwire/v4` tests cover single-owner setup transfer and cleanup, standard cancellation and timeout behavior, request-scoped part counting, warning privacy/cardinality, canonical metadata, text lifecycle, ordered non-terminal provider errors, authoritative finish, standard JSON complete-frame bounds, writer/flush failures, bounded drain, and clean EOF. Golden replay exercises production `DoStream` directly, while the AGPL registered-client integration exercises it through the pinned client. WP13 tests cover execution/dynamic/preliminary markers, current and historical result correlation, repeated previews, finalization, metadata allowlists, writer/cancellation paths and frame/part budgets. | Runtime support includes text, WP12 function tools and WP13 bounded provider tools/results; reasoning, approvals, files, sources, custom content, raw output, and later stream families remain gaps. Compound provider-tool responses including a deferred output family fail safely rather than being partially accepted. Pinned OpenAI image-generation preliminary image events can precede their tool call; that media-preview family remains WP16 (#110), while WP13 requires correlation before accepting preview results. Fixed-prose streaming warning normalization is an intentional privacy deviation from upstream warning-string passthrough. The stream-event schema is test-only; the production encoder, fixed terminal frames, and raw HTTP assertions are server authority. |
| Authenticated Grafana Gateway service composition | automated | `ai-gateway/cmd/grafana-ai-gateway` focused tests cover authenticated discovery, bounded transports, identity separation, exact routing, lifecycle, telemetry, real-listener flushing, and `anthropic` and `openai-compatible` provider construction over the shared bounded model transport. `ai-gateway/test/providerwire-v4/gateway-command.test.ts` builds and spawns the real command and uses `@ai-sdk/gateway@4.0.52` for discovery, canonical/alias unary calls, normal streaming, client abort, and process shutdown against a fake Anthropic backend, plus discovery privacy, streamed usage, sanitized upstream failures, rejected redirects, and client abort against a fake OpenAI-compatible backend. | This is a Grafana host composition over the registered public Gateway client, not a compatibility claim for Vercel's private Gateway service. Raw discovery closure/privacy assertions remain authoritative because the pinned client parser is permissive. Unauthenticated `GET /metrics` is an intentional work-package-5 operational route with no upstream protocol equivalent. OpenAI-compatible streams always request `stream_options.include_usage` so finish parts carry usage, and reasoning parts from compatible backends fail as unsupported stream families. Fake Anthropic, OpenAI-compatible, and JWKS servers establish deterministic service, cancellation, and transport evidence only; they are not provider conformance provenance and no recorded/upstream provider input is changed. WP13 command tests exercise native code-execution/MCP requests, aliases, public name-only metadata, stateless continuation, selected-provider isolation and metadata-only AO/log/metric privacy through both clients. The public registered client does not establish Vercel's private server egress or option-acceptance policy; Grafana accepts only reviewed HTTPS MCP URLs on direct Anthropic routes. Work package 5 bounds each inbound connection/request but defers aggregate connection/request budgets and health-route capacity strategy to work package 6. |

### Grafana Go Gateway Client

WP9 ordered fallback is a Grafana extension; the registered upstream baseline
has no generic equivalent. Root regression tests cover first-part commitment
(including provider errors), pre-commit failures, exact-once decisions,
cancellation, and bounded cleanup. Gateway tests cover strict ordered route
configuration, single logical composition, restart-at-primary semantics,
zero-invocation effect rejection, pure-auto preservation without tools across
unary/streaming failover, private allowlisted records, bounded queue
saturation/shutdown, supported socket deadlines, and logical-to-physical
correlation. Real FIFO deadline tests are Linux-only; macOS verifies nonblocking
sockets and fail-open rejection of blocking sockets without descriptor mutation.
The real command matrix verifies JWKS-authenticated Go and registered Vercel
discovery, unary/streaming primary success, secondary selection, non-retryable
stop, exhaustion, preserved requests, and public/logical privacy. Gateway tests
run with `GOWORK=off` against published root prerequisite `9dd11902673f`.
`fallback_acceptance_test.go` additionally exercises the configured fallback
under the real logical chain and ProviderWire unary/SSE mapping: invalid/empty
stream setup, error-part commitment/order, cancellation with a ready result,
silent/continuously ready blocked-consumer cleanup, aggregate error privacy,
hostile headers/bodies/provider metadata, one logical generation and closed
physical winner records. These are deterministic service tests, not recorded
provider fixtures. Production activation remains WP10 work; the local macOS run
does not verify Linux FIFO runtime behavior.

WP11 direct-route unary tools are covered by strict handler tests, both actual
clients, safe-JWKS command/native continuation, and metadata-only collector
privacy assertions. The authenticated command matrix rejects unary definitions,
non-automatic tool choices, call-only history and complete tool continuation on
fallback routes through both clients with zero primary or secondary requests.
Effectful fallback remains intentionally unsupported. Direct tool round trips
also verify distinct canonical logical generations, nonzero usage, normalized
finish, safe errors and private-safe exported payloads, logs
and metrics. Apache provider regressions preserve selected empty schemas
and content arrays against the registered Anthropic 4.0.38 baseline. These are
deterministic transport tests, not recorded provider fixtures. Gateway consumes
published immutable root `07aacebe97a2` and Anthropic `e9128cc3b35a` prerequisites;
standalone Apache and isolated Gateway tests verify those dependencies. The
registered upstream baseline remains unchanged; production activation remains
separate.

WP12 extends this evidence to input/call/basic-result streaming, required empty
deltas, ID/order validation, hostile termination/cancellation, and automatic
two-step Vercel/Go orchestration through the real handler with one local execution.
Safe-JWKS command/native tests separately cover two transport calls per client.
No tool runner or session state is introduced in the Gateway.

WP13 direct-route provider definitions, provider-owned calls/results, preliminary
replacement and deferred result-only continuation are covered by pinned-client
semantic goldens, both real clients, strict handler lifecycle/bounds/privacy tests,
authenticated native Anthropic code-execution/MCP fake transport and existing
provenance-valid Anthropic/OpenAI provider fixtures. An MCP name is returned only
for a server configured by the authenticated caller in that request; URLs/tokens
remain in the caller-owned request and selected native provider request, not
normalized response or operational telemetry. Other root provider options and
compound source/file/approval content remain unsupported. Effectful fallback
remains disabled even when the request has MCP configuration but no tool list.
These deterministic tests do not prove live MCP egress or a deployed image rollout.


`providers/grafana` is an Apache-licensed, independently buildable client for
the Gateway service, not a second Gateway implementation. Its focused Go
tests cover explicit request projection, atomic discovery, authentication,
closed public errors, bounded unary/SSE parsing, and cancellation ownership.
The existing exact-pinned ProviderWire workspace runs a test-only Go capture
process against the same HTTP cases as `@ai-sdk/gateway@4.0.52`; real-command
tests additionally exercise discovery, canonical/alias generation, streaming,
static/cloud authentication, acting-user propagation, and abort.

The following differences are explicit rather than claims of complete parity:

- Parity-preserving Go adaptations: byte slices become base64; URLs become JSON
  strings; timestamps become `time.Time`; local request bodies are serialized
  JSON rather than JavaScript objects; empty warning slices can disappear only
  when the capture process reserializes Go structs with `omitempty`.
- Parity-preserving Go normalizations: `IncludeRawChunks`, `ProviderExecuted`,
  `Preliminary` and unary `Dynamic` normalize absent and false where equivalent;
  nil direct-Go provider-tool args become `args: {}` on the wire. Streaming
  input-start `Dynamic *bool` instead retains absent/false/true because absence
  triggers tool-definition inference. Required selected empty values survive.
- Representation gaps: zero-value reasoning is omitted. Some optional Go
  descriptive strings and reasons cannot distinguish absence from explicit
  empty even when the TypeScript request type can; those distinctions are not
  claimed by the Go client.
- Intentional security boundaries: client-owned authentication/protocol headers
  cannot be overridden through case variants; URL prefixes are retained;
  discovery is atomic and bounded; response families are closed to text, the
  supported function-tool subset and reviewed provider-tool calls/results.
  Upstream's permissive output schema and wildcard supported URLs are not
  adopted. Raw unary response text is retained only within its configured bound.
  Token-exchange errors discard arbitrary token-service response prose, including
  through their error cause; caller cancellation retains its context identity.
  Output-only approval requests and Go-only fields outside the pinned input
  unions fail locally. Provider options preserve opaque object contents but
  reject nonobject provider entries; reasoning files retain only data/URL arms.
- Intentional error adaptation: categories/status/retryability match the closed
  registered matrix, but Go retains public envelope prose without upstream's
  authentication guidance or `generationId` message suffix. Generation IDs and
  additional envelope members are not promoted into the public error API.
- Public SSE warning/error prose remains bounded passthrough; the server owns
  its fixed-prose privacy policy. Unknown metadata is ignored. SSE framing
  supports LF, CRLF, bare CR, BOM, and ignored event/id fields; complete-event
  limits count CRLF-normalized bytes while the total limit counts wire bytes.
- Cancellation adaptation: Go preserves `context.Canceled` directly; the pinned
  client can wrap the underlying `AbortError` in its internal error. Both avoid
  I/O when pre-aborted, cancel an in-flight unary request, and close an established
  stream without fabricating a provider error. Aggregate Go tests exercise 128
  blocked readers and 128 blocked consumers and verify owner/body cleanup against
  both stack-specific and process goroutine baselines, including a live-owner
  negative control.
- Request evidence: the comprehensive pinned golden and selected request cases
  cover scalar/collection presence, selected bytes/URL/reference/text, opaque JSON,
  and all seven protected headers in three casings across unary and streaming.
  Isolated source-mutation controls rerun the same differential assertions and
  require semantic failures for body headers, scalar presence, reasoning omission,
  native byte conversion, URLs, opaque options, call-header precedence, and the
  stream flag. These reproducible red controls supplement the implementation's
  original red-first history; they do not claim newly invented historical TDD.
- Drift detection: the compile-time `CallOptions` struct conversion witness
  catches top-level fields. Go string constants are not compiler-sealed enums;
  an executable AST inventory instead locks all 22 reachable request declarations
  and 47 finite constants, with explicit mapped/omitted/rejected classifications.
  Baseline validation rejects nested field/discriminator changes and changes to
  the upstream commit or gateway/provider/provider-utils pins until the reviewed
  client evidence is updated. Mutation tests prove these failures independently.
- Coverage limits: generic HTTP differential tests cover all 11 registered error
  rows. The real command covers 10: 400, 401, 404, 424, 429, 499, 500, 502, 503,
  and 504. Its production composition has no permission policy producing 403;
  that row remains generic-runtime/client evidence, not fabricated command
  coverage. Closed command results, discovery, request/response metadata, stream
  parts, public errors, logs, and metrics are scanned for credentials and private
  backend/topology markers. Arbitrary application text and bounded raw responses
  remain caller-visible by contract; the client does not promise to redact every
  possible secret returned by an untrusted endpoint. Hostile limits and cancellation
  schedules are focused Go evidence, not provider-provenance fixtures; exhaustive
  union permutations and every possible scheduling interleaving are not claimed.

No provider recordings or provenance fixtures were fabricated or regenerated.

### Trusted Cloud Gateway Host Coverage

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| Trusted reverse-proxy application composition | automated | Gateway Go tests cover distinct trusted identities, malformed assertions, authentication before body reads, internal JWT isolation, mode-specific dependencies, separate listeners, shared shutdown deadlines, flushing, and fixed-value authentication telemetry. `ai-gateway/test/providerwire-v4/gateway-command.test.ts` exercises the real command through a local edge shim with `@ai-sdk/gateway@4.0.52`. It separates fixed edge denials, overwritten client assertions, and malformed post-edge assertions, and checks credential privacy. Actual Go `StreamText` and `ai@7.0.65` `streamText` tests capture the automatic choice without rewriting requests and verify backend execution and expected text. | Grafana host extension with no upstream service equivalent. The shim uses dummy credentials and fixed authorization outcomes; it does not prove deployed proxy authentication, access-policy enforcement, or proxy-only ingress. Deployment owners must verify ingress before activation. TypeScript `generateText` body headers and default unary token limits remain compatibility gaps. Paired Gateway fixture replay is not yet covered. |

### Provider Implementation Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| Anthropic request conversion | automated | `test/conformance/anthropic/**/expected-requests.jsonl` compares behavior-affecting requests; focused provider tests cover prompt-cache-preserving replay of code-execution subtool inputs, result field order, and serialized `tool_references` for successful searches with zero and one match. | Add fixtures for new Anthropic options, content parts, and beta headers. Intentional compatibility deviation from `@ai-sdk/anthropic@4.0.38`: with an empty input tool list, Go omits `auto` and `none` choices but preserves explicit `required` and named-tool choices; upstream omits every tool choice. No provenance-valid provider fixture exercises a zero-match tool search, so the empty `tool_references` array on a successful `tool_search_tool_result` is unit-test covered; every recorded tool-search input has at least one reference. |
| Direct Anthropic client construction authority | automated | `providers/anthropic/model_environment_test.go` poisons SDK base-URL, API-key/auth-token, explicit/fallback profile, federation/identity, organization, and custom-header environment sources across unary and streaming calls. The implementation passes `WithoutEnvironmentDefaults` and the explicit API key directly to SDK construction. | Intentional security/Go deviation from `@ai-sdk/anthropic@4.0.38`: direct Go construction ignores every ambient SDK default, including `ANTHROPIC_BASE_URL`; upstream reads that base URL when no explicit `baseURL` is supplied. This keeps the required explicit API key and reviewed request options authoritative; the Go SDK's profile, federation, identity-file, organization, and custom-header environment defaults have no registered TypeScript equivalent. Existing request snapshots cannot express process-environment discovery, so focused tests are the regression authority and provider inputs remain unchanged. Anthropic base-URL path/trailing-slash normalization still lacks provenance-valid provider fixture coverage. |
| Anthropic response stream parsing | automated | Anthropic fixture replay compares Go stream output with upstream expectations, including duplicate/spliced message lifecycle handling and advisor stop reasons; provider tests cover fallback iteration attribution, stop details, recommended-model metadata, streamed finish metadata, initial-error preflight, and post-preflight `stream-start` warning placement. | Newly observed event types need fixtures before release. Moving conversion warnings from finish to the initial provider start corrects an implementation bug against `@ai-sdk/anthropic@4.0.38`. |
| Anthropic provider-defined tools | mixed | Web search, code execution and MCP pinned provider fixtures cover current paths; focused native request tests cover MCP token/enabled absence versus explicit empty/false, empty allowed-tools arrays and code-execution/MCP combination. The authenticated Gateway command tests native conversion and continuation with a deterministic fake Anthropic endpoint, including unary use of the model's unmodified large default output budget. | No provenance-valid provider fixture covers optional MCP token/enabled permutations or the full Gateway native continuation; synthetic tests are not recorded provider inputs. Provider tool configuration fields beyond these cases remain fixture-driven. |
| Anthropic unary context deadlines | automated | `providers/anthropic/unary_timeout_test.go` verifies that a future context deadline bypasses `anthropic-sdk-go@v1.61.0`'s non-streaming token-estimate refusal without changing max tokens; explicit SDK request timeouts retain precedence and past deadlines avoid native I/O. Authenticated Gateway command tests exercise the bounded default unary MCP path through the published Anthropic module. | Intentional Go safety boundary versus registered `@ai-sdk/anthropic@4.0.38`: direct calls without a context deadline or explicit SDK timeout still use the Go SDK guard and can reject large default output budgets before HTTP. The Gateway always supplies its model-duration deadline; this adaptation does not impose a new output-token cap. |
| OpenAI Responses request and stream conversion | automated | OpenAI request/output snapshots cover text, reasoning, structured output, hosted tools, native provider-tool continuation items, multi-step calls, stored references, client-executed `openai.computer` actions, three-step `openai.programmatic_tool_calling` with caller/result replay, and output-index correlation when compatible Responses APIs rotate item IDs. Provider unit tests cover legacy computer calls, stored-item replay, screenshot URL/file-ID output, missing output, action variants, future-family model defaults, web-search allowed/blocked/explicit-empty domain filters, tool-search output item IDs, `response.in_progress` pre-output error handling with fallback eligibility, and requested Responses logprobs metadata presence, null/missing handling, empty-array retention, ordering, and normalized shape. Provider-adapter tests verify that custom identity remains separate from the stable OpenAI/Azure metadata namespace across continuation calls, while public-constructor tests preserve legacy OpenAI-first/Azure-fallback options for generate and stream. | Intentional Go deviation from `@ai-sdk/openai@4.0.41`: apply-patch tool results resolve configured provider-tool aliases before taxonomy dispatch; the baseline checks the literal `apply_patch` name. This preserves native call/output pairing for aliased provider tools and is covered by Go unit tests, while conformance snapshots use the canonical name. HTTP-level API errors remain unit-tested when replay cannot represent them. No provenance-valid provider fixture currently exercises requested logprobs metadata, so that provider boundary remains unit-test covered. |
| OpenAI-compatible request and stream conversion | mixed | Existing provider fixtures cover text, structured output, and tool-call deltas; focused provider tests cover recoverable malformed chunks and non-zero, non-contiguous, reused, or omitted tool-call indexes. | The upstream irregular-index evidence is SSE-framed rather than a chunks fixture, so this edge remains provider-unit-tested. Malformed new tool-call deltas are validated with Go error timing while valid stream wire output matches upstream. |
| Bedrock request conversion | automated | `test/conformance/bedrock/**/expected-requests.jsonl` compares behavior-affecting requests; provider tests cover inline and S3 video input/tool results, unsupported video types, strict-tool warnings, and structured-output routing for newer Claude families. | Header assertions intentionally exclude volatile SigV4 fields. Bedrock video request parity is unit-test covered because no provenance-valid provider fixture currently exercises video input. Intentional Go adaptation: unsupported tool-result file URLs emit warnings and are omitted rather than failing request construction, following the Go provider warning convention. |
| Bedrock response conversion | automated | Bedrock streaming fixtures cover text, tools, JSON, reasoning, guardrails, and S3 image inputs. The pinned non-streaming JSON-tool fixture is replayed through `DoGenerate` and compared against `expected-generate.json`. | Add fixtures as Bedrock support expands. |
| Bedrock Mantle provider | mixed | `providers/bedrock/mantle` exposes Responses by composing the official `openai-go/v3/bedrock` client with the shared OpenAI Responses adapter. Focused tests cover native model IDs, generic `/v1/responses`, every currently documented `/openai/v1/responses` exception, custom routing, bearer rollback, retry-safe SigV4 scope/body hashing/session tokens, generate and stream attribution, and OpenAI-namespaced continuation metadata. | Intentional post-baseline adaptation: registered `@ai-sdk/amazon-bedrock@5.0.55` routes Mantle Responses through `/v1`, while current AWS model cards require `/openai/v1` for GPT-5.4, GPT-5.5, GPT-5.6 variants, Grok 4.3/4.6, and Gemma 4. No provenance-valid live Mantle fixture is available. Upstream's callable/default Chat surface and Chat-only safeguard models remain a gap. |
| Provider error mapping | mixed | Go tests and upstream/source comparison cover HTTP and stream errors; focused unit tests assert lossless provider error data. OpenAI Responses tests verify transport errors and SSE errors before output, including the `response.in_progress` grace window, structured `response.failed` errors, and fallback eligibility. Synthetic provider error streams are intentionally excluded from provider conformance fixtures because they cannot establish provider provenance. | Intentional Go deviation: Anthropic `api_error` and `overloaded_error` SSE failures are retry-eligible by provider cause, including after output, while the registered upstream only marks an initial `overloaded_error` retryable. Promoted Anthropic errors retain the full envelope in `Data` for gateway normalization while preserving the inner provider error in `ResponseBody`. Core only retries `DoStream` call failures; it never replays an established stream from a retryable `PartError`, because callers must separately prove that no output or effects escaped. OpenAI's current top-level error events retain subsequent `response.failed` usage and metadata; legacy nested-error envelopes terminate `openai-go`'s decoder, so Go emits the structured error plus a terminal error finish but cannot consume a later authoritative `response.failed` frame. HTTP-200 Anthropic SSE `rate_limit_error` remains non-retryable pending evidence of its service timing and delay metadata. |
| Unsupported option warnings | manual | Go tests and source review are required today. | Track accepted gaps in this file or `upstream.yaml`. |

### Frontend Interop Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| `@ai-sdk/react` `useChat` wire compatibility | mixed | Hook-level integration tests cover successful text, ordered status changes, HTTP and UI-stream errors, stop with retained partial text, a multi-step tool boundary, and approved and denied tool approval resumption. Separate cross-language tests compare pinned `ai@7.0.65` text-stream and UI chunk dynamic inference for known dynamic, known ordinary and unknown tools, including explicit false. | Coverage remains selective. Metadata-only deltas and invalid provider-tool errors are lower-level SSE/reader evidence, not hook state-machine evidence; regenerate, reconnect, and other callback paths remain uncovered at hook level. |
| Agent UI stream helpers | mixed | Root unit tests cover pre-stream validation, UI-to-model conversion before streaming, original-message preservation, chunk equality with `StreamTextResult.ToUIMessageStream`, and HTTP SSE framing through the existing writer. | Helpers intentionally reuse the existing UI chunk/SSE path, so no new fixture is required unless emitted chunks change. Validation is a minimum Go-model validator; remaining upstream `validateUIMessages` differences, deeper schema/provider-specific validation, and unsupported upstream UI part features are gaps. |
| `useCompletion` compatibility | mixed | Hook-level integration tests cover successful text consumption, HTTP error callback and loading reset, and stop with retained partial completion. | Other stream protocols, concurrent requests, and callback paths remain uncovered at hook level. |
| `useObject` compatibility | mixed | Hook-level integration tests cover successful streamed JSON and final schema mismatch through the exact `onFinish` result; conformance fixtures separately validate object output where `expected-object.json` exists. | HTTP/stream errors, stop, and other lifecycle callbacks remain uncovered at hook level. |
| Chunk ordering and state transitions | mixed | `expected.jsonl` automates stream-part ordering for covered fixtures, except between adjacent locally-executed tool outputs in one step; hook-level integration tests separately cover selected chat status, step-boundary, tool approval, completion loading, and cancellation transitions. | Lower-level chunk ordering does not establish exhaustive React hook state-machine coverage; add fixtures and hook assertions at their respective layers as lifecycle states expand. |

### Conformance Harness Layer

| Capability | Status | Confidence Source | Gap / Notes |
| --- | --- | --- | --- |
| Baseline package validation | automated | `mise run validate-parity-baseline` checks `upstream.yaml` against all retained parity TypeScript consumers: conformance tools, integration tests, CLI tooling, and the ProviderWire V4 contract workspace. | The canonical baseline is the npm package versions recorded in `upstream.yaml`. |
| Upstream expectation generation | automated | `mise run generate-conformance` produces streaming `expected.jsonl`, unary `expected-generate.json`, request snapshots, structured output expectations, and opt-in per-step usage snapshots. | Generation only covers fields supported by `config.yaml`. |
| Frozen-target baseline upgrade | automated | `parity-select` writes a coherent mature target record without changing the baseline; `parity-upgrade` requires and applies that explicit record before regenerating expectations. | Selection and application do not certify semantics; assessment and reviewed verification remain required. |
| Fixture config expressiveness | automated | Streaming provider fixtures use YAML for models, prompts, tools, provider tools, approvals, provider options, JSON response format, `toolChoice`, `activeTools`, `streamOptions`, and tool provider options. Unary Bedrock fixtures currently support prompts/configured messages, system text, headers, provider options, and response format; other providers and unsupported unary fields fail during config loading/generation. Provider-independent core UI fixtures replay `LanguageModelV4` stream parts directly. | Expand unary or streaming fields only when an authentic fixture needs a new upstream-visible option. |
| CI enforcement | automated | Baseline validation, full conformance replay, and integration are all required status checks on `main`, so any parity divergence blocks the merge. | Regenerated expectations must land in the same pull request as the behavior change, because a stale expectation now blocks merges rather than emitting a warning. |
| Upstream fixture import tracking | automated | Provider `upstream/INDEX.yaml` files track imported and missing upstream fixtures; `mise run parity-coverage` rejects unmapped local cases, nonexistent source names, and imported inputs that are not byte-identical to the pinned checkout. | `null` entries are intentional non-streaming or unavailable coverage gaps until imported through a matching operation. |

## Review Rules

Parity-sensitive changes must classify every observed difference from upstream
as one of:

- parity-preserving Go adaptation
- intentional deviation
- implementation bug
- coverage gap

Durable intentional deviations and accepted coverage or support boundaries belong
in `upstream.yaml` or this coverage map. Actionable implementation or proof work
belongs in an `upstream-sync` issue. A pinned-version upgrade lists created and
reused issues in its PR rather than duplicating their contents here.
