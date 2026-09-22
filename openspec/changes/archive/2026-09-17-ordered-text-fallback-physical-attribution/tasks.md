## 1. Rebase and Contract Baseline

- [x] 1.1 Reconcile the synthetic integration base `f40ba33` with accepted WP7 merge `ff25ba9` and WP8 merge `4f597f3`: the WP7 client implementation and WP8 observation/factory seams match; retain the service-local correlation accessor without reading private context keys. The final branch is retargeted and restacked on `main` at `1877271`, including the subsequently merged cloud-auth and OpenAI-compatible Gateway work.
- [x] 1.2 Confirm `test/conformance/upstream.yaml` still pins `ai@7.0.65`, `@ai-sdk/gateway@4.0.52`, and Vercel commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e`; record that the pinned upstream tree has no generic client-side ordered fallback primitive and classify WP9 as a Grafana extension that preserves ProviderWire bytes.
- [x] 1.3 Record the disposition of the original test-first process requirement: historical red-before-implementation evidence is unavailable and is not claimed. Current primitive regression tests and the composed acceptance suite establish behavior, not historical execution order; retain this explicit process deviation in the archive.

## 2. Apache Fallback Commitment and Lifecycle

- [x] 2.1 Add stable pre-commit invalid-result and premature-stream-end errors and update `DoStream` so only setup errors, invalid results, and EOF before the first part reach the decider.
- [x] 2.2 Change first-part handling so every received `provider.StreamPart`, including leading or repeated `PartError`, selects the candidate and is relayed exactly once with all later parts in order.
- [x] 2.3 Give every stream attempt a child context and implement a cancellation-aware relay whose sends/receives terminate when the caller stops consuming.
- [x] 2.4 Replace unbounded drains with one asynchronous cleanup path bounded by a positive package duration and part budget; prove silent, continuously ready, late, and cooperative producers cannot retain fallback-owned goroutines.
- [x] 2.5 Extend `Attempt` additively with exactly three selected/failed/canceled outcomes plus `WillFallback` as live decision-time intent, allowing later cancellation to prevent the next invocation; emit exactly one ordered decision per invoked unary or streaming candidate, normalize outcome/error together on the post-decider cancellation snapshot, and recover observer panics without changing results. Prove observer cancellation, cancel-inside-decider, and deadline-during-decider behavior in both modes.
- [x] 2.6 Update fallback package docs and `docs/guides/fallback-and-registry.md` for leading-error commitment, premature EOF, decision timing, observer prompt-return requirements, provider-owned leak limits, and retry × fallback amplification.
- [x] 2.7 Run focused fallback tests with `-race`, root `go test ./...`, formatting/vet, and the root module's independent `GOWORK=off` check.

## 3. Strict Gateway Route Configuration

- [x] 3.1 Extend the strict model YAML DTO with an ordered `fallback` list using the same direct provider/backend reference shape as `primary`, while keeping credentials and public-route references unrepresentable.
- [x] 3.2 Add fail-fast validation for empty candidate fields, unknown providers, duplicate `(provider, model)` tuples across primary/fallback, recursion-shaped/nested route input, unknown fields, and empty effective routes before secret resolution or listener binding.
- [x] 3.3 Add table-driven configuration tests for multiple named Anthropic instances, preserved order, duplicate/missing references, route-shaped recursion attempts, strict YAML duplicate keys, and secret-free validation errors.

## 4. Gateway Construction and Middleware Placement

- [x] 4.1 Refactor startup construction to create each route's direct candidates once through the hardened Anthropic path, use the direct model when no fallback exists, and otherwise build one fallback model in configured order.
- [x] 4.2 Attach the WP9 physical observer directly to the fallback model, apply WP8's innermost canonical-identity override above it, and wrap the final route exactly once with WP8's enrichment → Agent Observability → logger/Prometheus logical chain.
- [x] 4.3 Add identity/composition tests proving canonical and alias resolution share the same immutable final model, physical candidates never receive logical wrappers, later calls restart at primary, and candidate request options remain identical.

## 5. Private Physical Attribution

- [x] 5.1 Define the WP9-owned private physical record and bounded sink/queue, then implement an allowlisted projection that reads only the service-local correlation accessor over WP8's immutable observation snapshot and records candidate index, configured provider instance/backend ID, bounded timing, one of three outcomes, `WillFallback`, and winner.
- [x] 5.2 Make the WP9-owned projection/enqueue non-blocking and fail-open with bounded-cardinality dropped-event/lifecycle-defect accounting; do not launch one goroutine per observation and do not let observer panic or sink saturation affect the call.
- [x] 5.3 Add privacy tests with hostile credentials, URLs, headers, bodies, provider metadata, and raw/aggregate errors proving none enter physical records beyond closed classification or any public/logical surface. `fallback_acceptance_test.go` joins real route composition, ProviderWire unary/SSE, logical logs/metrics/Agent Observability and closed physical records.
- [x] 5.4 Add correlation tests proving one logical WP8 lifecycle joins the ordered physical decisions and selected winner, while absent correlation remains absent and does not fail execution.

## 6. End-to-End Text Evidence

- [x] 6.1 Add deterministic unary service tests for primary success, retryable setup failure then secondary success, non-retryable stop, cancellation, full exhaustion, stable order, and safe aggregate-error mapping. Covered by `TestFallbackAcceptance_UnarySelectionAndPrivacy`, `TestFallbackAcceptance_CancellationBeforeSelection`, existing route order and authenticated command tests.
- [x] 6.2 Add deterministic streaming service tests for setup failure, premature EOF, nil/invalid result, leading and later `PartError`, multiple ordered error parts followed by text, post-commit close/error, cancellation races, and blocked-consumer cleanup. Covered by `TestFallbackAcceptance_StreamCommitmentAndPrivacy`, `TestFallbackAcceptance_CancellationBeforeSelection` and `TestFallbackAcceptance_CanceledBlockedConsumer`.
- [x] 6.3 Exercise authenticated direct and fallback routes with the exact registered Vercel Gateway client and the integrated WP7 black-box Go-client matrix; assert unchanged discovery, request, unary, and SSE schemas and zero public topology leakage.
- [x] 6.4 Keep synthetic provider/lifecycle inputs in focused unit or provider-independent service tests, do not add them as recorded provider conformance evidence, and update `test/conformance/PARITY.md` with the fallback-extension evidence boundary.
- [x] 6.5 Run root and `ai-gateway` formatting, vet, tests, race-focused suites, `mise run validate-parity-baseline`, and committed-pin `GOWORK=off` module checks; document any environment-only limitation without weakening acceptance.

## 7. Documentation and Handoff

- [x] 7.1 Update Gateway configuration/operator documentation with direct-only and ordered-fallback examples, immutable order, text-only scope, retry amplification, private telemetry fields, and the rule that deleting fallback entries restores direct routing.
- [x] 7.2 Verify WP6 image/capacity files, WP7 client APIs, and WP10 production deployment/rollout assets remain unchanged; leave production enablement and rollback smoke to WP10.
- [x] 7.3 Run `openspec validate ordered-text-fallback-physical-attribution --strict` and retain an apply summary listing the intentional leading-`PartError` behavior break, module pin order, WP8 dependency, validation commands, and privacy evidence.

## 8. Tools integration guard

- [x] 8.1 Add and verify the WP9 fallback-route guard for non-empty tools, tool choice, tool-call/result history, and effectful content before the first physical invocation (`TestFallbackRoute_RejectsEffectsBeforeAnyCandidate`). Successor integration remains owned by WP11 `add-gateway-unary-function-tools` task 3.5 and WP12 `add-gateway-streaming-function-tools` task 3.6; this checkbox does not claim those later capabilities are accepted or merged.
- [x] 8.2 Wire the private sink to a concrete private operational output with bounded worker writes/shutdown and saturation tests; do not reuse the logical logger projection or stop at an in-memory sink.

2026-09-17 completion: composed ProviderWire and logical/physical acceptance now
covers the remaining lifecycle and hostile-data cases against published Apache
fallback prerequisite `9dd11902673f` with `GOWORK=off`. The earlier registered
Vercel/Go command matrix remains the real-JWKS and exact-client evidence. The new
service tests supplement it with fake-model states unavailable at a normal
Anthropic transport boundary; they are not recorded provider fixtures.

The original wording of task 1.3 required failing tests before changing
`fallback.Model`. That historical sequence is not recoverable from passing tests
today. Its checkbox records an explicit process-deviation disposition, not proof
that the original sequence happened. Tasks 5.3 and 6.1/6.2 have new executable
evidence; 8.1 separates the implemented guard from successor acceptance rather
than treating future packages as prerequisites for their own base.

Production output/rollout evidence stays with WP10. A blocking macOS stderr
socket disables physical output rather than risking an unbounded write;
supported destinations are documented. Provider setup calls may still retain
provider-owned invocation goroutines until they return. Public protocol-defined
content and `finishReason.raw` remain successful result fields; privacy tests
target credentials, error details and provider-private metadata rather than
silently changing that existing protocol contract.

Final publication evidence: the dependency-order stack was replayed onto
`main` at `1877271`. The cumulative head passes Gateway ProviderWire,
service/config/process race tests; the 26-test real-command matrix including
cloud-auth and OpenAI-compatible composition; the full registered-upstream
parity pipeline; and all 67 strict canonical OpenSpec specifications.
