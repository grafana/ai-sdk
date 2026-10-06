## Why

The SDK currently supports free-form text generation via `StreamText`/`GenerateText` but has no structured output capability. Vercel's AI SDK provides `Output.object()`, `Output.array()`, `Output.choice()`, and `Output.json()` as first-class citizens integrated into `generateText`/`streamText`. Without structured output, users of this Go SDK cannot reliably extract typed data from LLMs -- the most common use case beyond chat (classification, extraction, data generation, form filling). The provider-level `ResponseFormat` plumbing already exists but nothing orchestrates schema generation, response parsing, or validation on top of it.

## What Changes

- New `output` package providing the `Output` interface and factory functions: `Object[T]()`, `Array[T]()`, `Choice()`, `JSON()`
- New `Output` field on `StreamTextParams` accepting any `Output` implementation
- `StreamText`/`GenerateText` pass the output's `ResponseFormat` to the provider and validate the LLM response against the schema on completion
- Partial JSON streaming via `PartialOutputStream()` returning `<-chan json.RawMessage` on `StreamTextResult`
- Element-level streaming for array mode via `ElementStream()` returning validated complete elements
- Typed result access via generic `output.Value[T](result)` free function
- Convenience wrappers: `GenerateObject[T]()` and `StreamObject[T]()` that delegate to `GenerateText`/`StreamText` internally
- New JSON schema infrastructure: generation from Go struct tags (via `invopop/jsonschema`) and validation of LLM responses (via `santhosh-tekuri/jsonschema`)
- Support for loading JSON schemas from files as an alternative to struct-based generation
- Two new external dependencies added to the root module: `invopop/jsonschema`, `santhosh-tekuri/jsonschema`

## Capabilities

### New Capabilities

- `structured-output`: The orchestration layer for structured data generation. Covers the `Output` interface and its sealed implementations (object, array, choice, json, text), integration with `StreamText`/`GenerateText` params and result types, response parsing and validation, partial object streaming, element streaming for arrays, typed result accessors, and convenience `GenerateObject`/`StreamObject` wrappers.
- `json-schema`: JSON schema infrastructure for the SDK. Covers schema generation from Go struct tags using `invopop/jsonschema`, schema validation of LLM responses using `santhosh-tekuri/jsonschema`, loading schemas from JSON files, and a unified `Schema` type that bridges generation and validation through `json.RawMessage`.

### Modified Capabilities

None. No existing specs to modify.

## Impact

- **Root package (`aisdk`)**: `StreamTextParams` gains an `Output` field. `StreamTextResult` gains `PartialOutputStream()`, `ElementStream()`, and `Output()` methods. `GenerateTextResult` gains an `Output` field. The `run()` loop in `streamtext.go` must set `ResponseFormat` from the output spec and run validation on the final step.
- **New package (`output/`)**: Houses the `Output` interface, factory functions, and typed accessor helpers. Depends on the `json-schema` infrastructure.
- **New package or internal (`jsonschema/` or internal)**: Wraps `invopop/jsonschema` for generation and `santhosh-tekuri/jsonschema` for validation behind a unified API.
- **Provider package (`provider/`)**: No changes needed. `ResponseFormat` and `CallOptions` already support JSON mode with schema.
- **Anthropic module (`anthropic/`)**: Currently warns on `ResponseFormat`. Will need to either implement JSON mode support or continue warning (provider-level concern, outside this change's scope).
- **Dependencies**: Root module gains two new external dependencies, breaking the previous stdlib-only policy (already approved).
- **Wire compatibility**: The SSE/chunk format for `@ai-sdk/react` needs no changes for structured output. The `partialOutputStream` is a server-side concern; the SSE stream continues to carry text deltas that the client can parse.
