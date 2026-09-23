## MODIFIED Requirements

### Requirement: StreamPart Preliminary field
The `StreamPart` struct SHALL have a `Preliminary bool` field. True SHALL indicate an intermediate tool result that will be replaced by a subsequent result, such as a preview. A final non-preliminary result SHALL follow a preliminary series. Absent and explicit false SHALL normalize to false and indicate a final result. This field applies to `PartToolResult`. `GenerateContentPart.Preliminary` SHALL use the same boolean semantics for `ContentToolResult`.

#### Scenario: Preliminary tool result
- **WHEN** a `StreamPart` of type `PartToolResult` has `Preliminary` set to true
- **THEN** it SHALL indicate an intermediate result that will be replaced

#### Scenario: Final tool result
- **WHEN** a tool-result marker is omitted or explicitly false
- **THEN** decoding SHALL produce `Preliminary: false` and indicate a final result

## ADDED Requirements

### Requirement: Provider-domain dynamic marker normalization
`provider.GenerateContentPart.Dynamic` SHALL be a bool field. `provider.StreamPart.Dynamic` SHALL remain `*bool` because the flat stream union includes presence-sensitive tool-input-start events. On input-start, absence, false and true SHALL remain distinct through provider output, Gateway mapping/encoding and Go client decoding. In the text stream, explicit values SHALL take precedence; only absence SHALL infer dynamic classification from the application tool definition. In the UI projection, a known dynamic application tool SHALL produce dynamic true, a known non-dynamic tool SHALL omit dynamic, and an unknown tool SHALL use the text-stream value. These are separate pinned upstream behaviors. Call/result arms SHALL normalize absent/false only where equivalent. Function-tool Strict presence and unrelated core/UI APIs SHALL remain unchanged.

#### Scenario: Equivalent unary disabled markers
- **WHEN** unary provider output is decoded with absent and false dynamic markers
- **THEN** both SHALL yield `Dynamic: false` while enabled true remains distinct

#### Scenario: Absent input-start marker permits inference
- **WHEN** an input-start omits dynamic and its application tool is dynamic
- **THEN** the transport SHALL retain absence and actual Go and pinned upstream text-stream and UI output SHALL infer dynamic true

#### Scenario: Explicit input-start marker overrides inference
- **WHEN** an input-start has dynamic false or true and its application tool is dynamic
- **THEN** the transport and text stream SHALL preserve the explicit value, while the UI chunk SHALL use true for the known dynamic tool in both cases

#### Scenario: Known ordinary tool omits UI dynamic
- **WHEN** a known function or provider tool's input-start infers false from an absent marker
- **THEN** its text-stream part SHALL carry false and its UI chunk SHALL omit dynamic

#### Scenario: Unknown tool retains UI marker
- **WHEN** an unknown tool's input-start carries explicit false or true
- **THEN** its UI chunk SHALL preserve the explicit value
