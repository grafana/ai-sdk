# anthropic-input-transformations Specification

## Purpose
Define how the Anthropic provider reports input blocks that Anthropic transformed before inference, such as a dropped thinking block, in provider metadata for unary and streaming calls.

## Requirements

### Requirement: Input transformations in unary provider metadata

A unary response that carries `input_transformations` SHALL expose them as `inputTransformations` in the Anthropic provider metadata. Each entry SHALL keep only string `type`, `path` and `reason`.

#### Scenario: Reported transformation
- **WHEN** a response contains an entry with `type`, `path`, `reason` and an extra field
- **THEN** the metadata SHALL contain that entry with only `type`, `path` and `reason`

#### Scenario: Empty array
- **WHEN** a response contains `input_transformations: []`
- **THEN** the metadata SHALL contain `inputTransformations: []`

#### Scenario: Null or absent
- **WHEN** a response omits `input_transformations` or sets it to null
- **THEN** the metadata SHALL NOT contain `inputTransformations`

#### Scenario: Malformed entry
- **WHEN** an entry lacks a string `type`, `path` or `reason`
- **THEN** the call SHALL fail and SHALL NOT return provider metadata

### Requirement: Input transformations in streaming provider metadata

A stream SHALL report input transformations from `message_start.message.input_transformations` and from the top-level `input_transformations` of `message_delta`. A non-null value SHALL replace the previous one, a null or absent value SHALL keep it, and the value SHALL persist across messages like other stream-level metadata.

#### Scenario: Reported on message start
- **WHEN** `message_start` carries input transformations and no delta mentions them
- **THEN** the finish metadata SHALL contain them

#### Scenario: Replaced by a delta
- **WHEN** a `message_delta` carries a non-null `input_transformations`
- **THEN** the finish metadata SHALL contain the delta's value

#### Scenario: Null delta keeps the value
- **WHEN** a later `message_delta` sets `input_transformations` to null
- **THEN** the finish metadata SHALL keep the earlier value

#### Scenario: Carried into the next message
- **WHEN** a second message reports none
- **THEN** its finish metadata SHALL repeat the previous value

#### Scenario: Malformed value
- **WHEN** `message_start` or `message_delta` carries a malformed `input_transformations`
- **THEN** the stream SHALL emit an error part at that event, SHALL stop consuming, and SHALL NOT report the value
- **AND** this is an accepted deviation: upstream drops the invalid chunk with an error part and keeps reading
