# Spec Delta

## MODIFIED Requirements

### Requirement: Synthetic terminal adapter errors

Before valid finish, unsupported families, lifecycle violations, invalid output, premature close, mapping or framing failure, timeout and cancellation SHALL retain existing bounded terminal error behavior. Reasoning start/delta/end and atomic reasoning-file events SHALL be supported under gateway-reasoning-content, URL/document sources SHALL be supported under gateway-sources, and atomic custom events SHALL be supported under gateway-custom-content. Ordinary generated files, raw output, approvals and unsupported tool behavior SHALL remain unsupported.

#### Scenario: Reasoning lifecycle violation
- **WHEN** reasoning ends without a matching active start, reuses an ended reasoning ID, or remains open at finish
- **THEN** the handler SHALL produce at most one safe terminal error and no synthetic finish

#### Scenario: Unsupported stream family appears
- **WHEN** the text runtime receives provider-executed/dynamic/preliminary tool behavior, ordinary generated file, raw, approval, or another unsupported part
- **THEN** it SHALL emit at most one terminal internal error rather than serializing the provider-domain part

#### Scenario: Custom stream part is supported
- **WHEN** the runtime receives a well-formed custom part between stream start and finish
- **THEN** it SHALL emit the custom event and continue the stream rather than emitting a terminal error

#### Scenario: Concurrent reasoning
- **WHEN** two reasoning blocks and a text block sharing one raw ID interleave
- **THEN** family-specific active state SHALL preserve each block independently and IDs SHALL not be rewritten

### Requirement: Native response metadata and text block state

After public start and within the provider-part limit, the state machine SHALL accept at most one response-metadata part before payload output, sequential text blocks, independent tool events under gateway-streaming-function-tools, supported reasoning/sources/custom under their capabilities, non-terminal provider errors and exactly one finish. Response metadata SHALL copy representable native ResponseID, ModelID and Timestamp at registered id, modelId and timestamp fields.

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

Each text-start, text-delta and text-end SHALL preserve supported opaque providerMetadata at its original event position under gateway-provider-metadata, including absent versus explicit empty presence. Content metadata SHALL NOT be moved into response-metadata.

#### Scenario: Sequential text blocks are valid
- **WHEN** a provider emits multiple non-overlapping text start/delta/end blocks with unique IDs
- **THEN** all events SHALL be emitted in order and empty delta strings SHALL remain present

#### Scenario: Text lifecycle is invalid
- **WHEN** a text ID is empty, invalid UTF-8, reused, mismatched, opened while another text block is active, or ended without an active matching block
- **THEN** the handler SHALL cancel provider work and attempt at most one synthetic terminal internal error

#### Scenario: Metadata placement is invalid
- **WHEN** metadata is duplicated or arrives after text, tool, reasoning, source or custom output starts
- **THEN** it SHALL not be forwarded and the existing safe terminal behavior SHALL apply

#### Scenario: Metadata representation exceeds bounds
- **WHEN** native identity contains invalid UTF-8, an invalid timestamp or yields an over-limit complete frame
- **THEN** no metadata bytes SHALL be written and at most one safe terminal error SHALL be attempted

#### Scenario: Text metadata changes at end
- **WHEN** text-start supplies an object, text-delta omits metadata and text-end supplies an empty or replacement object
- **THEN** those exact metadata positions and presence states SHALL reach both clients in order, with no deep merge or relocation
