## Context

The SDK orchestrates LLM calls via `StreamText`/`GenerateText`, which build `provider.CallOptions` and call `model.DoStream`/`model.DoGenerate`. The provider layer already has `ResponseFormat` with `Type` ("text"/"json"), `Schema` (`json.RawMessage`), `Name`, and `Description` -- but nothing in the orchestration layer generates schemas, sets `ResponseFormat`, or validates responses.

Vercel's AI SDK recently consolidated structured output into `generateText`/`streamText` via an `Output` specification (replacing the older standalone `generateObject`/`streamObject`). The `Output` object controls what `responseFormat` is sent to the provider, how to parse the final result, and how to stream partial results. We follow this same architecture.

The SDK currently uses `json.RawMessage` as opaque schema representation throughout (tool `InputSchema`, `ResponseFormat.Schema`). This continues to be the internal currency. Two external libraries handle the typed operations on top: `invopop/jsonschema` for struct-to-schema generation (rich struct tags: enum, pattern, title, etc.) and `santhosh-tekuri/jsonschema` for validating LLM responses against schemas.

## Goals / Non-Goals

**Goals:**

- Provide structured output as a first-class feature of `StreamText`/`GenerateText`, not a separate code path
- Support object, array, choice, and unstructured JSON output modes
- Enable Go-idiomatic schema definition via struct tags with `invopop/jsonschema`, including enum, pattern, format, title, default, and description
- Validate LLM responses against the provided schema before returning typed results
- Stream partial JSON objects during generation and stream validated array elements individually
- Provide typed convenience wrappers (`GenerateObject[T]`, `StreamObject[T]`) without duplicating orchestration logic
- Support loading schemas from JSON files as an alternative to struct reflection

**Non-Goals:**

- Implementing JSON mode in the Anthropic provider (it currently warns on `ResponseFormat`; provider support is a separate concern)
- Built-in retry/repair when the LLM produces invalid output (users can implement this via `PrepareStep` or application-level retry)
- Partial JSON schema validation during streaming (partials are best-effort; only the final result is validated)
- Custom schema DSL or builder API (struct tags and raw JSON cover the use cases)

## Decisions

### 1. Hybrid architecture: Output interface on StreamTextParams + typed wrappers

**Decision**: Add a type-erased `Output` interface to `StreamTextParams`. Provide generic convenience functions (`GenerateObject[T]`, `StreamObject[T]`) that wrap `GenerateText`/`StreamText` with typed result access.

**Rationale**: `StreamTextParams` is a concrete struct used across the entire orchestration layer. Making it generic would ripple through every function signature. A type-erased interface keeps the core untouched while generic wrapper functions provide type safety at the edges.

**Alternatives considered**:
- **(A) Fully generic `StreamText[T]`**: Would require generic `StreamTextResult[T]`, generic `StepResult[T]`, etc. Too invasive; Go generics don't propagate well through deep struct hierarchies.
- **(B) Separate `GenerateObject`/`StreamObject` with independent orchestration**: Duplicates the multi-step loop, tool execution, and SSE translation. Vercel moved away from this exact pattern.
- **(C) Hybrid (chosen)**: Output interface plugs into existing orchestration. Typed wrappers are thin: they set the Output field and type-assert the result. Single code path, type safety where it matters.

### 2. Output interface design

**Decision**: The `Output` interface is sealed (unexported marker method) with three capabilities:

```
Output interface:
    outputSpec()                                    // sealed marker
    ResponseFormat() *provider.ResponseFormat        // what to send to the provider
    ParseComplete(text string) (any, error)          // validate + parse final result
    ParsePartial(text string) (any, bool)            // best-effort parse of incomplete JSON
```

Factory functions return concrete implementations: `ObjectOutput[T]`, `ArrayOutput[T]`, `ChoiceOutput`, `JSONOutput`, `TextOutput`.

**Rationale**: Sealed interface follows the SDK's established pattern (`TextStreamPart`, `ContentPart`, etc.). Type-erased return (`any`) is necessary because `StreamTextParams` isn't generic. The type recovery happens via `output.Value[T](result)` generic free function that type-asserts the stored `any`.

### 3. Schema wrapping for arrays and choices

