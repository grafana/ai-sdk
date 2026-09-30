## Why

Issue [#30](https://github.com/grafana/ai-sdk/issues/30) remains valid: TypeScript and Go conformance request capture both discard URL queries, so different API versions or repeated query values can falsely pass parity checks. Query-bearing provider URLs still exist at the registered upstream reference; the harness must retain this behavior-affecting input.

## What Changes

- Preserve escaped path and nonempty query in the existing request snapshot `path` field on both implementations; omit origin and fragment, retain original query spelling and pair order, and normalize a bare empty `?` away.
- Add focused TypeScript and Go request-target tests covering query presence, escaping, repeated values, queryless requests and empty query markers.
- Commit a TypeScript-generated, synthetic harness-only request snapshot with `api-version` and repeated parameters; replay it through Go's real loader/capture/comparator and demonstrate rejection of query-only mismatches.
- Document request-target semantics and expectation migration without changing provider response inputs or upstream pins.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `conformance-testing`: Request snapshots include queries in their request targets, and request comparison rejects differing versions, escaping or repeated-value order; shared harness regression evidence is distinct from provider recordings/imports.

## Impact

Implementation will touch `test/conformance/tools/common.mts`, `common.test.mts`, `test/conformance/request_snapshot_test.go`, and the request-target assignment in `runner.go`. Harness testdata will live outside provider `recorded/` and `upstream/` directories, with a small explicit regeneration entry point in the conformance tools. `test/conformance/README.md` will describe the format and regeneration command.

The JSON schema remains `{method,path,headers,body}`; existing queryless snapshots remain unchanged. Query-bearing expectations must be regenerated alongside the capture change, rather than accepting legacy path-only expectations. No public Go API, provider implementation, frontend/SSE protocol, module dependency or registered baseline change is proposed. Azure source is contextual evidence, not a proposal to register or implement an Azure provider.
