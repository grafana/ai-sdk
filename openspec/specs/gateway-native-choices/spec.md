# Native multiple choices

## Purpose
Preserve native Chat alternatives in one provider request while retaining shared
output lifecycle validation and the default single-choice V4 contract.

## Requirements

### Requirement: Explicit native alternatives

The provider domain SHALL expose a Go-only ChoiceCount opt-in, indexed unary
GenerateChoice alternatives, indexed stream parts and a global PartChoicesFinish.
Default calls SHALL retain the pinned single-result LanguageModelV4 wire shape.
These extensions SHALL NOT be accepted implicitly by ProviderWire.

#### Scenario: Single-choice default
- **WHEN** a call does not request multiple alternatives
- **THEN** the runtime rejects unsolicited Choices, ChoiceIndex or choices-finish.

### Requirement: Validate each alternative

The runtime SHALL validate every requested alternative with an independent copy
of the prompt's tool history and the existing unary or stream lifecycle rules.
Indices SHALL be unique and cover zero through ChoiceCount minus one. Streaming
SHALL finish globally only after each alternative has finished.

#### Scenario: Interleaved output
- **WHEN** choices interleave text or tools with identical local IDs
- **THEN** their lifecycle state remains isolated by index and malformed ordering
  in any choice fails the call without a successful terminal marker.

#### Scenario: Missing or duplicate alternative
- **WHEN** a result omits an index, duplicates an index or exceeds the request
- **THEN** shared validation rejects the output.

### Requirement: Aggregate usage once

Native usage SHALL belong to the enclosing result or global choices-finish part,
not to each alternative. A global choices-finish SHALL have no invented finish
reason; each alternative SHALL retain its own native finish reason.

#### Scenario: Different finish reasons
- **WHEN** one choice stops and another reaches its token limit
- **THEN** each keeps its reason while native token usage is reported once.

### Requirement: Native compatible execution

The compatible provider SHALL issue one request with native n, preserve every
choice index and decode interleaved deltas into independent native choice state.
It SHALL NOT implement alternatives by issuing several requests.

#### Scenario: Native multiple-choice response
- **WHEN** an opted-in native response contains all requested indexed choices
- **THEN** each choice's text, reasoning, tools, refusal, log probabilities and
  finish reason remain associated with that choice and aggregate usage appears once.

#### Scenario: Invalid native index or incomplete stream
- **WHEN** native output duplicates, omits or exceeds the requested indices
- **THEN** decoding fails without emitting an aggregate successful finish.