**Decision**: Follow Vercel's pattern. Arrays are wrapped in `{"type":"object","properties":{"elements":{"type":"array","items":<schema>}},"required":["elements"]}`. Choices are wrapped in `{"type":"object","properties":{"result":{"type":"string","enum":[...]}},"required":["result"]}`.

**Rationale**: LLMs cannot reliably produce bare JSON arrays or bare string values. Wrapping in an object with a known key is a proven pattern from Vercel's production experience. `ParseComplete` unwraps the outer object transparently.

### 4. invopop/jsonschema for schema generation

**Decision**: Use `invopop/jsonschema` for Go struct-to-JSON Schema conversion. Expose it via a generic `SchemaFor[T]()` function in the output package that returns `json.RawMessage`.

**Rationale**: invopop supports rich struct tags (enum, pattern, title, description, format, default, minLength, maxLength, minimum, maximum, examples, oneOf), the `JSONSchema()` interface for custom types, and `JSONSchemaExtend()` for post-generation modification. These features are critical for structured output: enum constraints guide LLM behavior and enable accurate validation. Without rich tags, users must manually patch schemas after generation, which is error-prone and not co-located with the type definition.

**Alternatives considered**:
- **google/jsonschema-go**: Single library for both generation and validation, uses Go generics. However, struct tags only support description -- no enum, pattern, title, default, etc. This means incomplete schemas that don't properly constrain LLM output and don't properly validate responses. The tag limitation is a fundamental usability problem for the primary convenience path.

### 5. santhosh-tekuri/jsonschema for validation

**Decision**: Use `santhosh-tekuri/jsonschema` v5 for validating LLM JSON responses against schemas. Compile the schema once per Output construction, validate on each completion.

**Rationale**: Full JSON Schema Test Suite compliance across all draft versions. Supports loading schemas from files, HTTP, strings, bytes, and `io.Reader`. Rich error messages with JSON pointer locations. Thread-safe compiled schemas. The validation library only sees `json.RawMessage` (the marshaled output from invopop or loaded from file), so the two libraries are fully decoupled.

### 6. json.RawMessage as the schema bridge

**Decision**: The internal schema representation throughout the SDK remains `json.RawMessage`. invopop generates a schema, which is marshaled to `json.RawMessage`. santhosh-tekuri compiles from those bytes. The `Output` implementations hold both forms: the `json.RawMessage` for the provider's `ResponseFormat.Schema`, and a compiled `*jsonschema.Schema` for validation.

```
invopop (generation)                    santhosh-tekuri (validation)
        |                                         ^
        | Marshal()                               | Compile()
        v                                         |
    json.RawMessage  ─────────────────────────────┘
        |
        └──> provider.ResponseFormat.Schema
```

**Rationale**: Decouples the two libraries completely. Either can be swapped without affecting the other. Also means users who bring their own `json.RawMessage` schema (from files, external tools, etc.) get the same validation path without needing invopop.

### 7. Partial streaming: json.RawMessage for partials, typed T for finals

**Decision**:
- `PartialOutputStream()` returns `<-chan json.RawMessage` -- raw partial JSON snapshots
- `ElementStream()` returns `<-chan json.RawMessage` -- complete validated array elements
- `output.Value[T](result)` provides typed final access via `json.Unmarshal` into `T`
- `output.TypedElementStream[T](result)` wraps `ElementStream()` with per-element unmarshal

**Rationale**: Go cannot express `DeepPartial<T>` at the type level. Partial JSON is inherently untyped -- fields may be missing, values truncated. `json.RawMessage` is honest about this. For array elements and final results, the data IS complete and validated, so typed access makes sense. Users who want typed partials can unmarshal `json.RawMessage` into their own pointer-heavy struct.

### 8. Integration point in StreamText's run() loop

**Decision**: Output integration happens at two points in `streamtext.go`:

1. **Before `DoStream` call** (line ~289): If `params.Output` is set, override `callOpts.ResponseFormat` with `params.Output.ResponseFormat()`. If `params.ResponseFormat` is also set, `Output` takes precedence.

