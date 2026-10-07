## RENAMED Requirements

- FROM: `### Requirement: Canonical metadata and text block state`
- TO: `### Requirement: Native response metadata and text block state`

## MODIFIED Requirements

### Requirement: Normalized stream start and value-safe warnings

Every committed writable stream SHALL emit exactly one public stream-start as its first JSON event. The handler SHALL read the first provider part before selecting it: a provider stream-start is valid only as the first provider part and SHALL be consumed with its warnings mapped through the same explicit registered warning union used by unary output. Unsupported/compatibility SHALL preserve required feature and optional nonempty details; deprecated SHALL preserve required setting/message; other SHALL preserve required message. Required strings SHALL remain present even when empty, order/multiplicity SHALL survive and inactive fields SHALL be omitted. Optional empty Go details SHALL normalize to omission as a documented representation adaptation, not a privacy policy. Native backend names and ordinary application strings SHALL NOT be replaced with generic prose or heuristically censored.

Unknown warning discriminators, invalid represented strings or over-budget starts SHALL cause one empty public start followed by at most one synthetic terminal internal error when writable. Cardinality and aggregate active string bytes SHALL be bounded before mapped allocation and UTF-8 validation; standard JSON encoding and complete-frame limits SHALL still govern the final write. When the provider omits start, warnings SHALL be an empty array and the first part SHALL then be processed. Built-in Anthropic SHALL retain initial upstream-event error preflight, emit its start with conversion warnings before processing the pre-read event, and attach no warnings to finish. Duplicate/late starts SHALL remain invalid.

#### Scenario: Provider start carries known warnings
- **WHEN** the first provider part carries all registered warning variants with distinct native strings
- **THEN** exactly one public start SHALL preserve their active fields and order

#### Scenario: Required empties and optional details
- **WHEN** feature, setting or message is empty and details is absent/empty
- **THEN** required fields SHALL remain present and optional details SHALL be omitted

#### Scenario: Native warning strings resemble private identifiers
- **WHEN** a valid warning contains a backend model name, URL or token-looking application string not sourced from a credential-bearing structure
- **THEN** its registered value SHALL survive unchanged without enabling operator payload capture

#### Scenario: Warning cannot be represented safely
- **WHEN** the start contains an unknown warning type, invalid UTF-8, excessive warning count, aggregate string bytes or escaping-expanded frame size
- **THEN** no partial provider start SHALL be written and the handler SHALL emit one empty start and at most one safe terminal error when writable

#### Scenario: Provider omits start
- **WHEN** the first part is metadata, content, provider error or finish
- **THEN** the client SHALL receive one empty-warning start followed by that part in order

#### Scenario: Provider start is late or duplicated
- **WHEN** a start appears after an earlier provider part
- **THEN** the handler SHALL attempt at most one synthetic terminal error and SHALL NOT emit a second start

#### Scenario: Anthropic warnings follow successful preflight
- **WHEN** conversion produces warnings and initial event preflight succeeds
- **THEN** Anthropic SHALL emit them on PartStreamStart before the pre-read event and its finish SHALL have no warnings

#### Scenario: Anthropic initial API failure remains setup failure
- **WHEN** initial upstream-event preflight returns an API error
- **THEN** DoStream SHALL return it before exposing a StreamResult or provider start

### Requirement: Native response metadata and text block state

After public start and within the provider-part limit, the state machine SHALL accept at most one response-metadata part before payload output, sequential text blocks, independent tool events under gateway-streaming-function-tools, supported reasoning/sources under their capabilities, non-terminal provider errors and exactly one finish. Response metadata SHALL copy representable native ResponseID, ModelID and Timestamp at registered id, modelId and timestamp fields. Nonempty native strings SHALL remain unchanged; optional empty Go strings and zero timestamps SHALL be omitted as documented unrepresentable-presence adaptations. A nonzero timestamp SHALL encode a validated UTC RFC3339Nano instant. A provider metadata part with no identity SHALL contain only its type. The runtime SHALL NOT synthesize missing metadata or substitute requested/canonical route identity. Provider identity, headers and opaque metadata remain outside this scoped identity change.

Canonical identity SHALL remain authoritative for resolution and operator logical metrics, separately from returned identity. Each text-start ID SHALL remain nonempty, valid UTF-8 and globally unique within its stream, opening the only active text block. Deltas/ends SHALL use its active ID and required empty deltas SHALL survive. Provider errors SHALL not change text/metadata state. Existing reasoning/source/tool first-output placement rules SHALL remain effective. New native identity strings and timestamp bytes SHALL participate in preflight and complete-frame bounds before any write.

#### Scenario: Native metadata precedes output
- **WHEN** a provider emits native response identity different from requested alias and canonical route before payload output
- **THEN** the event SHALL preserve the native values without changing route resolution or canonical operator metrics

#### Scenario: Metadata has partial or no native identity
- **WHEN** the provider emits partial identity or an identity-free metadata part
- **THEN** only supplied representable fields SHALL appear, with no canonical fallback
- **AND** no metadata part SHALL be manufactured for a provider that emitted none

#### Scenario: Native model ID is not a public route
- **WHEN** modelId contains valid native Unicode, spaces or punctuation outside public route syntax
- **THEN** the native modelId SHALL remain unchanged within the complete-frame budget

#### Scenario: Sequential text blocks are valid
- **WHEN** several non-overlapping text blocks use unique IDs
- **THEN** all parts SHALL remain ordered and empty deltas SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, malformed, reused, mismatched, concurrently opened or ended without an active block
- **THEN** the handler SHALL cancel work and attempt at most one synthetic terminal error

#### Scenario: Metadata placement is invalid
- **WHEN** metadata is duplicated or arrives after text, tool, reasoning or source output starts
- **THEN** it SHALL not be forwarded and the existing safe terminal behavior SHALL apply

#### Scenario: Metadata representation exceeds bounds
- **WHEN** native identity contains invalid UTF-8, an invalid timestamp or yields an over-limit complete frame
- **THEN** no metadata bytes SHALL be written and at most one safe terminal error SHALL be attempted

## ADDED Requirements

### Requirement: Native-value regressions preserve terminal authority

Native warning/source/identity changes SHALL preserve existing stream-start normalization, metadata placement, non-terminal provider errors, idle reset, authoritative finish, cancellation, writer failure and bounded cleanup rules. Raw/schema and exact-pinned TS/independent Go consumption tests SHALL establish these properties without relying on permissive client parsing.

#### Scenario: Sources interleave and finish remains final
- **WHEN** repeated and equal-cross-variant native source IDs interleave with active content and provider errors before finish, followed by later source/metadata parts
- **THEN** accepted values SHALL retain order without closing active blocks, and finish SHALL suppress every later public part

#### Scenario: Cancellation or writer failure occurs
- **WHEN** processing native-value events is canceled or a full-frame write/flush fails
- **THEN** existing cancellation and single-owner bounded cleanup SHALL apply with no second write after writer failure or authoritative terminal output
