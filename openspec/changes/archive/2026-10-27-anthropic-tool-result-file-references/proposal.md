# Proposal

## Why

Issue #452: a tool-result `file` entry with provider-reference data (`{"anthropic":"file_…"}`) is dropped with the warning `unsupported tool content part type: file with data type: reference`, so uploaded attachments vanish on tool continuation. The registered upstream (`@ai-sdk/anthropic` 4.0.82, `b7f6255a`, fix `e254e96c`) sends them as `image`/`document` blocks with a `file` source plus the `files-api-2025-04-14` beta.

The prompt-conversion goldens in `test/conformance/testdata/anthropic-prompt-conversion/` were generated at 4.0.71 (#530) and not regenerated for 4.0.82 (#458), so they still encode the old drop.

## What Changes

- A tool-result `file` entry with reference data resolves the exact `anthropic` key of the reference and becomes, at its original position:
  - an `image` block with `source: {type: "file", file_id}` when the media type is `image/*`;
  - a `document` block with `source: {type: "file", file_id}` for every other media type (PDF, `text/plain`, archives, ...).
- Such an entry adds the `files-api-2025-04-14` beta once per request (deduplicated with user-content references and caller betas) for both `DoGenerate` and `DoStream`, on the direct provider and the shared Vertex conversion.
- An empty-string ID is accepted and sent as `file_id: ""`, as on the user-content path and upstream (`!= null`).
- Behavior change: a tool-result reference without an `anthropic` key now fails request conversion with the same error the user-content path returns (`anthropic: no provider reference found for provider anthropic; available providers: …`) and issues no provider request, instead of being dropped with a warning. This matches upstream, which ships it as a fix (`resolveProviderReference` throws); the Go user-content path already errors on the same input; no Go API changes.
- Inline text data in tool-result file entries keeps its existing warning; URL and inline-data entries are unchanged.
- Caller prompt data is not mutated, and mixed text/reference/URL/base64/bytes entries keep their order.
- Regenerate the synthetic Anthropic prompt-conversion goldens against the registered 4.0.82 oracle and add a synthetic case for referenced tool-result files, so conformance proves the new behavior.
- Out of scope: tool-result cache-control precedence, dropped-file warnings and hosted-MCP routing (#421); other file-data arms and URL/media-type edge cases (#109, #267); `containerUpload` in tool results (upstream has none there); key normalization of reference namespaces.

## Capabilities

### New Capabilities

### Modified Capabilities
- `anthropic-prompt-conversion`: "Tool-result file entries" no longer omits reference data (only inline text data and unsupported inline media still warn); "Wire-format and public API are unchanged" lists tool-result file references among the exercised behaviors; new requirement "Tool-result file references" specifies the `file`-source image/document blocks, the Files API beta, empty IDs, and the conversion error for a reference without an `anthropic` key.

## Impact

- `providers/anthropic/convert_request.go`: tool-result file conversion (`serializeToolOutput`, `convertToolResultFileContent`), an error return threaded through `appendToolResultBlock` and `convertToolContent` to `buildParamsWithCapabilities`, and a shared reference-ID resolver used by the user-content and tool-result paths.
- `providers/anthropic` tests: `convert_request_test.go`, `denial_presence_test.go` (signature change), `advisor_test.go`-style `buildParams` tests, and one table-driven `httptest` request test across direct/Vertex capabilities and unary/stream.
- `test/conformance/testdata/anthropic-prompt-conversion/`: `cases.json` (one new synthetic case, one description update) and regenerated `expected-results.jsonl`. No `recorded/` or `upstream/` provider inputs change.
- `test/conformance/upstream.yaml`: the `anthropic-mcp-result-fallthrough-warning` knownGap description is refreshed from 4.0.71 to 4.0.82 once regeneration confirms the warning is still present. `test/conformance/PARITY.md` is unchanged (the surface and its evidence location already exist).
- AI Gateway callers routed to the Anthropic backend now see referenced tool-result files forwarded, and receive a request error instead of a warning for references without an `anthropic` key. No Gateway schema or wire change.
- No public Go API, `provider` type, or UI chunk change.
