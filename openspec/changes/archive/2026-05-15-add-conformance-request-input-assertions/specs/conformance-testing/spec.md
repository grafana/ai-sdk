## ADDED Requirements

### Requirement: Expected request input format
Each conformance test case SHALL contain an `expected-requests.jsonl` file that captures the upstream TypeScript provider request inputs for that test case. The file SHALL contain one JSON object per provider API request, in request order. Each request snapshot SHALL include the HTTP method, request path, normalized behavior-affecting headers, and decoded JSON request body.

#### Scenario: Single request fixture
- **WHEN** a test case performs one provider API request
- **THEN** `expected-requests.jsonl` contains exactly one request snapshot line

#### Scenario: Multi-step request fixture
- **WHEN** a test case performs multiple provider API requests
- **THEN** `expected-requests.jsonl` contains one request snapshot line per request in the same order as the requests occurred

#### Scenario: Request snapshot shape
- **WHEN** a request snapshot is parsed
- **THEN** it includes `method`, `path`, `headers`, and `body` fields
- **AND** `headers` is a JSON object of normalized header names to normalized values
- **AND** `body` is the decoded JSON request body as a JSON object

### Requirement: TypeScript request input capture
The TypeScript conformance generation and recording tools SHALL capture provider request inputs while producing fixture output. The generation tool SHALL capture requests sent to its replay server. The recording tool SHALL capture requests sent through its provider proxy and SHALL redact secrets before writing request snapshots.

#### Scenario: Generate request snapshots
- **WHEN** `test/conformance/tools/generate.mts` regenerates expected output for a test case
- **THEN** it writes `expected-requests.jsonl` from the upstream TypeScript provider requests observed during that run

#### Scenario: Record request snapshots
- **WHEN** `test/conformance/tools/record.mts` records a fixture from a real provider API
- **THEN** it writes `expected-requests.jsonl` from the upstream TypeScript provider requests observed during recording
- **AND** committed request snapshots do not contain API keys, bearer tokens, or other secret header values

### Requirement: Go request input comparison
The Go conformance runner SHALL capture actual Go provider requests during replay and compare them against `expected-requests.jsonl`. The runner SHALL fail when request counts differ, method or path differs, normalized headers differ, or decoded JSON bodies differ.

#### Scenario: Matching request input
- **WHEN** the Go provider sends the same request inputs as the upstream TypeScript provider
- **THEN** the conformance test passes the request input assertion

#### Scenario: Request count mismatch
- **WHEN** the Go provider sends fewer or more provider API requests than `expected-requests.jsonl` contains
- **THEN** the conformance test fails with a request count mismatch

#### Scenario: Request body mismatch
- **WHEN** the Go provider request body has a missing field, extra field, or different value compared with the expected request body
- **THEN** the conformance test fails and identifies the mismatched request index

#### Scenario: Request method or path mismatch
- **WHEN** the Go provider sends a different HTTP method or request path than the expected snapshot
- **THEN** the conformance test fails and identifies the mismatched request index

### Requirement: Order-insensitive JSON object comparison
Request body comparison SHALL ignore JSON object field ordering by comparing decoded JSON values instead of raw request body bytes. The comparison SHALL preserve exact ordering for JSON arrays and for the sequence of request snapshots in `expected-requests.jsonl`, except tool declaration arrays SHALL be normalized by tool identity before comparison.

#### Scenario: Same object fields in different order
- **WHEN** the expected and actual request bodies contain the same JSON object fields with the same values but in different serialized order
- **THEN** the request input assertion passes

#### Scenario: Ordered array differs
- **WHEN** the expected and actual request bodies contain an array with the same elements in different order
- **THEN** the request input assertion fails

#### Scenario: Tool declaration array differs only by order
- **WHEN** the expected and actual request bodies contain a `tools` array with the same tool declarations in different order
- **THEN** the request input assertion passes

#### Scenario: Multi-step request order differs
- **WHEN** the Go provider sends semantically matching requests in a different step order than the expected request snapshots
- **THEN** the request input assertion fails

### Requirement: Request header normalization
Request header comparison SHALL use provider-specific allowlists of behavior-affecting headers. Header names SHALL be normalized to lowercase, values SHALL be trimmed, secret values SHALL be redacted, and volatile transport headers SHALL be excluded from snapshots and comparisons.

#### Scenario: Header name casing differs
- **WHEN** the expected and actual requests use different casing for the same included header name
- **THEN** the request input assertion compares them as the same header

#### Scenario: Volatile header differs
- **WHEN** a volatile transport header such as `host`, `content-length`, `user-agent`, `accept-encoding`, or connection management differs
- **THEN** the request input assertion ignores that header

#### Scenario: Behavior-affecting header differs
- **WHEN** an included provider header such as a beta or version header differs
- **THEN** the request input assertion fails

#### Scenario: Beta header order differs
- **WHEN** expected and actual Anthropic beta headers contain the same comma-separated beta values in different order or with different whitespace
- **THEN** the request input assertion passes

#### Scenario: Secret header present
- **WHEN** an included auth header is present in a captured request
- **THEN** the snapshot records a redacted value rather than the secret header value
- **AND** the comparison verifies the normalized redacted representation

### Requirement: Provider-specific request value normalization
Provider-specific request value normalization SHALL be narrow and documented. For Anthropic, tool-result JSON content SHALL compare semantically whether it is serialized as a raw JSON string or as a single text content block containing JSON, and `web_search_result.page_age: null` SHALL compare the same as an omitted `page_age`.

#### Scenario: Anthropic tool result JSON serialization differs
- **WHEN** expected and actual Anthropic request bodies contain equivalent `tool_result` JSON content serialized with different JSON object field order or different raw-string versus text-block shape
- **THEN** the request input assertion passes

### Requirement: Grafana provider-wire conformance boundary
Grafana provider-wire conformance tests SHALL continue validating provider-wire method, path, headers, auth, streaming mode, and request body decodability at the Grafana transport boundary. Upstream TypeScript request input snapshots SHALL be compared against the downstream provider API request produced after the fake hosted endpoint decodes `provider.CallOptions`, not against the provider-wire request body.

#### Scenario: Grafana downstream request matches upstream
- **WHEN** the Grafana conformance fake hosted endpoint decodes provider-wire `CallOptions` and forwards them through the Anthropic provider conversion path
- **THEN** the downstream Anthropic provider API request matches `expected-requests.jsonl`

#### Scenario: Provider-wire transport is invalid
- **WHEN** the Grafana provider-wire request has invalid method, path, required headers, auth, streaming mode, content type, accept header, or undecodable body
- **THEN** the Grafana conformance test fails at the provider-wire boundary validation

#### Scenario: Provider-wire body shape differs from upstream provider body
- **WHEN** the Grafana provider-wire body serializes Go `provider.CallOptions`
- **THEN** the conformance suite does not compare that body directly to the upstream TypeScript provider API request snapshot
