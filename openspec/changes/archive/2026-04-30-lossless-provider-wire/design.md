## Context

The `provider/` package defines the canonical Go types for V4 language-model interactions: `LanguageModel`, `CallOptions`, `Message`, `Tool`, `ContentPart`, `StreamPart`, `GenerateResult`, `APICallError`. These types were designed for in-process Go ergonomics and use sealed interfaces (`Message`, `Tool`, `UserContentPart`/`AssistantContentPart`/`ToolMessageContentPart`, `ProviderOption`) plus `json:"-"` tags to keep complex fields out of `encoding/json`.

In-process this works well: type switches are clean, content typing is enforced at compile time. Across a wire it does not work at all: the runtime type cannot be reconstructed from JSON. Concretely, `CallOptions.Prompt`, every `ProviderOptions` field, `StreamPart.FileData`, `StreamPart.Error`, and the unexported fields on `APICallError` (`message`, `cause`) all drop on the boundary between two Go processes.

We are about to add a hosted Grafana provider whose entire job is to be a transparent transport for `LanguageModel` calls — one Go process forwards `DoStream`/`DoGenerate` to another over HTTP. The wire must be lossless. The earlier `add-grafana-provider` design picked Connect/protobuf to address this; the user has since revisited that choice. Upstream Vercel AI SDK already solves the same problem with its `gateway` package using plain JSON+SSE over HTTP, with no DTO layer — the runtime types map natively to wire JSON because TypeScript discriminated unions ARE JSON.

This change adopts the same shape in Go: refactor the provider runtime types so they are themselves JSON-serializable losslessly, and add a thin `provider/wire/` HTTP+SSE transport module on top. The internal provider wire is JSON; the `@ai-sdk/react` SSE remains unaffected at the orchestration layer.

## Goals / Non-Goals

**Goals:**

- Provider types in `provider/` round-trip losslessly through `encoding/json` for all fields that matter to a remote `LanguageModel` proxy: `CallOptions` (including `Prompt` and `ProviderOptions`), `StreamPart` (every variant), `GenerateResult`, `APICallError`.
- Single source of truth: the runtime type IS the wire payload. No parallel DTO layer, no codegen.
- Mirror the upstream Vercel AI SDK gateway shape: JSON request body for `Generate`, `text/event-stream` for `Stream`, single `/language-model` endpoint with a streaming header, `APICallError`-shaped error envelopes.
- Preserve `provider.APICallError.IsRetryable` and other API-call-error metadata across the wire so `aisdk.StreamText`'s retry semantics work identically against a remote provider.
- Keep the Anthropic provider behavior bit-identical (conformance tests pass unchanged) after the type refactor.
- Land before `add-grafana-provider` so that change becomes a thin client of `provider/wire/`.

**Non-Goals:**

- Cross-language wire (TypeScript/etc. clients). The wire is Go-to-Go for now. JSON is incidentally interoperable, but no design effort is spent on it.
- Wire compatibility with OpenAI, Anthropic, or `@ai-sdk/react` formats. The internal provider wire is its own JSON shape that mirrors `LanguageModelV4*` upstream types.
- Backward compatibility with the current `Message`/`*ContentPart`/`Tool` interface API. The codebase is in active development and we are explicitly free to break call sites.
- Server-side hosted endpoint implementation. Tracked separately in `grafana-assistant-app`.
- New encoding formats (protobuf, CBOR, msgpack). JSON only.
- Optimizing wire size. JSON+base64 is acceptable for an internal Go-to-Go provider proxy.

## Decisions

### D1. Refactor sealed-interface unions to flat discriminated structs

Replace `Message`/`Tool`/`UserContentPart`/`AssistantContentPart`/`ToolMessageContentPart`/their concrete implementations with flat structs discriminated by a typed string field, mirroring how `provider.StreamPart` is already modeled and how upstream `LanguageModelV4Message`/`LanguageModelV4Content`/`LanguageModelV4FunctionTool` map directly to JSON.

Concrete shapes:

