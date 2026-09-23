## Context

This implements issue #107 / WP13 of `~/ai-sdk-ai-gateway-plan.md`, after merged WP11 (#105) and WP12 (#106). It is a capability extension, not a baseline upgrade or a new transport.

Current boundaries:

- `ai-gateway/providerwire/v4/schema/request.json` already defines the closed provider-tool arm; runtime `mapFunctionTools` still rejects it. `mapWirePart` rejects provider-executed calls, assistant tool results and nonempty part options.
- Unary output supports text and client calls only. `stream_tools.go` rejects enabled execution/dynamic/preliminary markers and only permits one result per call.
- `providers/grafana/request.go` already projects provider definitions and opaque top-level provider options and rejects definition-level provider options; unary/SSE readers remain narrower. Strict Gateway runtime rejects nonempty root options today.
- `provider.StreamPart` and `GenerateContentPart` use pointer dynamic/preliminary fields. Root orchestration and native providers consume them, so the change crosses module boundaries.
- Native Anthropic/OpenAI implementations already have provider-tool conversion, alias resolution and result handling. Agent Observability already recognizes provider execution and preliminary results. Extend these paths rather than replacing them.

### Registered reference and evidence

Planning source was read using `git show d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e:<path>` in `~/src/ai`. Its checked-out HEAD differs; no behavior was taken from that HEAD. Package manifests at the registered commit confirm provider 4.0.7, gateway 4.0.52, anthropic 4.0.38, openai 4.0.41 and ai 7.0.65.

Relevant upstream paths:

- `packages/provider/src/language-model/v4/language-model-v4-{provider-tool,tool-call,tool-result,stream-part,prompt}.ts`: definition shape, marker placement, result semantics and roles.
- `packages/gateway/src/gateway-language-model.ts`: request serialization, including `providerOptions.anthropic.mcpServers`, permissive consumption and unary client-owned replacement. The public client does not reveal the hosted Gateway's validation, egress or forwarding policy.
- `packages/ai/src/generate-text/stream-language-model-call.ts:724-731`: input-start uses `chunk.dynamic ?? tool?.type === 'dynamic'`, so absence and false are not equivalent.
- `packages/ai/src/generate-text/stream-text.test.ts:25992-26086`: a provider-owned call can finish without a result and receive an error result in a later step without a repeated call; the Anthropic code-execution case at `:24472-24498` also exercises deferred completion.
- `packages/anthropic/src/anthropic-prepare-tools.ts` and matching tests: function-only options, native provider tool arguments, names and beta selection.
- `packages/anthropic/src/anthropic-language-model.ts`, `anthropic-language-model-options.ts` and `convert-to-anthropic-prompt.ts`: hosted execution, request-level `mcpServers` -> Anthropic `mcp_servers`, MCP call/result `providerMetadata.anthropic.{type,serverName}`, dynamic code execution and inline replay. The matching Go provider already supports `AnthropicOptions.MCPServers` and the same MCP metadata in `providers/anthropic/`.
- `packages/openai/src/responses/openai-responses-prepare-tools.ts`: native tool IDs, arguments and selections; implementation must also read the exact matching response/prompt tests for each changed conversion.

`test/conformance/PARITY.md` classifies provider tools as mixed provider-contract/native evidence and unsupported Gateway runtime/client output. Existing Anthropic/OpenAI provider fixtures cover selected hosted tools. Those do not prove Gateway transport, and deterministic Gateway backend fakes do not establish provider fixture provenance.

## Goals / Non-Goals

**Goals:**

- Preserve registered definition shapes, execution ownership and result semantics through direct unary/streaming routes and both clients.
- Support assistant-side provider-result continuation, including deferred result-only responses, using request-local history correlation without persisted state or tool execution in the Gateway.
- Normalize equivalent absent/false domain markers, preserve the presence-sensitive input-start dynamic exception, and reject mixed tool definitions before native I/O.
- Carry only reviewed, bounded tool metadata publicly, including the MCP server name required for registered Anthropic replay; keep logical exports metadata-only and canonical.
- Support registered caller-supplied Anthropic MCP server definitions on direct routes without adding a general top-level provider-options capability.
- Prove native conversion, lifecycle safety, client equivalence and isolated module builds.

