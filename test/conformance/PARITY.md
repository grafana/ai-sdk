# Upstream Parity Coverage

This document maps validation surfaces to the evidence they provide. It is not a
feature backlog, release assessment or execution log.

- [upstream.yaml](upstream.yaml) owns the registered reference.
- [GitHub parity issues](https://github.com/grafana/ai-sdk/issues?q=is%3Aissue+label%3Aupstream-sync)
  own tracked differences, decisions, dependencies and acceptance criteria.
- [UPGRADING.md](UPGRADING.md) explains validation commands and provenance rules.
- Each upgrade PR owns its assessment, validation results, created/reused issue
  list and delivery status.

Do not copy issue descriptions or statuses here. Do not add dated assessments,
per-upgrade files or run references. Update the map when test coverage or an
evidence boundary changes, not merely because the pinned versions change.

## Coverage map

`automated` means the stated scope is checked; it does not mean exhaustive parity.
`mixed` means automated evidence covers only a subset of the named surface.

| Surface | Status | Evidence | Boundary |
| --- | --- | --- | --- |
| Core orchestration and tools | mixed | Root tests, provider-independent UI fixtures and HTTP/core integration tests exercise lifecycle, tool execution, approvals, continuation, ordering, cancellation and URL-backed generated-file resolution with deterministic network policy tests. Pinned core mock request/UI snapshots cover direct and nested deferred discovery and next-step activation; root race tests cover search state isolation. | Provider replay covers configured scenarios, not every core option or scheduling interleaving. Deferred discovery mock evidence does not establish live provider acceptance. Deterministic generated-file HTTP tests do not prove live provider-hosted downloads. |
| Structured output | mixed | Object snapshots and unit tests exercise schemas, partial values, array elements and parsing. | Final-value snapshots do not establish partial-delivery or failure behavior; those require focused tests. |
| UI messages and SSE | mixed | Chunk snapshots, framing tests and schema-parsed frontend tests exercise conversion, ordering and assembly, including opaque metadata replacement, omission versus explicit empty objects, metadata-only deltas, persisted UI parts and separate tool call/result history. | Reader/writer tests do not establish all browser or hook lifecycle behavior. |
| React hooks | mixed | Actual useChat, useCompletion and useObject tests exercise selected success, error, stop, tool, approval and generated-file delivery flows. | Other lifecycle paths are not established by these tests or by chunk snapshots alone. |
| Agent, middleware and registry | mixed | Root tests exercise configuration, wrapping, provider resolution, simulated generated-content projection and cancellation; a schema-parsed frontend scenario checks simulated UI chunks and assembly. | Synthetic simulated generation does not prove real provider emissions; delegation to another implementation does not independently prove every entry point. |
| Agent Observability integration | mixed | Tests against the agento11y versions in the middleware's `go.mod` cover cached unary/streaming usage, inclusive export/span/metric semantics, and hook fixtures replayed through the real HTTP decoder. Gateway tests cover cached unary/streaming inclusive exports with both source and published middleware, bounded HTTP/gRPC export attempts, and ambient configuration rejection. | Hook validation starts after SDK normalization; discarded wire fields cannot be validated, and lost provider-tool discriminators fail reconstruction. Local HTTP servers do not prove deployed hook-service compatibility. Experimental OTel generation export is outside this evidence. |
| LanguageModelV4 contract | mixed | Discriminator checks, finite ProviderWire witnesses and provider request snapshots exercise represented types and mappings. | Shape checks are not complete semantic interface verification. |
| Provider adapters | mixed | `recorded/` and `upstream/` fixtures exercise represented requests, stream events, usage and outputs; provider tests cover synthetic failures and local invariants, including native Anthropic default Messages targets and caller-option precedence, OpenAI capability-gated reasoning/configuration controls, async tool metadata and continuation, multipart function-result conversion, immutable Responses schema normalization, and synthetic forced-choice HTTP request assertions for shell, local-shell and tool-search against registered upstream shapes. | No recorded/imported OpenAI request fixture exercises multipart result references, cache breakpoints or `propertyNames` normalization; focused request tests do not prove live acceptance. Request capture does not prove returned SDK metadata. Volatile SigV4 headers are excluded from snapshots. |
| SDK provider-tool contract | mixed | Native-provider entry tests reject mixed tool variants before HTTP; the independent Grafana client tests provider calls/results, deferred results, opaque tool metadata and omitted/empty presence. Actual pinned core text/UI stream comparisons and schema-parsed Go SSE cover input-start dynamic presence and application-tool classification. Anthropic synthetic HTTP tests cover hosted MCP options and explicit unary deadlines with native output-token defaults. | Client decoding does not activate Gateway provider-tool/MCP support. Synthetic HTTP requests do not establish live acceptance, deployed MCP egress or published-module adoption. |
| SDK provider transport context | mixed | Anthropic/OpenAI deterministic HTTP tests compare returned JSON with actual outbound bodies, including overrides, retries, concurrency, Vertex transformation and preconfigured clients. Core/Agent tests cover step and final metadata; frame tests cover opt-in raw ordering, unknown fields, ignored events, errors and cancellation. [Mantle transport tests](mantle_transport_test.go) cover source-workspace SigV4 retries and returned metadata. | Synthetic transports do not prove live acceptance, full upstream event-schema validation, or published Mantle adoption of the local OpenAI producer change. |
| Bedrock Mantle Responses continuation | mixed | [Mantle assistant-history request tests](../../providers/bedrock/mantle/provider_test.go) capture unary and streaming reconstruction, phase, empty text and stored references; standalone readonly Bedrock tests with a publicly resolved OpenAI dependency exercise [#207](https://github.com/grafana/ai-sdk/issues/207)'s consumer boundary. | Fake transport validates request encoding, not live Mantle acceptance or a provider recording. OpenAI producer tests or workspace substitutions alone do not establish Bedrock consumer adoption. Mantle Chat remains unsupported. |
| ProviderWire request projection | automated | Registered-client HTTP goldens, type/schema witnesses and mapping mutation tests are replayed through Go handlers. | This establishes the public client projection, not Vercel's private service behavior. |
| Gateway runtime and Go client | mixed | Handler, differential and command tests exercise supported text/function-tool/file-input paths, framing, bounds, privacy, cancellation and ownership. Both-client direct and configured-fallback unary/streaming tests preserve mapped function definitions/history/choices, file arms/filename presence, reasoning, headers and opaque options at supported scopes without Gateway inventories. Synthetic Anthropic/OpenAI/compatible chains cover configured order, per-candidate consumption, namespace precedence, scoped bypass/ordinary controls, supplied local assistant-call caller, capture independence and shared-handler race isolation. Both-client unary/streaming witnesses preserve omitted output-token limits through direct/fallback mapping; native Anthropic/OpenAI/compatible command tests verify provider defaults. Focused native Anthropic tests also carry provider definitions through direct/fallback unary/streaming requests; lifecycle tests retain no replay after a selected provider-owned call. Both-client tool continuation and raw-wire tests preserve opaque extensions and omitted/empty metadata independently of telemetry capture. Consumer-owned function loops restart primary on continuation; composed and reusable tests cover first-part commitment, no replay after selected encoding failure, bounded cleanup and one logical observation over physical attempts. | Schema acceptance and runtime support differ. Permissive client parsing does not prove strict server output, privacy or resource bounds. Synthetic native requests do not prove live acceptance; ordinary Anthropic tool-role result caller is forwarded to the adapter but not consumed. BYOK and request-directed routing are outside this coverage; foreign options unconsumed by compatible adapters remain inert. Pre-selection failover may repeat hidden native generation/effects and does not guarantee exactly-once provider work. |
| Gateway Anthropic hosted MCP | mixed | Registered-client MCP request goldens, actual resolved Go configuration controls, native history projection, mixed compatible/Anthropic candidate consumption and resource-bound tests. Both-client authenticated native-fake responses are reused as continuation; synthetic deferred results exercise generic opaque transport. Existing fallback tests cover pre-selection advancement and no replay after an observed MCP-looking provider call. | Native fakes and synthetic deferred results are not provider recordings, live hosted MCP egress, deployed readiness or published-module adoption. Response metadata is opaque, not configured-server or routing authority. Names validate current consumed history, not cross-request endpoint/token identity; unknown-outcome failover may repeat billing and remote effects. |
| Gateway provider metadata | mixed | Shared bounded server transport and independent Apache client tests cover registered result/content/event scopes, unknown namespace objects, omission/empty presence, aggregate original-byte/cardinality limits, encoded response/frame limits and malformed UTF-8/JSON failures. Authenticated command tests prove actual first-output-derived Anthropic caller/thinking/redacted, OpenAI item/phase/encrypted reasoning and compatible Google thought-signature continuation through both clients on direct and configured fallback paths. TS unary uses high-level generateText; Go unary uses direct DoGenerate with a test-only actual-output ContentPart/RawProviderOption adaptation and public ToResponseMessages. Both Go high-level generation APIs use DoStream. | Synthetic native-boundary witnesses are not recordings or live/private-service parity. Compatible continuation covers the google namespace and initial streaming tool-call signatures, not arbitrary configured namespaces or late signatures. Unchanged Anthropic simple-text/tool-call/thinking-signature and OpenAI reasoning-text recordings match direct UI and canonical native-request expectations through both clients. The imported encrypted-reasoning fixture concatenates independent responses into one mock stream and remains native-adapter evidence, not single-response Gateway continuation proof. No authentic full continuation matrix is claimed. |
| Gateway reasoning transport | mixed | Strict runtime and pinned TS/Go differential tests cover reasoning-file data/URL, usage and metadata; authenticated command tests replay both clients' assembled Anthropic/OpenAI/compatible history. Schema-parsed frontend SSE covers concurrency, replacement and files; native Anthropic/Bedrock HTTP tests cover empty/opaque continuation. | Synthetic native requests are not recordings. Unwrapped high-level unary TypeScript calls forward their SDK-generated User-Agent; ordinary mapped body headers are forwarded while credential-bearing overrides remain rejected. Command Bedrock configuration is not provided. Workspace/conformance success does not establish published adoption of local producer fixes. |
| Gateway host composition | mixed | Real-command tests use fake providers/JWKS for identity separation, routing, fallback, tool continuation and shutdown; a dummy Cloud edge exercises Go and pinned Vercel stack/CAP bearer headers, credential stripping and scope outcomes. Go StreamText and unwrapped TypeScript generateText/streamText calls cover explicit and default output-token limits without rewriting SDK headers or choices. | The dummy edge does not prove production CAP validation, expiry/revocation, policy realms or deployed ingress isolation. Generic HTTP error coverage is broader than errors reachable through the command's configured policies. |
| Baseline and fixture inventory | automated | Baseline validation covers registered consumers; generation and INDEX checks verify source existence, streaming inventory and byte-identical imports. | Generation does not establish input provenance. Unimported operations remain explicit in INDEX files. |
| Published module dependencies | automated | Standalone readonly tests resolve published dependencies with GOWORK=off. | Workspace substitutions do not prove consumer adoption of a producer change. |

## Evidence boundaries

- Gateway sources have explicit URL/document DTO and schema checks, pinned-client
  differential and synthetic native OpenAI command tests. Source metadata uses
  the same opaque namespace transport and complete-response/event bounds as
  other supported metadata scopes; it is not projected into synthetic citations.
  Source IDs, display identity and filename behavior are separate scalar policies.
  Metadata transport evidence does not establish native scalar/display parity.
- Provider-independent `ui/sources` snapshots and schema-parsed frontend tests
  cover URL/document assembly, required empty document titles and metadata.
  Synthetic command responses do not establish live provider acceptance.
- Reusable observers treat source as first output without adding an unsupported
  Agent Observability capture representation. Candidate-source Gateway tests
  verify this behavior with local middleware modules. The Gateway image uses
  same-revision source through `go.gateway.work`, with the image build as its
  delivery gate. The merged published pins predate this behavior, so standalone
  middleware consumers require later module releases.

- Shared synthetic request-target cases and TypeScript-generated snapshots under
  `testdata/request-snapshots/` validate escaped paths and raw query spelling/order
  through the Go loader, capture and comparator, including query-only rejection
  witnesses. They prove harness sensitivity, not provider response provenance or
  live acceptance. Header redaction does not protect query credentials;
  query-authenticated recordings require an explicit secret/volatility policy.
- Conformance comparisons ignore ordering only between adjacent locally executed
  sibling tool outputs. Provider-executed outputs and rejected-input errors remain
  ordered; the comparator deviation is registered in `upstream.yaml`.
- SDK-backed providers return the final outbound JSON after SDK transformations,
  rather than upstream's sometimes-logical pre-transform request body. Non-JSON
  outbound bodies omit JSON request metadata. Frame-level raw evidence does not
  replace SDK typed decoding; existing permissive union decoding and OpenAI
  malformed-event recovery are not proof of upstream schema/recovery parity.
  Anthropic preserves SDK event-name filtering even for malformed ignored
  payloads. Both adapters apply a Go resource-safety adaptation: startup is
  limited to 64 data frames and 1 MiB before handoff, independent of raw selection;
  overflow fails setup without truncating evidence or enabling retries.
- Deferred discovery uses sorted tool names for equal-score matches because Go
  ToolSet maps have no insertion order; upstream ties preserve object insertion
  order. Context-dependent descriptions use the existing shared Go runtime
  context, not upstream's per-tool context or sandbox-session surface. Paired
  `ui/deferred-tool-discovery` core request/UI snapshots and schema-parsed frontend
  assembly cover discovery, early static-call errors and next-step execution,
  not provider-hosted search or live adapter acceptance.
- GenerateText and Agent.Generate collect StreamText. Provider DoGenerate tests
  therefore do not prove those high-level paths. Effective required/named choice
  enforcement has direct entry-point tests,
  [provider-independent UI snapshots](ui/effective-tool-choice/) and schema-parsed
  frontend assembly tests. These establish completed-response
  rejection and retained completion data, not live provider adherence to choice.
- Captured provider inputs, synthetic failures and provider-independent UI parts
  are distinct evidence sources; passing one does not establish the others.
- Gateway privacy assertions cover protocol metadata, errors, logs and metrics.
  Arbitrary application text and bounded raw responses remain caller-visible.
- Linux FIFO deadline tests are platform-specific; socket checks on another
  platform do not establish Linux runtime behavior.

## Retained deviations without issue ownership

These notes prevent matching tests from being mistaken for upstream equivalence.
If a deviation becomes tracked work, its rationale and disposition belong in the
issue rather than being maintained in both places.

- Core batches local approval handling after provider streaming. Completed
  tool-choice violations bypass new local approvals and execution; already streamed
  provider events and prior-message approvals keep their existing handling.
  Upstream resolves streaming approvals before completed-response validation, so
  approval suppression is a Go batching safety boundary, not identical timing.
  ToUIMessageStream supplies a default ID generator with original messages and
  avoids an unused generation on continuation. Timeout warnings are returned on
  successful results rather than logged globally before invocation.
- Anthropic uses explicit credentials/options rather than ambient SDK defaults.
  With no tools, required/named choices are retained. Its api_error/overloaded_error
  retry classification is broader than upstream's initial-overload rule; the full
  error envelope and inner response error are retained separately.
- OpenAI resolves configured apply-patch aliases. Legacy nested-error envelopes
  terminate consumption. Plain EOF without a terminal/error does not synthesize a
  provider finish, preserving Go's incomplete-stream distinction; that is not a
  claim of complete upstream EOF parity.
- Bedrock warns and omits unsupported tool-result file URLs instead of failing.
- Gateway is a Grafana extension with no private-service oracle. It deliberately
  uses strict response families, protected auth/protocol headers and fixed server
  warning prose. The client retains bounded public error prose without upstream
  auth guidance or generation-ID suffixes; Go cancellation preserves context
  identity. The [client contract](../../openspec/specs/grafana-gateway-client/spec.md)
  defines the detailed boundary.

### Vertex JSON-schema output

Vertex enables the Anthropic adapter's native JSON output capability, following
Google Cloud's documented `output_config.format` support. This is a
provider-configuration adaptation of the registered `@ai-sdk/anthropic` 4.0.59
capability gate, not a change to response mapping or the upstream baseline.
Strict tool capability remains separate. Synthetic request-serialization tests
cover supported models (including Sonnet 5.5), streaming and non-streaming
requests, and the absence of forced tool choice and synthetic tools. Existing
fallback tests cover older models and explicit JSON-tool mode. No live Vertex
recording is claimed; organization policy must enable structured outputs.
