## Why

Provider options are `map[string]json.RawMessage` everywhere -- a direct translation of TypeScript's `Record<string, JSONObject>`. In Go, this forces unnecessary JSON marshal/unmarshal round-trips within the same process: users create typed structs, marshal them to JSON, and providers immediately unmarshal them back. There is no wire boundary between construction and consumption; the JSON ceremony adds complexity, allocations, and loses type safety for no benefit.

## What Changes

- **BREAKING**: New `provider.ProviderOption` interface with `ProviderKey() string` method for typed option values
- **BREAKING**: All `ProviderOptions` fields change from `map[string]json.RawMessage` to `map[string]provider.ProviderOption` across `provider`, `aisdk`, and `anthropic` packages
- New `provider.RawProviderOption` wrapper for genuine JSON data from round-tripped provider metadata (SSE responses re-sent as input)
- New `provider.BuildProviderOptions()` helper for ergonomic option construction from variadic typed values
- New `provider.ResolveOption[T]()` generic helper that handles both typed (direct assertion) and raw (JSON unmarshal) option values
- Anthropic option types (`AnthropicOptions`, `AnthropicToolOptions`) gain `ProviderKey()` methods
- All anthropic consumption code switches from JSON unmarshal to type assertion via `ResolveOption`
- New `anthropic.CacheControl()` convenience helper for per-part cache control options
- `ConvertToModelMessages` bridge wraps `ProviderMetadata` JSON values in `RawProviderOption`
- Conformance test config adapts to new option construction pattern

## Capabilities

### New Capabilities
- `typed-provider-options`: The `ProviderOption` interface, `RawProviderOption`, `BuildProviderOptions`, and `ResolveOption[T]` -- the core typed option system in the provider package

### Modified Capabilities
- `provider-v4-core-types`: `CallOptions`, `FunctionTool`, and all message types change `ProviderOptions` field type from `map[string]json.RawMessage` to `map[string]provider.ProviderOption`
- `provider-v4-content-model`: All content part types change their `ProviderOptions` field type
- `anthropic-prompt-caching`: Cache control extraction switches from JSON unmarshal to typed assertion via `ResolveOption`
- `anthropic-tool-options`: Tool option consumption switches from JSON unmarshal to typed assertion via `ResolveOption`
- `conformance-testing`: Test config provider options construction adapts to new typed pattern

## Impact

- **Provider package**: Core type changes to `CallOptions`, all message types, all content parts, `FunctionTool`, `ToolResultOutput`, `ToolResultContentValue`
- **Root aisdk package**: `StreamTextParams`, `PrepareStepResult`, `SystemModelMessage`, `Tool` field types change; `ConvertToModelMessages` wraps metadata in `RawProviderOption`
- **Anthropic module**: All option consumption code in `convert_request.go`, `cache_control.go`, `reasoning.go` switches to type assertion; option types gain interface methods
- **Conformance tests**: Config structs and option construction adapt to new pattern
- **API consumers**: Breaking change -- any external code constructing `ProviderOptions` must switch from JSON marshaling to typed values
