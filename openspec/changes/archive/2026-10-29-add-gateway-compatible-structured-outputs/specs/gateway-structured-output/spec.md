## MODIFIED Requirements

### Requirement: Existing native and client output semantics
The Gateway SHALL transport structured output using existing text response and stream codecs and preserve supported native warnings, metadata, usage and finish behavior. Each native adapter SHALL retain its registered schema conversion, validation and support behavior. An `openai-compatible` backend SHALL receive JSON-schema response formats only when its provider enables `supportsStructuredOutputs`. High-level clients SHALL own final object parsing and validation.

#### Scenario: Unary and streamed object
- **WHEN** registered TypeScript and Go high-level output clients consume valid JSON text through unary or streaming generation
- **THEN** they SHALL produce the expected validated object without a new Gateway response codec

#### Scenario: Invalid generated object
- **WHEN** native text is not valid for the requested client schema
- **THEN** final output validation SHALL fail in the client without Gateway repair or a fabricated provider success

#### Scenario: Compatible backend with structured outputs enabled
- **WHEN** a request with a response schema reaches an `openai-compatible` candidate whose provider sets `supportsStructuredOutputs: true`, unary or streaming, directly or through configured fallback
- **THEN** the backend SHALL receive `response_format` of type `json_schema` carrying the schema and description, the name defaulting to `response` only when absent, and `strict` from the per-call compatible `strictJsonSchema` option, defaulting to `true`
- **AND** the Gateway output SHALL carry no `responseFormat` unsupported warning
- **AND** this SHALL hold through both the Go and TypeScript Gateway clients

#### Scenario: Per-call strict override with structured outputs enabled
- **WHEN** a request to a compatible candidate with `supportsStructuredOutputs: true` sets `strictJsonSchema: false` under its compatible provider-options namespace
- **THEN** the backend SHALL receive `response_format.json_schema.strict` as `false`

#### Scenario: Mixed compatible fallback candidates
- **WHEN** a primary compatible candidate fails (503) before producing output and a serving fallback compatible candidate has the opposite `supportsStructuredOutputs` setting
- **THEN** each backend request SHALL use `json_schema` when its own provider enables the setting and `json_object` otherwise
- **AND** when the serving candidate enables the setting, the Gateway output SHALL carry no `responseFormat` unsupported warning

#### Scenario: Compatible backend without structured outputs
- **WHEN** a request with a response schema reaches an `openai-compatible` candidate whose provider does not enable `supportsStructuredOutputs`
- **THEN** the backend SHALL receive `response_format` of type `json_object`
- **AND** the Gateway output SHALL carry a `responseFormat` unsupported warning with details `JSON response format schema is only supported with structuredOutputs`, matching upstream
