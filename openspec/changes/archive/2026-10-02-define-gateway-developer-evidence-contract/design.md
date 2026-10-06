## Context

This change implements the **decision and access probes** in [#321](https://github.com/grafana/ai-sdk/issues/321), using the revised plan's work package 34. It does not implement the contract's production transport. The owner approves the resulting decision before #322–#324 proceed.

The clean starting revision is `c0299776`, matching `origin/main` during planning. The registered upstream is `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`: `ai@7.0.116`, Gateway `4.0.94`, Provider `4.0.18`, Provider Utils `5.0.49`. Source was inspected at that commit, not the different HEAD of `~/src/ai`. Reconfirm the source, workspace pins and installed packages at execution time.

Relevant inspected surfaces:

- Pinned `packages/gateway/src/gateway-language-model.ts` and its tests: unary spreads the server body, then replaces `request`/`response`; streaming forwards permissively parsed events, converts timestamps and filters unrequested raw parts.
- Pinned `gateway-fetch-metadata.ts` and tests: discovery uses a normalized schema that strips additive fields, transforms pricing and filters unknown model types.
- Pinned `errors/{as-gateway-error,create-gateway-error,extract-api-call-response,gateway-error}.ts`: top-level Gateway errors normalize fields and retain a public cause; normalization alone does not prove convenient evidence access.
- Local `providers/grafana/{model,stream,errors,discovery}.go`: Go replaces unary transport, drops top-level metadata today, reconstructs a limited error body and returns only normalized discovery fields. Existing tests are the starting pattern, not permission to change codecs for a probe.
- `provider/api_call_error.go`, `fallback/fallback.go`, existing client/catalog specs and `test/conformance/PARITY.md`: available fields, selection hooks, bounds, retained deviations and ownership constraints.

Current Vercel routing documentation is a **service product reference**, separate from pinned-client execution evidence. The linked [provider routing](https://vercel.com/docs/ai-gateway/models-and-providers/provider-options) and [filtering/ordering](https://vercel.com/docs/ai-gateway/models-and-providers/provider-filtering-and-ordering) pages describe routing controls, not proof of complete native transport passthrough. Concrete routing response-field semantics must be verified and source-linked in the decision; search summaries are not evidence. No private-service oracle is available.

## Goals / Non-Goals

**Goals:**

- Prove precisely what low-level clients, high-level normalization and separately configured consumer middleware can inspect today.
- Select one concrete schema/access contract, with justified numeric bounds, provenance, source-specific exclusions and honest missing-producer dispositions.
- Produce `decision.md` beside these artifacts, linking focused tests and recording explicit owner approval and an acyclic implementation handoff.

**Non-Goals:**

- No production server/client codec, metadata mapper, public API, redactor, collector, stream reader, execution/retry policy or routing implementation.
- No BYOK selection/storage/capture change (#317), raw-stream activation (#116), provider/core error representation or termination change (#299), new baseline or full conformance harness.
- No extraction/resumption of stopped #303/#309 helpers, collectors or proposals; no retrospective claim that old backend concealment is the desired product contract.

## Decisions

### 1. Probe the existing clients before selecting a carrier

Use injected `fetch` responses in the existing AGPL TS workspace and fake HTTP/RoundTripper responses in Apache Go client tests. Keep payloads small and synthetic, clearly labeled as access witnesses. Do not add them to provider `recorded/` or `upstream/` directories. Reuse current test helpers rather than introduce a shared runner or new workspace.

For each case, record the original injected shape, an executable public access expression, observed normalized values and discarded fields. A test that observes field loss is valid evidence; changing a codec merely to make the field visible is outside this issue.

| Case | Minimum evidence |
| --- | --- |
| Unary success | Conflicting requested/canonical/selected/native identities; metadata and nested native request/response versus client-owned Gateway request/headers/body |
| Stream setup success/failure | Available initial headers/request, rejected setup envelope and public error/cause access |
| Stream success | Response metadata and finish metadata; low-level ordered parts versus assembled high-level result |
| Committed stream error | Attributed structured error followed by valid content/finish; record normalization/termination differences without fixing them |
| Direct and all-failed errors | Heterogeneous candidate evidence, native status/retry fields and Gateway status/client retryability, plus malformed envelopes |
| Discovery | Canonical/alias rows, additive candidate/order/provider-model facts, stock normalized losses and an explicit authenticated non-inference access alternative |
| Consumer observation | Existing configured middleware around each client observes available values independently of operator capture; error paths included |

Use real `generateText`/`streamText` normalization for TS. Go `GenerateText` collects `StreamText`, so a direct `DoGenerate` probe does not prove Go high-level access; test that actual streaming path and its consumer wrapper. Use existing middleware APIs and in-memory destinations, not a second observer framework. Where no stock middleware exposes a field, record the limitation instead of inventing a production adapter.

Raw HTTP/injected-body acceptance is insufficient. A test-only bounded access alternative can demonstrate feasibility, but must be labeled **proposed extension**, not stock-client support or a delivered public API. Do not assume private error trees: probe explicit public `GatewayError.cause` and `APICallError` fields with registered guards, and Go `errors.As`/`errors.Is`.

### 2. Resolve placement with a compact access matrix

Prefer registered metadata/error/discovery shapes when they survive the relevant clients. Compare the following candidates and select one concrete outcome per surface in `decision.md`:

| Surface | Preferred candidate to probe | Alternative/constraint |
| --- | --- | --- |
| Successful selection/attempt facts | Result and finish `providerMetadata.gateway` routing fields compatible with verified documented Vercel meanings | Go metadata transport is currently a gap; #280 owns its general codec |
| Native successful request/response diagnostics | A separately identified Gateway-owned evidence subtree in metadata | Nested unary transport remains inspectable inside low-level Gateway response body, but is not normalized native `Request`/`Response` |
| Direct/setup/all-failed errors | Existing Gateway error envelope with attributed additive evidence and public API-call data/body access | TS top-level normalization and Go reconstructed bodies may require a minimal explicitly approved access addition |
| Committed errors | Registered `type: error` payload plus attributed native/Gateway facts | Relevant high-level normalization may discard fields or terminate; coordinate #299 without adopting a new error type/policy |
| Configured discovery | Additive facts on the existing authenticated `/config` response | Stock normalized discovery drops extras; evaluate an explicit bounded access helper/minimal API, not a second endpoint/dialect or overloaded display strings |

The final decision must state exact paths, value shapes, event placement, replacement/merge behavior, low/high-level access and consumer middleware access. It must identify every proposed public addition, its implementing issue, and any stock-client limitation. No error-tree/sentinel protocol, new namespace, helper or API is approved merely by appearing in this comparison.

### 3. Preserve distinct facts and provenance

The decision will define separately:

- Requested ID/alias and resolved canonical public route, sourced from request/catalog resolution.
- Configured candidate provider/model and actual selected candidate, sourced from execution seams, not a canonical wrapper or joined-error prose.
- Native provider-reported response ID/model/timestamp, sourced only from returned producer values.
- Actual candidate invocation versus intent to fall through, selection at accepted unary result/first stream part versus eventual generation outcome. `fallback.AttemptSelected` is not proof of stream completion.
- Native retryability/status/type/code/details, fallback eligibility/intent, actual subsequent invocation, Gateway HTTP/category/retryability and consumer/core retries. Unknown native SDK retry counts remain unknown.
- Post-generation adaptation failure and replay/paid-generation risk; no invented exactly-once guarantee.

Define collision behavior when native opaque metadata already contains the chosen service namespace/path. Neither silently overwrite native values nor let them masquerade as trusted service attribution. Review whether separate provenance placement is necessary, and show a collision witness before approving it. Ordinary metadata remains opaque within its valid JSON/resource boundary; no temporary key allowlist or secret-looking string scan.

### 4. Decide credential sources and budgets together

Inventory producer availability from current result/stream/error surfaces for Anthropic, Vertex, Bedrock, OpenAI Responses and compatible adapters. Native request headers or bodies absent from those surfaces remain unavailable; no new capture seam is implemented here.

The decision must enumerate exclusions by actual source: Gateway access/acting-user/CAP credentials; provider API keys; OAuth/workload tokens; AWS authorization/signing/session material; cookies; credential-bearing URL components; credential references; and known BYOK subtrees. Verify exact native fields/header casing and distinguish credential material from authorized topology/resource identifiers. Protect known credential fields without censoring token-looking prompts, outputs or ordinary diagnostic strings. If a source cannot be safely projected, give it an explicit disposition rather than ban all native transport.

Define absent, unavailable, redacted, malformed and over-limit semantics per optional evidence component, including valid empty values and a bounded reason. Decide whether partially redacted evidence remains available with an explicit redaction marker. Optional evidence may become explicitly unavailable; supported content and ordinary native metadata must not be truncated, selectively dropped or displaced to make room. Never byte-truncate JSON.

Choose and justify numeric caps for source input scanned/copied, per-component and aggregate retained/encoded evidence, attempts, headers, discovery rows/candidates and complete unary/error/SSE/discovery documents. Account for escaping, wrapper/framing overhead, immutable copies, count limits and worst-case work. Existing Go defaults (64 KiB errors, 1 MiB events, 16 MiB unary, 4 MiB discovery) are compatibility constraints, not automatically approved diagnostic budgets. Exercise small boundary witnesses and record final constants/dispositions; use measurements and available headroom instead of arbitrary new limits. Required envelope/core data and optional diagnostics have explicit separate failure policies.

Operator metadata-only capture and consumer capture/destinations remain independent. Returning evidence must not require server capture. The pinned client's own request body includes submitted `gateway.byok`; a server cannot sanitize that caller-owned object retroactively. Secret-aware BYOK capture belongs to #317 and must be called out, not falsely claimed solved by server exclusions.

### 5. One owner per seam and an explicit approval gate

The final decision resolves conditional edges, rather than deferring them until multiple implementations exist:

| Owner | Sole responsibility / dependency |
| --- | --- |
| #321 | Access probes, reviewed representation/budget/credential decision and owner approval; no production dependency on the downstream issues |
| #280 | General bounded opaque metadata server transport and independent Go decoding; consumes #318, with #319 only for mapped-fallback continuation proof; does not define evidence fields |
| #322 | Request-scoped candidate attribution and attempt/error projection using existing fallback seams; consumes #321/#319 and #280 only if its chosen carrier needs it |
| #323 | Source-aware native transport projection; consumes #321, #280 if metadata-carried, and #322 for attributed per-attempt evidence; does not duplicate attempt/error capture |
| #324 | Authorized configured `/config` projection and approved discovery access; consumes #321, not runtime diagnostics/continuation |
| #316 | New request routing, reusing #322 execution evidence and #324 configured vocabulary/access; no competing codec/collector |

Any approved shared Apache accessor/client-retention addition receives exactly one owner in this map; dependents reuse it. #318/#319/#320/#280 may run alongside the decision. #299 producer/core investigation and #317 credential-aware capture remain coordinated separate scopes, not blanket closure dependencies. No issue waits for a consumer that in turn requires its completion; #303 closure is not a gate.

## Risks / Trade-offs

- [A permissive TS parser accepts unsupported fields] → Require an actual low/high-level access witness and distinguish stock behavior, proposed extension and Go implementation gap.
- [A stock client drops evidence needed by the product] → Surface the exact limitation and seek approval of the smallest access addition; do not hide useful data or silently invent another dialect.
- [Synthetic responses suggest unavailable native fields exist] → Inventory producer sources and label absent fields; access probes prove consumption only, not live producer availability.
- [Metadata namespace collision or error aggregation corrupts attribution] → Test distinct identities/collision shapes; approve provenance and actual-attempt ownership before transport work.
- [Bounds/redaction become a framework or change normal output] → Keep #321 to focused witnesses and a policy table; production mechanisms remain downstream, with explicit optional-evidence dispositions.
- [Old specs still contain concealment requirements] → Identify the precise downstream requirements that need revision; do not rewrite production specs in a design-only delivery.

## Migration Plan

No deployment or rollback is needed: implementation adds only tests and the reviewed decision. Wire/client feature migrations, exported API changes and relevant existing-spec updates are owned by the downstream issue that implements the approved contract. Run focused probes, registered baseline/ProviderWire/parity checks and relevant standalone module checks before presenting the decision for owner approval.

## Open Questions

These are the outputs to resolve during #321, not decisions deferred to #322–#324:

- Which concrete paths survive each normalization layer, and what minimal access additions, if any, does the owner approve?
- Which current documented Vercel routing response fields match actual local facts without fabricating private-service semantics?
- What collision/provenance rule preserves opaque native metadata and service authority at the selected placement?
- Which source components are available and safely projectable, and what justified numeric budgets/dispositions fit the complete transport limits?

`decision.md` must resolve these questions or explicitly identify a producer/support limitation with an approved disposition. Proposal readiness is not issue completion or owner approval of a future production contract.
