# structured-output Specification

## Purpose

Define structured output configuration, parsing and validation behavior, partial snapshot and array element streaming guarantees, typed result access, and object-generation convenience APIs for `StreamText` and `GenerateText`.

## Requirements

### Requirement: Output interface on StreamTextParams

The system SHALL provide an `Output` interface in the root `aisdk` package that can be set on `StreamTextParams`. When `Output` is set, `StreamText`/`GenerateText` SHALL use its `ResponseFormat()` to configure the provider call and its `ParseComplete()` to validate the final response. The interface SHALL consist of three methods: `ResponseFormat() *provider.ResponseFormat`, `ParseComplete(text string) (any, error)`, and `ParsePartial(text string) (any, bool)`. No marker method SHALL be included.

#### Scenario: Output sets provider ResponseFormat

- **WHEN** `StreamTextParams.Output` is set to an `Output` implementation
- **THEN** the `CallOptions.ResponseFormat` sent to `model.DoStream` SHALL be the value returned by `Output.ResponseFormat()`

#### Scenario: Output takes precedence over explicit ResponseFormat

- **WHEN** both `StreamTextParams.Output` and `StreamTextParams.ResponseFormat` are set
- **THEN** the `Output.ResponseFormat()` value SHALL take precedence

#### Scenario: No Output specified

- **WHEN** `StreamTextParams.Output` is nil
- **THEN** behavior SHALL be identical to current `StreamText`/`GenerateText` with no structured output

### Requirement: Object output mode

The system SHALL provide an `ObjectOutput[T]` implementation that accepts a `schema.Schema` value, uses its `.JSON()` for the provider response format, and uses its `.Validate()` for validating the LLM response. The parsed result SHALL be accessible as a typed value. Since the `Schema` is pre-compiled, `output.Object[T]()` SHALL NOT return an error -- construction cannot fail when given a valid `Schema`.

#### Scenario: Generate a typed object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` where `schema` is a `schema.Schema` and the LLM returns valid JSON matching the schema
- **THEN** `output.Value[Recipe](result)` SHALL return the parsed `Recipe` value with no error

#### Scenario: LLM returns invalid JSON for object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns JSON that does not match the schema
- **THEN** `output.Value[Recipe](result)` SHALL return an error wrapping `ErrNoObjectGenerated`
- **AND** `result.Text()` SHALL still contain the raw LLM response

#### Scenario: LLM returns unparseable text for object

- **WHEN** `Output` is set to `output.Object[Recipe](schema)` and the LLM returns text that is not valid JSON
- **THEN** `output.Value[Recipe](result)` SHALL return an error wrapping `ErrNoObjectGenerated` with a JSON parse error as cause

### Requirement: Array output mode

The system SHALL provide ArrayOutput[T] accepting element schema.Schema. Its JSON SHALL be wrapped as {"elements": [...]} in a new schema.Schema via schema.SchemaFromJSON. This SHALL remain the strict provider format with additionalProperties false and elements required.

#### Scenario: Generate an array of typed elements

- **WHEN** `Output` is set to `output.Array[City](elementSchema)` where `elementSchema` is a `schema.Schema` and the LLM returns valid JSON with a wrapped array
- **THEN** `output.Value[[]City](result)` SHALL return the parsed slice of `City` values

#### Scenario: LLM returns array with invalid element

- **WHEN** the LLM returns JSON where one element does not match the element schema
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

#### Scenario: LLM adds an unrelated wrapper property

- **WHEN** the LLM returns `{"elements":[{"name":"Paris","population":2161000}],"extra":true}`
- **THEN** parsing SHALL succeed and return the single `City` element

#### Scenario: LLM omits the elements array

- **WHEN** the LLM returns `{}`, `{"elements":null}`, `{"elements":{"name":"Paris"}}` or a document that is not a JSON object
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

### Requirement: Complete array parsing validates elements not the wrapper

