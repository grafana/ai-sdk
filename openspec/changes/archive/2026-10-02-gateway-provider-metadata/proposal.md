## Why

Gateway response projection currently drops text/tool/result/finish metadata and filters reasoning/source metadata, losing native continuation values before either client can reuse them. Issue [#280](https://github.com/grafana/ai-sdk/issues/280) delivers ordinary bounded provider metadata and actual response-derived continuation on the native-option and mapped-fallback stack, as required by `~/ai-sdk-ai-gateway-plan.md`.

## What Changes

- Preserve opaque object-valued provider namespaces and nested JSON at every registered metadata position in currently supported unary and streaming output; remove namespace/key inventories from the service and independent Go client.
- Retain omitted versus explicit empty metadata, nested null/false/zero/empty values, original stream placement, and latest non-nullish object replacement rather than deep merge.
- Use one bounded AGPL metadata transport path across supported response DTOs and independent Apache decoding. Account for aggregate original bytes and cardinality before parsing, validate JSON/UTF-8, and enforce complete unary/SSE bounds without silent omission or partial success.
- Prove that each client's real first output assembles into a subsequent native Anthropic/OpenAI/compatible request on direct and configured-fallback paths. Fix only demonstrated core/UI metadata presence or assembly gaps, with pinned frontend evidence.
- Keep caller responses independent from operator capture and consumer middleware configuration. Preserve specific credential/tenant protections without censoring token-shaped application values or trusting returned metadata for authorization.
- **BREAKING**: Replace the closed reasoning/source metadata projection and suppression contract with native opaque metadata. Source metadata no longer becomes a synthetic `citation` namespace; malformed/oversized metadata fails explicitly instead of being selectively discarded.

## Capabilities

### New Capabilities

- `gateway-provider-metadata`: Shared bounded opaque metadata semantics, supported scope inventory, two-client actual continuation evidence, and observer/authorization separation.

### Modified Capabilities

- `providerwire-v4-unary-runtime`: Preserve result/content metadata within complete precommit response bounds.
- `providerwire-v4-streaming-runtime`: Preserve metadata on registered content events and finish without altering commitment, lifecycle or ownership.
- `grafana-gateway-client`: Independently retain supported unary/stream metadata instead of adopting server namespace/key filters; keep Gateway-hop request/response replacement.
- `gateway-reasoning-content`: Replace the continuation-key allowlist with opaque metadata and retain replacement semantics.
- `gateway-sources`: Replace numeric-only metadata projection with native namespaces and aggregate bounds; leave scalar identity/display behavior to #320.
- `gateway-unary-function-tools`: Preserve supported call metadata without expanding execution ownership or the output union.
- `gateway-streaming-function-tools`: Preserve registered input/call/basic-result metadata at its original event position.
- `ui-message-conversion`: Preserve explicit empty metadata through affected UI chunks, persisted parts and model-message conversion, with frontend assembly proof.

## Impact

- Server: `ai-gateway/providerwire/v4/` response, stream, reasoning, source and tool DTOs/preflight, output schemas, handler tests and `ai-gateway/test/providerwire-v4/` pinned contract/command tests.
- Client: `providers/grafana/` unary/SSE decoders and independent tests; narrowly evidenced root SDK/UI serialization or assembly fixes and `test/integration/` scenarios.
- Evidence/docs: Existing authentic conformance inputs and direct expectations remain authoritative; update stable coverage boundaries and relevant centralized guides. No baseline upgrade or new external dependency is planned.
- Stack: This branch starts at `44968b44`, the current `nrbrd/gw-fallback` tip (PR #328), above #318/PR #326. Those implementations are present even though their issues remain open. Do not resume or extract #303/#309 stopped code.
- Reference: `test/conformance/upstream.yaml` pins ai `7.0.116`, Gateway `4.0.94`, Provider `4.0.18`, Anthropic `4.0.65`, OpenAI `4.0.78`, compatible `3.0.57`, React `4.0.119`, commit `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. Local upstream HEAD differs; reference source is read explicitly at that commit.
- Non-goals: #320 scalar/warning/source-display/response-identity fixes; #321–#324 evidence, attempts, failures, native debugging or discovery; #316 routing; #317 BYOK/capture controls; #107/#238–#240 missing tool/MCP codecs; #116 raw events; unrelated provider fixes, a second codec, core termination redesign, or Vercel private-service parity. #201 full-matrix proof remains separate.
