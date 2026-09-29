## 1. Establish Gateway regression evidence

- [x] 1.1 Confirm the registered upstream baseline, current parity map, ProviderWire schema, and exact pinned Gateway client behavior; use the Apache file-input producer source contract from #234.
- [x] 1.2 Generate focused registered-client requests for all four file arms in user, assistant and supported tool-result positions, including binary/base64, selected empties, filename presence, and scoped opaque options. Regenerate only reviewed semantic goldens.
- [x] 1.3 Add production Go golden replay assertions for selections, media types, filename presence, scopes and invocation counts in both modes; retain comprehensive-golden rejection for unrelated deferred families.

## 2. Validate candidate prerequisites

- [x] 2.1 Keep published SDK/provider/middleware requirements on already-merged revisions without production replacements or root-workspace Gateway registration.
- [x] 2.2 Verify the Gateway candidate workspace, merged internal pins, and Apache/Gateway source boundary; leave standalone release validation to the release gates.

## 3. Enable bounded Gateway file mapping

- [x] 3.1 Extend private file DTOs and shallow mapping for user/assistant files and supported nested tool-result files in unary and streaming requests, preserving selection, order, media type and filename presence.
- [x] 3.2 Preserve message/file-entry scoped opaque JSON, including empty namespace objects; apply shared host protections and selected-backend filtering to file-entry and function-tool scopes without changing existing call-level options, body headers, or text-part options.
- [x] 3.3 Make focused golden replays pass without enabling reasoning files, generated outputs, custom content, approvals or provider-executed history.
- [x] 3.4 Add negative handler tests for missing/null/inactive members, mixed arms, invalid references, role unions and reserved namespaces with zero execution-boundary, resolver and model calls.
- [x] 3.5 Test encoded complete-request byte limits below/at/above the boundary, cancellation, no URL fetch, safe errors and unchanged response/SSE contracts.
- [x] 3.6 Preserve text-only fallback eligibility for empty message-option objects retained by the selected-backend policy, while rejecting files, backend-relevant active options and effectful history; invalid or reserved namespaces fail earlier.

## 4. Prove cross-client host and privacy behavior

- [x] 4.1 Compare registered Vercel and independent Go semantic file requests in both execution modes.
- [x] 4.2 Exercise authenticated real-command user-file and nested file-result calls from both clients against candidate Anthropic source, asserting native inline-text, URL and PDF mapping and supported response consumption.
- [x] 4.3 Use hostile markers in file arms, filenames, URLs/query strings, references and options to prove metadata-only logs/records/metrics and safe errors on success, failure and cancellation.
- [x] 4.4 Run ProviderWire, Gateway command and Go suites; retain deferred-family, byte-boundary, privacy and lifecycle regression checks.

## 5. Document and close the consumer boundary

- [x] 5.1 Clarify direct file support and text-only fallback in the Go-client guide, remove work-package-only guidance, and retain the source-offer obligation in the container guide.
- [x] 5.2 Review fixture provenance and affected request/UI/object expectations; update `PARITY.md` only for the stable Gateway evidence change, without synthetic recorded provider inputs.
- [x] 5.3 Run candidate-source build, tests, vet, lint, formatting, examples, integration, parity, merged-pin checks, strict OpenSpec validation and the AI Gateway license-boundary check.

## Verification

Registered reference: `test/conformance/upstream.yaml` at `08ae5ad05bc12496dd1ffcf64e34419e0831300d`. Passed: `mise run build`, `mise run test-short`, `mise run vet`, `mise run lint`, `mise run fmt-check`, `mise run test-integration`, `mise run test-providerwire-v4`, `mise run test-ai-gateway-source-integration`, `mise run typecheck-conformance`, `mise run parity-check`, `mise run verify-merged-pins`, `mise run verify-gateway-workspace`, `mise run verify-ai-gateway-boundary`, `mise run verify-sdk-gateway-isolation`, `mise run test-module-policy`, `mise run test-ci-workflow`, `mise run lint-docs`, and `openspec validate --all --strict`. The authenticated Vercel/Go smoke covers native text files declared with non-text media and URL/PDF file-result content through candidate source. No provider recordings were added or rewritten. Generated media output (WP16), reasoning-file runtime (WP17), file fallback, live-provider acceptance, and deployed ingress behavior remain outside this deterministic evidence.
