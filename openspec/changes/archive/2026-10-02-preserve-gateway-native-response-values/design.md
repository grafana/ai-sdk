## Context

This implements issue #320/work package 33, independently from canonical main. The revised plan explicitly supersedes stopped #303/#309 work and blanket response concealment. This proposal does not reuse their patches.

The baseline is `test/conformance/upstream.yaml`: ai 7.0.116, Gateway 4.0.94, Provider 4.0.18, OpenAI 4.0.78 and React 4.0.119 at `ee3169b3c4880e2abe4d0d7c781243bb81822ec4`. Planning inspected that exact commit using `git show` in `~/src/ai`; the checkout HEAD differs and was not used as the behavioral reference. Relevant source/tests:

- `packages/gateway/src/gateway-language-model.ts` and `.test.ts`: unary spreads server output, then replaces request/response and combines server warnings before local warnings; local warnings are currently empty. Streaming forwards server starts and converts timestamp strings to Date. Its permissive schemas are not server validation.
- `packages/provider/src/shared/v4/shared-v4-warning.ts`: four variants with required feature, setting/message or message; only unsupported/compatibility details are optional.
- `packages/provider/src/language-model/v4/language-model-v4-{source,response-metadata,generate-result}.ts`: source IDs are strings, document title is required, URL title/document filename and response identity fields are optional.
- `packages/openai/src/responses/openai-responses-language-model.ts`: unary/stream `file_path` sources use a generated source ID and the file ID as title/filename. Its matching tests cover both paths; the Gateway must preserve the adapter-produced source ID, not infer it from file metadata.
- `packages/ai/src/ui/process-ui-message-stream.ts`: source URL/document chunks append to message parts with unchanged sourceId/display; repeated IDs are not a mandate to deduplicate or namespace variants.

Local `response.go` drops warnings and response identity. `stream.go` substitutes warning strings and canonical model IDs. `sources.go` retains a map for sequential IDs, applies a 1024-byte ID cap and rewrites file-path display. The independent client already decodes warning/source fields, but `providers/grafana/stream.go` requires modelId and public route-ID syntax. Existing raw/schema/differential/command tests intentionally assert the old substitutions. `PARITY.md` classifies them as privacy adaptations; it must be corrected without claiming the separate metadata gap is solved.

Existing normative cross-capability clauses also need narrow reconciliation: gateway-ordered-text-fallback currently forbids normal ProviderWire backend identity and requires canonical stream identity/no unary response; providerwire-v4-http-contract incorrectly describes warning replacement; gateway-provider-configuration's private-identity headings must distinguish configuration/discovery/error/operator surfaces from normal registered response identity. Their deltas do not change eligibility, routing, attempts, discovery projection or secret protection. Requirement renames are limited to headings that otherwise contradict the revised contract.

This crosses Gateway/Apache-client/front-end evidence surfaces. Gateway implementation and API contract tests remain under `ai-gateway/`; the Apache client never imports its DTOs/validators. The actual candidate-source module/workspace/image policy is governed by current repository commands, not historical plan mechanics.

## Goals / Non-Goals

**Goals:**

- Restore represented native warnings, source IDs/display and registered response identity with explicit Go presence adaptations.
- Keep returned values separate from requested/canonical routing and metadata-only operator capture.
- Preserve existing lifecycle, safe failure, cancellation, work/byte limits and independent client semantics.
- Establish failing regression witnesses first, followed by raw/schema, both-client, command, consumer-capture and frontend proof.

**Non-Goals:**

- No opaque metadata mapper/helper, diagnostic carrier or native body/header transport. #280 owns metadata/continuation; #321/#323 own fuller access.
- No attempt/failure transport, discovery, options/fallback eligibility, routing/BYOK, missing tools/content activation, upstream upgrade or stream-lifecycle redesign.
- No promise of native unary typed response identity surviving pinned-client replacement, complete native output parity or live provider acceptance from synthetic tests.

## Decisions

### 1. Map registered warning unions once, explicitly

Use a small protocol-private warning mapping shared by unary and stream-start output, not provider JSON marshalers or a general metadata helper. Emit only the active variant fields. Required feature, setting and message fields remain present even when empty; unsupported/compatibility details are emitted when nonempty. Preserve order and multiplicity; nil unary warnings become `[]`.

`provider.Warning.Details` is a string: absent and explicitly empty details cannot be distinguished, so both map to omission and both decode to empty Go strings. Document this as a Go representation adaptation rather than inventing presence. Do not widen the reusable domain API just to preserve optional descriptive presence. Other variants do not emit details or unrelated fields. Unknown warning types fail through existing safe unary/stream adaptation paths.

