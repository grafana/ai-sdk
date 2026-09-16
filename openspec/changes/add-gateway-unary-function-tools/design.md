## Context

Baseline integration merge `c055bec` combines exact WP8 `a2177d9` with WP7 `e0e6c01`. Nara's plan/review-guide/product-design at bundle revision `e8f443d88763e2b96f615c2f3a9ff348accfce2d` and issue #105 govern scope. The Road to ObsCon classifies WP11/12 as should-scope; planning them does not activate production.

Registered upstream is `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`: ai 7.0.65, gateway 4.0.52, provider 4.0.7, anthropic 4.0.38. Relevant source: `packages/provider/src/language-model/v4/language-model-v4-function-tool.ts`, prompt/tool-result types; `packages/gateway/src/gateway-language-model.ts`; `packages/anthropic/src/anthropic-prepare-tools.ts`; `packages/ai/src/generate-text/generate-text.ts`. The Gateway serializes call options and replaces unary warnings/request/response; ai orchestrates execution outside the transport.

Current server `request.go` rejects non-empty Tools/ToolChoice and tool history, while the full request schema already describes those arms. `response.go` permits only text. The Go client's `projectTool`, `projectOutput` and role mapper already cover broad tool requests; `provider.Tool.Strict` is already *bool. Reuse and test these surfaces. WP8 `NewModelObservabilityFactory` wraps once, and `filterAgentGeneration` strips ToolChoice and private metadata; its process client is metadata-only and hooks-disabled.

## Goals / Non-Goals

**Goals:** Unary function calls and selected results through both clients, native provider evidence, bounded strict mapping, canonical one-call observation.

**Non-Goals:** Streaming tools (WP12), provider tools/execution and dynamic/preliminary behavior (WP13), approvals/denial and nested result options (WP14), file/custom result content (WP15/18), root provider options/headers (WP21), raw output, tool execution in the Gateway, and effectful fallback.

## Decisions

### Supported slice and mode gate

Support function definitions with name, optional description, object inputSchema, object input examples, *bool strict, and namespaced function-tool providerOptions. Support auto/none/required/named tool choice. Support assistant tool-call history with JSON input and tool-role results with text, json, error-text, error-json, and content containing only text (including empty arrays/text). JSON null is a valid selected JSON result; do not confuse it with absent value. Execution-denied, approvals, file/custom content and non-empty nested result/part options remain explicitly unsupported under their owning packages. Function-tool options belong here, not WP21; consume only provider namespaces, rejecting host-owned gateway/grafana controls rather than forwarding them.

Use private typed DTOs after complete schema validation. Thread execution mode into mapping so these arms execute only in unary until WP12. Preserve role and inactive-arm validation; never use provider marshalers as HTTP authority. Avoid adding presence wrappers where existing semantic normalization suffices. Existing *bool Strict already preserves absent/false/true.

### Output and provider work

Emit ordered text and client-executed function tool calls through private unary DTOs. Tool-call output input is a string, whereas assistant prompt input is JSON; preserve this distinction instead of double encoding. Reject provider output with providerExecuted true or dynamic true using the existing fixed internal-error document before HTTP 200. False/absence may normalize to disabled, but never remove an enabled marker and forward the call: that changes execution ownership and could cause duplicate execution by the client. Provider-tool result output and enabled preliminary behavior likewise remain outside the unary text/function-call union. Strip arbitrary metadata and raw provider details only after supported-union checks. Add all new IDs/names/input bytes to overflow-safe preflight before scanning/encoding, then enforce the final encoded limit. Keep warnings/request/response replacement unchanged.

Use existing Anthropic native conversion and deterministic request snapshots to prove schema, examples, strict false and named choice; inspect existing Bedrock/OpenAI conversion tests for any shared-domain change and touch only affected converters. Upstream Anthropic explicitly distinguishes strict false from absent and flattens inputExamples to native input_examples. No new provider factory belongs here. A broad converter rewrite has no current consumer.

### Client and logical observation

Extend the existing WP7 codecs only where differential tests show a gap; no secondary client. A two-call unary round trip receives a tool call, executes a deterministic local function in the test application, sends assistant call plus tool result, then receives final text. No server session state or executable function crosses HTTP.

Extend reusable normalized observation only as needed. Gateway retains metadata-only capture: definitions, schemas, tool names/IDs/inputs/results and options do not become metric labels, logs or exported payload. Verify normalized tool-call finish and mode, usage, timing, safe error state, and one generation per HTTP model call. Reusable SDK recording can preserve its established content-aware behavior for other hosts; Gateway's filter and exporter policy remain authoritative. Full workflow parentage is not inferred from tool IDs or untrusted headers.

### WP9 coexistence and source references

Direct-only routes support tools; fallback-configured routes reject non-empty definitions, tool choice or tool history before first candidate. Do not silently bypass fallback by choosing primary. The guard is Gateway-owned and uses a closed safe unsupported error; no topology leaks. WP11 can land without WP9, but composition tests must cover both when integrated.

PR #177 is a reference for the server/Vercel slice, not accepted authority or a substitute for Go-client and observability evidence. Split any useful work by WP11 then WP12. Do not copy its broader claims or combined milestone completion.

## Risks / Trade-offs

- Broad schema versus narrow runtime → keep valid unsupported arms distinct from malformed requests and name the closed unsupported family.
- Tool data carries arbitrary secrets → explicit DTOs, metadata-only exported evidence and hostile markers in tests.
- Provider warning differences → preserve the existing privacy normalization and document deviations in PARITY, not raw warning passthrough.
- Mixed modules → Apache prerequisite commits precede Gateway; actual immutable proxy-resolvable pins are required before merge checks. Local overlay is diagnostic only.

## Migration Plan

Implement Apache deltas first, then pin them in isolated Gateway with GOWORK=off and no committed replace. Enable unary direct-route handling and tests; WP12 follows as a separate change. If already enabled internally when implemented, extend that existing deployment smoke; do not invent deployment assets before WP10. Rollback returns the mapper to explicit tools-unsupported behavior.

## Open Questions

No product decision blocks apply. Accepted prerequisite heads must be checked before implementation. Actual production activation remains a separately authorized deployment step.
