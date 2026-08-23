## MODIFIED Requirements

### Requirement: Constructed strict unary handler
The `gateway/providerwire/v4` package SHALL provide one production HTTP handler for relative `POST /language-model` unary and streaming requests. Construction SHALL require a `catalog.ModelResolver`, SHALL accept an optional host-policy boundary, and SHALL require named positive limits for request bytes, JSON depth, JSON token count, numeric token bytes, unary response bytes, error response bytes, provider stream-part count, complete SSE frame bytes, total model duration, stream idle duration, and bounded post-cancellation drain duration. Construction SHALL reject nil dependencies and byte limits that cannot safely use `limit+1`. It SHALL reject an error-response limit too small for the canonical internal-error document and a frame limit too small for the canonical empty start or terminal internal-error frame. The unary-response limit SHALL have no fallback-fit requirement because oversized success encoding transitions to the separate bounded error path.

#### Scenario: Valid handler construction
- **WHEN** a caller supplies a non-nil resolver and valid named unary limits plus positive stream-part, frame, idle, and drain limits
- **THEN** construction SHALL return one handler whose unary and streaming runtime behavior is fixed by those values

#### Scenario: Unsafe limit configuration
- **WHEN** a limit is zero or negative, a byte limit overflows safe `limit+1` arithmetic, the error-response limit cannot contain the canonical internal-error document, or the frame limit cannot contain a required canonical stream frame
- **THEN** construction SHALL fail before the handler serves a request

#### Scenario: Small positive unary limit
- **WHEN** the unary-response limit is positive and supports safe `limit+1` arithmetic but cannot contain a successful response
- **THEN** construction SHALL succeed
- **AND** any oversized unary success SHALL transition to the separately bounded error path at runtime

### Requirement: Strict unary HTTP envelope
The handler SHALL accept only `POST /language-model` with JSON content and exactly one effective value for each `ai-language-model-specification-version`, `ai-language-model-id`, and `ai-language-model-streaming` header. The required values SHALL be specification version `4`, a non-empty model ID preserved without trimming or rewriting, and streaming value exact `false` for unary execution or exact `true` for streaming execution. Additional host headers SHALL be accepted but SHALL NOT become `provider.CallOptions.Headers` automatically.

#### Scenario: Valid unary envelope
- **WHEN** a request uses the exact route and method, a JSON media type, specification version `4`, a non-empty model ID, and streaming `false`
- **THEN** envelope validation SHALL succeed, retain the exact model ID bytes represented by the header value, and select unary execution

#### Scenario: Valid streaming envelope
- **WHEN** a request uses the same valid envelope with streaming `true`
- **THEN** envelope validation SHALL succeed, retain the exact model ID, and select streaming execution through the phase 4 runtime

#### Scenario: Invalid protocol envelope
- **WHEN** the method or path is wrong, content is not JSON, a required protocol header is missing, empty, repeated, or has an invalid value, or streaming is neither exact `false` nor exact `true`
- **THEN** the handler SHALL return a bounded invalid-request error before reading semantic request values, applying policy, resolving a model, or invoking a model

#### Scenario: Unrelated host headers are present
- **WHEN** a valid request contains additional HTTP headers
- **THEN** envelope validation SHALL accept them
- **AND** mapped provider call headers SHALL remain empty unless a future explicit body-header capability and host policy allow them

### Requirement: Runtime contract and cross-language evidence
Go tests SHALL replay every committed phase 2 request golden according to this stage matrix:

| Golden record | Expected stage and result |
| --- | --- |
| `streaming.json` | supported streaming text execution through `DoStream` |
| `sequence.json` record 1 | supported unary text execution through `DoGenerate` |
| `sequence.json` record 2 | supported streaming text execution through `DoStream` |
| `scalar-presence.json` | schema success, then first unsupported capability `body-headers` because the map contains an empty-valued header member |
| `headers.json` records 1 and 2 | schema success, then first unsupported capability `body-headers` |
| `comprehensive-unions.json` | schema success, then first unsupported capability `provider-options` from the first system message |

A separate dedicated supported scalar request SHALL assert exact scalar presence and unary execution. Focused requests SHALL activate each unsupported capability independently; a multi-capability golden SHALL be required to report only the deterministic first capability. Raw HTTP tests SHALL cover malformed protocol input, privacy, exact safe-error bytes/classes, canonical identity, and every configured boundary below, at, and above its limit. A pinned `@ai-sdk/gateway@4.0.52` client test SHALL complete supported unary and streaming text requests through the production Go handler and assert client-observable content, usage, finish behavior, ordered stream events, clean EOF, and non-success classification while raw Go assertions remain response authority.