Complete array parsing SHALL extract elements at runtime, not validate wrapper schema. Response SHALL be a JSON object with an elements array and each element SHALL validate against its schema. Extra wrapper properties SHALL NOT fail. Non-object, missing elements, null or non-array elements SHALL error wrapping ErrNoObjectGenerated.

#### Scenario: Extra wrapper property does not weaken element checks
- **WHEN** an object has an elements array plus an extra property but an invalid element
- **THEN** complete parsing SHALL fail element validation with ErrNoObjectGenerated, not reject the extra wrapper property.

### Requirement: Choice output mode

The system SHALL provide ChoiceOutput wrapping options in {"result": "..."} with enum constraint and unwrapping to selected string. Strict provider format SHALL require result and additionalProperties false. Complete parsing SHALL extract at runtime, not validate wrapper schema: response SHALL be an object with result a string in the option set; extra wrapper properties SHALL NOT fail.

#### Scenario: Generate a choice from options

- **WHEN** `Output` is set to `output.Choice("sunny", "rainy", "snowy")` and the LLM returns `{"result": "sunny"}`
- **THEN** `output.Value[string](result)` SHALL return `"sunny"`

#### Scenario: LLM returns value not in options

- **WHEN** the LLM returns `{"result": "cloudy"}` which is not in the option set
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

#### Scenario: LLM adds an unrelated wrapper property

- **WHEN** the LLM returns `{"result":"sunny","extra":true}`
- **THEN** parsing SHALL succeed and return `"sunny"`

#### Scenario: Partial snapshot carries no choice yet

- **WHEN** a single-option choice receives the partial snapshots `{`, `{"other":`, `{"result":null,` or `{"result":1}`
- **THEN** partial parsing SHALL publish no value

#### Scenario: Partial snapshot carries a unique prefix

- **WHEN** `output.Choice("sunny", "rainy", "snowy")` receives the partial snapshot `{"result":"rai`
- **THEN** partial parsing SHALL publish `"rainy"`

### Requirement: Partial choices require represented strings and matching options

Partial parsing SHALL publish nothing unless the snapshot is an object with present string result. Missing/null/non-string result SHALL NOT become empty string. For present strings, successfully parsed snapshots SHALL publish only exact options; repaired snapshots SHALL publish only unique prefix matches.

#### Scenario: Ambiguous repaired prefix publishes nothing
- **WHEN** a repaired partial choice string matches multiple option prefixes
- **THEN** partial parsing SHALL publish no value rather than choose an arbitrary option.

### Requirement: JSON output mode

The system SHALL provide a `JSONOutput` implementation that requests JSON mode from the provider without a schema constraint. The response SHALL be validated as parseable JSON but not against any schema.

#### Scenario: Generate unstructured non-null JSON

- **WHEN** `Output` is set to `output.JSON()` and the LLM returns valid non-null JSON
- **THEN** `output.Value[any](result)` through an `OutputAccessor` wrapper SHALL return the parsed JSON value (map, slice, string, number, or bool)

#### Scenario: Generate JSON null

- **WHEN** `Output` is set to `output.JSON()` and the final generated response is `null` with a `stop` finish
- **THEN** `ParseComplete("null")` SHALL succeed with a nil value and the generated result SHALL have nil `Output` and nil `OutputError`
- **AND** `output.Value[any]` through an `OutputAccessor` wrapper of the generated result SHALL report missing output

#### Scenario: LLM returns invalid JSON in JSON mode

- **WHEN** `Output` is set to `output.JSON()` and the LLM returns text that is not valid JSON
- **THEN** the result SHALL return an error wrapping `ErrNoObjectGenerated`

### Requirement: Final output parsing follows operation semantics

With Output configured, StreamText SHALL ParseComplete final-step accumulated text regardless of finish reason, populating OutputValue on success or OutputError on failure. GenerateText/ToolLoopAgent.Generate SHALL parse final text for stop, or non-tool-calls finishes with nonempty text. They SHALL NOT parse empty non-stop or any tool-calls response. This SHALL apply to object, array, choice and JSON modes.