- `Message{Role Role, Content []ContentPart, ProviderOptions ProviderOptions}` — single struct, role-discriminated. Constructor helpers `NewSystemMessage(text)`, `NewUserMessage(parts...)`, `NewAssistantMessage(parts...)`, `NewToolMessage(parts...)` preserved for ergonomics; they all return `Message{Role: ..., Content: ...}`.
- `ContentPart{Type ContentPartType, ...all fields...}` — single flat struct holding the union of fields used by every part type (text, file, reasoning, tool-call, tool-result, custom, reasoning-file, tool-approval-response). The set of populated fields depends on `Type`.
- `Tool{Type ToolType, Name, Description, InputSchema, InputExamples, Strict, ID, Args, ProviderOptions}` — single flat struct discriminated by `Type` (`function` vs `provider`). Function tools use `Description`/`InputSchema`/`InputExamples`/`Strict`/`ProviderOptions`; provider tools use `ID`/`Args`. Unset fields are simply zero/nil.

**Why**: this is the upstream-aligned shape (`@ai-sdk/provider` V4 types are TS discriminated unions that serialize 1:1 to JSON). It makes the runtime type the wire payload, eliminating DTO duplication. It mirrors the existing `StreamPart` pattern in the same package, so the codebase has one consistent way of expressing variants. The trade-off is losing the compile-time guarantee that "a user message contains only user-allowed parts" — same trade-off upstream TS already accepts; runtime validation lives at producer/consumer boundaries.

**Alternative considered: keep sealed interfaces and add `MarshalJSON`/`UnmarshalJSON` per interface family with discriminator dispatch (Approach B from the prior discussion).** Rejected because it produces two mental models (structural in Go, discriminated on the wire), requires a registry-like decoder per interface family, and is a larger surface area for silent drift when new variants are added. The "compile-time content-typing" benefit it preserves does not appear to be load-bearing in the current codebase: orchestration already operates on the union, and the Anthropic provider type-switches by content kind regardless.

**Alternative considered: parallel `wire.*` DTO types in `provider/wire/`.** Rejected because it duplicates the type tree, requires conversion functions for every field, and offers no benefit JSON+flat-struct doesn't already provide.

**Alternative considered: protobuf + Connect (the prior `add-grafana-provider` design).** Rejected because both ends of the wire are Go, the upstream reference uses JSON+SSE, and the maintenance cost of two parallel type trees + codegen is unjustified for our use case.

### D2. ProviderOptions becomes a typed map alias with lossless `MarshalJSON`/`UnmarshalJSON`

`type ProviderOptions = map[string]ProviderOption` already exists conceptually but is used inline as `map[string]ProviderOption`. Promote it to a named type so it can carry custom marshalers:

- **Marshal**: iterate the map, `json.Marshal` each `ProviderOption` value (typed providers serialize their concrete struct; `RawProviderOption` writes its raw bytes), emit the resulting `map[string]json.RawMessage`.
- **Unmarshal**: parse a `map[string]json.RawMessage`, wrap each entry as `RawProviderOption{Key: k, Raw: v}`. Consumers reach typed values via the existing `ResolveOption[T]` helper, which already handles the `RawProviderOption` case.

Every `ProviderOptions` field across the provider package gains `json:"providerOptions,omitempty"` (replacing `json:"-"`). No call site needs to change for typed options to round-trip; `ResolveOption[T]` is already the canonical consumer path.

**Why**: the `RawProviderOption` pattern was introduced precisely to handle "a typed option came back from the wire as JSON" — this change just extends it to apply at every wire boundary uniformly. Asymmetry is acceptable: a typed option goes out as `AnthropicOptions{...}`, comes back as `RawProviderOption{Raw: ...}`. The consumer that knows the key calls `ResolveOption[AnthropicOptions]` to recover the typed view.

### D3. APICallError fields become exported and JSON-tagged; `cause` stops at the wire

```go
type APICallError struct {
    Message           string              `json:"message"`
    StatusCode        int                 `json:"statusCode"`
    URL               string              `json:"url,omitempty"`
    RequestBodyValues any                 `json:"requestBodyValues,omitempty"`
    ResponseHeaders   map[string][]string `json:"responseHeaders,omitempty"`
    ResponseBody      string              `json:"responseBody,omitempty"`
    IsRetryable       bool                `json:"isRetryable"`
    Data              json.RawMessage     `json:"data,omitempty"`
    cause             error               // in-process only; not serialized
}
```

