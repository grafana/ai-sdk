## Why

The provider data model (`provider/`) is designed for in-process Go ergonomics and is lossy on the wire: sealed interfaces (`Message`, `Tool`, `UserContentPart`/`AssistantContentPart`/`ToolMessageContentPart`, `ProviderOption`) cannot round-trip through `encoding/json`; key fields are tagged `json:"-"` (`CallOptions.Prompt`, every `ProviderOptions` map, `StreamPart.FileData`, `StreamPart.Error`); `APICallError` keeps `message` and `cause` unexported, dropping retryability and cause across any wire. Before adding the hosted Grafana provider, which must faithfully transport `provider.CallOptions`, `provider.StreamPart`, and `provider.GenerateResult` between two Go processes, we need a lossless representation. Upstream Vercel AI SDK already solves this with naturally JSON-serializable discriminated unions and a JSON+SSE wire (`gateway` package); we should mirror that approach so the runtime types ARE the wire types — single source of truth, no parallel DTO layer.

## What Changes

- **BREAKING**: Replace the `provider.Message` sealed interface and its four concrete variants (`SystemMessage`, `UserMessage`, `AssistantMessage`, `ToolMessage`) with a single flat `Message{Role, Content []ContentPart, ProviderOptions}` struct discriminated by `Role`. Constructor helpers (`NewSystemMessage`, `NewUserMessage`, etc.) preserved for ergonomics; new role-text shortcuts `UserText` / `AssistantText` added.
- **BREAKING**: Replace the three `*ContentPart` sealed interfaces (`UserContentPart`, `AssistantContentPart`, `ToolMessageContentPart`) and their concrete types (`TextContentPart`, `FileContentPart`, `ReasoningContentPart`, `ToolCallContentPart`, `ToolResultContentPart`, `CustomContentPart`, `ReasoningFileContentPart`, `ToolApprovalResponseContentPart`) with a single flat `ContentPart{Type, ...}` struct discriminated by `Type`, mirroring how `StreamPart` is already modeled. Add per-variant constructor helpers (`TextPart`, `FilePart`, `ReasoningPart`, `ReasoningFilePart`, `ToolCallPart`, `ToolResultPart`, `CustomPart`, `ToolApprovalResponsePart`) so producer call sites stay readable.
- **BREAKING**: Replace the `provider.Tool` sealed interface and its concrete types (`FunctionTool`, `ProviderTool`) with a single flat `Tool{Type, ...}` struct discriminated by `Type`.
- Add `MarshalJSON`/`UnmarshalJSON` to a typed `ProviderOptions` alias of `map[string]ProviderOption` so it round-trips losslessly: typed values marshal via their concrete struct, decode wraps unknown values as `RawProviderOption` (existing pattern, already supported by `ResolveOption[T]`).
- Remove `json:"-"` from `CallOptions.Prompt`, every `ProviderOptions` field across messages/content parts/tools/tool-result types, `StreamPart.FileData`, and `StreamPart.Error`.
- **BREAKING**: Promote `APICallError` fields to exported with JSON tags (`Message`, `IsRetryable`, etc.). Drop the in-process-only `cause` from the wire (`Unwrap` still works in process).
- **BREAKING**: Drop `StreamPart.Error error` (`json:"-"`); add `*APICallError` field on `StreamPart` populated for `Type == PartError`. The error chain stops at the wire boundary; retryability survives via the JSON-encoded `APICallError`.
- `StreamPart.FileData []byte` JSON-encodes natively as base64 via `json:"fileData,omitempty"`.
- Add a new `provider/wire/` package: HTTP+SSE transport helpers (request envelope, SSE encode/decode, error envelope, route + header constants). Mirrors upstream Vercel `gateway` shape: `POST /language-model` with `ai-language-model-streaming` header switch, JSON body for unary, `text/event-stream` response for streaming.
- Remove the empty `provider/wire/proto/` and `provider/wire/wirepb/wirepbconnect/` placeholder directories (no protobuf).
- Update Anthropic provider call sites and orchestration code (`aisdk` root package, `convert.go`, `streamtext.go`, `text.go`, `tool.go`) to construct messages, content parts, and tools via the new flat-struct shape.

## Capabilities

### New Capabilities

- `provider-wire`: HTTP+SSE wire format and transport helpers in `provider/wire/` for transporting `provider.CallOptions`, `provider.StreamPart`, and `provider.GenerateResult` between two Go processes losslessly. Defines the request envelope, SSE event encoding for stream parts, error envelope, route paths, and header conventions.

### Modified Capabilities

- `provider-v4-content-model`: collapse `Message`/`SystemMessage`/`UserMessage`/`AssistantMessage`/`ToolMessage` and the three `*ContentPart` sealed interfaces (with their eight concrete content-part types) into two flat structs — `Message{Role, Content, ProviderOptions}` and `ContentPart{Type, ...}`. `CallOptions.Prompt` gains `json:"prompt,omitempty"`. `StreamPart.Error error` removed in favor of `*APICallError` field populated for `PartError`; `StreamPart.FileData` gains `json:"fileData,omitempty"`.
- `v4-tool-type-split`: collapse the `Tool` sealed interface and `FunctionTool`/`ProviderTool` concrete types into a flat `Tool` struct discriminated by `Type`.
- `typed-provider-options`: every `ProviderOptions` field across the provider package gains `json:"providerOptions,omitempty"` (replacing `json:"-"`); the typed `ProviderOptions` map type implements `MarshalJSON`/`UnmarshalJSON` for lossless round-trip via `RawProviderOption`.
- `api-call-error`: struct fields exported with JSON tags; `Message` becomes a public field (existing `Error()` method reads it); `cause` retained for in-process `Unwrap` but dropped on the wire; `RequestBodyValues` becomes `json.RawMessage`.

## Impact

- **`provider/` package**: substantial refactor of `message.go`, `content.go`, `language_model.go` (`Tool`, `CallOptions.Prompt`), `stream_part.go` (drop `Error`, gain `*APICallError`), `api_call_error.go`, `provider_option.go` (typed `ProviderOptions` alias). New `wire/` subpackage replaces the empty proto placeholders.
- **`aisdk` root package**: every site constructing or pattern-matching on `Message`/`*ContentPart`/`Tool` updates to the flat struct shape (`convert.go`, `streamtext.go`, `text.go`, `tool.go`, `message.go`, `message_json.go`, `chunk.go`).
- **`anthropic/` module**: `convert_request.go`, `convert_response.go`, `convert_stream.go`, `wrap_api_error.go`, all tests update to new shape. The provider's behavior is unchanged; only the type construction differs.
- **`fallback/`, `middleware/`, `output/`, `registry/`, `schema/`**: any direct use of the affected types updates.
- **Tests**: every test that constructs `SystemMessage{...}`, `TextContentPart{...}`, `FunctionTool{...}` etc. updates to the flat shape. Conformance suite (`test/conformance/`) keeps the same fixture-based equivalence; only the test setup updates.
- **Dependencies**: no new dependencies. Removes the implicit dependency on Connect/protobuf that the empty `wire/proto/` placeholders implied.
- **`add-grafana-provider` change**: must be updated after this lands — drops protobuf/Connect, becomes a thin HTTP+SSE client of `provider/wire/`. That update is separate work, not part of this change.
- **`@ai-sdk/react` SSE wire**: unaffected. This change touches the *internal* provider-to-provider wire only; consumer-facing UI SSE continues to flow through `WriteUIMessageStream` unchanged.
