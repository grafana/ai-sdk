## 1. Baseline and Workspace Foundation

- [x] 1.1 Scaffold the private `test/providerwire-v4` pnpm workspace with exact registered `ai`, `@ai-sdk/gateway`, `@ai-sdk/provider`, and required utility dependencies, then add it to `test/pnpm-workspace.yaml`, mise test dependency sources, and the lockfile.
- [x] 1.2 Document the workspace claim boundary, evidence provenance, registered package versions, generated versus maintained artifacts, and the distinction from provider recordings, legacy interop, and production runtime tests.
- [x] 1.3 Implement and test the package/source-equivalence check against Vercel commit `d76eb85a9a7f2dbe44ab2f3dc858ad5cdcb5242e` for the relevant installed provider request declarations and unions, Gateway request serializer and header composition path, and Gateway failed-response and error-normalization path, failing without fallback when equivalence cannot be established.
- [x] 1.4 Register `test/providerwire-v4/package.json` in default baseline validation and coordinated upgrade consumer lists.
- [x] 1.5 Add focused baseline and upgrade tooling tests that fail for workspace omission, missing baseline packages, and exact-version drift.

## 2. Authoritative Request-Surface Classification

- [x] 2.1 Inventory the exact registered `LanguageModelV4CallOptions` keys and every referenced prompt, file-data, tool, tool-choice, tool-result, approval, response-format, and provider-option request union.
- [x] 2.2 Inventory the registered `gateway-language-model` serializer transforms, `/language-model` composition relative to `baseURL`, configured-header normalization, case-sensitive exact-key object spread, pre-Fetch lower-case last-value normalization, reserved collision behavior, fields it excludes, and differences between declared call options and emitted requests.
- [x] 2.3 Commit one canonical typed TypeScript coverage map mapping every finite key and discriminator to source, exact/presence observations, or an explicit exclusion rationale, and generate the reviewer-facing classification from it.
- [x] 2.4 Apply `satisfies` constraints to the canonical map so newly pinned finite keys or discriminators fail typechecking.
- [x] 2.5 Add generic coverage validation that rejects missing scenarios, missing semantic paths, and exact-value drift without maintaining a second inventory.

## 3. Deterministic Request Capture

- [x] 3.1 Implement the local recording HTTP server and deterministic Gateway client harness for ordered unary, streaming, and multi-request flows, asserting `/language-model` relative to the configured `baseURL` and the planned `/api/v1/aisdk/language-model` composition.
- [x] 3.2 Evaluate maintained JSON parsing packages, select the smallest duplicate-aware strict parser or visitor, and add focused tests rejecting duplicate members, comments, trailing commas, invalid numbers, and trailing data before normalization.
- [x] 3.3 Implement stable semantic capture serialization for method, relative path, final Fetch-emitted content type, required protocol headers, classified behavior-affecting outer headers, strict-envelope classification, request order, and JSON body while normalizing only object ordering, insignificant formatting, and explicitly allowed authentication, user-agent, and observability values.
- [x] 3.4 Add outer-header scenarios proving call-level `options.headers` appears in both the JSON body and outer request; separately assert same-exact-key object-spread replacement, pre-Fetch lower-case last-value normalization for custom and protocol case variants, exact and case-variant `Content-Type` overrides, and strict-envelope exclusion of invalid content-type collisions.
- [x] 3.5 Add grouped direct unary and streaming scenarios covering presence-sensitive scalar settings, explicit zero and false values, empty collections, reasoning, raw-chunk selection, and structured output.
- [x] 3.6 Add grouped prompt scenarios covering every classified role and content arm, including assistant tool calls/results, tool-role results/approval responses, every file-data arm, provider options, and body-carried headers.
- [x] 3.7 Add grouped function-tool scenarios with provider options, provider-tool scenarios restricted to `type`, `id`, `name`, and `args`, input examples, strict values, and every classified tool-choice arm.
- [x] 3.8 Add a deterministic multi-step client tool flow that captures request count, sequence, continuation prompt content, tool results, and tool-role approval responses.
- [x] 3.9 Add runtime observation assertions proving every supported classification entry appears at its designated semantic path and presence state.
- [x] 3.10 Generate and review the stable semantic request corpus, ensuring compatible members are packed into readable behavior groups, supported captures pass strict envelope validation, content-type collision captures are explicit exclusions, and each capture exists for a distinct serializer, schema, presence, header-composition, pre-Fetch normalization, sequencing, or exclusion behavior.

## 4. Strict Request Schema

- [x] 4.1 Author the normative JSON Schema draft 2020-12 root and shared definitions for the ProviderWire V4 language-model request body.
- [x] 4.2 Encode finite-object closure, requiredness, nullability, presence-sensitive empties, role-specific content, and discriminated union arms without constraining opaque provider option JSON recursively.
- [x] 4.3 Add HTTP envelope and schema validation for every committed semantic capture.
- [x] 4.4 Add focused negative schema tests for unknown members and discriminators, inactive-arm fields, role-incompatible content, missing required-empty members, and invalid standardized nulls.
- [x] 4.5 Verify the schema is hand-reviewable and independently exercised by pinned-client captures and negative inputs rather than generated and self-validated by one tool.

