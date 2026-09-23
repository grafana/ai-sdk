# Upstream Parity Coverage

This is the current coverage index, not a claim of total parity or an upgrade log.
[upstream.yaml](upstream.yaml) owns the package baseline and registered gaps;
[UPGRADING.md](UPGRADING.md) owns workflow and evidence rules. Linked issues own
detailed findings, design questions and acceptance criteria. The retained
[upgrade verification](../../openspec/changes/archive/2026-09-22-upgrade-pinned-reference-september-2026/verification.md)
records the assessment and validation behind this reference.

## Status Values

- `automated`: checks exercise the stated scope, not every possible input.
- `mixed`: partial automation with implementation or evidence gaps.
- `manual`: source/test comparison is still the primary evidence.
- `gap`: the capability or required proof is absent.

Snapshots prove represented behavior. Unit tests cover synthetic failures and
local invariants; frontend tests establish separate hook/wire behavior. Existing
evidence does not become exhaustive proof when the baseline changes.

## Scope

Supported surfaces are language-model orchestration, Output, UI/SSE, selected
provider adapters and Grafana Gateway. LanguageModel file/reference parts and
hosted image outputs remain in scope. Excluded product families are embeddings,
reranking/evaluation, image/video model APIs, speech/transcription/translation,
realtime/live, durable batch, upload/managed-file/skill convenience APIs, RSC/other
frameworks and sandbox-provider implementations. Legacy generateObject/streamObject
repairText APIs are excluded in favor of Output.