2. **At stream finish** (line ~336-341): When the last step completes with `FinishReason == "stop"`, call `params.Output.ParseComplete(accumulatedText)` to validate and store the parsed output. If validation fails, store the error (accessible via `result.OutputError()`).

Partial streaming is handled by an additional goroutine that reads text deltas from the existing `fullStream` channel, accumulates them, and calls `ParsePartial` on each delta, forwarding changed partials to the `partialOutputStream` channel.

**Rationale**: Minimal changes to the existing orchestration. The run loop already accumulates text via `textBuilder`. The Output just observes the same text and adds parsing/validation on top. No changes to the provider interface or the streaming protocol.

### 9. Package layout

**Decision**:

```
aisdk/
  output/              New package
    output.go          Output interface + TextOutput
    object.go          ObjectOutput[T]
    array.go           ArrayOutput[T]
    choice.go          ChoiceOutput
    json.go            JSONOutput
    schema.go          SchemaFor[T](), file loading, validation wrapping
    value.go           Value[T](), TypedElementStream[T]()
```

**Rationale**: Separate package avoids bloating the root `aisdk` package with generic functions (which interact awkwardly with non-generic types in the same package). Import path `github.com/grafana/ai-sdk/output` is clean. The package depends on the root (for `StreamTextResult`, etc.) -- this creates a one-way dependency from `output` -> `aisdk` -> `provider`, consistent with the existing structure where `aisdk` imports `provider`.

**Open concern**: Circular dependency -- `output` needs `StreamTextResult` (from `aisdk`) and `aisdk` needs the `Output` interface (from `output`). Resolution: define the `Output` interface in the root `aisdk` package (alongside `StreamTextParams`). The `output` package provides implementations and generic helpers. This mirrors how `provider.LanguageModel` interface lives in `provider/` but implementations live in `anthropic/`.

### 10. Error type for invalid output

**Decision**: Define `ErrNoObjectGenerated` as a sentinel error. When `ParseComplete` fails, wrap the cause: `fmt.Errorf("aisdk: %w: %v", ErrNoObjectGenerated, cause)`. The error preserves the raw text, response metadata, and usage for debugging.

**Rationale**: Follows the SDK's error convention (sentinel errors via `errors.New("aisdk: ...")`, `fmt.Errorf` wrapping). Users can check `errors.Is(err, aisdk.ErrNoObjectGenerated)` and access the raw text to debug or retry.

## Risks / Trade-offs

**[Two external dependencies]** The root module goes from zero to two external deps. Both are well-maintained (invopop: active community fork; santhosh-tekuri: v5 stable) but pin the SDK to their release cycles.
-> Mitigation: Both are accessed through a thin wrapper in `output/schema.go`. Swapping either requires changing only that file.

**[Partial JSON parsing reliability]** Parsing incomplete JSON (for `PartialOutputStream`) is inherently fragile. Truncated strings, unclosed brackets, and partial numbers can all cause parse failures.
-> Mitigation: `ParsePartial` returns `(any, bool)` -- failures are silently skipped (no partial emitted). Only changed partials are forwarded to avoid noise. This matches Vercel's approach.

**[Type safety gap]** The Output interface is type-erased. Nothing prevents `output.Object[Recipe]()` being created but `output.Value[User](result)` being called at access time. This would fail at runtime, not compile time.
-> Mitigation: `Value[T]` returns a clear error on type mismatch. The convenience wrappers (`GenerateObject[T]`, `StreamObject[T]`) eliminate this gap entirely by coupling construction and access in a single generic function.

**[invopop schema generation vs LLM expectations]** invopop generates Draft 2020-12 schemas. Some LLM providers may expect older drafts (e.g., draft-07 for OpenAI). The generated schema may include features (like `$defs`, `$id`) that providers don't support.
-> Mitigation: The `SchemaFor[T]()` wrapper can strip `$schema`, `$id`, and flatten `$defs`/`$ref` for simple schemas. For complex cases, users load provider-compatible schemas from files.

**[Output on final step only]** Following Vercel's pattern, structured output parsing runs only when the last step's `FinishReason` is `"stop"`. If the LLM stops for another reason (length, content filter), no output is produced.
-> Mitigation: `result.OutputError()` exposes why output wasn't produced. The raw text is always available via `result.Text()`.
