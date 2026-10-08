## MODIFIED Requirements

### Requirement: Bounded normalized unary consumption
For a successful unary response, the client SHALL require a JSON media type, read no more than the configured unary-response byte limit, accept one complete JSON document, and map the currently supported registered content, finish reason, usage and supplied result/content providerMetadata into provider.GenerateResult. It SHALL reject malformed required fields, unknown content or finish discriminators, negative or non-JavaScript-safe known usage, trailing JSON, and oversized input. The client SHALL replace server-supplied request and response: Request.Body SHALL be the locally encoded request, and Response.Headers and Response.Body SHALL come from the bounded HTTP response. Warnings SHALL preserve valid server warning fields in order, defaulting to a non-nil empty slice when absent or null. Warning decoding SHALL use the same registered types and validation as streaming. Server response identity SHALL retain the registered Gateway-hop replacement behavior. Ordinary providerMetadata SHALL be decoded independently as opaque object-valued namespaces under gateway-provider-metadata without Gateway imports, namespace/key inventories, or IncludeRawChunks gating. Present null, scalar, array, malformed or invalid-UTF-8 metadata SHALL fail explicitly under the existing bounded protocol-error path rather than being selectively omitted.

#### Scenario: Minimal unary success is consumed
- **WHEN** the server returns valid ordered text content, registered finish reason, and valid usage
- **THEN** the client SHALL return those fields plus local request metadata, HTTP response headers/body, and empty warnings when none were supplied

#### Scenario: Server supplies client-owned fields
- **WHEN** a successful body includes warnings, request metadata, response ID, model ID, timestamp, headers, body, or additional provider metadata
- **THEN** the result SHALL preserve valid warnings and ordinary result/content providerMetadata while replacing server request/response fields with the registered client-owned Gateway-hop values

#### Scenario: Unary response exceeds its bound
- **WHEN** the response is one byte larger than the configured unary limit or has trailing data
- **THEN** the call SHALL fail without returning a partial result or retaining an unbounded body

#### Scenario: Unary result is not in a supported family
- **WHEN** a successful body contains an output discriminator outside the client's currently supported output union
- **THEN** the client SHALL fail explicitly until the capability's later work package extends the closed mapper

#### Scenario: Unary warning is malformed
- **WHEN** a warning has an unknown discriminator or lacks a required field
- **THEN** the call SHALL fail rather than exposing unvalidated warning content

### Requirement: Optional bounded provider raw usage consumption
On a successful unary result and a streaming finish, the Grafana client SHALL preserve a supplied `usage.raw` as a `provider.Usage.Raw` JSON object, including nested provider-native fields and `{}`; absent raw SHALL remain absent. The client SHALL reject present null, scalar, array, malformed or incomplete JSON, and a retained raw object larger than 1,048,576 bytes. This raw-object limit applies to the retained representation after JSON decoding/compaction removes insignificant whitespace around and inside the value; the original complete unary response or event SHALL remain bounded by its configured `UnaryBytes` or `StreamEventBytes`. The client SHALL validate those original bounded documents for UTF-8 and JSON syntax before Go decoding can normalize invalid bytes. Valid JSON with lone or paired escaped surrogates SHALL be accepted, with raw-object escapes preserved in `provider.Usage.Raw`. The existing cumulative-stream and event-count limits SHALL still apply, and intermediate `decodeFields`/usage-map copies SHALL remain bounded by the full-response or event limits. The client SHALL continue to validate known normalized token counts and filter unrelated unknown usage and unrepresented transport fields without filtering ordinary registered providerMetadata; distinct `type: "raw"` stream-part filtering SHALL remain governed by `IncludeRawChunks` and SHALL NOT filter `usage.raw`.

#### Scenario: Unary and finish have present or absent raw
- **WHEN** a bounded valid unary result or finish contains nested raw usage, `{}`, or no raw member
- **THEN** the resulting Go usage SHALL preserve the supplied JSON object semantics, retain an empty object when supplied, or leave `Raw` absent while preserving validated normalized counts

#### Scenario: Hostile unary response contains invalid raw
- **WHEN** a successful HTTP 200 unary response contains malformed, null, non-object, or over-limit retained `usage.raw`
- **THEN** the client SHALL return a bounded non-retryable protocol error without any partial result or raw data in its error text

#### Scenario: Hostile streaming finish contains invalid raw
- **WHEN** a streaming finish contains malformed, null, non-object, or over-limit retained `usage.raw`
- **THEN** the client SHALL emit at most one bounded terminal non-retryable protocol `PartError` and close, without delivering the invalid finish

#### Scenario: UTF-8 and JSON escapes in both paths
- **WHEN** bounded unary or streaming finish JSON contains invalid UTF-8 in `usage.raw`, including a nested key or value
- **THEN** the client SHALL reject the response with the path's bounded protocol error, without retaining normalized replacement characters in raw usage
- **WHEN** `usage.raw` contains valid JSON with lone or paired escaped surrogates
- **THEN** both paths SHALL accept the object under the same size and shape limits and preserve its raw JSON escapes

#### Scenario: Raw part filtering does not erase usage
- **WHEN** a finish includes `usage.raw` and `IncludeRawChunks` is false
- **THEN** the finish SHALL retain its raw usage even when independent `type: "raw"` stream parts are filtered

## ADDED Requirements

### Requirement: Independent opaque stream metadata consumption
The Grafana client SHALL independently decode providerMetadata on supported text/reasoning start/delta/end, reasoning-file, source, tool-input start/delta/end, function call, basic result and finish parts. It SHALL preserve namespace objects, nested values, original event order and absent versus empty presence without IncludeRawChunks. Original accepted events, retained copies and metadata cardinality SHALL remain bounded by the existing event, cumulative byte and count limits. Invalid metadata SHALL use the bounded terminal non-retryable protocol-error path without reflecting the value or delivering the invalid part. Gateway lifecycle and raw filtering SHALL remain unchanged.

#### Scenario: Unknown namespace survives all supported placements
- **WHEN** valid events carry future object-valued namespaces with nested null/false/zero/empty values and explicit empty metadata at supported positions
- **THEN** the independent Go client SHALL preserve those values and positions under existing limits without importing server code

#### Scenario: Malformed metadata on finish
- **WHEN** a bounded finish contains a null namespace or malformed metadata shape
- **THEN** the client SHALL emit at most one bounded protocol PartError and close without delivering the invalid finish or metadata-free success
