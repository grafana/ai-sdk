## 1. Establish pinned regression evidence

- [x] 1.1 Compare provider, UI, Gateway client projection, and native conversions with the exact registered upstream commit and the current parity coverage map.
- [x] 1.2 Add provider-domain, native-request, UI filename, and independent Go-client tests for all four file arms, selected empties, conflicts, invalid references, and absent/empty/non-empty filenames.
- [x] 1.3 Preserve fixture provenance: use pinned TypeScript client evidence, focused native tests, and deterministic UI integration rather than synthetic provider recordings.

## 2. Implement the Apache producer contract

- [x] 2.1 Add public file-data constructors/inspectors, selected-arm validation, and conflict-safe tagged/legacy decoding; validate direct prompts before native I/O.
- [x] 2.2 Preserve request-file, nested tool-result-file, and ordinary UI filename presence while retaining source/generated descriptive filename behavior.
- [x] 2.3 Align Anthropic/Vertex, OpenAI Responses, OpenAI-compatible, and Bedrock conversion with provider-specific arm, media, reference, URL, filename, and warning semantics.
- [x] 2.4 Preserve Anthropic inline text across declared media types and URL/PDF tool-result content; assert URL-only versus inline-PDF beta behavior.
- [x] 2.5 Update the independent Go client, reusable Agent Observability/logger, centralized input docs, examples, and cross-language UI filename scenario.

## 3. Publish and validate independently

- [x] 3.1 Publish owner-approved immutable SDK/provider prerequisite revisions, including corrected Anthropic `206427960ce2`, without committed production replacements or Apache imports of Gateway implementation.
- [x] 3.2 Validate root/native/client/middleware builds, tests, vet, lint, examples, parity and UI integration against the registered baseline.
- [x] 3.3 Verify fresh-cache readonly standalone modules and the Apache/Gateway license boundary. Leave Gateway runtime mapping, authenticated host acceptance, and its privacy/bounds/fallback checks to the stacked consumer change.

## Verification

Passed on this Apache branch: `mise run build`, `mise run test`, `mise run vet`, `mise run lint`, `mise run fmt-check`, `mise run parity-check`, `mise run test-integration`, `mise run verify-module-resolution`, and `mise run verify-ai-gateway-boundary`. Published Anthropic revision `206427960ce24c946cffd7c7bbe0f37b3930c5a7` resolves through the public Go proxy as `v0.0.0-20260923173532-206427960ce2`. No provider `recorded/` or `upstream/` fixture inputs changed; live-provider acceptance remains unproven.
