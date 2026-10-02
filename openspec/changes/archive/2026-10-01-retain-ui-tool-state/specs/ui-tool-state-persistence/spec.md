## ADDED Requirements

### Requirement: Supported tool fields survive UI JSON persistence

The root package SHALL preserve the seven represented static/dynamic tool states through UIMessage JSON with title, toolMetadata, Input, RawInput, Output, errorText, preliminary, approvals and call/result provider metadata. `Title` SHALL be `*string`, `Preliminary` SHALL be `*bool`, `ToolMetadata` SHALL be `map[string]json.RawMessage`, and `RawInput` SHALL be `json.RawMessage` on both tool structs. `ErrorText` SHALL migrate to `*string`. Nil optional fields SHALL be absent; explicitly empty strings, false preliminary, empty metadata objects and raw JSON null SHALL remain present. Required state fields SHALL NOT be inferred from nonzero string values. Existing persisted ProviderExecuted, IsAutomatic and Signature scalar serialization normalization SHALL remain unchanged and SHALL NOT be described as lossless optional presence. It SHALL NOT permit sticky-true reader semantics: decoded providerExecuted false SHALL clear a prior true value as specified by reader transitions.

#### Scenario: Every supported static and dynamic state round-trips
- **WHEN** valid static and dynamic parts in input-streaming, input-available, approval-requested, approval-responded, output-available, output-error and output-denied states are encoded and decoded as UI messages
- **THEN** all represented supported tool/approval fields SHALL retain their values and presence
- **AND** the static `tool-{name}` and dynamic `dynamic-tool` discriminators SHALL retain their identity

#### Scenario: Optional empty and false fields are not omitted
- **WHEN** a tool part carries title `""`, toolMetadata `{}`, preliminary false or required errorText `""` in its applicable state
- **THEN** JSON SHALL retain those present fields
- **AND** otherwise equivalent nil fields SHALL be omitted

#### Scenario: Raw input retains arbitrary JSON
- **WHEN** an output-error part has RawInput containing a JSON string, object or null and absent Input
- **THEN** JSON round-trip SHALL retain RawInput separately from Input
- **AND** decoding SHALL NOT silently normalize legacy RawInput into Input

#### Scenario: Present empty provider metadata remains distinguishable
- **WHEN** a persisted tool contains an explicitly empty callProviderMetadata or resultProviderMetadata object
- **THEN** UI JSON SHALL preserve that object's presence instead of omitting it
- **AND** model conversion SHALL select metadata using presence before provider-domain serialization normalization

### Requirement: Invalid optional field types do not disappear during decoding

Typed tool/approval JSON decoding SHALL reject explicit null and wrong JSON types for supported optional non-null strings, booleans and object metadata before Go nil/zero-value decoding can treat them as absent. Input, Output, RawInput and approval Descriptor SHALL retain their target-supported arbitrary JSON values, including null. Invalid persisted optional fields SHALL NOT reach Agent/provider invocation through normalization to absence.

#### Scenario: Optional null string or boolean is not silently accepted
- **WHEN** persisted tool JSON includes title null, preliminary null or an approval reason of the wrong JSON type
- **THEN** decoding/validation SHALL return a contextual error before provider invocation
- **AND** the invalid field SHALL NOT be normalized into an absent optional value

#### Scenario: Opaque null remains data
- **WHEN** valid tool JSON includes null Input, Output, RawInput or approval Descriptor in a target-supported state
- **THEN** decoding SHALL retain that null value as data
- **AND** tool schema/state checks SHALL determine subsequent validity at their specified gates

### Requirement: Supported approval data survives persistence

`ToolApproval` SHALL retain ID, Approved, Descriptor, RequestReason, Reason, IsAutomatic and Signature. Descriptor SHALL be opaque `json.RawMessage`; RequestReason SHALL be `*string`; Reason SHALL migrate to `*string`. Request reason and decision reason SHALL remain distinct. Approval-response merging SHALL preserve earlier descriptor, request reason, signature and automatic status. Raw JSON and pointer fields SHALL be independently cloned in snapshots.

#### Scenario: Approval request is retained after a response
- **WHEN** a tool's approval request contains descriptor, request reason, signature and automatic status and receives a response
- **THEN** the resulting approval SHALL retain those request fields and include the response's approved value and supplied decision reason
- **AND** persistence SHALL retain false approved and explicitly empty reason

#### Scenario: Snapshot mutation cannot alias new fields
- **WHEN** a caller mutates a prior snapshot's title/preliminary/error pointers, raw input, tool metadata, call/result metadata or nested approval data
- **THEN** subsequent snapshots and reader internal state SHALL remain unchanged

### Requirement: Tool chunk presence transfers to persisted parts

The existing chunk JSON codec SHALL preserve decoded optional tool title, approval reason, preliminary and providerExecuted presence through decode/re-encode and reader transfer, without changing their public scalar types. Decoded providerExecuted false SHALL remain distinguishable from an absent field for reader updates. Direct Go scalar zero-value construction SHALL keep its existing omission behavior for optional fields. The chunk SHALL expose `ApprovalDescriptor json.RawMessage` on the registered `approvalDescriptor` wire field; an approval-request's registered `reason` SHALL map to persisted RequestReason. No requestReason chunk field SHALL be introduced. Required empty errorText SHALL remain valid by discriminator. SSE framing and unrelated chunk fields SHALL remain unchanged.

#### Scenario: Decoded explicit presence survives assembly
- **WHEN** valid tool chunks decoded from JSON include empty title/reason or preliminary false
- **THEN** re-encoding SHALL retain each explicitly present supported field
- **AND** assembly SHALL transfer it to the appropriate persisted pointer field

#### Scenario: Decoded provider execution false survives re-encoding
- **WHEN** a valid tool chunk decoded from JSON explicitly contains providerExecuted false
- **THEN** re-encoding SHALL retain providerExecuted false
- **AND** reader transfer SHALL distinguish that supplied false from omission without migrating the public bool

#### Scenario: Approval request wire fields use registered names
- **WHEN** an approval-request chunk contains approvalDescriptor and reason
- **THEN** schema-parsed SSE SHALL retain those registered fields
- **AND** the reader SHALL assemble Approval.Descriptor and Approval.RequestReason rather than using decision Reason

#### Scenario: Existing direct-construction normalization is bounded
- **WHEN** a directly constructed Go chunk has optional scalar title/reason empty or preliminary/providerExecuted false without decoded presence
- **THEN** the codec SHALL retain existing zero-value omission behavior
- **AND** documentation and differential expectations SHALL identify this construction boundary rather than claim full optional-presence producer parity
