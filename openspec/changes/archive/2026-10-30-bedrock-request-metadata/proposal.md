## Why

Issue #361 identifies missing typed request metadata for Amazon Bedrock. Valid
raw metadata already reaches Converse through opaque pass-through, but malformed
values also reach HTTP. Registered upstream `@ai-sdk/amazon-bedrock` 5.0.118
validates an optional string record and sends it at the request top level.

## What Changes

- Add typed `BedrockOptions.RequestMetadata` for invocation-log filtering and
  cost attribution, shared by Converse and ConverseStream.
- Validate raw metadata as a string record before sending HTTP, preserving
  omitted versus empty values and existing namespace precedence.
- Keep diagnostic errors free of metadata keys and values.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: typed and validated top-level Converse request metadata.

## Impact

Changes are limited to the Bedrock module, its guide, and capability spec. Valid
raw requests retain their wire shape. Previously accepted non-string metadata
becomes an error. No dependencies or upstream pins change.

## Non-goals

No AWS entry-count or string-length limits, logging, response metadata changes,
Mantle support, namespace resolver changes, or reasoning configuration changes.
