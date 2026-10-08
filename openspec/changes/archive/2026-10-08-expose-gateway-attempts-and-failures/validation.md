# Validation

## Scope and reference

Runtime rewrite based on #373 at ea668bef, not the historical 4d0f9bfc collector implementation. The old branch is retained as backup/failure-visibility-before-overview-rewrite. This PR owns one active OpenSpec change; the foundation remains synced/archived.

Registered reference: ai 7.0.118, Gateway 4.0.96, Provider 4.0.18 and Provider Utils 5.0.49 at 5d12eaa6caa193d3901cbab98a734403eb6bf622. Matching upstream Gateway/model/error source informed carrier/access behavior. Shared capture and private Gateway overview serialization are Go adaptations, not a private-service parity claim.

## Publication

Runtime commit 6399d319 was pushed to nrbrd/failure-visibility with an explicit lease against historical 4d0f9bfc. Draft #370 remains based on #373; its description/title now reflect this rewrite, not the obsolete collector or historical equivalence. Exactly one active PR-owned OpenSpec change was verified. The unrelated four-line go.work.sum patch remains unchanged and uncommitted.

## Regression evidence

- Catalog copies configured candidates and private sources on construction/resolution; credentials are excluded from discovery/serialized resolution.
- Direct/fallback result and failure carriers, noneligible/exhausted failures, leading error selection and error/content/finish ordering preserve invocation counts and no replay.
- Request isolation, direct cancellation/late native failure exclusion and saturated operator-output independence pass under races.
- Exact fitting response/frame enrichment succeeds; one-byte-short optional enrichment preserves the original native namespace/output. Primary-invalid JSON metadata and complete encoded primary failures remain adaptation failures.
- Configured credentials, protected request headers, explicit Anthropic/OpenAI MCP credentials and native other-tenant fields are protected without raw diagnostics, token-pattern DLP or general application-data scanning.
- Complete HTTP error enrichment stays within the existing Go-client default 64 KiB read bound; oversized optional information preserves the original classified error.
- Go-produced namespace and unary/setup/committed carriers validate against strict Gateway-owned schemas.
- Native-fake authenticated command tests compare Go/registered TypeScript carriers with actual candidate counts, public classifications, request conversion, cancellation and operator privacy.
- Real-handler low/high-level/middleware tests cover streaming errors followed by content/finish and both generation APIs. Go GenerateText uses streaming, not unary DoGenerate.
- Frontend scenario parses UI chunks with parseJsonEventStream/uiMessageChunkSchema and verifies ordered errors/content/finish and assembled text without automatic raw-diagnostic forwarding.

## Checks

- Root/Gateway/Grafana-client tests and races; root/Gateway/client vet and lint.
- Go 1.26.8 root/Gateway races and standalone readonly Grafana-client races/vet; minimums and dependency pins unchanged.
- mise run test-providerwire-v4: 134 schema and 112 contract/runtime tests.
- mise run test-ai-gateway-command: 74 authenticated command tests, no skips.
- mise run test-integration: 24 frontend files, 133 tests, plus Gateway dependency suites.
- mise run parity-check: registered replays pass; optional provider-shape drift reporting skips unavailable registered package source.
- Docs, module/license boundaries, SDK/Gateway isolation, source workspace, merged pins and published Grafana module gates.
- Strict OpenSpec: 95 items, zero failures. Whitespace and unrelated go.work.sum checksum preservation checked.

## Review-fix loop

Three fresh review rounds covered correctness, maintainability and regression evidence; a forked oracle checked the approved design in round one. The oracle found no architectural drift; its heterogeneous delivery proof gap was closed without a product/API redesign. The workflow reached its three-round cap, not a clean final review.

- Round one fixed accepted body credentials, explicit provider API-key headers, case-distinct MCP header values and valid mixed-type siblings. It removed repeated unary content mapping and strengthened actual late-callback, shared-fallback, heterogeneous both-client and frontend metadata/ordering proof.
- Round two fixed known credential values in named MCP destination URLs, reusing the existing URL extractor. Six unary/committed regressions failed before the fix and passed after; ordinary URLs remain unprotected application data. Added exact-fit, one-byte-short and canonical-only committed-error bounds plus configured direct invalid/EOF setup proof.
- Round three's simplicity and validation reviews found no actionable issues. Correctness found a pre-publication cancellation ownership race: the Gateway rejected a unary worker outcome but could publish its late native fallback SourceErr. Parent reproduced it, then sealed/discarded unverifiable history at the existing rejection boundary. Regression covers observations before and after rejection, preserves the SDK observer's native SourceErr, cancellation classification and zero secondary calls.

After the last fix, full Gateway races/vet/lint, Go 1.26.8 focused Gateway races/vet and fallback races, schema/runtime/frontend and 74 authenticated command tests pass. Root/full Gateway/standalone client Go 1.26.8 checks and registered parity passed after the second batch; SDK/client/provider adapters and fixtures did not change in the last batch. Docs/boundary/workspace/pin/OpenSpec checks are recorded above.

All verified in-scope findings were fixed; no owner-level decision or known actionable finding remains. The final ownership fix has parent red/green/source verification but **has not received a fresh independent follow-up review**, because the round cap was reached. No clean or merge-ready verdict is claimed.

## Owner-requested simplification