`Error()` reads `Message` directly. `Unwrap()` returns `cause` for in-process `errors.Is`/`errors.As`. The wire side reconstructs an `APICallError` with `cause=nil`. `IsRetryable` is now a first-class JSON field, so `aisdk.StreamText`'s retry decisions key on the same value regardless of whether the model is local or remote.

**Why**: the previous unexported `message`/`cause` design was meant to force construction through `NewAPICallError`, but it makes the type unserializable. The retryability bit is the only piece the wire really needs to preserve; `cause` is a Go-internal chain that has no meaning across processes. We accept losing it on the boundary, same compromise upstream TypeScript makes.

### D4. Drop `StreamPart.Error error`; add `*APICallError` field for `PartError`

`StreamPart.Error error` is unserializable (any Go error type) and `json:"-"`. Replace it with `APICallError *APICallError` (`json:"apiCallError,omitempty"`) populated only when `Type == PartError`. Producers wrap any error into an `APICallError` (already the convention in `anthropic/wrap_api_error.go`). Stream-part error events on the wire carry the full retryable/non-retryable signal without needing a separate "error envelope" type.

> **Superseded (encode) by `provider-wire-upstream-full-compat`, 2026-07-07.**
> The `PartError` stream part now *emits* `{"type":"error","error":{...}}`
> (upstream `LanguageModelV4StreamPart` shape) instead of `apiCallError`, and
> `StreamPart.FileData` emits the upstream `data` tagged union instead of the
> flat `fileData` field. Decoding remains tolerant of both the legacy
> (`apiCallError` / `fileData`) and the upstream shapes, so mixed-version peers
> keep interoperating. The `*APICallError` in-memory field is unchanged.

`StreamPart.FileData []byte` gains `json:"fileData,omitempty"`. Go's `encoding/json` base64-encodes `[]byte` natively, so no special handling is needed. This adds ~33% wire overhead vs raw bytes; acceptable for an internal Go-to-Go provider proxy where file sizes are bounded by upstream LLM provider limits.

### D5. Wire transport: JSON over HTTP + SSE, single endpoint with header switch

Mirror the upstream Vercel `gateway-language-model.ts` shape. Single endpoint:

```
POST <baseURL>/language-model
  Content-Type: application/json
  ai-language-model-specification-version: 4
  ai-language-model-id: <modelID>
  ai-language-model-streaming: true|false
  Authorization: Bearer <token>            (set by provider, not wire)
  X-Grafana-Id: <user-id-token>            (set by provider, optional)

Body: JSON of CallOptions (+ any wire envelope fields, see open question O1)
```

- `streaming: false` → response is `application/json` of `GenerateResult`. Errors are HTTP non-2xx with body `{ "type": "error", ...APICallError fields... }`.
- `streaming: true` → response is `text/event-stream`. Each event is `data: <JSON of StreamPart>\n\n`. Stream end is normal HTTP body close (no `[DONE]` sentinel). Mid-stream errors emit a `StreamPart{Type: PartError, APICallError: ...}` event then close.

`provider/wire/` exports:
- Path/header constants (`PathLanguageModel`, `HeaderModelID`, `HeaderStreaming`, `HeaderSpecVersion`).
- `EncodeCallOptions`/`DecodeCallOptions` for the request body.
- `WriteSSEStreamPart`/`ReadSSEStreamPart` for chunked SSE encoding.
- `WriteErrorResponse`/`DecodeErrorResponse` for HTTP error envelopes.

Both client (Grafana provider) and server (hosted assistant) use the same `provider/wire/` package — single source of truth on the wire format.

**Why one endpoint with header switch**: matches upstream Vercel gateway 1:1, which is the closest reference point. Easier for any future shared infra (rate limiting, observability, routing) that doesn't need to distinguish stream vs unary.

