## Why

The upstream Vercel AI SDK uses LanguageModelV3/V4 type shapes that our Go port has not caught up with. The current upstream V3 and V4 specs are identical for the types changed here (Usage, FinishReason, ResponseMetadata). This is the first of three sequential PRs (#66 from #32) that reshapes existing core types to match current upstream semantics. It must land first because PR 2 (content model expansion) and PR 3 (tool types) depend on these type shapes.

## What Changes

- **BREAKING**: `Usage` restructured from flat fields + optional detail sub-structs into nested `InputTokenUsage`/`OutputTokenUsage`. `TotalTokens` removed entirely. `InputTokenDetails` and `OutputTokenDetails` deleted.
- **BREAKING**: `FinishReason` changes from a `string` type alias to a struct with `Unified` (standardized) and `Raw` (provider-specific) fields. `StreamPart.RawFinishReason` removed (folded into `FinishReason.Raw`).
- **BREAKING**: `Metadata` type alias renamed to `ProviderMetadata` for output contexts. `ProviderOptions` verified consistent for input contexts.
- **BREAKING**: `ResponseMetadata` slimmed to only `ID`, `Timestamp`, `ModelID`. `Headers` and `Body` move to result-level `GenerateResult.Response` as an extended struct.
- `SpecificationVersion()` return value changes from `"v3"` to `"v4"`.

## Capabilities

### New Capabilities
- `provider-v4-core-types`: Requirements for the V4 core type shapes -- Usage, FinishReason, ProviderMetadata, ResponseMetadata, and specVersion. Covers the type contracts that all providers and the orchestration layer must satisfy.

### Modified Capabilities

(none -- the empty `provider-v4-types` and `provider-v4-stream` specs are not yet populated, so no existing requirements change)

## Impact

- `provider/types.go` -- Usage, FinishReason, Metadata/ProviderMetadata type definitions
- `provider/stream_part.go` -- StreamPart fields (RawFinishReason removal, FinishReason type change, Usage type change)
- `provider/language_model.go` -- ResponseMetadata, GenerateResult, StreamResult, LanguageModel interface
- `anthropic/` -- provider implementation (Usage construction, FinishReason mapping, response metadata)
- Orchestration layer (`streamtext.go`) -- usage aggregation, finish reason handling
- SSE serialization -- UIMessageChunk usage/finish encoding
- All tests touching these types across both modules
