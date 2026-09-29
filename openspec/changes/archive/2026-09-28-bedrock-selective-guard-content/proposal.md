## Why

Bedrock currently sends every user text and image part as ordinary Converse content, even when that part requests selective `guardContent` protection; top-level `guardrailConfig` and the existing guardrail stream recording do not provide selective input protection. Issue #223 remains valid against the registered `@ai-sdk/amazon-bedrock` **5.0.90** baseline at `4e8c387622ee1bb0d55841664416d38754d5c9a3` (the issue's 5.0.88 reference is older).

## What Changes

- Propose typed Bedrock text-part and image-part controls for `guardContent`, with the text-only `guardContentQualifiers` enum; validate recognized controls in both typed and raw provider options, with modern `amazonBedrock`/legacy `bedrock` resolution.
- Convert only opted-in user text and inline image parts into Converse `guardContent` blocks, preserving unselected text/image and existing video, document, system, assistant, and tool conversion. Cover both Converse generate and ConverseStream requests.
- Import the byte-identical registered upstream `amazon-bedrock-guard-content-intervened.json` **unary** fixture through the existing generate replay contract, assert its response finish reason, trace metadata and usage, and add focused mixed-content, qualifier and malformed-option tests. Never fabricate recorded provider events.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: Add selective user text/inline-image guard-content request conversion, per-part option validation and unary intervention response regression coverage.

## Impact

Bedrock-only public per-part option types in `providers/bedrock/options.go`, option reading in `provider_options.go`, content/request serialization in `convert_messages.go` and `api_types.go`, focused Bedrock tests, and one authentic fixture plus expectations/index under `test/conformance/bedrock/upstream/`. This proposes (but does not approve) a new Go public API; owner/OpenSpec API approval is required before implementation. No provider-contract, Gateway file/provider-option transport, baseline pin, or global guardrailConfig behavior changes.
