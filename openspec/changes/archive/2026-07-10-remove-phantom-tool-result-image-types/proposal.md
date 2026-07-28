## Why

`v4-tool-result-alignment` lists three image-specific `Type` values (`image-data`, `image-url`, `image-file-reference`) that are not included in `typed-string-enums` spec nor the shipped code (`provider/types.go`). Upstream marks them as deprecated in favour of `file-data` with an image `mediaType` (https://ai-sdk.dev/docs/reference/ai-sdk-core/model-message); commit `f093330` removed them. The spec was never updated.

## What Changes

- Correct *ToolResultContentValue expanded types* to the five shipped types, dropping the image variants and their scenarios.
- No code changes — `provider/types.go` already matches.

## Impact

Specs only: `openspec/specs/v4-tool-result-alignment/spec.md`.