## 5. Go Provider-Model Delta Evidence

- [x] 5.1 Compare the complete classified request surface and semantic corpus with current `provider.CallOptions`, message/content, tool, file-data, response-format, header, and provider-option representations.
- [x] 5.2 Commit the Phase 2 delta table with one row per demonstrated loss: valid distinction, evidence scenario/path, current Go behavior, executable witness, and required model change.
- [x] 5.3 Add passing Phase 1 Go loss witnesses that explicitly demonstrate each current lost or unrepresentable distinction without leaving ordinary tests red.
- [x] 5.4 Exclude byte-format differences, invalid inputs, and losslessly representable permissive flat unions from the provider redesign delta.
- [x] 5.5 Apply and document the coherence gate: confirm the delta is one coherent Phase 2 provider-contract change or revise the later phase plan before handoff.

## 6. Pinned-Client Response Consumption Probes

- [x] 6.1 Add a locally authored minimal unary JSON response probe and assert the registered Gateway client consumes the expected result.
- [x] 6.2 Add a locally authored SSE probe that emits one complete JSON stream part per `data:` event, ends at clean EOF without `[DONE]`, and is consumed in order by the registered client.
- [x] 6.3 Add a locally authored black-box safe non-2xx probe whose expected behavior derives from the source-equivalent installed Gateway failed-response and error-normalization path together with the represented HTTP status, and assert it through the public pinned client.
- [x] 6.4 Label response inputs and tests as smoke-level client-consumption probes and explicitly exclude exhaustive response arms, server taxonomy, privacy, and lifecycle claims.

## 7. Public Workflows and Parity Classification

- [x] 7.1 Add `mise run check-providerwire-v4` to typecheck and test the workspace, verify exact pins and committed source-equivalence evidence against installed package inputs without repeating the upstream source-tree comparison, regenerate into temporary storage, compare committed evidence, validate classifications/observations/schemas, run Go loss witnesses, and run response probes.
- [x] 7.2 Add a regression test proving `check-providerwire-v4` is non-mutating and reports stale artifacts without rewriting them.
- [x] 7.3 Add `mise run update-providerwire-v4-artifacts` to rewrite generated semantic requests and reviewer classification deterministically and then invoke the complete non-mutating check.
- [x] 7.4 Integrate relevant source-equivalence refresh, generated ProviderWire V4 artifact regeneration, maintained coverage/schema/delta/probe review, and checking into the coordinated parity-upgrade workflow without changing the registered package versions in this phase.
- [x] 7.5 Update `test/conformance/PARITY.md` with separate confidence statements for automated pinned-client request/schema evidence, Go loss witnesses, smoke-level response consumption, unchanged legacy interop, and absent strict runtime coverage.

## 8. Phase Validation and Handoff

- [x] 8.1 Run `mise run update-providerwire-v4-artifacts` once, review generated changes, then run `mise run check-providerwire-v4` and verify a clean worktree diff from the check itself.
- [x] 8.2 Run focused workspace, baseline-tooling, Go loss-witness, legacy `gateway/providerwire`, and `providers/grafana` transport tests.
- [x] 8.3 Run `mise run validate-parity-baseline`, `mise run parity-check`, `mise run test`, `mise run test-integration`, `mise run test-interop`, `mise run test-conformance`, `mise run vet`, `mise run lint`, and `mise run lint-docs`.
- [x] 8.4 Run `openspec validate --all --strict` and `git diff --check`, and confirm no production V4 runtime, provider API redesign, legacy wire change, or unproven response claim entered the phase.
- [x] 8.5 Record the exact baseline/source comparison, generated artifacts, validation commands, residual gaps, branch head, and Phase 2 handoff decision in the implementation summary.

## 9. Review Remediation

- [x] 9.1 Replace the independently maintained request inventories and generic presence assertions with one canonical typed TypeScript coverage map and a generated reviewer-facing classification artifact.
- [x] 9.2 Derive ProviderWire artifact metadata from the registered baseline and workspace manifest, validate every committed baseline claim, and add stale-metadata regression coverage.
- [x] 9.3 Rebuild the Phase 2 delta and external-package witnesses around semantic provider-domain representation loss, then recompute the coherence decision.
- [x] 9.4 Narrow source-equivalence evidence to an explicit reviewable relevant-source closure without adding dependency-analysis machinery.
- [x] 9.5 Require the non-mutating ProviderWire check from aggregate parity, repository check, and CI workflows, and retain the required stable workspace sums.
- [x] 9.6 Regenerate artifacts, prove stale metadata and artifacts fail without rewriting, and rerun the complete validation matrix from the intended baseline.
- [x] 9.7 Run the evidence-gated review/fix loop and an independent simplification review, applying all valid findings.
- [x] 9.8 Synchronize canonical specifications, archive the change, and verify that no active OpenSpec changes remain.
