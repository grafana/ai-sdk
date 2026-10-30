## Purpose

Define language-model middleware composition, parameter transformation, generate and stream wrapping, metadata overrides, error propagation, and stream transformation.

## Requirements

### Requirement: Middleware struct with optional hooks

`Middleware` SHALL be a struct with optional function fields. Nil hooks SHALL pass calls through unmodified. Hooks SHALL be `TransformParams` (modifies `provider.CallOptions` before the model), `WrapGenerate` (intercepts `DoGenerate`), and `WrapStream` (intercepts `DoStream`). Metadata hooks SHALL be `OverrideProvider`, `OverrideModelID`, and `OverrideSupportedURLs`, overriding the wrapped model's `Provider()` string, `ModelID()` string, and `SupportedURLs()` return value respectively.

#### Scenario: Middleware with only TransformParams set
- **WHEN** a `Middleware` is created with only `TransformParams` set (all other fields nil)
- **THEN** `TransformParams` SHALL be called before each model invocation
- **AND** `DoGenerate` and `DoStream` SHALL pass through to the inner model unmodified

#### Scenario: Middleware with no hooks set
- **WHEN** a `Middleware` is created with all function fields nil
- **THEN** the wrapped model SHALL behave identically to the inner model for all operations

### Requirement: WrapLanguageModel composes middleware onto a model
`WrapLanguageModel` SHALL accept a `provider.LanguageModel` and one or more `Middleware` values, returning a new `provider.LanguageModel`.

The returned model SHALL satisfy the full `provider.LanguageModel` interface, making it transparent to all consumers (`StreamText`, `GenerateText`, etc.).

The original model SHALL NOT be mutated.

#### Scenario: Single middleware wrapping
- **WHEN** `WrapLanguageModel` is called with a model and one middleware
- **THEN** it SHALL return a new `LanguageModel` that delegates to the original model through the middleware hooks

#### Scenario: Multiple middleware composition
- **WHEN** `WrapLanguageModel` is called with middlewares `[A, B, C]`
- **THEN** the resulting model SHALL apply middleware in order: A is outermost (processes input first), C is innermost (closest to the model)
- **AND** `TransformParams` SHALL execute in order A -> B -> C -> model
- **AND** wrap hooks SHALL nest as A(B(C(model)))

#### Scenario: Wrapped model is transparent to consumers
- **WHEN** a wrapped model is passed to `StreamText` or `GenerateText`
- **THEN** the orchestration layer SHALL treat it identically to an unwrapped model

### Requirement: Wrap with top-level overrides
`Wrap` SHALL accept a `WrapOptions` struct containing the model, middlewares, and optional `ModelID`/`ProviderID` string overrides. These top-level overrides SHALL take highest precedence, above any middleware-level `OverrideModelID`/`OverrideProvider` hooks.

This matches the upstream `wrapLanguageModel({ model, middleware, modelId, providerId })` call signature.

#### Scenario: Top-level ModelID takes precedence over middleware
- **WHEN** `Wrap` is called with `ModelID: "top-level"` and a middleware that sets `OverrideModelID`
- **THEN** the wrapped model's `ModelID()` SHALL return `"top-level"`

#### Scenario: No top-level overrides delegates to middleware
- **WHEN** `Wrap` is called without `ModelID`/`ProviderID` but with a middleware that sets `OverrideModelID`
- **THEN** the wrapped model's `ModelID()` SHALL return the middleware's override value

### Requirement: TransformParams receives operation type
The `TransformParams` hook SHALL receive a `type` parameter indicating whether the current call is `"generate"` or `"stream"`, allowing the middleware to apply different transformations based on the operation.

#### Scenario: TransformParams for generate vs stream
- **WHEN** `DoGenerate` is called on a wrapped model
- **THEN** `TransformParams` SHALL be invoked with type `"generate"`
- **WHEN** `DoStream` is called on a wrapped model
- **THEN** `TransformParams` SHALL be invoked with type `"stream"`

### Requirement: Cross-mode access in wrap hooks
Both `WrapGenerate` and `WrapStream` SHALL receive closures for both `DoGenerate` and `DoStream` on the inner model (with transformed params already applied).

This enables cross-mode patterns: a generate wrapper can fall back to streaming, and a stream wrapper can call generate.

#### Scenario: WrapStream calls DoGenerate
- **WHEN** a `WrapStream` hook calls the provided `DoGenerate` closure
- **THEN** it SHALL receive the result of `DoGenerate` on the inner model with transformed params

#### Scenario: WrapGenerate calls DoStream
- **WHEN** a `WrapGenerate` hook calls the provided `DoStream` closure
- **THEN** it SHALL receive the result of `DoStream` on the inner model with transformed params

### Requirement: Context propagation through closures
The `DoGenerate` and `DoStream` closures provided to wrap hooks SHALL accept a `context.Context` parameter, allowing middleware to propagate a modified context to the inner model.

#### Scenario: Middleware adds timeout to context
- **WHEN** a middleware creates a derived context with a timeout and passes it to `DoGenerate(ctx)`
- **THEN** the inner model's `DoGenerate` SHALL receive the timeout-bearing context

