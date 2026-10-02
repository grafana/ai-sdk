## 1. Confirm the implementation base

- [x] 1.1 Reconfirm #318/PR #326 ancestry in the existing `nrbrd/gw-fallback` stack and implement/review on that branch without restructuring or waiting for parent merge. While #326 is open, the child PR targets `nrbrd/opt-forwarding`; merge after the parent and rebase/retarget to main after the parent merges. Do not reuse stopped #303/#309 code, duplicate #318 or claim the parent is already merged.
- [x] 1.2 Reconfirm parent changes, `test/conformance/upstream.yaml`, corresponding pinned Gateway/provider/native source and affected main specs; validate cumulative source and refresh this proposal if the baseline or protection/composition seams changed.
- [x] 1.3 Audit service/catalog/mapper admission and candidate option flow, retaining concrete consumption-backed bypass protections and identifying any remaining route-wide policy intersections. Record the existing Gateway/runtime and provider-boundary coverage limits from `PARITY.md`.

## 2. Establish failing eligibility regressions

- [x] 2.1 Add table-driven service regressions for mapped function definitions/history/results and auto/none/required/named choices, file arms/filename presence, reasoning text/files/controls, ordinary headers and scoped empty/active options in unary and streaming modes; confirm the current text guard rejects the intended success/failover cases.
- [x] 2.2 Add compact registered-TypeScript and independent-Go client witnesses using existing production-handler/command helpers for representative direct/configured-fallback requests in both modes; confirm the fallback cases fail before removing the guard.
- [x] 2.3 Preserve malformed/deferred/reserved/protected negative controls at their owning mapper/native boundaries, proving prohibited native I/O remains blocked rather than relying on the retired text guard.

## 3. Compose mapped fallback directly

- [x] 3.1 Remove `fallbackTextModel`, `fallbackTextRequest` and `emptyMessageOptions` from `fallback_route.go`; compose `fallback.New(candidates...)` directly in the catalog while preserving candidate construction, native protections, configured order, attempt hook and single logical factory placement.
- [x] 3.2 Remove any confirmed leftover candidate-policy intersection/filtering with no real protection consumer; keep the same mapped options/scopes for each attempted adapter without translation, new admission inventories, decider changes or request-routing controls.
- [x] 3.3 Replace obsolete rejection-only tests with mapped success/failover assertions, retaining existing automatic-choice/empty-namespace cases and strict unsupported-feature tests; run the focused service suite to make the regressions green.

## 4. Prove client and native behavior

- [x] 4.1 Complete both-client direct/fallback unary/streaming coverage for representative tools/history/choices, files/presence, reasoning and headers/options. Use recording models for all mapped selections/scopes and preserve existing registered goldens unless a reviewed missing witness requires regeneration.
- [x] 4.2 Extend authenticated command fake-native tests with a heterogeneous Anthropic/OpenAI/compatible chain carrying multiple namespaces; assert actual per-candidate HTTP body/header consumption, namespace precedence, scoped values and irrelevant-namespace non-consumption without route intersections.
- [x] 4.3 Exercise primary success, eligible setup failover through three candidates in declared order, exhausted/noneligible failures and cancellation with mapped requests. Use synthetic model channels separately for empty pre-part EOF; do not label native HTTP or model fakes as provider recordings.
- [x] 4.4 Add deterministic local function loops for both clients and unary/streaming invocation through direct and fallback routes; assert each expected consumer function executes once, continuation history reaches native requests, each new request restarts at primary and the Gateway executes no functions.

## 5. Preserve commitment, isolation and observation

- [x] 5.1 Add mapped tool/reasoning cases proving first stream-start, first/later error and tool input/call commitment; make any subsequent candidate invocation fail the test. Assert no replay after selected unary/stream output fails strict encoding, bounds or completion validation.
- [x] 5.2 Run and retain reusable and composed invalid/nil/result-plus-error/late-setup, decider/cancellation race, blocked consumer, silent/continuously-ready producer, cleanup time/part-budget and observer panic/saturation regressions without adding an executor or parallel channel owner.
- [x] 5.3 Add shared-model concurrent mapped-request tests checking original maps/raw JSON/file presence/history remain unchanged across attempts and invocations, candidate state/correlation stays isolated and one logical generation covers each request's attempts.
- [x] 5.4 Verify observer-enabled/disabled mapped requests preserve supported response semantics independently of metadata-only capture; keep credentials/payloads out of operator records without using observation policy to refuse mapped content.

## 6. Align documentation and validation

- [x] 6.1 Sync the five delta specs, update their stale purpose wording where applicable and remove text-only/tool/file/active-option fallback claims from centralized Gateway/client guides. Replace the Cloud-auth spec's blanket remaining-effectful-fallback clause with evidence-specific mapped local-tool/file/reasoning coverage, preserving auth/edge/credential limits and separate missing provider-executed/MCP codecs and actual Cloud/BYOK support. Document backend-specific acceptance, first-part commitment, consumer execution ownership, paid-generation risk and separate #280 metadata/continuation work.
- [x] 6.2 Update `test/conformance/PARITY.md` only for stable delivered mapped-fallback evidence, retaining missing-codec/live-acceptance/output-derived-continuation boundaries and the registered baseline. Verify no synthetic provider inputs were added to `recorded/` or `upstream/`.
- [x] 6.3 Run focused root fallback tests/race tests (`go test ./fallback`, `go test -race ./fallback`) and Gateway service/providerwire race tests through the verified absolute `go.gateway.work`; run applicable format, vet and lint checks.
- [x] 6.4 Run `mise run test-ai-gateway-source`, `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, `mise run test-ai-gateway-source-integration`, `mise run test-integration` and `mise run parity-check`; report failures/gaps rather than weakening tests. If implementation changes UI/SSE behavior, add the required schema-parsed frontend scenario before accepting that scope change.
- [x] 6.5 Run required candidate-source/module-boundary checks, including `mise run verify-gateway-workspace`, `mise run verify-merged-pins`, `mise run verify-ai-gateway-boundary` and `mise run verify-sdk-gateway-isolation`; keep Apache modules independent and introduce no committed replacement or new dependency. Validate Gateway image checks if image/build sources change.
- [x] 6.6 Compare the completed behavior against pinned upstream projection/native semantics and the existing Go fallback contract, classify any difference, verify issue #319 acceptance without absorbing its non-goals and run `openspec validate support-mapped-gateway-fallback --strict`.
