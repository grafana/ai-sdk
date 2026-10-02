## 1. Reconfirm reference and establish failing regressions

- [x] 1.1 Start implementation from canonical main, re-read issue #320/work package 33 and the then-registered `upstream.yaml`/`PARITY.md`; resolve matching upstream source/tests and record any baseline change before adapting this plan. Do not extract stopped #303/#309 patches.
- [x] 1.2 Add focused failing Gateway unary/stream witnesses for all four native warning variants/order, required empty fields, optional-details normalization and unary warning retention; confirm old behavior fails before changing production mapping.
- [x] 1.3 Add failing Gateway witnesses for native URL/document IDs/display, repeated/equal-cross-variant IDs, empty required IDs/titles, long IDs above the old cap, Title/legacy Text precedence and OpenAI/Azure file_path display.
- [x] 1.4 Add failing identity witnesses for supplied/partial/absent response identity, present-empty unary response, native model strings outside route syntax and native-vs-requested/canonical separation; use focused synthetic tests, not invented or rewritten provider recording inputs.
- [x] 1.5 Reconcile the scoped `gateway-ordered-text-fallback`, `providerwire-v4-http-contract` and `gateway-provider-configuration` deltas with current specs, including native-vs-canonical success clauses and warning combination. Preserve all eligibility/commitment, operator telemetry, configured-secret and existing discovery/error boundaries; coordinate overlapping #318 spec edits without depending on #280 or adding #322/#324 work.

## 2. Restore bounded Gateway warning and identity output

- [x] 2.1 Replace generic stream warnings with one small explicit protocol-private registered-union mapping shared by unary/stream output. Preserve active fields/order, required empties and optional-empty normalization; emit unary warnings as an array and reject unknown variants.
- [x] 2.2 Add registered unary response identity DTO/mapping and native stream identity projection with no canonical fallback, preserving nil/present-empty response semantics and valid nonzero timestamp instants. Leave native request/body/header diagnostics and opaque metadata codecs untouched.
- [x] 2.3 Extend preflight to bound warning/content cardinality and aggregate represented source/warning/identity/timestamp bytes before mapped allocation, UTF-8 scans and encoding; retain standard JSON plus final complete unary/SSE byte limits and existing safe failure behavior.
- [x] 2.4 Update `schema/unary_success.json` and `schema/stream_event.json` only for represented warnings/source IDs/native optional response identity. Add independent schema cases for active/inactive union fields, required empty values, omitted optional fields and malformed/null/wrong-type identity/timestamps.

## 3. Preserve sources and independent Go consumption

- [x] 3.1 Remove source identity-map state/cloning/sequential rewriting and its 1024-byte key cap; preserve native required-string IDs/order without deduplication or collision repair, using containing-document resource bounds.
- [x] 3.2 Remove file_path detection/display substitution while retaining existing Title/Text and optional title/filename semantics. Leave the bounded numeric metadata codec unexpanded and identify its loss as the separate #280 gap.
- [x] 3.3 Update `providers/grafana` stream identity decoding to accept absent/explicit-empty fields and native strings outside public route syntax; retain strict original-document UTF-8/types/null handling, timestamps, bounded SSE ownership and cancellation.
- [x] 3.4 Extend independent Apache-client fake-server tests for warning/source/identity values, required/missing/empty fields, repeated IDs, malformed output and unary raw-body identity versus typed transport replacement. Do not import Gateway DTOs/validators.

## 4. Prove resource and lifecycle regressions

- [x] 4.1 Add table-driven unary and SSE below/exact/one-byte-over boundaries for native warning/source/identity fields, aggregate combinations and escape-heavy strings; cover excessive warning count and long valid source IDs without introducing new scalar caps or truncation.
- [x] 4.2 Cover invalid UTF-8 in every newly represented scalar family, unknown warning/source discriminators, invalid timestamps and malformed client JSON/types/null; assert unary precommit errors and complete SSE safe terminal behavior without reflecting invalid data.
- [x] 4.3 Verify omitted/duplicate/late starts, metadata before/after source/reasoning/text/tool output, interleaved/repeated sources, non-terminal errors, idle activity, finish/post-finish suppression, cancellation, writer failure and single-owner bounded cleanup remain unchanged.

