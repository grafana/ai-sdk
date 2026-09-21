## MODIFIED Requirements

### Requirement: Shared strict streaming request pipeline
The handler SHALL accept `ai-language-model-streaming` only when its single exact value is `true` or `false`. It SHALL route `false` to the existing unary path and `true` to the streaming path only after the same bounded body read, standard Go JSON and complete request-schema validation, explicit supported text/function-tool subset mapping, exact-once catalog resolution, and validation of a non-empty valid-UTF-8 canonical ID with a non-nil V4 model. A supported streaming request SHALL invoke `DoStream` exactly once and SHALL NOT invoke `DoGenerate`. Any failure before stream invocation SHALL select a fixed non-2xx JSON document and SHALL produce no SSE commitment.

The shared mapping SHALL admit schema-valid automatic tool choice with absent or empty tools for otherwise supported streaming requests, preserving it in `provider.CallOptions.ToolChoice`. Omitted choice SHALL remain nil. Function definitions, explicit choices, and tool history SHALL retain the gateway-streaming-function-tools supported subset on direct routes. Provider tools, provider-executed history, approvals, and other deferred families SHALL retain their pre-resolution rejection behavior. Fallback-configured routes SHALL retain their stricter effect guard.

#### Scenario: Supported streaming envelope executes once
- **WHEN** a valid text request uses streaming value `true` and passes mapping and resolution
- **THEN** resolution and `DoStream` SHALL each run once, and `DoGenerate` SHALL not run

#### Scenario: Streaming request fails before invocation
- **WHEN** a streaming request fails envelope, body, standard JSON, schema, mapping, or resolution processing
- **THEN** resolution or model invocation SHALL not run after an earlier failure and the response SHALL remain a fixed non-2xx JSON error rather than SSE

#### Scenario: Invalid streaming selector
- **WHEN** the streaming header is missing, empty, repeated, or has a value other than exact `true` or `false`
- **THEN** envelope validation SHALL fail before body mapping, resolution, or model invocation

#### Scenario: Text-only automatic choice streams
- **WHEN** an otherwise supported streaming request has automatic choice with tools absent or empty
- **THEN** `DoStream` SHALL receive non-nil automatic choice and no tools exactly once
- **AND** `DoGenerate` SHALL not run
- **AND** a valid provider text stream SHALL retain the existing strict SSE and clean-EOF behavior

#### Scenario: Low-level streaming omission remains absent
- **WHEN** an otherwise supported streaming request omits tool choice and has absent or empty tools
- **THEN** `DoStream` SHALL receive nil `ToolChoice`

#### Scenario: Unsupported tools do not commit streaming
- **WHEN** a streaming request combines auto with provider tools or provider-executed tool history
- **THEN** the handler SHALL return the fixed unsupported-tools JSON response with zero catalog and model calls and no SSE commitment

#### Scenario: Automatic choice does not bypass schema or other unsupported families
- **WHEN** a streaming request contains malformed tool choice or combines valid text-only auto with another unsupported family
- **THEN** the existing schema-invalid or unsupported-family JSON response SHALL occur before resolution or model invocation and without SSE commitment