#### Scenario: StreamText parses valid output after a length finish

- **WHEN** the final `StreamText` step completes with `FinishReasonLength` and accumulated text that is valid for the configured `Output`
- **THEN** the system SHALL call `ParseComplete` on the accumulated text
- **AND** `OutputValue()` SHALL return the parsed value
- **AND** `OutputError()` SHALL return nil

#### Scenario: StreamText exposes invalid output after a length finish

- **WHEN** the final `StreamText` step completes with `FinishReasonLength` and truncated or invalid text for the configured `Output`
- **THEN** `OutputValue()` SHALL return nil
- **AND** `OutputError()` SHALL return an error wrapping `ErrNoObjectGenerated`
- **AND** `Text()` SHALL retain the raw accumulated response

#### Scenario: Generated stop finish attempts complete parsing even with empty text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `stop` with built-in object, array, choice, or JSON output configured, whether its text is valid, invalid, or empty
- **THEN** the system SHALL call `ParseComplete` on that text
- **AND** valid text (non-null for JSON mode) SHALL yield the complete output with nil `OutputError`, while invalid or empty text SHALL yield nil output and an `OutputError` wrapping `ErrNoObjectGenerated`

#### Scenario: Generated non-tool-call non-stop finish parses nonempty valid text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `length`, `content-filter`, `error`, or `other` with nonempty valid object, array, or choice output text, or non-null valid JSON output text
- **THEN** the system SHALL call `ParseComplete` on that text
- **AND** `Output` SHALL contain the validated value and `OutputError` SHALL be nil
- **AND** `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL return the value without error

#### Scenario: Generated non-tool-call non-stop finish reports invalid text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `length`, `content-filter`, `error`, or `other` with nonempty malformed JSON or JSON invalid for the configured object, array, or choice schema
- **THEN** the system SHALL call `ParseComplete` on that text
- **AND** `Output` SHALL be nil and both `OutputError` and the error from `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL wrap `ErrNoObjectGenerated`
- **AND** the generated call SHALL still return its result, including its raw text and finish reason

