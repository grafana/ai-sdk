# Spec Delta

## Purpose

Define the Anthropic provider's opt-in compaction request and conversion of signed and legacy compaction output.

## ADDED Requirements

### Requirement: On-demand compaction is explicit and incompatible with context editing

The Anthropic provider SHALL expose an optional call-level compaction configuration with type `summarize` and optional string instructions. Generate and stream SHALL send `compaction` only when configured, include explicit empty instructions without treating them as absent, and include `compact-2026-09-04` once when required for the request. Invalid compaction options SHALL fail before native I/O.

#### Scenario: Caller requests a summary
- **WHEN** a generate or stream call configures on-demand compaction
- **THEN** the native body SHALL contain `compaction: {type: "summarize"}` with instructions only when provided
- **AND** the native header SHALL include `compact-2026-09-04` once

#### Scenario: Incompatible call options
- **WHEN** a caller configures on-demand compaction together with a non-null raw Anthropic `contextManagement` option, even if its edits are empty
- **THEN** the call SHALL fail before sending a native request
- **AND** the provider SHALL NOT start consuming unsupported raw context editing on its own or reinterpret low-level transport overrides

### Requirement: Compaction output keeps its summary and signature

The provider SHALL project nonempty unary compaction as text, and stream compaction as text start and end with the supplied summary emitted as text deltas. Unary text and the stream start SHALL carry Anthropic metadata `type: "compaction"` and `signature` when the native signature is a string.

#### Scenario: Stream returns a complete signed block
- **WHEN** a compaction start carries both nonempty content and a string signature
- **THEN** the provider SHALL emit text-start with the compaction marker and signature and a single text-delta with the unmodified summary
- **AND** a redundant compaction delta for that block SHALL NOT duplicate or change the summary

#### Scenario: Legacy stream returns split content
- **WHEN** an unsigned compaction start is followed by compaction deltas
- **THEN** the provider SHALL emit the marker without a signature and retain nonempty delta text in order

#### Scenario: Native unary response has no compaction summary
- **WHEN** a unary compaction block has missing, null or empty content
- **THEN** it SHALL contribute no text or content metadata

#### Scenario: Native compaction fields have the wrong type
- **WHEN** a unary compaction block or streamed compaction start has non-null, non-string content or signature, or an otherwise applicable compaction delta has non-null, non-string content
- **THEN** the provider SHALL report a conversion error without turning the invalid values into text or replay metadata
- **AND** the Go provider SHALL stop a stream at the offending event under its existing conversion-error policy, whereas the upstream adapter continues after reporting an invalid chunk

#### Scenario: Case variants are not native compaction fields
- **WHEN** Anthropic content includes an additional `Content` or `Signature` member
- **THEN** neither member SHALL supply or override the corresponding lowercase wire field
