## ADDED Requirements

### Requirement: Optional bounded provider raw usage consumption
On a successful unary result and a streaming finish, the Grafana client SHALL preserve a supplied `usage.raw` as a `provider.Usage.Raw` JSON object, including nested provider-native fields and `{}`; absent raw SHALL remain absent. The client SHALL reject present null, scalar, array, malformed or incomplete JSON, raw input longer than 1,048,576 bytes, and raw input exceeding its applicable configured `UnaryBytes` or `StreamEventBytes`, before retaining it in `provider.Usage.Raw`. It SHALL validate original bounded unary and event JSON bytes for UTF-8 and escaped surrogate pairs before Go JSON decoding normalizes invalid Unicode; the same rule applies to raw object keys and nested values. Unpaired escaped high or low surrogates and invalid UTF-8 SHALL be rejected; valid pairs SHALL be accepted. The existing full unary, complete-event, cumulative-stream, and event-count limits SHALL still apply, and intermediate `decodeFields`/usage-map copies SHALL remain bounded by those full-envelope limits. The client SHALL continue to validate known normalized token counts and filter all unrelated unknown usage and server-owned metadata; distinct `type: "raw"` stream-part filtering SHALL remain governed by `IncludeRawChunks` and SHALL NOT filter `usage.raw`.

#### Scenario: Unary and finish have present or absent raw
- **WHEN** a bounded valid unary result or finish contains nested raw usage, `{}`, or no raw member
- **THEN** the resulting Go usage SHALL preserve the supplied JSON object semantics, retain an empty object when supplied, or leave `Raw` absent while preserving validated normalized counts

#### Scenario: Hostile unary response contains invalid raw
- **WHEN** a successful HTTP 200 unary response contains malformed, null, non-object, or over-limit `usage.raw`
- **THEN** the client SHALL return a bounded non-retryable protocol error without any partial result or raw data in its error text

#### Scenario: Hostile streaming finish contains invalid raw
- **WHEN** a streaming finish contains malformed, null, non-object, or over-limit `usage.raw`
- **THEN** the client SHALL emit at most one bounded terminal non-retryable protocol `PartError` and close, without delivering the invalid finish

#### Scenario: Unicode is validated before Go normalization in both paths
- **WHEN** bounded unary or streaming finish JSON contains invalid UTF-8 or an escaped lone surrogate in `usage.raw`, including a nested key or value
- **THEN** the client SHALL reject the response with the path's bounded protocol error, without retaining normalized replacement characters in raw usage
- **WHEN** `usage.raw` contains a valid escaped surrogate pair
- **THEN** both paths SHALL accept the object under the same size and shape limits

#### Scenario: Raw part filtering does not erase usage
- **WHEN** a finish includes `usage.raw` and `IncludeRawChunks` is false
- **THEN** the finish SHALL retain its raw usage even when independent `type: "raw"` stream parts are filtered
