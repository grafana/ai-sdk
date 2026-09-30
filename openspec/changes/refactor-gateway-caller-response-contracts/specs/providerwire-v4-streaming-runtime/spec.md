## MODIFIED Requirements

### Requirement: Normalized stream start and value-safe warnings
Every committed writable stream SHALL emit exactly one initial stream-start. Initial provider warnings SHALL preserve registered union/values/order rather than fixed prose, including required empty strings and documented optional-empty Go details normalization. Each warning string SHALL be at most 4096 UTF-8 bytes; count/aggregate and complete escaped-frame bounds SHALL precede output. Inactive fields SHALL be omitted. Unknown/invalid/oversized warnings SHALL yield the existing empty start and at most one fixed terminal internal error, never partial warning output.

Provider start SHALL remain valid only as the first part. Missing start SHALL emit warnings: [] before processing the first part; late/duplicate starts remain invalid. Anthropic initial API preflight SHALL remain setup-owned; successful preflight emits one start with request-conversion warnings before the pre-read event. Finish warnings remain empty. No additional reader/lifecycle layer SHALL be introduced.

#### Scenario: Known warnings retain values
- **WHEN** the first provider start contains valid unsupported/compatibility/deprecated/other warnings
- **THEN** one public start SHALL preserve meaningful fields/order within all bounds without arbitrary prose entering telemetry

#### Scenario: Start or warnings are invalid
- **WHEN** warning validation fails or a start is late/duplicated
- **THEN** the existing bounded safe failure path SHALL remain without a second or partially encoded public start

### Requirement: Canonical metadata and text block state
The state machine SHALL retain current start/output/response-metadata/text/tool/source/reasoning/finish authority. At most one response-metadata part before output SHALL preserve supplied valid response id/actual modelId/timestamp; absent actual modelId SHALL remain absent rather than become canonical. Logical route identity remains authoritative for routing/observation, not response substitution. Native transport/configuration/topology SHALL remain excluded.

Text IDs SHALL remain nonempty valid UTF-8 and globally unique within their family; one active block with matching deltas/ends and required empty deltas SHALL remain. Existing metadata placement/transport SHALL remain unchanged until #280; no future metadata codec or finish placement is a foundation completion gate. Provider errors SHALL not alter block/metadata state. All current part/frame/cancellation/finish/cleanup limits remain effective.

#### Scenario: Actual identity precedes output
- **WHEN** an alias resolves to a canonical route but supplied response identity differs or omits modelId
- **THEN** supplied registered values SHALL survive and absent modelId SHALL not be fabricated
- **AND** discovery/logical telemetry SHALL retain public route identity

#### Scenario: Text or metadata lifecycle is invalid
- **WHEN** a block is reused/mismatched or response metadata is duplicated/late
- **THEN** existing cancellation and at most one fixed terminal safe error SHALL remain

### Requirement: Ordered non-terminal provider errors
Reviewed structured errors available on trusted direct routes SHALL use gateway-caller-response-policy's minimal bounded projection in the existing PartError processing path. Existing public error API and source/stream lifetime SHALL remain unchanged. Whole native error/Data/body/header/URL/request/cause serialization SHALL remain forbidden. Unsupported message-only/unreviewed/internal/ambiguous aggregate sources SHALL retain safe diagnostics and explicit follow-ups, not an invasive representation layer.

Existing valid statusCode/retryable fields and ordered non-terminal provider behavior SHALL remain, including represented status 200. Status-zero transport and invalid status retain existing safe classification. Provider errors SHALL not alter active blocks, finish authority, part order, fallback commitment or core termination. Any richer Gateway error transport/API need SHALL be separately registered and coordinated with #299, not made a foundation dependency.

#### Scenario: Reviewed error precedes valid output
- **WHEN** a trusted direct provider emits available structured error evidence followed by valid parts and finish
- **THEN** the bounded reviewed error and later parts SHALL remain ordered through the existing reader/cleanup ownership

#### Scenario: Error source is unsupported or credential-bearing
- **WHEN** diagnostics are unreviewed/ambiguous or contain protected auth/transport sources
- **THEN** only minimal safe contracted diagnostics SHALL be emitted, with fixed provider-account auth prose where needed and no native transport leak