OpenAI Chat/legacy Completions, Bedrock Anthropic Invoke and Mantle default Chat
are absent adapters, not aliases for Responses/Converse. Mantle Chat overlaps an
unscoped [#48](https://github.com/grafana/ai-sdk/issues/48); acceptance is unresolved.
No separate google-vertex package baseline or live Vertex attestation is claimed.
Private Vercel Gateway routing, billing and deployment are not an available oracle.
Constructor URL policy remains [#228](https://github.com/grafana/ai-sdk/issues/228).
Issue registration does not approve a new API or permanent deviation.

## Layered Coverage Map

### Core ai-sdk Layer

| Capability | Status | Evidence and remaining scope |
| --- | --- | --- |
| Stream lifecycle and part identity | mixed | UI snapshots, root tests and frontend assembly cover steps, ordered content, overrides, usage, timeouts and invocation-wide text/reasoning IDs. PrepareStep has unit rather than fixture coverage. Optional stream retry is missing ([#209](https://github.com/grafana/ai-sdk/issues/209)). |
| Tool execution and continuation | mixed | Fixtures and root tests cover calls, approvals/signatures, choices, active tools and multi-step results. The finish/approval matrix and real HTTP/core wrapper tests cover [#208](https://github.com/grafana/ai-sdk/issues/208): only stop/tool-calls dispatch, and continuation requires matching results/denials. Explicit ordering and repair/refinement remain [#210](https://github.com/grafana/ai-sdk/issues/210); caller-aware routing and preliminary execution APIs remain [#211](https://github.com/grafana/ai-sdk/issues/211). |
| Tool result ordering | mixed | Completion-order emission is tested; mixed rejected/denied/executed result-list ordering remains [#166](https://github.com/grafana/ai-sdk/issues/166). Concurrent sibling tie ordering is normalized as described below. |
| Agent | mixed | Root tests cover defaults, identity, settings/context/callback merging, tools, approvals and stream errors. Preparation/templates/model-call hooks and validation remain [#212](https://github.com/grafana/ai-sdk/issues/212). GenerateText and Agent.Generate collect StreamText, so unary adapter tests do not prove those paths. |
| Output | mixed | Object snapshots and unit tests cover schemas, partial values, array elements and parse failures. GenerateText/Agent.Generate terminal parsing on non-stop text remains [#226](https://github.com/grafana/ai-sdk/issues/226). OutputError/sentinels and channel APIs are Go adaptations. |
| UI chunks and SSE | automated | `expected.jsonl`, framing tests and schema-parsed frontend scenarios cover represented chunk fields/order, files, metadata, multi-step IDs and failed/unfinished tool states. New chunk families need their own proof. |
| UI readers/conversion and Agent helpers | mixed | Root tests cover assembly, cloning, IDs, partial input, metadata, callbacks and minimum pre-stream validation. Persisted tool fields, data conversion and validation gaps remain [#213](https://github.com/grafana/ai-sdk/issues/213). Reader message/onError/terminateOnError options are not exposed; default error-chunk handling continues the stream. |
| Composed UI streams | mixed | Root tests cover writer framing, continuation identity, finish/abort callbacks and failure draining. Blocking Merge lacks asynchronous registration ([#181](https://github.com/grafana/ai-sdk/issues/181)); newer outcome/step hooks have unresolved scope overlap with that issue. |
| Cancellation | mixed | `ui/stream-abort` fixtures and root/provider tests cover selected partial-output, tool and transport cancellation paths. Abandoned FullStream cancellation remains [#165](https://github.com/grafana/ai-sdk/issues/165); exhaustive interleavings are not claimed. |
| URL downloads | gap | Native provider URL handling is not core validated downloading; unsupported-URL fetching, DNS pinning and redirect validation remain [#225](https://github.com/grafana/ai-sdk/issues/225). |
| Middleware/registry | mixed | Wrapper/default/extraction and provider resolution tests exist. Simulated streams lose content/metadata ([#215](https://github.com/grafana/ai-sdk/issues/215)). Grafana observability/enrichment/logging are extensions, not TS telemetry parity. |

Core adaptations and evidence limits:

- Go batches local approval handling after provider streaming rather than emitting
  requests adjacent to calls. Recorded provider content retains its order.
- Adjacent locally executed tool outputs are compared without sibling tie order;
  provider-executed outputs and rejected-input errors remain exactly ordered.
  The comparator boundary is registered in `upstream.yaml`.
- ToUIMessageStream supplies a default ID generator with original messages;
  upstream requires an explicit generator except when reusing the last assistant
  ID. Go also avoids an unused generator call on continuation. CreateUIMessageStream
  retains its separately tested identity/framing contract.
- GenerateText/Agent streaming-only timeout warnings are returned on successful
  results, not logged globally before invocation; failed calls cannot expose them.

### Provider Contract Layer

| Capability | Status | Evidence and remaining scope |
| --- | --- | --- |
| LanguageModelV4 shape | mixed | Installed-provider discriminator checks and finite ProviderWire witnesses are automated; exact field/semantic equivalence remains manual. Shape enforcement work is [#35](https://github.com/grafana/ai-sdk/issues/35). |
| Requests, messages and options | mixed | Provider request snapshots cover represented prompts/options/tool definitions, including multipart UI tool outputs. Config breadth and validation remain [#33](https://github.com/grafana/ai-sdk/issues/33), [#34](https://github.com/grafana/ai-sdk/issues/34); UI conversion differences belong to [#213](https://github.com/grafana/ai-sdk/issues/213). |
| Stream parts, usage and warnings | mixed | Provider/UI snapshots, optional `expected-usage.json` and focused warning/error tests exercise represented variants. New fields and families require source comparison plus boundary proof. |

Context/channels, opaque JSON, flat unions and time.Time are Go adaptations.
Zero-valued provider-default reasoning conflates omission with explicit default
in provider-domain JSON; the strict wire layer tests its normalization. Optional
string/boolean presence losses remain real gaps, not proof of full V4 equivalence.

### Provider Implementation Layer

| Capability | Status | Evidence and remaining scope |
| --- | --- | --- |
| Anthropic requests/tools/models | mixed | Request snapshots and tests cover cache-sensitive replay, multimodal/structured input, tool references and native continuations. System/thinking/skill controls remain [#216](https://github.com/grafana/ai-sdk/issues/216); dated Vertex IDs, newer web tools and validation remain [#217](https://github.com/grafana/ai-sdk/issues/217). Constructor URL policy is [#228](https://github.com/grafana/ai-sdk/issues/228). |
| Anthropic responses | mixed | Replay and unit tests cover citations, signatures, usage/stop/fallback metadata and caller-preserving server call/search/fetch round trips, including full error payloads. Zero-match tool-search results have unit but no authentic provider-fixture proof. |
| SDK-backed transport metadata | gap | Anthropic call headers and both Anthropic/OpenAI returned request/stream-response metadata and requested raw frames remain [#229](https://github.com/grafana/ai-sdk/issues/229). Server-captured request snapshots do not prove returned metadata. |
| OpenAI requests/results | mixed | Snapshots and unit tests cover easy-input assistant history, phase/stored references, hosted tools, native continuation, custom provider namespaces and apply-patch finish mapping. Forced choices remain [#32](https://github.com/grafana/ai-sdk/issues/32), allowedTools [#218](https://github.com/grafana/ai-sdk/issues/218), strict schema normalization [#219](https://github.com/grafana/ai-sdk/issues/219), async/configuration controls [#220](https://github.com/grafana/ai-sdk/issues/220), multipart/cache/reference conversion [#221](https://github.com/grafana/ai-sdk/issues/221). |
| OpenAI streaming/parallel wrappers | mixed | Imported replay plus focused tests cover atomic expansion/fallback, scalar result grouping, incomplete/conflicting groups, cache breakpoints, truncated input, malformed frames and single SDK decoder ownership. Real HTTP/core tests prove local execution and second requests, including suppression after errors. Logprobs and hostile transport cases have unit rather than provider-recording proof; multipart grouping inherits [#221](https://github.com/grafana/ai-sdk/issues/221). |
| OpenAI-compatible Chat | mixed | Fixtures/tests cover text, structured output, irregular tool indexes, malformed chunks and placeholder metadata/zero times. Builder-field protection is implemented by [#194](https://github.com/grafana/ai-sdk/pull/194), resolving [#193](https://github.com/grafana/ai-sdk/issues/193). Custom response metadata extractors are not exposed. Irregular-index proof is unit-based. |
| Bedrock Converse | mixed | Snapshots/tests cover text/tools, usage, guardrails, video and selected structured/reasoning paths; one pinned unary JSON-tool fixture is replayed. Family/profile/document/tool routing remains [#222](https://github.com/grafana/ai-sdk/issues/222), selective guardContent [#223](https://github.com/grafana/ai-sdk/issues/223). Video has unit-only proof; volatile SigV4 headers are excluded from snapshots. |
| Mantle Responses | mixed | Routing, custom clients, bearer/SigV4, identity and continuation have focused tests, not live conformance recordings. Owner-approved producer-first adoption remains [#207](https://github.com/grafana/ai-sdk/issues/207); web-search includes are [#227](https://github.com/grafana/ai-sdk/issues/227). Workspace success does not prove the pinned consumer adopted the OpenAI correction. |

Provider deviations and limits:

- Anthropic construction deliberately disables ambient SDK defaults in favor of
  explicit credentials/options; environment-poisoning tests cover this boundary.
  With no tools, Go preserves required/named choices whereas upstream omits all
  choices. Base-URL normalization remains an API/evidence gap.
- Anthropic api_error/overloaded_error events remain retry-eligible by cause,
  including after output; upstream only retries an initial overloaded_error.
  Go retains the envelope in Data and inner error in ResponseBody. Core does not
  replay established streams from retryable PartError events. HTTP-200 SSE
  rate_limit_error remains nonretryable pending timing/delay evidence.
- OpenAI resolves configured apply-patch aliases before output dispatch, unlike
  upstream's literal-name check. The adapter retains termination on legacy nested
  errors; top-level errors can retain later failed-response usage. Malformed JSON
  remains recoverable and cannot be turned into success by later completion.
- Plain OpenAI EOF without a terminal/error does not synthesize a provider finish,
  preserving Go's incomplete-stream distinction. Upstream always flushes a finish;
  broader alignment remains a separate decision, not claimed by wrapper recovery.
- Compatible options cannot replace builder-owned fields. Go also protects unary
  stream/stream_options and warns for discarded fields; upstream allows those
  unary transport overrides and silently overwrites its later builder fields.
- Bedrock warns and omits unsupported tool-result file URLs rather than failing.
  Mantle's AWS-specific `/openai/v1` routing remains an adaptation to model cards;
  the registered upstream factory uses `/v1`.

### ProviderWire V4 Contract Layer

The public registered client is the request/consumption oracle, not Vercel's
private service. Runtime and schema acceptance are distinct.

| Capability | Status | Evidence and remaining scope |
| --- | --- | --- |
| Request projection and drift | automated | `ai-gateway/test/providerwire-v4` checks finite type witnesses, schema, HTTP goldens and field/constant inventory. Production Go handlers replay committed requests. Mutation controls challenge field mapping and baseline/witness drift. |
| Unary/streaming runtime | mixed | Handler/golden/client tests cover text and the bounded client-executed function-tool subset, usage, ordered errors, framing, cancellation, privacy and byte/part limits. Remaining request/response families: [#107](https://github.com/grafana/ai-sdk/issues/107), [#108](https://github.com/grafana/ai-sdk/issues/108), [#109](https://github.com/grafana/ai-sdk/issues/109), [#110](https://github.com/grafana/ai-sdk/issues/110), [#111](https://github.com/grafana/ai-sdk/issues/111), [#112](https://github.com/grafana/ai-sdk/issues/112), [#113](https://github.com/grafana/ai-sdk/issues/113), [#114](https://github.com/grafana/ai-sdk/issues/114). Options/headers and raw output remain [#115](https://github.com/grafana/ai-sdk/issues/115), [#116](https://github.com/grafana/ai-sdk/issues/116). |
| Provider configuration | mixed | Command tests exercise Anthropic and compatible construction. Vertex, Bedrock and OpenAI configuration remain [#117](https://github.com/grafana/ai-sdk/issues/117), [#118](https://github.com/grafana/ai-sdk/issues/118), [#119](https://github.com/grafana/ai-sdk/issues/119). |

Strict Go schema/runtime checks, raw HTTP assertions and the server encoder own
unobserved response shape, resource bounds and privacy. SDK parsing alone does
not prove those properties. Standard JSON uses last duplicate member and replaces
escaped lone surrogates; bounded encoding can allocate a constant multiple of the
limit. Multi-capability rejection precedence is not a contract. Server warnings
use fixed privacy-safe prose rather than upstream warning passthrough.

### Grafana Go Gateway Client

`providers/grafana` is an independently buildable Apache client, not another
Gateway implementation. Unit/differential/mutation tests cover explicit projection,
bounded atomic discovery, protected headers, validated unary warnings, closed
errors and stream/body ownership. Real-command tests exercise both registered
clients. Post-header transport retryability remains
[#224](https://github.com/grafana/ai-sdk/issues/224).

Retained boundaries (full contract: [client spec](../../openspec/specs/grafana-gateway-client/spec.md)):

- Only text and supported client-executed function-tool response families are
  accepted; permissive upstream outputs and wildcard URLs are not adopted.
  Provider options must contain object entries; reasoning files support data/URL
  arms. Unsupported approval/Go-only request fields fail locally.
- Authentication/protocol headers cannot be replaced via casing tricks, URL
  prefixes are retained, and token-service error prose is discarded. Errors keep
  public envelope prose but omit upstream auth guidance/generation-ID suffixes.
- Bounded public SSE warning/error prose and raw unary response text remain
  caller-visible. The client does not redact arbitrary application text; the
  server owns its separate privacy policy.
- Go preserves context cancellation identity; upstream may wrap AbortError.
  Tests cover pre-abort, active requests, blocked readers/consumers and cleanup,
  including a live-owner negative control. Optional empty/false presence remains
  narrower than JavaScript; required selected empty values are retained.
- Framing covers LF/CRLF/bare CR/BOM and ignored event/id fields. Complete-event
  limits count normalized bytes; total limits count wire bytes. HTTP timeouts
  remain responsible for slow body availability.

### Trusted Cloud Gateway Host Coverage

Command tests build the real service with fake providers/JWKS and exercise both
clients through static and trusted-proxy modes: discovery, identity separation,
unary/streaming calls, tool continuation, errors, abort, flushing and shutdown.
These are synthetic service tests, not provider recordings or proof of deployed
proxy authentication/access-policy/ingress. High-level TS generateText body headers
and default unary token limits remain compatibility gaps; paired Gateway fixture
replay is not covered. Operational metrics have no upstream protocol equivalent;
aggregate capacity and production activation remain separate.

Ordered fallback is a Grafana extension. Tests cover pre-commit retry, first-part
commitment (including errors), cleanup, logical/physical attribution and private
backend isolation. Effectful/tool fallback remains unsupported and is tested for
zero backend invocation. Direct routes support client-executed tools, not an
in-service runner or session store. Linux FIFO deadline proof is platform-specific;
macOS checks sockets and rejects unsafe blocking descriptors without mutating them.

Generic runtime/client evidence covers all 11 registered HTTP error rows. The real
command covers 10; its composition has no policy producing 403. Credentials and
backend markers are checked in protocol output, errors, logs and metrics, but not
promised absent from arbitrary caller-visible text. Standalone module checks use
published dependencies; workspace replacements do not establish consumer adoption.

### Frontend Interop Layer

| Capability | Status | Evidence and remaining scope |
| --- | --- | --- |
| useChat | mixed | Actual hooks cover text, selected status/error/stop flows, tool steps and approval resumption. Regeneration, reconnect and broader callbacks/overlap remain [#214](https://github.com/grafana/ai-sdk/issues/214). |
| useCompletion/useObject | mixed | Hooks cover text/error/stop and streamed JSON/final schema mismatch respectively. Other protocols, failures and lifecycle paths remain [#214](https://github.com/grafana/ai-sdk/issues/214). |
| Chunk ordering and assembly | mixed | Snapshot and schema/reader tests cover represented metadata, IDs and failed/unfinished tools. These lower-level assertions do not establish exhaustive hook state-machine behavior. |

### Conformance Harness Layer

| Capability | Status | Evidence and remaining scope |
| --- | --- | --- |
| Pins and generation | automated | Baseline validation covers all four TS consumers. Frozen selection/application and expectation generation retain exact source/provenance checks; pins are a reference, not full parity. |
| Fixture configuration | mixed | Streaming config covers prompts/tools/approvals/options/output; unary Bedrock supports a narrower field set. Queries/config breadth/validation remain [#30](https://github.com/grafana/ai-sdk/issues/30), [#33](https://github.com/grafana/ai-sdk/issues/33), [#34](https://github.com/grafana/ai-sdk/issues/34). Provider-independent UI fixtures cover core streams. |
| Source inventory | automated | INDEX files enforce source existence, byte-identical imports and streaming inventory. Null entries retain explicit unimported unary/other-operation gaps; generation does not establish provenance. Per-provider source inventory limitations remain [#33](https://github.com/grafana/ai-sdk/issues/33). |
| Required checks | automated | Baseline, replay and integration checks enforce represented behavior. Expectations must accompany behavior changes; warnings/skips and unrepresented behavior still need review. |

Record observed differences as Go adaptations, intentional deviations,
implementation bugs or coverage gaps. Keep decisions here or in `upstream.yaml`,
with detailed actionable work in issues rather than another duplicate backlog.
