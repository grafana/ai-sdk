## MODIFIED Requirements

### Requirement: Minimal unary success response

A successful unary response SHALL contain only ordered supported text and function-tool-call `content`, `finishReason`, and `usage`. The handler SHALL accept only registered finish reasons and non-negative usage counts no greater than JavaScript's maximum safe integer. Provider warnings, request data, response IDs, timestamps, model IDs, provider identity, headers, bodies, raw usage, and top-level provider metadata SHALL be omitted. Eligible text/function-tool-call content SHALL contain only the closed OpenAI itemId or Anthropic direct caller metadata projection defined by the streaming runtime, without adding new content types. Unary OpenAI itemId SHALL be a nonempty ASCII `[A-Za-z0-9_-]+` identifier; unary text has no public text ID to compare against. The registered Gateway client owns unary `warnings`, `request`, and `response`; raw response-body details outside this minimal contract are not guaranteed.

#### Scenario: Valid text result
- **WHEN** the model returns text, a registered finish reason, and valid usage
- **THEN** the handler SHALL preserve those values and emit no other top-level members

#### Scenario: Unsupported provider result
- **WHEN** the model returns content outside the supported text/function-tool-call subset, an unknown finish reason, invalid usage, `nil, nil`, or panics
- **THEN** the handler SHALL return the fixed internal-error document before committing HTTP 200

#### Scenario: Provider-private fields
- **WHEN** the model result contains warnings, response metadata, raw usage, backend identity, top-level provider metadata or unknown content metadata
- **THEN** none of those values SHALL appear in the unary response document; eligible approved content metadata alone SHALL survive

#### Scenario: Unary supported content metadata
- **WHEN** supported unary text includes a valid OpenAI itemId or a basic function-tool-call has valid OpenAI itemId or Anthropic direct caller metadata
- **THEN** only that approved content metadata SHALL survive on its original part, without adopting top-level result metadata

### Requirement: Bounded preflight and standard success encoding

Before encoding, the handler SHALL reject content cardinality or aggregate content, recognized-namespace raw-metadata and raw-finish string bytes that cannot fit the configured unary budget using overflow-safe accounting. Unknown provider-domain namespaces SHALL be discarded without sizing or parsing their values; each recognized namespace SHALL be byte-bounded before parsing and SHALL obey the streaming runtime's shallow-shape and field validation policy even if it contains only unknown fields. It SHALL validate UTF-8 only after the size preflight so scanning remains bounded. The complete minimal private DTO SHALL then be encoded with standard Go JSON, rejected when the final bytes exceed the configured limit, and committed only after successful encoding and the final size check. Provider-domain JSON marshalers SHALL NOT control the response. Standard encoding MAY allocate a bounded constant multiple of the configured limit for worst-case escaping.

#### Scenario: Preflight rejects oversized provider values
- **WHEN** content count or aggregate raw string bytes exceed the unary budget
- **THEN** the result SHALL fail before UTF-8 scanning or JSON encoding

#### Scenario: Invalid supported metadata
- **WHEN** a recognized content metadata namespace is malformed, has duplicate keys, wrong approved-field type or value, hostile identifier, excessive depth in an unknown sibling, or excess raw bytes even if only unknown fields are present
- **THEN** no partial success document SHALL be committed and only the fixed safe internal error SHALL be returned

#### Scenario: Unary unknown namespace and bounded unknown sibling
- **WHEN** supported content carries an unknown provider-domain namespace with deep, oversized or malformed raw value and a recognized namespace with bounded well-formed shallow unknown fields
- **THEN** the unknown namespace and unknown fields SHALL be dropped without exposing their values, and only approved fields (if present) SHALL be emitted

#### Scenario: Escaping crosses the final boundary
- **WHEN** raw bytes pass preflight but standard JSON escaping makes the encoded response exceed the limit
- **THEN** the handler SHALL return the fixed internal error before committing HTTP 200

#### Scenario: Response byte boundary
- **WHEN** the encoded response is below, exactly at, or above the configured limit
- **THEN** only complete in-limit documents SHALL receive HTTP 200
