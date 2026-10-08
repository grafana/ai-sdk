## ADDED Requirements

### Requirement: Caller-requested raw output

The ProviderWire V4 handler SHALL accept `includeRawChunks` from an authenticated caller and pass its value to the selected model as `IncludeRawChunks`, in unary and streaming calls, including configured fallback candidates. An absent value SHALL pass false. The request SHALL NOT be rejected as an unsupported capability. The flag SHALL reach the model unless the selected candidate's execution policy rejects it with `catalog.ErrUnsupportedRequest`; no policy rejects it today.

#### Scenario: Flag reaches the selected model
- **WHEN** an authenticated request sets `includeRawChunks` to true, false, or omits it
- **THEN** the selected model SHALL receive true, false, or false respectively

#### Scenario: Unary call requests raw output
- **WHEN** a unary request sets `includeRawChunks` to true
- **THEN** the handler SHALL execute the call and return its ordinary unary result with no raw part or new response field

### Requirement: Raw stream events

When a streaming request set `includeRawChunks` to true, the handler SHALL write each `PartRaw` with a valid JSON value as `{"type":"raw","rawValue":<value>}` in provider order, after the credential projection below. A `PartRaw` without a value, with invalid JSON or invalid UTF-8, or with an object that repeats a key after unescaping SHALL be dropped. When the request did not set `includeRawChunks` to true, every `PartRaw` SHALL be dropped. Dropped raw parts SHALL NOT end the stream. Every raw part SHALL count toward `StreamParts`, and a written raw event SHALL be bounded by `StreamFrameBytes` like any other frame: an oversized event SHALL produce one terminal internal error and no finish, and SHALL NOT be truncated.

#### Scenario: Requested raw parts stream in order
- **WHEN** the adapter emits raw parts between normalized parts and the caller requested raw output
- **THEN** both the registered TypeScript client and the Go client SHALL receive the raw values in the same position relative to normalized parts

#### Scenario: Unrequested raw parts are dropped
- **WHEN** the adapter emits raw parts and the caller omitted `includeRawChunks` or set it false
- **THEN** the SSE body SHALL contain no raw event and the stream SHALL finish normally

#### Scenario: Raw part without a usable value
- **WHEN** the adapter emits a raw part whose native frame was not valid JSON, whose value is not valid UTF-8, or whose value repeats an object key
- **THEN** the handler SHALL drop it and continue the stream

#### Scenario: Oversized raw event
- **WHEN** an encoded raw event exceeds `StreamFrameBytes`
- **THEN** the handler SHALL write one terminal internal error, no raw bytes for that event, and no finish

### Requirement: Raw credential projection

Before writing a raw event, the handler SHALL remove credential-bearing fields that a provider echoes from the request. Every JSON object of type `mcp` anywhere in the value, after key unescaping, is treated as an echoed MCP tool definition: the handler SHALL delete its `authorization`, replace non-null `headers` with null, and remove userinfo and the `access_token`, `api_key`, `X-Amz-Credential`, `X-Amz-Signature` and `X-Amz-Security-Token` query keys (case-insensitive) from a string `server_url`; a non-null `server_url` that is not a string, or whose URL or query cannot be parsed, SHALL be deleted; a null `server_url` SHALL be kept. All other fields SHALL keep their JSON values, including large numbers. Whitespace, key order and string escaping MAY differ from the native bytes.

#### Scenario: OpenAI Responses echo of MCP tools
- **WHEN** a raw `response.created` value echoes an MCP tool with headers and a credential-bearing server URL
- **THEN** the written raw event SHALL contain null headers and the server URL without userinfo or credential query keys, and its other fields unchanged

### Requirement: Raw values stay out of Gateway telemetry

Gateway logs, metrics and metadata-only Agent Observability exports SHALL NOT contain raw part values, whether or not the caller requested raw output.

#### Scenario: Requested raw stream is observed
- **WHEN** a requested raw stream passes through the Gateway's model observability wrapper
- **THEN** the raw part SHALL reach the handler unchanged and no raw value SHALL appear in logs, metrics or the exported generation