## 5. Cross-client, authenticated command and observation evidence

- [x] 5.1 Extend Gateway testserver/runtime/differential scenarios so pinned TS and Go clients consume identical native warnings, source display/IDs and partial/native stream identity; add raw HTTP/SSE/schema assertions independent of permissive TS parsing.
- [x] 5.2 Extend pinned-client consumption probes to prove server-warning preservation and registered server-before-local combination (local warnings are currently empty), full unary request/response replacement, raw native identity retention and absence from typed Response fields. Do not invent a warning-generation option or diagnostic carrier.
- [x] 5.3 Update authenticated command fake-native scenarios to assert native source/display and response identity through both clients, requested-alias/canonical/native separation and ordinary token-looking strings, while configured dummy credentials/other-tenant state remain absent.
- [x] 5.4 Extend service metadata-only logger/Prometheus/Agent Observability exclusions and consumer hook tests using existing `middleware.WrapLanguageModel`/`Middleware.WrapGenerate`/`WrapStream` around `providers/grafana`: delegate once, inspect unary Warnings/Content/Response.Body and unset typed identity, and forward observed PartStreamStart/PartSource/PartResponseMeta unchanged through a context-aware test tee to a bounded test sink. Prove hook access/immutability without claiming universal built-in capture or adding middleware/API surface.
- [x] 5.5 Configure existing consumer `logger.Middleware` with its own slog destination, `CaptureOptions.ResponseBody` and sufficient capture budget; assert actual `ai_sdk.response.body` log output contains the Gateway unary body's nested native identity/warnings/sources while server capture remains metadata-only. Raw-body availability alone is not capture proof. Do not claim native transport capture, automatic logger stream-warning/source export or Agent Observability source capture.

## 6. Frontend and durable documentation

- [x] 6.1 Add a provider-neutral scenario under `test/integration/testserver/` with `WithUIMessageStreamSources(true)` and an explicit test-only `WithUIMessageStreamMessageMetadata` callback copying supplied id/modelId/timestamp from non-nil `StreamFinishStep.Response` into scenario-owned metadata. Matching Vitest uses `parseJsonEventStream`/`uiMessageChunkSchema` to assert source chunks/assembled parts, repeated/equal-cross-variant IDs, empty required title/native display and explicitly mapped metadata. Do not claim warnings/response identity are automatic UI fields or frontend assembly proves Gateway absence semantics; keep Gateway-specific tests under `ai-gateway/`.
- [x] 6.2 Add/update provider-independent UI snapshots only where they establish new frontend regression evidence; distinguish those inputs from authentic recorded/imported provider responses and never rewrite provider inputs to conceal mismatches.
- [x] 6.3 Update applicable Gateway sources/observability and shared client guidance, including navigation, to explain native returned values, Go optional-presence adaptations, breaking old substitutions, separate route/operator identity and pinned unary typed replacement. Keep fuller diagnostics outside this scope.
- [x] 6.4 Update `PARITY.md` stable evidence/boundaries: remove fixed-prose/source-ID/file-path substitutions as accepted deviations once proved, record new evidence and retain metadata projection/diagnostic access as explicit remaining gaps without a dated issue catalog or complete parity claim.

## 7. Validate and prepare delivery

- [x] 7.1 Run focused server/client/root tests and applicable race tests, then `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, `mise run test-ai-gateway`, `mise run test-integration` and `mise run parity-check`; inspect any expectation changes against the pinned reference and authentic fixture provenance.
- [x] 7.2 Run formatting, applicable vet/lint/build and `mise run lint-docs`; run `mise run verify-ai-gateway-boundary`, `mise run verify-sdk-gateway-isolation` and relevant `mise run verify-published-module` checks with standalone readonly resolution, reporting candidate-source and published-pin evidence separately.
- [x] 7.3 Run applicable Gateway image/source gates (`mise run build-ai-gateway-image` and `mise run test-ai-gateway-image-source`), verify actual delivery dependencies, and record exact blockers rather than claiming unrun image/module proof. Review the final diff against #320 non-goals and registered upstream behavior.
- [x] 7.4 Validate this change with `openspec validate preserve-gateway-native-response-values --strict`; verify every acceptance item has concrete evidence, then verify/sync/archive through the OpenSpec completion workflow before merge.