### Requirement: Metadata method delegation
A wrapped model SHALL delegate `SpecificationVersion()` to the inner model. `Provider()`, `ModelID()`, and `SupportedURLs()` SHALL use their respective override functions if set, otherwise delegate to the inner model.

#### Scenario: No metadata overrides
- **WHEN** a model is wrapped with a middleware that has no override functions
- **THEN** `Provider()`, `ModelID()`, `SpecificationVersion()`, and `SupportedURLs()` SHALL return the inner model's values

#### Scenario: Provider and ModelID overridden
- **WHEN** a middleware sets `OverrideProvider` and `OverrideModelID`
- **THEN** the wrapped model's `Provider()` and `ModelID()` SHALL return the override values
- **AND** `SpecificationVersion()` and `SupportedURLs()` SHALL still delegate to the inner model

#### Scenario: SupportedURLs overridden
- **WHEN** a middleware sets `OverrideSupportedURLs`
- **THEN** the wrapped model's `SupportedURLs()` SHALL return the override value
- **AND** `Provider()`, `ModelID()`, and `SpecificationVersion()` SHALL still delegate to the inner model

### Requirement: Error propagation
If `TransformParams`, `WrapGenerate`, or `WrapStream` returns an error, the wrapped model's `DoGenerate` or `DoStream` SHALL return that error to the caller. Errors SHALL NOT be silently swallowed.

#### Scenario: TransformParams returns error
- **WHEN** `TransformParams` returns an error
- **THEN** `DoGenerate` or `DoStream` SHALL return that error without calling the inner model

#### Scenario: WrapGenerate returns error
- **WHEN** `WrapGenerate` returns an error
- **THEN** `DoGenerate` SHALL return that error to the caller

### Requirement: Normalized streaming usage aggregation

Stream-summary middleware SHALL share normalized usage aggregation over every part with non-nil `Usage`, regardless of type. A `PartFinish` SHALL close a response segment, whose usage takes each normalized counter from the finish when it reports it, and otherwise the greatest value observed in that segment. Once a finish was observed, the aggregate SHALL be the last closed segment's usage. Before any finish, each counter SHALL retain its greatest observed value.

#### Scenario: Counters are selected independently
- **WHEN** stream parts report usage
- **THEN** input total, input without cache, cache-read input, cache-write input, output total, output text and output reasoning SHALL each be selected independently by these rules
- **AND** `Usage.Raw` SHALL follow the same selection with the latest non-empty payload in place of the greatest value, without a provider-independent raw merge

#### Scenario: Split usage across stream parts
- **WHEN** an early stream part reports input and cache counters
- **AND** a later stream part reports output counters
- **THEN** the aggregate SHALL contain both sets of counters

#### Scenario: Provisional terminal usage is lower
- **WHEN** an early stream part reports a normalized counter
- **AND** the finish part that closes its segment reports a lower value for that counter
- **THEN** the aggregate SHALL use the finish part's value
- **AND** a counter the finish part omits SHALL retain the greatest value observed in that segment

#### Scenario: Several finishes use the last segment
- **GIVEN** a stream emits two finish parts, each reporting its own usage
- **WHEN** the stream completes
- **THEN** the aggregate SHALL be the second segment's usage
- **AND** counters SHALL NOT be summed or take the greatest value across segments

#### Scenario: Usage after the last finish
- **WHEN** a part after the last finish reports usage and no later finish closes its segment
- **THEN** the aggregate SHALL remain the last closed segment's usage

#### Scenario: Stream without a finish
- **WHEN** a stream ends without a finish part, for example on a provider error or cancellation
- **THEN** each counter SHALL retain its greatest observed value

#### Scenario: Raw usage selection is deterministic
- **WHEN** multiple stream parts of one segment report non-empty raw provider usage
- **THEN** the segment SHALL retain its finish part's non-empty raw payload, or otherwise the most recently observed non-empty raw payload
- **AND** a later omitted raw payload SHALL NOT clear it

### Requirement: Stream transformation utility

`TransformStream` SHALL accept `context.Context`, `*provider.StreamResult`, a transform, and optional flush, returning a new `*provider.StreamResult` with a transformed channel. The transform SHALL receive each `provider.StreamPart` and an `emit` callback producing zero, one, or many parts per input, supporting stateful buffering. The nil-safe flush SHALL run on input closure to emit buffered data. The transform goroutine SHALL respect cancellation.

#### Scenario: One-to-one stream transformation
- **WHEN** `TransformStream` is called with a transform that modifies each part
- **THEN** the returned stream SHALL yield the modified parts in order

#### Scenario: One-to-many stream transformation
- **WHEN** a transform emits multiple parts for a single input part
- **THEN** all emitted parts SHALL appear in the output stream in emission order

#### Scenario: Buffering transform
- **WHEN** a transform buffers input parts and emits them later
- **THEN** the buffered parts SHALL be emitted when the transform decides to flush

#### Scenario: Context cancellation during transform
- **WHEN** the context is cancelled while a stream transform is running
- **THEN** the transform goroutine SHALL stop and clean up