Alternative: separate mappers could drift; provider marshalers would omit required empty fields; generic prose loses native semantics. None is appropriate. Ordinary warning strings, including backend names or token-looking application text, are not scanned/redacted heuristically. Authentication/signing values from known credential-bearing structures are not copied by these DTOs.

### 2. Preserve sources without identity state or metadata-driven display

Remove the source-key/public-ID map and its clone/allocation/state paths. Emit `SourceInfo.ID` unchanged, including the empty string allowed by the registered required-string contract. Equal IDs within or across variants remain equal and events remain ordered; no deduplication, hashing, renaming or URL fetching is added.

Keep required document title, nonempty Title precedence over legacy unary Text, and empty Title fallback to Text. Optional empty URL title/filename remain omitted because the Go string API normalizes absence/empty. File-path metadata must not control title/filename; remove that detection/display branch independently of the existing numeric metadata projection.

Remove the 1024-byte ID cap: it bounded retained identity-map keys, which no longer exist. The containing unary response or complete SSE frame bounds all emitted source strings, and content/provider-part cardinality bounds work. Test IDs longer than the old cap that fit the containing document. Retain existing bounded numeric source-metadata processing for now, explicitly classified as the #280 implementation gap, not a new privacy contract. No opaque metadata decoding/transport is added here.

Alternative: keeping a no-op identity map or introducing per-field caps preserves unnecessary work/arbitrary restrictions. Source identities need no catalog/public-route syntax.

### 3. Keep native identity at registered positions and routing separate

Add a private unary response DTO containing only optional `id`, `modelId` and timestamp; nil provider Response omits `response`, a present response with no representable identity emits `{}`. Nonempty Go identity strings are copied verbatim; empty optional Go strings are omitted because absence/explicit-empty presence is unrepresentable. Nonzero timestamps encode as validated UTC RFC3339Nano instants; zero means absent. Provider, headers, body and request diagnostics are not copied.

Stream metadata takes `part.ResponseID`, `part.ModelID` and `part.Timestamp`, not canonical route identity. All three fields are optional; an identity-free provider metadata part emits only its type. No metadata part is fabricated when the provider supplies none. Preserve current single-metadata/before-output placement. Canonical ID validation remains at resolution and canonical model wrappers continue to govern operator metrics, never response values.

The independent Go client accepts absent modelId and any valid represented string, including explicit empty wire identity; it stops applying `publicModelID` to returned metadata. Optional wire empty strings decode to the Go empty string. Keep strict timestamp parsing, document validation, SSE bounds and cancellation semantics. Wrong types/null are rejected on the strict Go path even where TS permissively accepts them; tests distinguish strict server correctness from permissive-client consumption.

Alternative: falling back to canonical ID fabricates identity; top-level custom fields or a providerMetadata diagnostic tunnel would cross #321/#323/#280 ownership.

### 4. Prove unary replacement rather than changing it

Both clients retain existing unary request/response transport replacement. Native identity is present at `rawBody.response` (and within the client-owned response body), but absent from typed response ID/model/timestamp after normalization. Pinned TS tests prove object replacement plus warning ordering using native-looking values distinct from route identity. At Gateway 4.0.94 local warnings are `[]`; test that registered behavior rather than inventing a client option that generates local warnings.

Consumer middleware sees warnings/source content, native streaming metadata and the bounded Gateway unary body it actually receives. It must not be advertised as receiving native typed unary identity or native transport bodies/headers. Fuller access is separately owned.

### 5. Account for the newly represented values before scanning/allocating

Extend existing protocol-local budget accounting, not a generic JSON/metadata traversal. Reject warning/content cardinality using conservative minimum registered encoded sizes before allocating corresponding output slices. Count active warning strings, native IDs/model IDs, serialized timestamps and source display bytes together with existing represented data using overflow-safe remaining-budget subtraction. Aggregate unary accounting must not independently grant each list/field a full response budget. Check aggregate sizes before UTF-8 scans/JSON encoding so an oversized later field cannot trigger unbounded earlier validation work.

Keep standard JSON encoding and the exact final complete unary/SSE limit, including `data: ` and `\n\n`. Escaping expansion may use the existing bounded constant multiple of the configured budget; no partial success/event is written. Invalid unary values use existing precommit safe errors. Invalid/over-limit initial warning output yields one empty start then at most one terminal safe error; invalid later metadata/source output yields at most one terminal safe error. No second event after writer failure or authoritative finish.