Rechecked the implementation against the higher-level Gateway plan: this delivery remains configured-route attribution and protected failure delivery, not a tracing, routing/BYOK or native-transport framework. Optional encoding now belongs to the existing safeErrorDocument in errors.go; the separate invocation-error document/helper file is removed. Accepted error, unary and finish encodings are reused for writing rather than encoded again. The operator documentation now states the response-versus-telemetry distinction directly.

HTTP/SSE regression cases cover selected, absent and malformed optional attribution, current-error-only delivery, exact fit/no room, unchanged classification/retry fields, original-byte fallback and input isolation. Full Gateway races/vet/lint, Go 1.26.8 focused Gateway races/vet and standalone client races/vet, frontend/schema/runtime and 74 authenticated command tests, registered parity, docs/license/isolation/workspace/pins and strict OpenSpec checks pass. No independent review beyond the previously recorded three-round loop is claimed; the unrelated go.work.sum patch remains untouched.

## Typed public-error design

The subsequent owner-approved structural refactor supersedes the earlier document consolidation: errors.go now holds classification only; error_response.go defines each public error once and builds typed HTTP/SSE payloads directly. Encoded bytes are not stored alongside mutable wire fields, self-generated JSON is not decoded, and optional error encoding no longer depends on fit-callback state. Execution boundaries finalize and project context before writing; classification carries neither invocation state nor native errors. HTTP and committed-stream classification remain intentionally distinct.

Before changing production code, frozen HTTP/SSE byte and classification-boundary regressions passed on the prior implementation. Original literal response/frame catalogs are retained only as test expectations. Tests preserve all public category/capability bytes, host responses, status/retry fields and unknown-category fallback, plus wrapped cancellation/timeout and invalid/native-zero status differences. Optional-field tests cover malformed metadata, surviving current-only output, malformed/oversized current summaries retaining canonical output, complete-frame bounds and primary-response stability.

Full Gateway races/vet/lint, Go 1.26.8 Gateway/client compatibility, frontend 24 files/133 tests, strict schemas and runtime scenarios, 74 authenticated command tests without skips, registered parity, docs/boundary/isolation/workspace/pins and strict OpenSpec pass. These remain deterministic synthetic witnesses, not live-provider/private-service proof or a new independent review. The unrelated go.work.sum diff is preserved.

The follow-up cleanup removes the unreachable reasoning refusal, its fixtures and obsolete tool-mode gating. Public definitions use named fields; request-failure reasons distinguish capability gaps from policy refusals without changing reachable response bytes or adding shared protocol abstractions. The expanded request-rejection suite passed before and after production changes, checks schema-valid inputs and exact existing responses for every remaining reason in both invocation modes, and proves rejection precedes resolution/invocation. Reasoning coverage now exercises both production handler modes and verifies mapped content reaches the model. The Gateway/compatibility/integration/parity/docs/boundary/OpenSpec gates above were rerun successfully for this cleanup; no new independent review is claimed.

## Explicit capture and finalized views

The owner-approved rewrite removes invocation.go/invocation_sources.go and the Gateway context key. execution_capture.go separates copied request identity/credentials, a short-lived synchronized attempt buffer, and a finalized view. The existing SDK observer remains context-scoped; model helpers receive the buffer explicitly. Projection detaches observations before inspecting native errors, and established-stream code receives no collector. Existing rejection boundaries discard unowned observations while preserving locally owned direct cancellation. Direct projection does not write synthetic fallback events into the buffer. Credential extraction now lives in internal/execution/credentials.go with separate Anthropic/OpenAI MCP handling; no new credential policy, provider wrapper or cleanup owner was added.

The pre-rewrite Gateway execution/ProviderWire race suites passed. Existing HTTP/SSE, ownership, late-callback, protection, no-replay and exact-fit tests pass after the rewrite. Focused tests additionally prove detached observations remain empty after concurrent late entry/callback attempts, finalized output is stable, request inputs are copied, rejected direct/fallback outcomes remain distinct, and reentrant/panicking error inspection runs after sealing without holding the capture lock. The former context-introspection test now checks the actual canceled HTTP result and physical candidate count; explicit-buffer tests retain the sealed-state/late-mutation proof.

Full Gateway races/vet/lint (zero issues), Go 1.26.8 focused Gateway and standalone-client races/vet, frontend/schema/runtime integration, 74 authenticated command tests without skips, registered parity, docs/boundary/isolation/workspace/pins and strict OpenSpec pass. No provider fixtures, SDK contracts, wire shapes, dependency pins or go.work.sum changes were made. No fresh independent review or live-provider/private-service proof is claimed.

## Limits and disposition

No provider input fixture was added or changed; synthetic endpoints/UI scenarios are scoped behavior witnesses, not recorded inputs, live-provider acceptance or deployed/private-service parity. The independent reviews and the final unreviewed fix are bounded as described above. Native transport diagnostics #323, discovery #324, routing #316, BYOK #317, producer/core fixes #299 and broader client acceptance #375 remain separate.

Overview absence does not prove no attempts; native namespace presence does not establish Gateway provenance. Mixed/incomplete capture can be omitted, selection is commitment rather than completion, and caller retries/preselection fallback do not establish exactly-once generation or remote effects.

The owner authorized sync/archive on 2026-10-08. The six capability specs are synchronized, including reconciliation of legacy precomputed-only error wording and blanket error-detail suppression with the approved typed encoding/protected optional carriers. All 15 implementation tasks are complete; strict OpenSpec and documentation checks cover the synchronized archive. Merge remains pending separate approval.
