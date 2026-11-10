# Spec Delta

## MODIFIED Requirements

### Requirement: Logger options and capture controls

Logger options SHALL configure logging, levels, request attributes, capture, redaction, per-part logging, and time. Capture SHALL be explicit opt-in with bounded payload controls.

#### Scenario: Configure logger lifecycle options
- **WHEN** a caller constructs `Options`
- **THEN** it SHALL include `Logger *slog.Logger`, `Level slog.Leveler`, `ErrorLevel slog.Leveler`, `PartLevel slog.Leveler`, `Attrs []slog.Attr`, `DynamicAttrs func(ctx context.Context) []slog.Attr`, `Capture CaptureOptions`, `Redactor Redactor`, `LogStreamParts bool`, `IsDenied func(error) bool`, and `Clock func() time.Time`

#### Scenario: Configure capture categories and bounds
- **WHEN** a caller constructs `CaptureOptions`
- **THEN** explicit opt-in controls SHALL include `Inputs`, `Outputs`, `Reasoning`, `ToolInputs`, `ToolOutputs`, `Files`, `RawChunks`, `Headers`, `ProviderOptions`, `RequestBody`, `ResponseBody`, `ProviderMetadata`, and `ErrorMessages`
- **AND** bounded payload controls SHALL include `MaxStringLen` and `MaxJSONBytes`

#### Scenario: Nil options select standard defaults
- **WHEN** logger, level, redactor, or clock options are nil
- **THEN** nil `Logger` SHALL use `slog.Default()`, nil `Level` SHALL use `slog.LevelInfo`, nil `ErrorLevel` SHALL use `slog.LevelError`, and nil `PartLevel` SHALL use `slog.LevelDebug`
- **AND** nil `Redactor` SHALL use `DefaultRedactor()` and nil `Clock` SHALL use `time.Now`

## ADDED Requirements

### Requirement: Denied call logging
When `Options.IsDenied` reports a call's error, a stream setup error or a stream error part as a denial, the terminal record SHALL use outcome `denied`, `ai_sdk.success=false`, error classification `denied` and the success level instead of the error level. A nil `IsDenied` SHALL leave logging unchanged.

#### Scenario: Denied generate call
- **WHEN** `DoGenerate` returns an error that `IsDenied` reports as a denial
- **THEN** exactly one `EventGenerateError` record SHALL be emitted at `Level` with `ai_sdk.outcome="denied"` and `ai_sdk.error.type="denied"`
- **AND** the original error SHALL be returned unchanged

#### Scenario: Denied stream
- **WHEN** a stream ends after an error part whose error `IsDenied` reports as a denial
- **THEN** the terminal stream record SHALL carry `ai_sdk.outcome="denied"` and the stream parts SHALL pass through unchanged

#### Scenario: Denied stream error part with per-part logging
- **WHEN** `LogStreamParts` is enabled and a stream error part's error is reported as a denial
- **THEN** its `EventStreamPart` record SHALL use `PartLevel` and error type `denied`, not `ErrorLevel`