### D6. SystemMessage uses unified `content: []ContentPart` on the wire

After the flatten, system messages serialize as `{role: "system", content: [{type: "text", text: "..."}]}` rather than the upstream TS shape `{role: "system", content: "..."}`. This deviation is intentional: the wire is Go-to-Go internal, and uniformity (every role uses `[]ContentPart`) is simpler than special-casing one role. The `NewSystemMessage(text)` helper continues to accept a string and packs it into `[TextContentPart]` automatically.

> **Superseded by `provider-wire-upstream-full-compat`, 2026-07-07.** The wire is
> no longer Go-to-Go internal: a stock upstream `@ai-sdk/gateway` client
> interoperates with the hosted server. `Message.MarshalJSON` now *emits* the
> upstream `{role:"system", content:"..."}` string shape, while
> `UnmarshalJSON` stays tolerant of both the string and the `[]ContentPart`
> array forms.

**Why deviate**: cross-language compatibility was explicitly out of scope (Goal/Non-Goal). Internal uniformity wins.

### D7. Producer-side validation, consumer trusts the wire

With flat structs there is no compile-time check that "a user message can hold only text/file parts". Validation happens at producer boundaries:

- `aisdk` orchestration validates when constructing prompts from the user-facing message types.
- Provider implementations may validate during `DoStream`/`DoGenerate`, surfacing problems as `Warning`s or `APICallError`s.
- The wire layer does not validate (consumes JSON as-is).
- Constructor helpers (`NewUserMessage`, etc.) accept `...ContentPart` variadic; misuse is detectable in code review and caught by integration tests, not the type system.

**Why**: matches upstream TS behavior, avoids runtime overhead in the hot path, and concentrates validation responsibility at boundaries that already have validation infrastructure (orchestration, provider).

### D8. Migration: one cohesive change, conformance fixtures hold behavior

Because the refactor touches every call site that constructs `SystemMessage{...}`, `TextContentPart{...}`, `FunctionTool{...}` etc., it must land as a single coherent change. Migration plan:

1. Refactor `provider/` types in place (message.go, content.go, language_model.go, stream_part.go, api_call_error.go, provider_option.go).
2. Update orchestration in the root `aisdk` package (`convert.go`, `streamtext.go`, `text.go`, `tool.go`, `chunk.go`, `message.go`, `message_json.go`).
3. Update `anthropic/` module (`convert_request.go`, `convert_response.go`, `convert_stream.go`, `wrap_api_error.go`).
4. Update `fallback/`, `middleware/`, `output/`, `registry/`, `schema/`.
5. Update all unit tests to the new type shape.
6. Add `provider/wire/` (routes, request/response envelopes, SSE encode/decode, error envelope) and unit tests.
7. Run the existing conformance suite (`test/conformance/`) end-to-end. Expected output is byte-identical because the orchestration layer (`UIMessageChunk` SSE) is unchanged — only the provider-internal type shapes moved.
8. Add wire round-trip tests covering every `StreamPartType`, every `ContentPartType`, every `Tool` variant, every `APICallError` field, and every notable `CallOptions` field.

The conformance suite is the safety net: if Anthropic-via-fixtures still produces the same `expected.jsonl`, the type refactor is behavior-preserving.

### D9. Remove protobuf placeholders

Delete the empty `provider/wire/proto/` and `provider/wire/wirepb/wirepbconnect/` directories. The `provider/wire/` package is repurposed as the JSON+SSE transport package described in D5. No Connect dependency, no codegen step, no `buf.gen.yaml`.

## Risks / Trade-offs

