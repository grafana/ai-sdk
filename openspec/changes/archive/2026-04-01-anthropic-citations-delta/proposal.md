## Why

The Anthropic stream adapter silently drops `citations_delta` events (#81). When using provider-executed tools like `web_search`, the Anthropic API sends inline citation references as `citations_delta` deltas interleaved with `text_delta` events. These are dropped because the delta handler in `convert_stream.go` has no case for them. This causes missing inline citation references, text fragmented into multiple separate content blocks (each text-start/text-end cycle creates gaps where citations should be), and broken rendering in frontends consuming the stream.

## What Changes

- Add `citations_delta` handling in the Anthropic stream adapter's delta handler, converting citation events to `PartSource` stream parts with appropriate `SourceInfo` (url or document source type).
- Add `citationDocuments` tracking to the `streamAdapter` struct, populated from user message file parts (PDF/text with citations enabled) at init and from streaming content blocks. Required to resolve `page_location` and `char_location` citations by document index.
- Add `citations_delta` handling in the non-streaming response converter for completeness.
- Update the existing `server-tools` spec to include citation handling requirements.

## Capabilities

### New Capabilities

_None_ — this is an extension of existing server tool support, not a new capability.

### Modified Capabilities

- `server-tools`: Add requirements for `citations_delta` streaming/non-streaming handling, `citationDocuments` tracking, and citation-to-source conversion for all citation types (`web_search_result_location`, `page_location`, `char_location`).

## Impact

- `anthropic/convert_stream.go` — New `citations_delta` case in delta handler, `citationDocuments` tracking on `streamAdapter`, `createCitationSource` helper function
- `anthropic/convert_response.go` — Citation handling in non-streaming path
- `anthropic/convert_request.go` — Extract citation documents from prompt file parts (init-time population)
- `anthropic/convert_stream_test.go` — Tests for citation delta conversion
- `openspec/specs/server-tools/spec.md` — Updated spec with citation requirements
