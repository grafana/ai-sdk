## Why

A successful Bedrock Converse `DoGenerate` can return no answer when the text is wrapped in `citationsContent`. It also drops explicitly empty text blocks, losing separators between signed reasoning blocks.

## What Changes

- Preserve non-null direct text (including empty text). If it is absent or null, project the non-null citation-content text fragments in their original order.
- Send both forms through the same JSON instruction extractor when that route is active. Keep reasoning and tool conversion intact.
- Add offline regressions at the public `DoGenerate` boundary. The tests use synthetic responses, not provider recordings or imported fixtures.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `bedrock-provider`: Non-streaming response conversion.

## Impact

The Bedrock Converse adapter returns more complete text content. No changes to requests, streaming, Mantle, citation/source APIs, or package pins.
