## 1. Confirm authority and consuming boundaries

- [x] 1.1 Reconfirm canonical-main ancestry and `test/conformance/upstream.yaml`; use only the registered source/tests and record any baseline mismatch before implementation, without reusing stopped #303/#309 work.
- [x] 1.2 Trace all catalog/service/handler option-policy consumers and every supported option scope; verify which plumbing can be deleted without replacing it with another inventory.
- [x] 1.3 Audit current protected fields against Anthropic, OpenAI/Azure and compatible native consumption; record actual override code paths, namespace/scope, recognized spelling, precedence, disposition and paired bypass/harmless regression cases in the design. Include the Go MCP history branch before ProviderExecuted versus safe compaction type/local-call caller, without creating a new inventory or blanket all-provider validator.
- [x] 1.4 Complete that audit and document the smallest trusted native safety seam before weakening any global guard, including middleware-renamed identity and configured compatible namespace handling; stop for approval if the audit changes scope or needs an unrelated native adapter fix.

## 2. Establish failing request contracts

- [x] 2.1 Add focused mapper/model assertions for all supported scopes, unrelated namespaces, future fields, exact namespace case, empty objects and nested opaque JSON; confirm current filtering/protected-field behavior fails the intended assertions.
- [x] 2.2 Add unary/streaming synthetic native Anthropic cases for consumed call/cache/function-tool values and supplied local assistant-call caller; separately prove ordinary tool-role result caller is retained to adapter invocation but currently ignored in native requests. Confirm inventory-related loss on current main; do not change native converters or activate deferred assistant/provider-executed result branches to satisfy caller assertions.
- [x] 2.3 Add synthetic native OpenAI/Azure phase/item/reasoning and compatible custom-call/message/part extension assertions, including competing namespace precedence and ordinary negative controls; confirm any inventory-related failures.
- [x] 2.4 Add consumption-specific bypass tests for actual account/auth/destination/model/prompt/role/union/tool/transport/execution override paths and zero native I/O assertions, including local-call MCP history conversion, plus similarly named non-consuming ordinary values and safe contextual type/caller controls.

## 3. Preserve concrete safety without blanket field rules

- [x] 3.1 Only after the audit gate in 1.3–1.4 and paired regression cases, replace unjustified globally normalized protected-option checks with narrowly justified consuming-path checks or verified native precedence; preserve fixed safe failures before external I/O and add no generic allowlist, blanket all-provider scope validator or recursive opaque-JSON scanner.
- [x] 3.2 Keep complete schema/role/union and missing-codec checks explicit before invocation; prove malformed namespaces and unsupported siblings never succeed by losing their options.
- [x] 3.3 Verify reserved Gateway controls are consumed only by an existing owning feature or explicitly unsupported, never forwarded; preserve outer authentication and body-header credential/protocol separation with negative controls.

## 4. Remove Gateway option inventories

- [x] 4.1 Remove `applyProviderOptionPolicy` and `providerwire/v4/provider_option_policy.go`; preserve opaque namespace JSON and original call/message/part/function-tool/file-result placement.
- [x] 4.2 Remove service `provider_options.go` inventories and unused catalog policy fields/storage/cloning/intersection; replace tests that enforce inventories with forwarding and native-consumption contracts, without compatibility shims.
- [x] 4.3 Update testserver/harness/catalog construction and repository-wide policy references; verify no filtering remains under a replacement name.
- [x] 4.4 Retain the existing fallback guard/executor unchanged; add regressions proving preserved active namespaces are rejected, empty message objects survive and files/tools/disallowed history remain guarded before physical attempts.

## 5. Complete cross-client and isolation evidence

- [x] 5.1 Drive production direct unary/streaming handlers with both the exact registered TS Gateway client and independent Go Grafana client; assert equivalent mapping, scoped JSON and native consumed values using the existing differential harness.
- [x] 5.2 Extend authenticated real-command tests for Anthropic, OpenAI and compatible fake endpoints with both clients/modes, assistant-call caller consumption versus ordinary result-caller forwarding/ignore behavior, scoped extensions, native precedence and bypass/ordinary controls; keep any TS outer-hop transport adaptation explicit.
- [x] 5.3 Add immutable-input/raw-namespace and shared-handler/native-configuration concurrency tests with distinct request markers and no cross-scope or cross-request reuse; run relevant Gateway tests under Go race detection.
- [x] 5.4 Compare operator-observed and unobserved native requests/results, retaining capture defaults/settings and proving rejected credential-bearing controls are absent from server capture/responses.
- [x] 5.5 Keep synthetic native inputs in focused tests, not `recorded/` or `upstream/`; use only the registered client generator for any necessary golden changes and inspect the generated diff/provenance.

## 6. Document and validate the bounded delivery

- [x] 6.1 Update `docs/providers/grafana-gateway.md` with opaque direct-route options, concrete protections, native interpretation, supplied-history evidence and the still-separate fallback/metadata boundaries; remove stale inventory claims without unrelated response/capture rewrites.
- [x] 6.2 Update `test/conformance/PARITY.md` only for stable evidence/support-boundary changes; compare the implementation against pinned upstream source/tests and classify remaining differences without claiming live native acceptance or full continuation.
- [x] 6.3 Run `openspec validate forward-native-provider-options --strict`, formatting, applicable Go vet/lint and docs lint; ensure no generated/debug/unrelated files remain.
- [x] 6.4 Run `mise run test-providerwire-v4`, `mise run test-ai-gateway-command`, `mise run test-ai-gateway-source-integration`, `mise run test-ai-gateway` and `mise run parity-check`; record exact results and resolve failures without changing pins silently.
- [x] 6.5 Run candidate-source module/license checks (`mise run verify-gateway-workspace`, `mise run verify-merged-pins`, `mise run verify-ai-gateway-boundary`, `mise run verify-sdk-gateway-isolation`) and applicable build/tests; run readonly standalone verification for any changed published module and Gateway image/source delivery checks as required by candidate-source CI.
- [x] 6.6 Review the final diff against every #318 acceptance item: direct forwarding, concrete protections, no mutation/isolation leaks, capture independence and delivered specs/docs/checks; keep #319/#280 and unrelated adapter work separately scoped.
