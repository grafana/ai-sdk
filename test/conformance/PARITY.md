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
| Structured output | mixed | Object snapshots and focused unit tests exercise schemas, partial values, array elements, parsing, repair failure paths and raw-response preservation. | Final-value snapshots alone do not establish partial-delivery or failure behavior; those require focused tests. |
| UI messages and SSE | mixed | Chunk snapshots, framing tests and schema-parsed frontend tests exercise conversion, ordering and assembly, including opaque metadata replacement, omitted versus explicit empty objects, metadata-only deltas and separate tool call/result history. Pinned differentials cover seven static/dynamic tool states through persistence, resume and conversion, including accumulated raw-string partial-input restoration, output tool-metadata replacement and superseded approvals. | Reader/writer tests do not establish all browser or hook lifecycle behavior; scalar construction and provider-domain optional-empty representations remain bounded adaptations. |
| React hooks | mixed | Actual useChat, useCompletion and useObject tests exercise selected success, error, stop, tool, approval and generated-file delivery flows. Tool-state tests persist/remount/resume preliminary and approval history through Go and compare the fake provider prompt with the pinned Agent. | Other lifecycle paths are not established by these tests or by chunk snapshots alone; the deterministic model does not prove live provider acceptance. |
| Agent, middleware and registry | mixed | Root tests exercise configuration, wrapping, provider resolution, simulated generated-content projection and cancellation; schema-parsed frontend tests check simulated UI chunks, isolated history validation/normalization and zero provider calls on invalid history. | Dynamic tools skip static schemas; application metadata/data schemas, unrepresented provider-tool schemas, approval input-schema provenance and transforms/refinements remain outside validation proof. Shared schema cases are not exhaustive. Synthetic generation does not prove real provider emissions; delegation to another implementation does not independently prove every entry point. |
| Fallback observation | mixed | SDK tests cover request-scoped observation on shared models, independent observer panic recovery, original candidate errors across cancellation, setup/EOF validation, first-part commitment and exclusion of unowned late results. High-level GenerateText/StreamText tests preserve selected metadata and capture earlier candidate failures. | Go-specific observation hooks, not automatic trace grouping or metadata serialization. The registered upstream has no matching fallback implementation. Native errors remain in-process; these tests do not establish Gateway projection, runtime delivery or live-provider behavior. |
| Agent Observability integration | mixed | Tests against the agento11y versions in the middleware's `go.mod` cover cached unary/streaming usage, inclusive export/span/metric semantics, and hook fixtures replayed through the real HTTP decoder. Gateway tests cover cached unary/streaming inclusive exports with both source and published middleware, bounded HTTP/gRPC export attempts, and ambient configuration rejection. | Hook validation starts after SDK normalization; discarded wire fields cannot be validated, and lost provider-tool discriminators fail reconstruction. Local HTTP servers do not prove deployed hook-service compatibility. Experimental OTel generation export is outside this evidence. |
| LanguageModelV4 contract | mixed | Discriminator checks, finite ProviderWire witnesses and provider request snapshots exercise represented types and mappings. | Shape checks are not complete semantic interface verification. |
| Provider adapters | mixed | `recorded/` and `upstream/` fixtures exercise represented requests, stream events, usage and outputs; provider tests cover synthetic failures and local invariants, including native Anthropic default Messages targets and caller-option precedence, OpenAI capability-gated reasoning/configuration controls, async tool metadata and continuation, multipart function-result conversion, immutable Responses schema normalization, and synthetic forced-choice HTTP request assertions for shell, local-shell and tool-search against registered upstream shapes. | No recorded/imported OpenAI request fixture exercises multipart result references, cache breakpoints or `propertyNames` normalization; focused request tests do not prove live acceptance. Request capture does not prove returned SDK metadata. Volatile SigV4 headers are excluded from snapshots. |
| Anthropic on Microsoft Foundry | mixed | Synthetic HTTP tests exercise endpoint/authentication, retry credential refresh, deployment-name request mapping, canonical generation/stream identity, tools, reasoning and usage through the shared Anthropic adapter. | The registered `@ai-sdk/anthropic` baseline has no Foundry constructor. This Go extension follows the native Foundry contract and composes an older published Anthropic adapter for consumer compatibility; no live Foundry recording or hosting-version capability proof is claimed. |
| SDK provider-tool contract | mixed | Native-provider entry tests reject mixed tool variants before HTTP; the independent Grafana client tests provider calls/results, deferred results, opaque tool metadata and omitted/empty presence. Pinned-client differential tests cover variant-local decoding, ignored unrelated fields and preserved false markers; optional null flags normalize to Go absence/zero. UI classification uses the original tool types independently of text-stream markers and deferred execution registries. Actual pinned core text/UI stream comparisons and schema-parsed Go SSE cover input-start dynamic presence and application-tool classification. Anthropic synthetic HTTP tests cover hosted MCP options and explicit unary deadlines with native output-token defaults. | Client decoding does not activate Gateway provider-tool/MCP support. Typed Go decoding rejects unrepresentable consumed field types rather than duplicating upstream's untyped response passthrough; unrelated variant fields are ignored. Synthetic HTTP requests do not establish live acceptance, deployed MCP egress or published-module adoption. |
| SDK provider transport context | mixed | Anthropic/OpenAI deterministic HTTP tests compare returned JSON with actual outbound bodies, including overrides, retries, concurrency, Vertex transformation and preconfigured clients. Core/Agent tests cover step and final metadata; frame tests cover opt-in raw ordering, unknown fields, ignored events, errors and cancellation. [Mantle transport tests](mantle_transport_test.go) cover source-workspace SigV4 retries and returned metadata. | Synthetic transports do not prove live acceptance, full upstream event-schema validation, or published Mantle adoption of the local OpenAI producer change. |
| Bedrock Mantle Responses continuation | mixed | [Mantle assistant-history request tests](../../providers/bedrock/mantle/provider_test.go) capture unary and streaming reconstruction, phase, empty text and stored references; standalone readonly Bedrock tests with a publicly resolved OpenAI dependency exercise [#207](https://github.com/grafana/ai-sdk/issues/207)'s consumer boundary. | Fake transport validates request encoding, not live Mantle acceptance or a provider recording. OpenAI producer tests or workspace substitutions alone do not establish Bedrock consumer adoption. Mantle Chat remains unsupported. |
| ProviderWire request projection | automated | Registered-client HTTP goldens, type/schema witnesses and mapping mutation tests are replayed through Go handlers. Pinned Gateway BYOK capture and independent Go-client tests preserve multiple-provider credential maps, array order and caller-owned request metadata; pinned captures also cover explicit bearer JWT emission. | This establishes the public client projection, not Vercel's private service behavior. |
| Gateway request-only engine | mixed | Focused synthetic native transports cover explicit keys, native endpoint defaults, exact service-approved baseURL overrides, OpenAI organization/project, poisoned environment, ordered default fallback and concurrent account isolation; generic handler tests cover catalog-independent selection, the shared deadline and the whole-request byte limit before selection. Focused decoder tests cover exact control names, temporary account-attempt policy, ordinary Go casing/duplicates/additive fields and explicit meaningful-mapping refusal; actual HTTP transports reject malformed account headers before I/O. Pinned TS/Go client tests preserve extended account objects and caller metadata; opt-in Go capture applies its reusable configured field redactor without rewriting returned data. | Account configuration is a Grafana extension, not established Vercel hosted-service behavior. Engine decoding uses ordinary Go struct matching, ignores unrelated additive fields and rejects malformed known values or meaningful unsupported controls without an untyped normalization layer. BYOK adds no separate field/subtree byte ceilings; host composition owns approved URL syntax. The command separates private configured and trusted Cloud request-only selection. Custom destination-policy configuration and deployed DNS/proxy/network isolation are not established. No Vercel hosted-service retry algorithm or live native acceptance is claimed. |
| Gateway runtime and Go client | mixed | Handler, differential and command tests exercise supported text/function-tool/file-input paths, framing, bounds, privacy, cancellation and ownership. Both-client direct and configured-fallback unary/streaming tests preserve mapped function definitions/history/choices, file arms/filename presence, reasoning, headers and opaque options at supported scopes without Gateway inventories. Synthetic Anthropic/OpenAI/compatible chains cover configured order, per-candidate consumption, namespace precedence, scoped bypass/ordinary controls, supplied local assistant-call caller, capture independence and shared-handler race isolation. Both-client unary/streaming witnesses preserve omitted output-token limits through direct/fallback mapping; native Anthropic/OpenAI/compatible command tests verify provider defaults. Focused native Anthropic tests also carry provider definitions through direct/fallback unary/streaming requests; both-client authenticated MCP request-boundary tests preserve Anthropic configuration and supplied history, OpenAI MCP definitions, native approval defaults, and separate inference/MCP credentials without operator capture. Lifecycle tests retain no replay after a selected provider-owned call. Both-client tool continuation and raw-wire tests preserve opaque extensions and omitted/empty metadata independently of telemetry capture. Consumer-owned function loops restart primary on continuation; composed and reusable tests cover first-part commitment, no replay after selected encoding failure, bounded cleanup and one logical observation over physical attempts. | Schema acceptance and runtime support differ. Permissive client parsing does not prove strict server output, privacy or resource bounds. Synthetic native requests do not prove live acceptance; ordinary Anthropic tool-role result caller is forwarded to the adapter but not consumed. Synthetic hosted-MCP tests establish native request conversion and Anthropic supplied-history mapping, not live egress or the complete hosted-result/fallback continuation matrix. Request-directed configured routing remains unsupported; foreign options unconsumed by compatible adapters remain inert. Pre-selection failover may repeat hidden native generation/effects and does not guarantee exactly-once provider work. |
| Gateway Anthropic hosted MCP | mixed | Registered-client MCP request goldens, actual resolved Go configuration controls, native history projection, mixed compatible/Anthropic candidate consumption and resource-bound tests. Both-client authenticated native-fake responses are reused as continuation; synthetic deferred results exercise generic opaque transport. Existing fallback tests cover pre-selection advancement and no replay after an observed MCP-looking provider call. | Native fakes and synthetic deferred results are not provider recordings, live hosted MCP egress, deployed readiness or published-module adoption. Response metadata is opaque, not configured-server or routing authority. Names validate current consumed history, not cross-request endpoint/token identity; unknown-outcome failover may repeat billing and remote effects. |
| Gateway provider metadata | mixed | Shared bounded server transport and independent Apache client tests cover registered result/content/event scopes, unknown namespace objects, omission/empty presence, aggregate original-byte/cardinality limits, encoded response/frame limits and malformed UTF-8/JSON failures. Authenticated command tests prove actual first-output-derived Anthropic caller/thinking/redacted, OpenAI item/phase/encrypted reasoning and compatible Google thought-signature continuation through both clients on direct and configured fallback paths. TS unary uses high-level generateText; Go unary uses direct DoGenerate with a test-only actual-output ContentPart/RawProviderOption adaptation and public ToResponseMessages. Both Go high-level generation APIs use DoStream. | Synthetic native-boundary witnesses are not recordings or live/private-service parity. Compatible continuation covers the google namespace and initial streaming tool-call signatures, not arbitrary configured namespaces or late signatures. Unchanged Anthropic simple-text/tool-call/thinking-signature and OpenAI reasoning-text recordings match direct UI and canonical native-request expectations through both clients. The imported encrypted-reasoning fixture concatenates independent responses into one mock stream and remains native-adapter evidence, not single-response Gateway continuation proof. No authentic full continuation matrix is claimed. |
| Gateway execution overviews | mixed | Pure projection tests cover ordered observed decisions and configured identity, candidate-local shallow native summaries and credential-echo preservation, precise numeric codes, complete arrays without a diagnostic cap, input isolation, native namespace relocation and exact complete-response/SSE-frame fit checks. Shared fallback composition preserves error/text/finish order and independent request observation; Go-produced namespace values and strict TypeScript schema tests exercise the compact shape. | Production handler, native-fake authenticated command and both-client tests prove direct/fallback delivery, mapped classifications and physical counts; low/high-level/middleware streaming access preserves event-local errors followed by content/finish. Schema-parsed frontend tests verify ordered UI assembly without automatic diagnostic forwarding. Races cover request isolation, cancellation/late-result exclusion, exact fit/native collision and saturated operator independence. Synthetic proof is not live-provider or private-service parity. Projection does not inventory credentials or suppress echoed field values; logger field policy is independent. Diagnostic JSON uses standard Go duplicate/string normalization; opaque metadata and independent client raw error retention are unchanged. Overview enrichment is optional under transitional existing source/read and complete-envelope limits; shared transport policy is separately owned. If native relocation cannot fit, original metadata remains opaque; namespace presence alone does not establish Gateway provenance or absence of attempts. No stream completion/replay claims or post-selection error history are projected. Catalog-owned copied candidate descriptors enable configured attribution. Plain request selections omit configured overviews without collecting a second request-local history; event-local native summaries remain eligible independently of catalog provenance. Focused handler and both-client tests preserve ordered errors, opaque native metadata and independently registered SDK observers. |
| Gateway request-only accounts | mixed | Catalog spies, poisoned-environment transport tests, Go logger capture tests and pinned-client command tests exercise private/configured versus Cloud/BYOK isolation. A local TLS proxy keeps command BYOK native destinations fixed while serving explicitly synthetic Anthropic and OpenAI Responses data to both clients. | Request-supplied API-key accounts use native Anthropic Messages and OpenAI Responses. The command permits native default endpoints only; custom destination-policy configuration is not supplied. Ordered accounts reuse the Go fallback default policy, not a claimed Vercel hosted-service algorithm. Authenticated native-HTTP and both-client Cloud SSE tests preserve producer echoes in message/type/code/status without changing Gateway classification or account isolation; logger field redaction and metadata-only exporter exclusions are independent. BYOK attempt overviews and unary/setup native summaries remain missing observation capabilities under #317, not confidentiality rules. Other credential families/routing, fuller native transport access, live acceptance and deployed edge/network isolation are outside this evidence. |
| Gateway reasoning transport | mixed | Strict runtime and pinned TS/Go differential tests cover reasoning-file data/URL, usage and metadata; authenticated command tests replay both clients' assembled Anthropic/OpenAI/compatible history. Schema-parsed frontend SSE covers concurrency, replacement and files; native Anthropic/Bedrock HTTP tests cover empty/opaque continuation. | Synthetic native requests are not recordings. Unwrapped high-level unary TypeScript calls forward their SDK-generated User-Agent; ordinary mapped body headers are forwarded while credential-bearing overrides remain rejected. Command Bedrock configuration is not provided. Workspace/conformance success does not establish published adoption of local producer fixes. |
| Gateway host composition | mixed | Real-command tests use fake providers/JWKS for identity separation, routing, fallback, tool continuation and shutdown; a dummy Cloud edge exercises Go and pinned Vercel stack/CAP bearer headers, credential stripping and scope outcomes. Go StreamText and unwrapped TypeScript generateText/streamText calls cover explicit and default output-token limits without rewriting SDK headers or choices. | The dummy edge does not prove production CAP validation, expiry/revocation, policy realms or deployed ingress isolation. Generic HTTP error coverage is broader than errors reachable through the command's configured policies. |
| Baseline and fixture inventory | automated | Baseline validation covers registered consumers; generation and INDEX checks verify source existence, streaming inventory and byte-identical imports. | Generation does not establish input provenance. Unimported operations remain explicit in INDEX files. |
| Published module dependencies | automated | Standalone readonly tests resolve published dependencies with GOWORK=off. | Workspace substitutions do not prove consumer adoption of a producer change. |

## Evidence boundaries

- Gateway native warning/source/response identity mapping has closed raw/schema,
  pinned TS/independent Go and synthetic authenticated command evidence. Native
  source IDs/display/order and optional registered response identity survive;
  required empty source/warning fields are covered. Optional empty Go strings
  normalize presence where the domain API cannot distinguish absence/empty.
  Unary clients replace typed request/response with Gateway-hop information;
  native identity remains in the raw body. Source metadata preserves opaque namespace objects under shared response/event bounds rather than synthetic citations; unknown fields and cited text are retained. This evidence does not claim full live/private-service parity.
- Provider-independent `ui/sources` snapshots and schema-parsed frontend tests
  cover URL/document assembly, required empty document titles and metadata.
  The native-source-identity frontend scenario additionally preserves repeated
  and equal-cross-variant IDs/display, and explicitly maps supplied response
  identity through a test-only message-metadata callback. It does not establish
  automatic UI identity/warning fields or Gateway absent-value semantics.
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
- Configured Gateway discovery is an intentional Grafana extension, not upstream
  private-service behavior. Startup config tests cover route semantics and
  cardinality/UTF-8 string policy. Discovery returns one canonical row per model
  with configured aliases, primary and ordered fallbacks; native destination IDs
  use providerModelId. Aliases remain callable without duplicate rows. Discovery
  projects the complete visible catalog without revalidation or a response-size cap. Server/Go/helper tests
  cover full large-catalog projection, typed retention, defensive copies and
  scoped listing/resolution. Clients retain independent read limits and atomic
  JSON/type errors, not server route-policy checks. Go uses standard encoding/json
  missing/null/case behavior; the TS helper checks runtime JSON shapes/types.
  This accepted Go adaptation is not identical malformed-shape acceptance.
  Go replaces escaped lone UTF-16 surrogates with U+FFFD while TS retains them,
  so no lossless cross-client identity agreement is claimed for those escapes.
  Valid pairs and genuine replacement characters remain ordinary data. Exact-pinned real-command
  witnesses compare raw HTTP, Go retention and the copyable TypeScript helper
  against stock `getAvailableModels()` listing canonical IDs and stripping the
  gateway extension (including aliases), without native inference.
  Command catalogs are static and private/configured-account only. Authenticated
  Cloud discovery returns an explicit unsupported-discovery error without catalog
  access. Dummy CAP-edge tests establish application policy isolation, not deployed
  CAP authorization or network isolation.
  Focused BYOK engine tests reuse the configured Anthropic consumption guard and
  prove zero native calls for invalid consumed MCP configuration/history, skills
  and server-side fallback. Valid MCP configuration/history, inert local markers
  and ordinary options remain accepted.
  Client reads default to 4 MiB independently of server configuration.
- Gateway privacy assertions cover configured-secret structures, safe errors
  and metadata-only canonical operator logs/metrics/exports (request-selected identity
  and a fixed model metric bucket for BYOK) without censoring
  supported native warning/source/response identity. Consumer WrapGenerate and
  WrapStream tests prove contracted hook access; separate consumer logger tests
  assert actual opt-in Gateway body capture at its own destination. BYOK logger
  protection covers typed SDK options and JSON request/error bodies, not arbitrary
  opaque application captures; ordinary gateway fields and timeout options remain
  intact. Direct application/TypeScript capture remains caller-owned. Hook access
  does not imply universal built-in stream-warning/source capture or an Agent
  Observability source representation, nor full native diagnostic access.
- Linux FIFO deadline tests are platform-specific; socket checks on another
  platform do not establish Linux runtime behavior.

- Tool-state fixtures are provider-independent core chunks, not recordings.
  Pinned UI/Agent APIs own expectations; comparisons preserve grouping, required
  empty text and metadata selection. Approval resumption retains call-scoped
  provider options in history without projecting them onto local result metadata.
  Prompt coalescing tests cover deep precedence and caller isolation.

## Retained deviations without issue ownership

These notes prevent matching tests from being mistaken for upstream equivalence.
If a deviation becomes tracked work, its rationale and disposition belong in the
issue rather than being maintained in both places.

- UI tool presence is bounded: persisted provider-executed/automatic scalars and
  signatures omit false/empty, as do direct chunk optional scalar zero values.
  Decoded chunks retain selected empty/false values; false clears prior true.
  Static tool JSON includes redundant toolName. Provider projections may omit
  empty options and optional approval reasons/signatures/false flags, but never
  required denied/text content or presence-based metadata selection.
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
- Foundry keeps canonical model IDs for capability selection and response
  identity while transmitting Azure deployment names. Its endpoint/auth facade
  deliberately preempts ambient authentication in the compatible published
  adapter and additionally supports per-attempt API-key rotation. Native
  Anthropic Messages conversion, beta handling and wire decoding are shared;
  a provider-owned forwarding stage adjusts existing response metadata without
  adding frontend stream-part types. Hosting-specific capability
  restrictions, approved deployments and residency policy belong to consumers.
- OpenAI resolves configured apply-patch aliases. Legacy nested-error envelopes
  terminate consumption. Plain EOF without a terminal/error does not synthesize a
  provider finish, preserving Go's incomplete-stream distinction; that is not a
  claim of complete upstream EOF parity.
- Bedrock warns and omits unsupported tool-result file URLs instead of failing.
- Gateway is a Grafana extension with no private-service oracle. It deliberately
  uses strict response families and protected auth/protocol headers. Go rejects
  reserved authentication-header overrides before I/O, unlike the pinned client's
  permissive header merging; client native selectors use a 2,048-byte transport bound
  rather than configured-catalog grammar. The client retains bounded public error prose without upstream
  auth guidance or generation-ID suffixes; Go cancellation preserves context
  identity. HTTP error envelopes and SSE error payloads use standard Go JSON
  struct-member matching, including case-insensitive names, as an intentional
  acceptance deviation for noncanonical input; retained HTTP bodies and opaque
  SSE error data preserve their original bytes. The [client contract](../../openspec/specs/grafana-gateway-client/spec.md)
  defines the detailed boundary.

### Vertex JSON-schema output

Vertex enables the Anthropic adapter's native JSON output capability, following
Google Cloud's documented `output_config.format` support. This is a
provider-configuration adaptation of the Anthropic capability gate, not a change
to response mapping or the upstream baseline.
Strict tool capability remains separate. Synthetic request-serialization tests
cover supported models (including Sonnet 5.5), streaming and non-streaming
requests, and the absence of forced tool choice and synthetic tools. Existing
fallback tests cover older models and explicit JSON-tool mode. No live Vertex
recording is claimed; organization policy must enable structured outputs.