**Non-Goals:**

- Approval/denial flows, nested output-content provider options, file/reasoning/custom/source families, root provider options other than the narrow Anthropic `mcpServers` exception, raw output or new provider factories (owned by later packages).
- Enabling tool fallback/replay, a Gateway-owned registry of tool implementations, local execution, or persisted conversations.
- Full support for every compound output a provider tool might produce. A web search can emit sources and a hosted image tool can emit files or a preliminary image preview before the tool call; those families remain explicitly unsupported until their work packages land (generated media and pre-call previews: WP16/#110). Do not silently discard them to claim success.
- Broadly redesigning core/UI tool types or upgrading upstream pins.
- A claim about Vercel's unpublished hosted Gateway policy, a Grafana-owned MCP proxy/tool executor, or host-managed MCP server alias configuration.

## Decisions

### 1. Preserve the registered union at each boundary

Use discriminator-specific private request mapping for provider and function definitions. Provider definitions contain only `type`, `id`, `name`, required object `args`; preserve `{}` and nested JSON values. IDs/names are not rewritten by the Gateway. Native providers own ID support, argument interpretation, alias mapping, warnings and beta/native selection rules.

The existing complete schema rejects `description`, `inputSchema`, `inputExamples`, `strict` and `providerOptions` on provider definitions, including explicit empty/false members, before policy/resolution/invocation. Direct Go callers cannot represent presence of an empty descriptive string, but can reject every populated incompatible field and non-nil incompatible collection. Direct Go provider-tool `Args == nil` denotes the same empty object as upstream provider-tool factories' default `{}` and the Go Gateway client must explicitly project `args: {}`; an HTTP document with missing or null args still fails the strict schema. Validate every non-nil `json.RawMessage` value instead of silently dropping malformed data. This is a parity-preserving Go adaptation, not a change to the registered V4 type. A small provider-domain validation operation shared by direct entry points is preferable to importing the Gateway schema; integrate with existing conversion entry points rather than a global protocol-validation framework. Structurally valid but unsupported provider IDs retain native unsupported-feature behavior, not a new Gateway registry.

Alternative rejected: treating provider tools as functions or moving provider options into args. Either changes the contract and may execute a hosted tool locally.

For Anthropic-hosted MCP, accept only the registered root `providerOptions.anthropic.mcpServers` member on a direct Anthropic route. Map each server's registered `type: 'url'`, name, URL, optional authorizationToken and toolConfiguration to the selected provider, preserving order, optional empty collections and explicit disabled flags. Reject other nonempty root options or namespaces until WP21. Keep the request-level shape the pinned client emits; do not invent a Gateway server alias or require Gateway configuration of remote servers. Validate the narrow option before catalog resolution; enforce route eligibility after resolution but before physical invocation through a service-owned wrapper composed around the lower model while its configured provider type and fallback topology are known. This wrapper uses a fixed construction-time capability bit, not a runtime `Provider()` comparison or a public catalog/discovery field. A public resolved model reports `grafana` after identity middleware, so the protocol handler must not infer physical type from `ResolvedModel.Model.Provider()`. Fallback routes reject nonempty MCP configuration even without definitions. All unsupported-route errors use the same safe category without disclosing backend identity. Empty `providerOptions` / empty namespaces normalize as no-op.

The existing Apache Anthropic `MCPServer.AuthorizationToken string` and `MCPToolConfiguration.Enabled bool` collapse omitted and explicit empty/false. The pinned Anthropic option schema accepts optional token and enabled, and its request projection omits absent members. Change these Go fields to `*string` and `*bool`, respectively; keep `AllowedTools []string` so nil and explicitly empty remain distinguishable. The native encoder forwards only present token/enabled fields and preserves explicitly empty values. This is a source-breaking Go API correction for the supported MCP option; test and publish it as a new immutable Anthropic prerequisite, then repin the Gateway before AGPL behavior lands.

`anthropic-sdk-go@v1.61.0` also estimates that 128,000 default output tokens require more than its ten-minute non-streaming limit and rejects unary calls before HTTP unless a request timeout is supplied. The pinned TypeScript Anthropic client emits that same default without the SDK's preflight rejection. In Apache `DoGenerate`, derive a request timeout from a future context deadline before the SDK call; append that default before explicit `WithRequestOptions` so caller-configured timeouts retain precedence. A past deadline returns `context.DeadlineExceeded` before I/O. Without any deadline or explicit timeout, retain the existing SDK guard. The Gateway already invokes models under `ProviderWire.ModelDuration`, so this makes default unary MCP calls usable within that bound without weakening cancellation or inventing a new model setting. Publish a further immutable Anthropic prerequisite and repin the isolated Gateway. This is not a change to `DoStream`, max-token semantics, or the external HTTP dialect.

Because the Anthropic API, rather than this Gateway, initiates the MCP connection, Gateway HTTP SSRF guards alone cannot attest to its outbound network policy. Authenticate callers first; apply explicit product restrictions to accepted URLs (e.g. HTTPS and disallowing URL-embedded credentials) and bounded server/field sizes, and use configured provider transports so secrets are sent only to the selected Anthropic API. This validation is a deliberate Grafana host-policy restriction, not a claim about Vercel's private server; if it blocks a required registered-client scenario, resolve the policy before enabling that scenario rather than silently loosening it. Never copy MCP URL or token to public errors, responses, logs, metrics or metadata-only AO. The client-owned request body is caller-visible by existing contract, so credential-bearing requests need clear operator guidance and non-capture tests.

### 2. Normalize only semantically optional domain markers

Use `bool` for `Preliminary` in `provider.StreamPart` and `GenerateContentPart`, and for `GenerateContentPart.Dynamic`; keep existing `ProviderExecuted bool`. Retain `StreamPart.Dynamic *bool`: the flat stream union includes input-start, whose absent and false values are operationally different. No extra presence flag or separate public stream type is needed. For call/result arms, absence and false may normalize where equivalent; function `Strict *bool` remains presence-aware.

Preserve input-start dynamic presence through native provider output, private Gateway DTOs, SSE encoding and Go client decoding. Omit an absent value; emit and retain explicit false. At text-stream input-start, match pinned `stream-language-model-call.ts`: an explicit value wins, and only absence falls back to the application tool definition. At UI conversion, separately match pinned `ui-message-stream/to-ui-message-chunk.ts`: known dynamic tools yield true even when the text part says false; known ordinary tools omit dynamic even when their text part says false; unknown tools preserve the text part's dynamic value. Current Go `isDynamic` already applies the latter UI policy on tool-call conversion but not input-start. Use the existing internal UI override pattern at input-start without changing unrelated public APIs. Cross-language tests must compare actual text-stream parts and assembled UI chunks for absent, false and true rather than claiming both layers share one boolean.

V4 output places `providerExecuted` on calls/input-start, `dynamic` on calls/input-start/results, and `preliminary` on results. A V4 result is provider output and has no registered `providerExecuted` member. Do not serialize the Go-only result marker; derive any internal ownership from the call/result context without inventing a wire field. Provider-defined does not imply provider-executed: computer and other native tools may still require client execution.

### 3. Extend the existing stateless mapper and bounded encoders

Accept provider-executed assistant calls and the registered assistant-side basic result arms in addition to WP11's tool-role results. Preserve selected empty values, error arms and alias/correlation IDs. Tool-call/result part `providerOptions` needed for continuation belong to this capability; validate their namespace objects and keep host-owned namespaces from reaching providers. Nested output/content options and approvals remain WP14 work.

Unary content adds explicit tool-result DTOs with non-null JSON result, `isError`, `dynamic`, `preliminary` and reviewed metadata; calls preserve execution/dynamic flags. Retain ordered content and precommit failure for invalid or unsupported output. Account for result and metadata bytes/cardinality before parsing/encoding, followed by the existing exact final-byte check.

Result correlation covers both calls emitted in the current response and eligible unresolved provider-executed assistant calls from the supplied request history. Reconstruct the latter for each request from call ID, name and ownership, excluding calls with a completed result. A historical client-owned call is not eligible for deferred provider-result matching. Do not require the provider to repeat a historical call before its success or error result. Apply the same correlation scope to unary output and streaming; unknown IDs, name mismatches and already-completed IDs remain invalid.

Streaming extends per-ID state instead of adding a parallel state machine. Preserve input-start/delta/end, standalone calls and independent interleaving. Both client-owned and provider-owned complete calls may reach finish without a result; a provider-owned call can complete in a later request. Once a preliminary series starts, multiple previews are permitted before exactly one final result; reject results after final and finish with an unfinished preliminary series. Preliminary results do not close the call. Dynamic names need not appear in the request tool list. Retain the same full-frame limit, safe error behavior, authoritative finish, cancellation and bounded drain.

History-derived state is bounded by the already bounded request body; state added by current provider output is bounded by the stream-part count (or unary content budget). Retain only correlation/lifecycle data, not copied history payloads or result sequences, and discard it at request completion. A history entry is not a consumed provider stream part. This supports deferred execution without a server session store, persistent pending-call registry or Gateway executor.

Alternative rejected: using provider JSON marshalers as HTTP authority or accumulating all deltas/results. Private DTOs and bounded incremental state already enforce the product boundary.

### 4. Separate protocol metadata from physical attribution

Tool metadata can be necessary for provider continuation, while arbitrary provider metadata cannot become a public escape hatch. Add a small explicit projection for reviewed tool metadata keys and shapes, with matching round-trip tests. Static protocol namespaces such as `anthropic` or `openai` are not configured provider instance/backend identity.

Before enabling each metadata shape, enumerate its exact pinned producer, replay consumer, field validation and privacy rationale in tests. Native examples include Anthropic tool type/caller correlation and OpenAI item correlation. For supported Anthropic MCP calls/results, allowlist `anthropic: {type:'mcp-tool-use', serverName:<bounded string>}` only when the name matches a caller-supplied server configured for this request; emit that name on both calls and results so registered continuation can reconstruct native blocks. It is a caller-chosen identifier exposed back to the authenticated caller, not the server URL, credential, provider instance or backend model identity. Unconfigured/malformed MCP metadata fails safely rather than being relabeled as a normal tool. Never forward arbitrary metadata members or a provider-supplied server name unverified. Unknown/private metadata is omitted; malformed necessary supported metadata fails bounded encoding. Raw response/request material and result-level private physical identity remain excluded.

The WP13 tool-part metadata inventory (subject to final exact-field tests) is:

| Native producer | Continuation consumer | Reviewed projection and boundary |
| --- | --- | --- |
| Anthropic MCP `mcp_tool_use`/`mcp_tool_result` (`anthropic-language-model.ts`, Go `convert_{response,stream}.go`) | `convert-to-anthropic-prompt.ts` / Go `convert_request.go` | `anthropic.type = 'mcp-tool-use'` and bounded `serverName` matching a server in the current request; never URL or token. The request-level root `mcpServers` option is needed for actual execution. |
| Anthropic function `tool_use` with code-execution caller | The next Anthropic assistant `tool_use` block | Closed caller `type: 'direct'` or registered code-execution version plus `toolId` only for the versioned case; preserve correlation but not arbitrary caller fields. |
| OpenAI Responses provider and function tool call/result (`openai-responses-language-model.ts`, Go `convert_{response,stream}.go`) | `convert-to-openai-responses-input.ts` / Go input conversion | For registered tool parts, `openai.itemId` (or the already supported `azure` namespace when selected), optional `namespace` and closed `caller` (`direct` or `program` with callerId) only where the pinned consumer uses them. Item IDs are provider item correlation, not backend model/instance IDs. Results need their own item ID where generated. |

Do not forward unrelated metadata (response metadata, reasoning encrypted content, tool approvals, source/file identifiers, raw request bodies) under this table; these belong to other capabilities. A necessary native MCP result shape such as a nested content arm that WP13 cannot encode still fails explicitly. Validate the selected provider namespace/shape and required bounded strings rather than copying whole namespaces. Exact producer/consumer tests decide which listed optional fields are actually needed for each supported provider-tool subtype.

Request tool-part options preserve opaque ordinary provider values subject to existing host namespace protection, except that the selected and validated Anthropic MCP `type/serverName` continuation fields are checked against the current request's MCP server definitions before dispatch. Public response metadata uses the reviewed projection. This is an intentional Gateway privacy boundary versus the permissive upstream Gateway, not a claim of arbitrary metadata passthrough. Do not add recursive redaction or a generic metadata registry.

Alternative rejected: dropping all metadata breaks replay, while forwarding whole namespaces leaks private details. The allowlisted fields and any unsupported compound scenarios must be explicit before acceptance.

### 5. Keep client and observability implementations independent

Extend `providers/grafana` readers without importing server DTOs/validators. Keep client-owned unary request/response/warnings replacement, SSE bounds, clean EOF, cancellation and error behavior. Differential tests compare normalized disabled booleans only where equivalent; input-start dynamic absence and explicit false must remain distinct. Result-only continuation is consumed without requiring a repeated call in the current response.

Reusable Agent Observability mapping must account for provider-owned calls and preliminary-to-final replacement without counting previews as extra completed tools. Gateway composition continues one metadata-only logical generation per HTTP request, not per tool event. Tool payloads, names, IDs, schemas, args, metadata and physical identity stay out of exported logs, metrics and metadata-only AO. Do not add tool names/IDs as metric labels.

Fallback-configured routes continue rejecting definitions, tool history, nonautomatic choices and nonempty MCP server configuration before any candidate runs. MCP can have effects without a `tools` definition, so host policy cannot treat `mcpServers` as an innocuous text-only option. No new replay/idempotency policy is implied by accepting provider tools on direct routes.

### 6. Layer the evidence and preserve provenance

- Capture real pinned-client provider definitions, request-level Anthropic `mcpServers` (including optional token, allowed tools and empty/false values), marker/continuation semantics and forbidden-field cases; replay through production Go. Keep the existing full request schema authoritative rather than regenerate it from goldens.
- Differential unary/SSE and real-handler tests cover true/false/absent, including separate text-stream inference and known/unknown UI projection of input-start dynamic, dynamic undeclared names, error results, preliminary/final order, metadata and zero local executions for hosted calls. Include a provider-defined client-executed control and a call → finish → next request with history → result-only success/error response. Assert unknown IDs, mismatched names, completed historical IDs and client-owned historical calls cannot satisfy deferred provider-result correlation.
- Authenticated real-command tests use deterministic Anthropic code-execution and MCP call/result responses within WP13: verify native definitions, aliases, request-level server/token forwarding to the fake Anthropic API only, tool metadata, and second-call assistant history. Assert no MCP URL/token in server output or telemetry; deny unconfigured/mismatched servers and malformed options without backend execution, and reject MCP configuration alone on fallback routes. Provider tests separately cover web-search native conversion without pretending its deferred source output is already supported by the Gateway.
- Reuse provenance-valid Anthropic/OpenAI fixtures and request expectations for changed native behavior. Add a failing fixture/replay first where those inputs express the change. Use focused synthetic unit tests for unsupported-field, transport, metadata/privacy and lifecycle cases; never place invented input under recorded/upstream fixtures.
- Cover exact/over-limit result and metadata bytes, invalid JSON/null results, malformed markers, preliminary floods, cancellation while input/results are active, terminal cleanup, and zero-invocation fallback rejection.
- Run provider shape/baseline checks, conformance, cross-language integration, Gateway contract/command tests and module-boundary checks. Update stable parity status only from demonstrated evidence.

## Risks / Trade-offs

- [Boolean field changes break separately versioned consumers] → Update all compiled consumers, retain stream dynamic presence, align the input-start inference exception with pinned upstream, publish Apache prerequisites, and validate released-root as well as workspace resolution.
- [Current-stream-only correlation rejects deferred results, while unbounded history tracking retains payloads] → Reconstruct unresolved provider-call identity from bounded request history, track only request-local lifecycle state, and test result-only completion plus invalid historical matches.
- [Native MCP option scalars collapse meaningful presence] → Use optional pointers for token/enabled, assert native request bytes for absent, empty/false and populated values, and publish a corrected Anthropic prerequisite.
- [Go SDK rejects large-default unary requests before native I/O] → Derive its per-attempt timeout from an already bounded caller context; keep explicit timeout precedence and test no-deadline and expired-deadline behavior. A direct Go caller with no deadline retains the SDK guard.
- [MCP continuation needs a server name visible to clients] → Restrict to a bounded caller-supplied name validated against request-level server definitions, expose only the name/type on matching tool parts, and test absence of URL/token/backend identity in public and operational output.
- [Request-level MCP URLs and credentials are sensitive] → Authenticate, validate the narrow option and target provider, bound input, isolate provider forwarding, test secret leakage; document that Vercel-hosted server policy is unknown and that Anthropic controls the remote connection. Maintain product egress review before deployment.
- [Other metadata needed for replay can expose deployment identity] → Explicit per-field projection and hostile-marker tests; stop for review if a required shape cannot satisfy privacy instead of widening passthrough.
- [Native tools emit later-package content] → Keep failures explicit, use a WP13-only acceptance flow, and document remaining compound-tool support boundaries.
- [Preliminary results exhaust memory or appear final] → Retain only bounded lifecycle state in the adapter, permit repeated previews, require final closure and test consumer finalization.
- [Provider-defined is confused with provider-executed] → Exercise both ownership modes and assert no local execution for true, with no fabricated result wire marker.
- [Synthetic evidence is overstated] → Separate Gateway determinism, native request snapshots and authentic provider fixture provenance in the work summary.

## Migration Plan

1. Land Apache domain/consumer/provider/client changes as ordered prerequisite commits with focused tests. If native MCP option presence needs correction, publish a further immutable Anthropic prerequisite before dependent Gateway code. No compatibility shim for fields converted to bool, presence-aware MCP scalars or forbidden provider definition fields; `StreamPart.Dynamic *bool` remains intentional.
2. Publish immutable prerequisite refs and pin the Gateway's root/provider/middleware dependencies to proxy-resolvable versions. No committed replacements or root workspace registration.
3. Enable the AGPL mapping/output capability with its exact-pinned contract tests. Validate against committed pins with `GOWORK=off` before merge.
4. Extend operator support documentation and rollout smoke if WP13 is activated in a deployed image; preserve notices, source revision and corresponding-source requirements. Otherwise record deployment activation as unverified, not completed.
5. Roll back by deploying the prior image/module set; prior runtimes reject provider tools explicitly. No persisted state migration is needed.

## Open Questions

- The exact non-MCP metadata allowlist is an implementation evidence gate: inventory registered producer/replay pairs before writing its mapper. MCP `type/serverName` is approved only in the narrowly validated, caller-configured case. Any other unavoidable conflict with private physical attribution requires owner review, not silent loss or blanket passthrough.
- Before deployed activation, confirm Grafana policy for caller-supplied MCP destinations and tokens and Anthropic's remote egress constraints. Public Vercel client source does not establish its hosted service policy.
- Native providers and fixtures cover only selected tool families. Record which compound flows require WP14–WP19/WP21; do not expand those work packages under this issue.
