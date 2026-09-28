## 1. Baseline and failing tests

- [x] 1.1 Recheck `@ai-sdk/openai@4.0.72` commit `4e8c387622ee1bb0d55841664416d38754d5c9a3` `openai-responses-prepare-tools.ts` and its allowedTools tests, comparing the Go `prepareTools` request path and existing `openai-responses-provider` coverage.
- [x] 1.2 Add focused synthetic OpenAI request tests for function, every supported hosted kind, custom and MCP server-label entry, canonical aliases/direct-name precedence (including two declarations with the same canonical alias and equivalent type-only entries resolving without an ambiguity warning), mixed ordering and duplicates, override and `auto`/`required` modes; confirm current Go request tests fail on mismatched kinds.
- [x] 1.3 Add tests for warnings/drop of ambiguous aliases, deferred/namespace functions and tool-search, unknown names as warned mapped function entries (including unknown-only), empty/all-dropped pre-HTTP errors, no-declaration omission, and reuse of caller-owned tools/options in unary and streaming requests. Assert warning feature/details and error boundary, not only JSON.

## 2. Provider implementation

- [x] 2.1 While preparing emitted declarations, index direct names and canonical aliases with correct allowed-tool entry shapes and unsupported reasons; keep direct resolution ahead of aliases and detect ambiguous aliases without mutating caller data.
- [x] 2.2 Resolve a non-nil `AllowedTools` selection in source order with warnings and filtering; fail only when no entries remain, then apply mode/default and override ordinary tool choice. Leave ordinary forced choices and mode runtime validation unchanged.
- [x] 2.3 Run the focused OpenAI module request tests and verify unary/stream requests, warnings, errors, and options immutability against pinned upstream behavior.

## 3. Evidence and validation

- [x] 3.1 Check whether a matching registered upstream request fixture or real recorded OpenAI input is available; import/capture only provenance-valid inputs and regenerate request snapshots if warranted. Otherwise document that synthetic request tests do not prove live OpenAI acceptance; do not invent `recorded/` or `upstream/` inputs.
- [x] 3.2 Run `cd providers/openai && go test ./...`, `mise run parity-check`, and `mise run validate-parity-baseline`; inspect resulting diffs, and update `test/conformance/PARITY.md` or `upstream.yaml` only if stable coverage status or an accepted support boundary changes.