Alternatives: output-only limits miss pre-encoding work; arbitrary warning length caps or truncation lose supported values; hand-written JSON adds unnecessary complexity.

### 6. Independent evidence and observation boundaries

Use deterministic provider-domain witnesses in the Gateway testserver and fake native API responses for authenticated command paths. Extend independent Go-client fake-server tests without importing Gateway code. Assert raw JSON/SSE against closed local schemas, inactive-field absence, exact values/order and complete document limits. Test malformed/UTF-8/type/null failures separately from permissive TS acceptance.

Add a deterministic provider-neutral `test/integration/testserver/` scenario using `ToUIMessageStream(WithUIMessageStreamSources(true), WithUIMessageStreamMessageMetadata(...))` and matching Vitest parsing with `parseJsonEventStream`/`uiMessageChunkSchema`. Source chunk/assembled part assertions cover native sourceId/display/order, repeated/equal-cross-variant IDs and empty document title. For identity, the explicit test-only metadata callback type-switches on `aisdk.StreamFinishStep`, copies its non-nil Response id/modelId/timestamp into scenario-owned message metadata and otherwise returns nil. Supply all three native values in this scenario: core orchestration has its own absent-value defaults, so frontend evidence must not be misrepresented as proof of Gateway absence semantics. Response identity is not an automatic universal UI field; warnings are proved at provider/client hooks, not invented as UI fields. No production UI callback, chunk or API is added. Gateway-specific tests remain under `ai-gateway/`; frontend evidence alone does not prove Gateway mapping.

Keep metadata-only service logger/Prometheus/Agent Observability payload exclusions and canonical labels. Consumer observation uses verified existing `middleware.WrapLanguageModel` with test-only `middleware.Middleware.WrapGenerate` and `WrapStream` around `providers/grafana`. WrapGenerate delegates once and observes returned Warnings/Content/Response.Body alongside unset typed response identity; WrapStream delegates once and forwards every PartStreamStart/PartSource/PartResponseMeta through a context-aware test tee unchanged to a test-owned bounded sink. This proves hook access, not automatic export by every built-in middleware.

Separately configure existing `logger.Middleware` with a consumer-owned slog destination and `CaptureOptions.ResponseBody` and a sufficient test capture budget; assert an actual `ai_sdk.response.body` log record contains the Gateway unary body with native nested identity/warnings/sources. Distinguish this opt-in logged Gateway body from native transport diagnostics and from mere raw-body accessibility. The logger does not automatically export every stream-start/source field and Agent Observability has no generic source capture representation; do not add either in this change or claim hook access establishes them. Known dummy credentials remain absent on returned/operator surfaces; ordinary scalar strings are not censored because they resemble credentials.

## Risks / Trade-offs

- [Clients relying on replacement IDs/display/canonical stream identity] → Document the behavior change; preserve requested model and canonical operator identity separately. No legacy mode.
- [Optional empty presence is lost in Go] → Explicit omission rules and tests for raw client inputs versus Go-produced output; no false exact-presence claim.
- [Pinned unary transport replacement hides typed native identity] → Test/document raw-body retention and typed absence; do not invent an access carrier.
- [Long warnings/IDs expand encoding work] → Aggregate preflight, minimum cardinality bounds and final complete-byte checks with escape-heavy boundary tests.
- [Existing metadata filtering is mistaken for approved privacy] → Mark it as an outstanding #280 gap in specs/docs/PARITY; leave its production codec unchanged here.
- [Strict local schemas disagree with permissive TS parsing] → Raw/schema checks remain independent; preserve test evidence boundaries.
- [Synthetic transports or workspace success overstate deployment parity] → Report candidate-source, published module and image evidence separately; never alter authentic provider fixture inputs.

## Migration Plan

1. Implement from canonical main after rechecking the then-registered baseline; run regression witnesses against the old implementation first.
2. Deliver server/client/tests/docs together; preserve module/license boundaries and current candidate-source checks. Run standalone module checks independently and image delivery gates where affected.
3. Deploy through the existing Gateway release process. Consumers use native source identity/display and native stream response identity; routing/metrics remain canonical.
4. Roll back by restoring the prior Gateway/client release if operationally necessary; rollback restores documented old substitutions rather than introducing a compatibility branch in the new encoder.

## Open Questions

None blocking proposal readiness. Reconfirm the registered versions and canonical-main implementation when applying; newly discovered credential-bearing producer fields or incompatible baseline changes require a focused design decision, not silent scope expansion into metadata/diagnostics.
