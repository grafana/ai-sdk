## ADDED Requirements

### Requirement: Middleware struct with optional hooks
The `Middleware` type SHALL be a struct with optional function fields for each interception point. A nil function field means the hook is not active -- the call passes through unmodified.

The hooks SHALL be:
- `TransformParams`: modifies `provider.CallOptions` before they reach the model
- `WrapGenerate`: intercepts `DoGenerate` calls
- `WrapStream`: intercepts `DoStream` calls

Metadata overrides SHALL be:
- `OverrideProvider`: overrides the `Provider()` string on the wrapped model
- `OverrideModelID`: overrides the `ModelID()` string on the wrapped model
- `OverrideSupportedURLs`: overrides the `SupportedURLs()` return value on the wrapped model

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

### Requirement: Stream transformation utility
A `TransformStream` utility function SHALL be provided for middleware authors. It SHALL accept a `context.Context`, a `*provider.StreamResult`, a transform function, and an optional flush function, returning a new `*provider.StreamResult` with the stream channel transformed.

The transform function SHALL receive each `provider.StreamPart` and an `emit` callback to produce zero, one, or many output parts per input part. This supports stateful buffering across chunks.

The flush function (nil-safe) SHALL be called when the input stream closes, allowing transforms to emit any buffered data.

The transform goroutine SHALL respect context cancellation.

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
