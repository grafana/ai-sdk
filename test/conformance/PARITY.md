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
| Core orchestration and tools | mixed | Root tests, provider-independent UI fixtures and HTTP/core integration tests exercise lifecycle, tool execution, approvals, continuation, ordering and cancellation. Cross-language tests distinguish presence-sensitive input-start text inference from UI tool-definition projection. | Provider replay covers configured scenarios, not every core option or scheduling interleaving. |
| Structured output | mixed | Object snapshots and unit tests exercise schemas, partial values, array elements and parsing. | Final-value snapshots do not establish partial-delivery or failure behavior; those require focused tests. |
| UI messages and SSE | mixed | Chunk snapshots, framing tests and schema-parsed frontend tests exercise conversion, ordering and assembly. | Reader/writer tests do not establish all browser or hook lifecycle behavior. |
| React hooks | mixed | Actual useChat, useCompletion and useObject tests exercise selected success, error, stop, tool and approval flows. | Other lifecycle paths are not established by these tests or by chunk snapshots alone. |
| Agent, middleware and registry | mixed | Root tests exercise configuration, wrapping, provider resolution, simulated generated-content projection and cancellation; a schema-parsed frontend scenario checks simulated UI chunks and assembly. | Synthetic simulated generation does not prove real provider emissions; delegation to another implementation does not independently prove every entry point. |
| LanguageModelV4 contract | mixed | Discriminator checks, finite ProviderWire witnesses and provider request snapshots exercise represented types and mappings. | Shape checks are not complete semantic interface verification. Direct tool validation rejects populated inactive fields before native I/O; nil Go provider args normalize to `{}` without relaxing HTTP missing/null-args rejection. |
| Provider adapters | mixed | `recorded/` and `upstream/` fixtures exercise represented requests, stream events, usage and outputs; provider tests cover synthetic failures and local invariants, including OpenAI multipart function-result request conversion. | No recorded/imported OpenAI request fixture exercises multipart result references or cache breakpoints; focused request tests do not prove live acceptance. Native Anthropic tests cover optional MCP token/enabled presence and context-derived unary timeout defaults, including explicit-timeout precedence. Request capture does not prove returned SDK metadata. Volatile SigV4 headers are excluded from snapshots. |
| Bedrock Mantle Responses continuation | mixed | [Mantle assistant-history request tests](../../providers/bedrock/mantle/provider_test.go) capture unary and streaming reconstruction, phase, empty text and stored references; standalone readonly Bedrock tests with a publicly resolved OpenAI dependency exercise [#207](https://github.com/grafana/ai-sdk/issues/207)'s consumer boundary. | Fake transport validates request encoding, not live Mantle acceptance or a provider recording. OpenAI producer tests or workspace substitutions alone do not establish Bedrock consumer adoption. Mantle Chat remains unsupported. |
| ProviderWire request projection | automated | Registered-client HTTP goldens, type/schema witnesses and mapping mutation tests are replayed through Go handlers. | This establishes the public client projection, not Vercel's private service behavior. |
| Gateway runtime and Go client | mixed | Handler, differential and command tests exercise supported text/function-tool paths, framing, bounds, privacy, cancellation and ownership. Focused Go client tests additionally cover provider-call/result markers and reviewed metadata; these readers do not activate service provider-tool or MCP support. | Schema acceptance and runtime support differ. Permissive client parsing does not prove strict server output, privacy or resource bounds. |
| Gateway host composition | mixed | Real-command tests use fake providers/JWKS for identity separation, routing, fallback, tool continuation and shutdown; a dummy Cloud edge exercises Go and pinned Vercel stack/CAP bearer headers, credential stripping and scope outcomes. | The dummy edge does not prove production CAP validation, expiry/revocation, policy realms or deployed ingress isolation. Generic HTTP error coverage is broader than errors reachable through the command's configured policies. |
| Baseline and fixture inventory | automated | Baseline validation covers registered consumers; generation and INDEX checks verify source existence, streaming inventory and byte-identical imports. | Generation does not establish input provenance. Unimported operations remain explicit in INDEX files. |
| Published module dependencies | mixed | Required ancestry checks verify declared and selected internal pins against canonical main; on-demand standalone readonly tests resolve selected published modules with GOWORK=off. | Candidate-source integration is not standalone publication evidence; standalone checks must pass before release or Gateway image publication. |

## Evidence boundaries

- Conformance comparisons ignore ordering only between adjacent locally executed
  sibling tool outputs. Provider-executed outputs and rejected-input errors remain
  ordered; the comparator deviation is registered in `upstream.yaml`.
- GenerateText and Agent.Generate collect StreamText. Provider DoGenerate tests
  therefore do not prove those high-level paths.
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

- Core batches local approval handling after provider streaming. ToUIMessageStream
  supplies a default ID generator with original messages and avoids an unused
  generation on continuation. Timeout warnings are returned on successful results
  rather than logged globally before invocation.
- Anthropic uses explicit credentials/options rather than ambient SDK defaults.
  With no tools, required/named choices are retained. Its api_error/overloaded_error
  retry classification is broader than upstream's initial-overload rule; the full
  error envelope and inner response error are retained separately. Direct unary
  calls without a context deadline or explicit SDK timeout retain the Go SDK's
  large-token non-streaming guard; bounded calls preserve the token budget.
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