- **Loss of compile-time content-type checking** (e.g. user message can no longer reject tool-call parts at compile time) → Mitigation: constructor helpers + producer-side validation in orchestration; conformance tests catch regressions; Code review for new construction sites.
- **`cause` chain stops at wire boundary** (in-process `errors.Unwrap` works only on the side that originated the error) → Mitigation: `APICallError.Message`, `StatusCode`, `ResponseBody`, `Data` carry enough context for retry/log/metric decisions; cross-process error attribution uses `URL`+`StatusCode` instead of cause chains.
- **`ProviderOption` typed-vs-raw round-trip asymmetry** (typed option marshals as concrete struct, comes back as `RawProviderOption`) → Mitigation: this is the existing pattern; `ResolveOption[T]` already handles both cases. Documented clearly in `provider_option.go` doc comment.
- **Refactor surface area is large** (every call site that constructs messages, content parts, or tools) → Mitigation: do it as one coherent change before adding the Grafana provider; conformance suite gates merge; Anthropic provider behavior unchanged so existing integration tests catch regressions.
- **Single big `ContentPart` struct accumulates many fields** → Mitigation: matches the existing `StreamPart` style; alternative (one struct per content type with shared methods) re-introduces type-safety overhead this change explicitly removes.
- **Base64 file data inflates wire size by ~33% vs raw bytes** → Mitigation: acceptable for an internal Go-to-Go provider proxy with bounded file sizes; future binary-friendly transport (gRPC/Connect/HTTP/2 binary framing) is not blocked by this design.
- **Anthropic provider has many type-switch sites** → Mitigation: most switches are on a discriminator value already; the change is largely mechanical (`case provider.TextContentPart:` becomes `case provider.ContentPartTypeText:` after switching to `cp.Type` keying).

## Migration Plan

This change is breaking and lands as a single PR. No deprecation window, no compatibility shim. The codebase is in active development per `AGENTS.md` and `add-grafana-provider` is paused pending this work, so the only consumers of the breaking surface are in this repo and the not-yet-tagged `grafana-assistant-app` integration.

Steps for the implementer:

1. Land the type refactor in `provider/` and propagate compile errors out through `aisdk`, `anthropic`, `fallback`, `middleware`, `output`, `registry`, `schema`. Keep existing tests passing.
2. Add `provider/wire/` package with HTTP+SSE helpers and full unit tests.
3. Add round-trip tests that exercise every `StreamPart`, `ContentPart`, `Tool`, and notable `CallOptions`/`GenerateResult`/`APICallError` field through `provider/wire/`.
4. Run `make check` (fmt + vet + test). Run conformance suite. Confirm Anthropic byte-identical output against existing `expected.jsonl`.
5. Update `add-grafana-provider` proposal/design/tasks separately to drop protobuf and target `provider/wire/`. That work happens in its own change after this lands.

Rollback: revert the PR. There is no persisted wire state to migrate; all callers either compile or do not.

## Open Questions

- **O1. Wire envelope shape**: should the request body be a separate `wire.ModelCallRequest{ModelID string, Options CallOptions}` struct, or should `ModelID` live exclusively in the `ai-language-model-id` HTTP header (upstream's choice) with the body being just `CallOptions`? Recommended: header for `ModelID` to mirror upstream; the body is then the natural `CallOptions` JSON.
- **O2. Path prefix**: `POST /language-model` (upstream gateway path) vs `POST /provider/language-model` vs `POST /v4/language-model`? Recommended: `POST /language-model` for upstream alignment; versioning lives in `ai-language-model-specification-version`.
- **O3. SSE event-name discrimination**: every event uses `data: <JSON>` with no `event:` field, or include `event: <type>` for easier in-browser inspection? Recommended: data-only, type embedded in JSON (`StreamPart.Type`). Matches upstream gateway and avoids two parsers.
- **O4. Naming**: `provider/wire/` vs `provider/transport/` vs `provider/jsonwire/`? Recommended: keep `provider/wire/` (already scaffolded as a directory; "wire" is conventional in this codebase per `AGENTS.md`).
- **O5. Should the typed `ProviderOptions` map alias be exported as `provider.ProviderOptions` (replacing inline `map[string]ProviderOption`)?** Recommended: yes, so the custom marshalers attach to a single named type and call sites read more cleanly. Existing `ProviderMetadata` is already a typed alias; same pattern.
- **O6. `RequestBodyValues any` on APICallError uses `interface{}` and is hard to round-trip**: should it become `json.RawMessage` for wire-friendliness? Recommended: yes — the field is captured opaquely from the provider call anyway; `json.RawMessage` preserves the JSON without forcing a type assertion.