#### Scenario: Generated non-stop finish with empty text skips complete parsing

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `length`, `content-filter`, `error`, or `other` with empty text
- **THEN** the system SHALL NOT call `ParseComplete`
- **AND** `Output` and `OutputError` SHALL both be nil
- **AND** `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL report missing output

#### Scenario: Generated tool-calls finish skips complete parsing regardless of text

- **WHEN** the final `GenerateText` or `ToolLoopAgent.Generate` step finishes `tool-calls` with valid, invalid, or empty text
- **THEN** the system SHALL NOT call `ParseComplete`
- **AND** `Output` and `OutputError` SHALL both be nil
- **AND** `output.Value[T]` through a generated-result `OutputAccessor` wrapper SHALL report missing output

#### Scenario: Generated output comes from the final step

- **WHEN** a generated call continues after an earlier tool step and its final step contains structured output
- **THEN** complete output parsing SHALL use the final step's accumulated text and finish reason, not the earlier step's

### Requirement: Generated structured output errors remain result fields

Successful generated parsing SHALL populate GenerateTextResult.Output and leave OutputError nil. Parse/validation failure SHALL leave Output nil and populate OutputError without failing the call; built-in object/array/choice/JSON failures SHALL wrap ErrNoObjectGenerated. Skipped parsing SHALL leave both nil. Final text/unified finish reason SHALL remain available in all cases.

#### Scenario: Generated validation failure retains the response
- **WHEN** eligible final generated text fails a built-in output parser
- **THEN** the call SHALL retain its result, raw text and unified finish reason with nil Output and OutputError wrapping ErrNoObjectGenerated.

### Requirement: Generated output accessors distinguish null and missing output

Through an output.OutputAccessor wrapper such as output.ObjectResult[T], output.Value[T] SHALL return available non-nil typed output, or explicit error on parse failure/missing output. GenerateTextResult SHALL have fields, not accessor methods. JSON null SHALL parse successfully with nil Output/OutputError, but the existing typed accessor SHALL report missing output.

#### Scenario: Parsed null is successful but not a typed output value
- **WHEN** a generated JSON output parses null successfully
- **THEN** Output and OutputError SHALL be nil while output.Value through a wrapper SHALL explicitly report missing output.

### Requirement: Structured output with tools

The system SHALL support combining structured output with tool calling in the same request. Structured output generation counts as a step in the multi-step execution model.

#### Scenario: Tools and structured output together

- **WHEN** `StreamTextParams` has both `Tools` and `Output` set, and `StopWhen` allows multiple steps
- **THEN** the system SHALL execute tool calls in earlier steps and produce structured output in the final step when the LLM finishes with `FinishReason == "stop"`

### Requirement: Partial output streaming

StreamTextResult SHALL expose PartialOutputStream() as a channel of json.RawMessage partial snapshots from configured output mode. Partial failures SHALL be silently skipped. Every distinct parsed snapshot SHALL arrive exactly once in production order, losslessly even if consumed after generation. The stream SHALL close only after generation finishes and queued snapshots are delivered.

#### Scenario: Receive partial output during streaming

- **WHEN** `Output` is set and the LLM streams text deltas that form partial JSON
- **THEN** `PartialOutputStream()` SHALL emit `json.RawMessage` values for each successfully parsed partial snapshot
- **AND** only emit when the parsed result differs from the previous emission
- **AND** deliver each emitted snapshot exactly once and in production order

#### Scenario: Partial output consumption starts after the channel buffer is exceeded

- **WHEN** generation produces more distinct partial snapshots than the public channel buffer can hold before the consumer starts reading
- **THEN** `PartialOutputStream()` SHALL deliver every snapshot exactly once and in production order
- **AND** structured output generation and result completion SHALL NOT block on the delayed consumer

#### Scenario: No Output set

- **WHEN** `Output` is nil
- **THEN** `PartialOutputStream()` SHALL return a closed channel immediately

### Requirement: Array element streaming

The system SHALL provide an `ElementStream()` method on `StreamTextResult` that returns a channel of `json.RawMessage` values, where each value is a complete validated array element. This SHALL only emit elements for array output mode. Every completed element SHALL be delivered exactly once and in array order. The stream SHALL remain lossless when consumption starts after generation, and SHALL close only after generation finishes and all queued elements are delivered.

#### Scenario: Receive validated elements during array streaming

- **WHEN** `Output` is set to array mode and the LLM streams array elements
- **THEN** `ElementStream()` SHALL emit each complete element as a `json.RawMessage` after it passes schema validation
- **AND** incomplete trailing elements SHALL NOT be emitted
- **AND** each completed element SHALL be delivered exactly once and in array order

#### Scenario: Element consumption starts after the channel buffer is exceeded

- **WHEN** generation completes more array elements than the public channel buffer can hold before the consumer starts reading
- **THEN** `ElementStream()` SHALL deliver every completed element exactly once and in array order
- **AND** structured output generation and result completion SHALL NOT block on the delayed consumer

#### Scenario: Non-array output mode

- **WHEN** `Output` is set to object, choice, or json mode
- **THEN** `ElementStream()` SHALL emit no values
- **AND** the channel SHALL close when generation finishes

### Requirement: Typed result access via generic functions

The system SHALL provide generic free functions in the `output` package for type-safe access to structured output results: `Value[T]` for final results, and `TypedElementStream[T]` for array elements.

#### Scenario: Value with correct type

- **WHEN** `output.Value[Recipe](result)` is called and the stored output is a `Recipe`
- **THEN** it SHALL return the typed value with no error

#### Scenario: Value with type mismatch

- **WHEN** `output.Value[User](result)` is called but the stored output is a `Recipe`
- **THEN** it SHALL return an error indicating type mismatch

#### Scenario: TypedElementStream

- **WHEN** `output.TypedElementStream[City](result)` is called with array output mode
- **THEN** it SHALL return a channel that emits each element unmarshaled into `City`

### Requirement: Convenience wrappers GenerateObject and StreamObject

The system SHALL provide `GenerateObject[T]()` and `StreamObject[T]()` generic functions that wrap `GenerateText`/`StreamText` respectively. These SHALL set the `Output` field internally and return typed result wrappers.

#### Scenario: GenerateObject returns typed result

- **WHEN** `output.GenerateObject[Recipe](ctx, params, objectOutput)` is called
- **THEN** it SHALL call `GenerateText` with `Output` set and return an `ObjectResult[T]` with a typed `Object()` accessor

#### Scenario: StreamObject returns typed streaming result

- **WHEN** `output.StreamObject[Recipe](ctx, params, objectOutput)` is called
- **THEN** it SHALL call `StreamText` with `Output` set and return a `StreamObjectResult[T]` with typed partial and element stream accessors

### Requirement: ErrNoObjectGenerated sentinel error

The system SHALL define `ErrNoObjectGenerated` as a sentinel error in the root `aisdk` package. All output validation failures SHALL wrap this error. The error context SHALL preserve the raw text, response metadata, and usage information.

#### Scenario: Check error type

- **WHEN** structured output validation fails
- **THEN** `errors.Is(err, aisdk.ErrNoObjectGenerated)` SHALL return true
- **AND** the raw LLM text SHALL be accessible from the result via `result.Text()`

### Requirement: Opt-in repair of invalid complete structured output

Optional aisdk.WithRepairText SHALL work with GenerateText/StreamText and their output.GenerateObject/StreamObject wrappers. Its callback SHALL receive original text and Output.ParseComplete error, returning repaired text, acceptance and error. It SHALL run exactly once only for an eligible complete-parse error wrapping both aisdk.ErrNoObjectGenerated and aisdk.ErrInvalidOutputText.

#### Scenario: Repair malformed JSON through GenerateObject
- **WHEN** `output.GenerateObject[T]` is passed `aisdk.WithRepairText` and the model returns malformed JSON for an `output.Object[T]` that the callback repairs to schema-valid JSON
- **THEN** the callback SHALL receive exactly the original generated text and the original parse error wrapping both `ErrNoObjectGenerated` and `ErrInvalidOutputText`
- **AND** `Object()` SHALL return the typed value from validated repaired text without an output error

#### Scenario: Repair schema-invalid JSON through StreamObject
- **WHEN** `output.StreamObject[T]` is passed `aisdk.WithRepairText` and the model returns valid JSON that fails the configured object schema but the callback returns schema-valid JSON
- **THEN** the callback SHALL receive exactly the original text and schema validation error wrapping both `ErrNoObjectGenerated` and `ErrInvalidOutputText`
- **AND** `Object()` SHALL return the typed validated repaired value without an output error

#### Scenario: Shared option on direct calls and other output modes
- **WHEN** direct `GenerateText` or `StreamText` configures `WithOutput` with Object, Array, Choice, JSON, or a custom `Output` wrapping both `ErrNoObjectGenerated` and `ErrInvalidOutputText`, along with `WithRepairText`, and the callback accepts repaired text valid for that output
- **THEN** `Output` or `OutputValue()` SHALL contain the parsed and validated repaired value
- **AND** the same output's `ResponseFormat` and normal parse/validation rules SHALL be used for the original and repaired text

#### Scenario: No repair configured
- **WHEN** `output.GenerateObject` or `output.StreamObject` is called without `WithRepairText` and its configured output rejects the model text
- **THEN** `Object()` SHALL return the original output error wrapping `ErrNoObjectGenerated` without an extra parse attempt

#### Scenario: Declined repair
- **WHEN** the callback returns `accepted == false` with no callback error after a validation failure
- **THEN** `OutputError` and the typed accessor SHALL return the original parse/validation error
- **AND** the system SHALL NOT parse returned text

#### Scenario: Repair callback fails
- **WHEN** the callback returns an error after an eligible parse/validation failure
- **THEN** `OutputError` and the typed accessor SHALL return the callback error
- **AND** the system SHALL NOT parse returned text again

#### Scenario: Accepted repaired output remains invalid
- **WHEN** the callback accepts text that fails the configured output's complete parser or schema validation
- **THEN** `OutputError` and the typed accessor SHALL return the repaired text's parse/validation error wrapping `ErrNoObjectGenerated`
- **AND** the callback SHALL NOT run a second time

#### Scenario: Schema-valid text fails Go typed conversion
- **WHEN** Object or Array schema validation succeeds but typed `json.Unmarshal` into `T` fails (including a custom unmarshaler failure), with `WithRepairText` configured
- **THEN** `OutputError` SHALL still wrap `ErrNoObjectGenerated` but SHALL NOT wrap `ErrInvalidOutputText`
- **AND** the callback SHALL NOT run and the original typed-conversion error SHALL be returned

#### Scenario: No eligible validation error
- **WHEN** original complete parsing succeeds, the output is nil, a custom output returns an error lacking either required sentinel, or complete parsing is not eligible under existing finish-reason rules
- **THEN** the callback SHALL NOT run
- **AND** original result behavior SHALL remain unchanged

### Requirement: Repairable text failures exclude Go conversion failures

ErrInvalidOutputText SHALL indicate generated JSON syntax/schema failure, not Go typed-conversion failure after successful schema validation. Repair SHALL apply to Object/Array/Choice/JSON schema and JSON validation and custom Output implementations wrapping both sentinels. The three-method Output interface and existing stop/non-stop parse eligibility SHALL NOT change.

#### Scenario: Custom output explicitly marks repairable text
- **WHEN** an eligible custom Output error wraps both required sentinels
- **THEN** configured repair SHALL be eligible without extending the Output interface or changing finish-reason eligibility.

### Requirement: Accepted structured repair is parsed only once

Accepted repaired text SHALL be passed to the configured Output.ParseComplete once more without invoking repair again.

#### Scenario: Accepted repair still fails validation
- **WHEN** the callback accepts text that the configured parser rejects
- **THEN** the repaired parse error SHALL become OutputError and the callback SHALL NOT run a second time.

### Requirement: Repair preserves original model response and inspectable errors

Repair SHALL preserve original text, content, full/UI chunks, partial/array-element streams, response metadata and usage; only final structured OutputValue/OutputError SHALL change. Built-in parse/validation failures SHALL retain underlying parse/schema error in the Go chain while wrapping ErrNoObjectGenerated; repairable JSON/schema failures SHALL also wrap ErrInvalidOutputText so cause and eligibility remain separately inspectable.

#### Scenario: Repaired output does not rewrite raw result
- **WHEN** a callback repairs generated text to a valid final value
- **THEN** the final typed value SHALL reflect the repaired text
- **AND** `Text`, `Content`, `Steps`, `Response`, `Usage` and `TotalUsage` SHALL reflect the original provider response
- **AND** text, partial-output and UI chunks SHALL NOT be replaced with repaired text or synthesized from it

#### Scenario: Inspect original failure cause
- **WHEN** a builtin structured output rejects malformed JSON or schema-invalid JSON
- **THEN** the callback's error SHALL satisfy `errors.Is(err, aisdk.ErrNoObjectGenerated)` and `errors.Is(err, aisdk.ErrInvalidOutputText)`
- **AND** the underlying JSON parse or schema validation error SHALL be discoverable with `errors.As`
