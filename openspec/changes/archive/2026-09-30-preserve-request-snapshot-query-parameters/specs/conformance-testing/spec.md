## MODIFIED Requirements

### Requirement: Expected request input format
Each provider-backed conformance test case SHALL contain an `expected-requests.jsonl` file that captures the upstream TypeScript provider request inputs for that test case. The file SHALL contain one JSON object per provider API request, in request order. Each request snapshot SHALL include the HTTP method, normalized escaped request target in the existing `path` field, normalized behavior-affecting headers, and decoded JSON request body. The request target SHALL include a nonempty URL query in addition to the escaped pathname.

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
- **AND** `path` contains the escaped pathname followed by the nonempty query, if present, without scheme, authority or fragment

### Requirement: Go request input comparison
The Go conformance runner SHALL capture actual Go provider requests during replay and compare them against `expected-requests.jsonl`. The runner SHALL fail when request counts differ, method or normalized escaped request target differs, normalized headers differ, or decoded JSON bodies differ. Comparison of the `path` field SHALL be exact, including query spelling and pair order; it SHALL NOT accept a query-bearing request as matching a legacy path-only expectation.

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
- **WHEN** the Go provider sends a different HTTP method or escaped request target than the expected snapshot
- **THEN** the conformance test fails and identifies the mismatched request index

#### Scenario: API version differs only in query
- **WHEN** the expected request target is `/v1/messages?api-version=2024-10-21&feature=a&feature=b` and the actual target differs only in the `api-version` value
- **THEN** the request comparison fails with a path mismatch and the request index

#### Scenario: Repeated values differ only in order
- **WHEN** otherwise identical expected and actual requests contain `feature=a&feature=b` and `feature=b&feature=a` respectively
- **THEN** the request comparison fails with a path mismatch and the request index

#### Scenario: Query is missing
- **WHEN** otherwise identical expected and actual requests differ only by presence of a nonempty query
- **THEN** the request comparison fails with a path mismatch and the request index

## ADDED Requirements

### Requirement: Escaped request target normalization
TypeScript and Go request capture SHALL normalize serialized HTTP request URLs to the escaped pathname plus the nonempty query. They SHALL omit scheme, authority and fragment, SHALL use `/` for an empty pathname, and SHALL omit an empty query marker. They SHALL preserve query parameter order, repeated keys and values, percent escaping, literal plus signs, key-only parameters and empty values without decoding, re-encoding, sorting or collapsing query pairs. Existing queryless escaped paths SHALL remain unchanged.

#### Scenario: Behavior-affecting query and repeated values
- **WHEN** either implementation captures `/v1/messages?api-version=2024-10-21&feature=a&feature=b`
- **THEN** `path` is `/v1/messages?api-version=2024-10-21&feature=a&feature=b`

#### Scenario: Escaped path and query delimiters
- **WHEN** either implementation captures `/v1/a%2Fb?q=a%26b%3Dc&space=a+b&space=a%20b`
- **THEN** `path` is `/v1/a%2Fb?q=a%26b%3Dc&space=a+b&space=a%20b`
- **AND** no escaped delimiter is interpreted as a path separator or query pair separator

#### Scenario: Interleaved duplicates and empty values
- **WHEN** either implementation captures `/v1/messages?feature=a&flag&feature=b&empty=&feature=a`
- **THEN** `path` preserves that complete target without regrouping, deduplicating or adding an equals sign to `flag`

#### Scenario: Percent-encoded UTF-8 and escape spelling
- **WHEN** either implementation captures `/v1/%E2%9C%93?q=%e2%9c%93`
- **THEN** `path` retains `/v1/%E2%9C%93?q=%e2%9c%93` without changing escape spelling

#### Scenario: Queryless request
- **WHEN** either implementation captures `/v1/messages`
- **THEN** `path` remains `/v1/messages`

#### Scenario: Empty query marker
- **WHEN** either implementation captures `/v1/messages?`
- **THEN** `path` is `/v1/messages`

#### Scenario: Absolute URL with empty path and fragment
- **WHEN** either implementation captures `https://example.test?x=1#ignored`
- **THEN** `path` is `/?x=1`

#### Scenario: Literal apostrophe in a serialized query
- **WHEN** either implementation captures `/v1/messages?q=O'Reilly` or `https://example.test/v1/messages?q=O'Reilly`
- **THEN** `path` is `/v1/messages?q=O'Reilly`, not `/v1/messages?q=O%27Reilly`

#### Scenario: Query delimiter appears only in a fragment
- **WHEN** either implementation captures `/v1/messages#ignored?not=a-query`
- **THEN** `path` is `/v1/messages`, without the fragment's apparent query

### Requirement: Cross-language request target regression evidence
The conformance harness SHALL maintain shared synthetic request cases and a committed TypeScript-generated `expected-requests.jsonl` under `test/conformance/testdata/request-snapshots/`, outside provider `recorded/` and `upstream/` inputs. At least one request snapshot SHALL contain a nonsecret behavior-affecting API-version query and ordered repeated values. TypeScript tests SHALL assert independently declared target values and verify the committed expectation is current without rewriting it during normal tests. Go tests SHALL load the same expectation through the production loader, capture corresponding requests through the production snapshot function, and exercise the production comparator for matching and mismatching queries. This evidence SHALL be identified as harness-only sensitivity testing, not recorded provider behavior or new provider support.

#### Scenario: Matching cross-language snapshot
- **WHEN** Go captures the requests represented by the shared synthetic cases
- **THEN** the production comparator accepts them against the committed TypeScript-generated request snapshots
- **AND** both implementations' target assertions retain the API version and repeated values

#### Scenario: Stale TypeScript expectation
- **WHEN** recomputing snapshots from the shared cases differs from the committed JSONL
- **THEN** the normal TypeScript test/check fails without changing the committed file

#### Scenario: Executable mismatch witness
- **WHEN** a focused test captures a request differing only in API version, repeated-value order or query presence from the committed TypeScript expectation
- **THEN** the production comparator reports a path mismatch and fails its isolated test invocation
- **AND** the enclosing regression test verifies that rejection without failing the normal suite

#### Scenario: Explicit harness regeneration
- **WHEN** a contributor explicitly regenerates the harness request expectations
- **THEN** the registered conformance tools' request snapshot normalizer and JSONL writer produce the expectations from controlled nonsecret cases
- **AND** no provider response inputs, upstream pins or fixture provenance are modified
