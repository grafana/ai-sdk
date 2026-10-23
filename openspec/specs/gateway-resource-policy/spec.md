# gateway-resource-policy Specification

## Purpose
Assign every Gateway resource limit to one shared boundary (the route request body, the outbound model transport, execution durations or the plugin host), so that API adapters own no size accounting and never reject, omit or truncate output because of its size.

## Requirements

### Requirement: Route-owned request-body bound
The Gateway SHALL apply the `max_request_bytes` setting (default 1 MiB) once to every plugin route, before any route handler reads the body. A route that reads the body to EOF SHALL observe at most that many bytes. Adding a route SHALL NOT require route-specific bound code. The bound SHALL NOT read the body itself, so a refused caller's body is never read.

#### Scenario: Body below, at and above the bound
- **WHEN** an authorized language-model request body is one byte below, exactly at, or one byte above `max_request_bytes`
- **THEN** the first two SHALL reach mapping and selection
- **AND** the third SHALL return the existing fixed ProviderWire invalid-request document with zero selection and zero model calls

#### Scenario: A non-ProviderWire route inherits the bound
- **WHEN** a body-reading handler that is not ProviderWire is served on a plugin route for a configured-access caller and for a BYOK caller
- **THEN** both callers SHALL read the body through the same bound, and reading past it SHALL fail

#### Scenario: Refused caller
- **WHEN** access policy refuses a caller on a body-carrying route
- **THEN** the permission document SHALL be written without reading the request body

#### Scenario: Setting keeps its meaning
- **WHEN** an operator sets or omits `max_request_bytes`
- **THEN** the configured value or the 1 MiB default SHALL apply, and a non-positive or non-integer value SHALL fail startup with a secret-safe reason naming the setting

### Requirement: Outbound model transport bounds
Every Gateway-constructed model, including each configured provider type and each BYOK account, SHALL use one shared model HTTP client. That client SHALL reject redirects, wait at most 10 seconds for response headers, and fail any single provider response whose decompressed body exceeds 16 MiB. Neither bound SHALL be an operator setting.

#### Scenario: Decompressed bytes are counted
- **WHEN** a provider returns a gzip-encoded body under 16 MiB compressed whose decompressed size exceeds 16 MiB
- **THEN** the read SHALL fail with the transport's response-too-large error once the decompressed bytes exceed the bound

#### Scenario: Each request has its own bound
- **WHEN** two sequential requests through the shared client each return just under 16 MiB
- **THEN** both SHALL succeed, because the bound applies to each response independently and not to the client or the logical call

#### Scenario: Response exceeds the bound
- **WHEN** a provider response exceeds 16 MiB after decompression
- **THEN** the call SHALL fail through the existing fallback and safe-error classification without returning a partial result

### Requirement: Execution time bounds
The language-model handler SHALL bound each request by the configured `max_model_duration`, each established stream by a fixed 30-second idle duration between represented parts, and each post-terminal drain by a fixed 1-second drain duration. Drain SHALL be bounded by its duration alone, without a part count.

#### Scenario: Continuously ready provider during drain
- **WHEN** a canceled provider channel stays ready with parts after terminal handling
- **THEN** the drain SHALL stop when the drain duration expires, regardless of how many parts it consumed

#### Scenario: Many provider parts within the deadlines
- **WHEN** a provider emits more than 100,000 valid parts before finish within the model and idle durations
- **THEN** every part SHALL be represented in order and the finish SHALL be written

### Requirement: Adapters own no size accounting
A Gateway API adapter SHALL NOT reject, omit or truncate a request-valid or output-valid value because of its size, cardinality or encoded length. Size SHALL be bounded only by the route request-body bound, the outbound transport, the execution durations and the plugin host.

#### Scenario: Large outputs succeed
- **WHEN** a provider returns any of the following, each valid and within the outbound bound:
  - a unary result whose encoded document exceeds 8 MiB;
  - a stream event whose complete frame exceeds 1 MiB;
  - a start with more warnings than a 1 MiB frame could hold;
  - raw usage above 1,048,576 bytes;
- **THEN** the Gateway SHALL return it complete and successful

#### Scenario: Large failure documents succeed
- **WHEN** an HTTP or SSE failure carries an execution overview and native summaries whose encoded document exceeds 64 KiB
- **THEN** the complete failure document SHALL be written with its overview and summaries

#### Scenario: Validity still fails
- **WHEN** an output of any size contains invalid UTF-8, an unknown discriminator, a non-object metadata namespace, non-object raw usage or an unsafe token count
- **THEN** the existing safe error behavior SHALL apply, without partial output

### Requirement: Single complete encoding
The Gateway SHALL encode each unary result, finish frame and HTTP or SSE error once, as a complete document that already contains its optional execution metadata and native summaries. An error SHALL fall back to its fixed base document only when that encoding fails. JSON SHALL never be truncated, and a primary output that fails validation or encoding SHALL never become a success.

#### Scenario: Enriched error cannot be encoded
- **WHEN** encoding an error document with its optional metadata fails
- **THEN** the fixed base error document with the same status and classification SHALL be written instead

#### Scenario: Primary success cannot be encoded
- **WHEN** a unary result or finish fails validation or encoding
- **THEN** the unary handler SHALL return the fixed internal error before HTTP 200, or the stream SHALL attempt one terminal internal error, and no candidate SHALL be replayed

### Requirement: Documented limit ownership
The "Limits" section of `ai-gateway/README.md` SHALL list each retained Gateway resource limit with exactly one owning boundary. It SHALL also list each removed adapter limit with the bound that replaces it, and the trade-offs of this placement.

#### Scenario: Ledger content
- **WHEN** an operator reads the Limits section
- **THEN** it SHALL name the owner of the request body, outbound response bytes and header timeout, model, idle and drain durations, BYOK accounts, platform route deadline and message size, and admission
- **AND** it SHALL list the removed unary, frame, part-count, raw-usage, error-document and summary-source limits with their replacements
- **AND** it SHALL state that per-request memory grows with response size, JSON encoding expansion and concurrency, and that the host owns admission

### Requirement: Large-output client evidence
Automated tests SHALL prove that both registered clients (the pinned TypeScript Gateway client and the Go `providers/grafana` client) consume large outputs through the real plugin route host. They SHALL NOT add or modify recorded provider fixtures.

#### Scenario: Both clients through the test host
- **WHEN** a fake native provider behind the built plugin returns a unary result above 8 MiB, a stream frame above 1 MiB and raw usage above 1 MiB, each within the outbound bound
- **THEN** both clients SHALL receive the complete values in unary and streaming mode, and no file under `test/conformance/*/recorded/` SHALL change