#### Scenario: Committed golden replay
- **WHEN** the phase 2 semantic request goldens are replayed through Go
- **THEN** each record SHALL stop at the exact stage and result in the stage matrix
- **AND** no multi-capability golden SHALL be treated as evidence for every activated capability

#### Scenario: Dedicated supported scalar request
- **WHEN** a unary request contains only supported text, scalar, stop-sequence, text-format, and reasoning values
- **THEN** it SHALL execute once with exact mapped presence and values

#### Scenario: Focused unsupported requests
- **WHEN** focused schema-valid requests activate one unsupported capability each
- **THEN** each SHALL report its own typed capability after schema validation and before policy, resolution, or invocation

#### Scenario: Registered client completes unary text
- **WHEN** the pinned Gateway client sends a supported unary text request to the production handler
- **THEN** the request SHALL invoke the recording model once and the client SHALL consume the successful result

#### Scenario: Registered client completes streaming text
- **WHEN** the pinned Gateway client sends a supported streaming text request to the production handler
- **THEN** the request SHALL invoke the recording model once and the client SHALL consume the strict ordered SSE result through clean EOF

#### Scenario: Client normalization does not hide server assertions
- **WHEN** the registered client replaces unary fields or permissively consumes streaming fields
- **THEN** raw Go tests SHALL independently assert server warnings, canonical response identity, response schemas, privacy, state, framing, and byte bounds

### Requirement: Strict unary text response mapping
A successful provider result SHALL be mapped through private DTOs. The runtime SHALL accept only ordered text content for this phase and SHALL preserve required empty text. It SHALL map every registered warning variant through one value-safe mapper shared with streaming output. That mapper SHALL never copy arbitrary provider `Feature`, `Setting`, `Message`, or `Details` strings. It SHALL map `unsupported` to `feature: "model capability"` and `details: "a requested model capability is unsupported"`; `compatibility` to `feature: "model compatibility"` and `details: "a requested setting was adjusted for model compatibility"`; `deprecated` to `setting: "model setting"` and `message: "a requested model setting is deprecated"`; and `other` to `message: "the model reported a warning"`. It SHALL include no provider or model identity in warning prose. Required warning keys SHALL always be emitted with their normalized values. Unknown warning discriminators or invalid canonical identity SHALL fail mapping. Before allocating mapped content or warning slices, the runtime SHALL reject cardinality that cannot fit the minimum complete representation within the unary response limit. It SHALL map only registered finish reasons and optional raw finish reason. Usage token counts SHALL be absent or non-negative integers no greater than JavaScript's maximum safe integer, with the registered input/output groups always present; provider raw usage SHALL be omitted.

#### Scenario: Complete text result maps successfully
- **WHEN** a provider returns ordered text, known warning variants, valid usage, and a valid finish reason
- **THEN** the strict response SHALL preserve public text and required empty text while warnings contain only approved normalized values

#### Scenario: Provider emits an unsupported output family
- **WHEN** a provider returns non-text generated content during a text-only call
- **THEN** response mapping SHALL fail safely before HTTP 200 is committed

#### Scenario: Usage is invalid
- **WHEN** any known usage count is negative or exceeds `9007199254740991`
- **THEN** response mapping SHALL fail safely before HTTP 200 is committed

#### Scenario: Warning contains hostile private values
- **WHEN** a known warning variant contains a credential, URL, request or response body, header, private backend model ID, provider identity, or arbitrary prose in any warning string field
- **THEN** none of those values SHALL appear in the unary response
- **AND** the warning SHALL use approved normalized values or mapping SHALL fail safely before HTTP 200

#### Scenario: Empty warning strings normalize safely
- **WHEN** a known warning variant contains empty values for its provider-domain strings
- **THEN** the mapper SHALL emit fixed required generic values rather than copying empty or arbitrary values
- **AND** the response SHALL remain valid

#### Scenario: Warning cardinality cannot fit the response
- **WHEN** content or warning slice cardinality exceeds the maximum that its minimum representation can fit within the unary response limit
- **THEN** mapping SHALL fail before allocating same-sized output slices

#### Scenario: Warning or finish discriminator is invalid
- **WHEN** a provider returns an unknown warning type or unknown unified finish reason
- **THEN** response mapping SHALL fail safely before HTTP 200 is committed
