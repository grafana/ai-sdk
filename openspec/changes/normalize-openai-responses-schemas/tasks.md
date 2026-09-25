## 1. Capture the pinned provider contract

- [ ] 1.1 In `providers/openai`, add failing focused tests based on `@ai-sdk/openai` 4.0.71 normalizer and Responses request tests: recursive supported `propertyNames` removal across all schema-bearing keywords, boolean branches and unrelated fields preserved, `propertyNames: null` removed without warning, one compatibility warning per affected schema, and caller `json.RawMessage` unchanged.
- [ ] 1.2 Add failing regular and namespaced function-tool request tests for normalized input and optional output schemas with separate warnings, and JSON response-format tests with strict enabled/disabled; compare equivalent generate and stream request payloads and warnings.
- [ ] 1.3 Add failing local-error tests for top-level and nested boolean/non-string (excluding null) `propertyNames` in response, function input, and optional function output schemas; also test malformed `json.RawMessage` for each of those three schema positions. Assert both `DoGenerate` and `DoStream` return errors with zero HTTP requests.

## 2. Normalize OpenAI Responses request schemas

- [ ] 2.1 Implement a provider-private schema normalizer using immutable decoding/copying and the pinned JSON Schema keyword traversal; remove string-schema `propertyNames`, preserve other keywords and boolean definitions, return one upstream-compatible warning per affected schema, remove null `propertyNames` without warning, and reject unsupported non-null `propertyNames` values.
- [ ] 2.2 Route `applyResponseFormat` through the shared normalizer and thread its warnings/error through `buildParamsForProvider` before HTTP without changing response name, description, `json_object`, or strict flag behavior.
- [ ] 2.3 Route regular and namespaced `functionToolParam` input and optional `OpenAIToolOptions.OutputSchema` through the same normalizer; propagate errors and warnings through `prepareTools`, retaining per-schema warning cardinality and existing tool metadata.

## 3. Validate provider parity and fixture provenance

- [ ] 3.1 Run focused `providers/openai` tests and the module's full test suite, including existing request conversion tests; verify no caller schema mutation, matching generate/stream behavior, and no HTTP requests on local errors.
- [ ] 3.2 Inspect `test/conformance/openai/` for an authentic existing recording or matching pinned upstream input covering schema normalization; update a request snapshot only when its existing input provenance supports the new expectation. Do not create synthetic `recorded/` or `upstream/` chunks; document a remaining live-provider proof gap if no authentic input exists.
- [ ] 3.3 Run `mise run parity-check` for the behavioral change and relevant module-resolution checks; update `test/conformance/PARITY.md` only if stable coverage, evidence, support boundaries, or accepted deviations actually change. Ensure the package passes required checks independently against the registered baseline, without unrelated module or pin edits.
