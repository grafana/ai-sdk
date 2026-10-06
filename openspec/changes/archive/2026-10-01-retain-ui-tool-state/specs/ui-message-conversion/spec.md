## ADDED Requirements

### Requirement: Tool conversion respects incomplete and preliminary states

ConvertToModelMessages SHALL omit input-streaming tool parts by default and with WithIgnoreIncompleteToolCalls, without emitting calls, approvals or results for them. WithIgnoreIncompleteToolCalls SHALL retain only approval-responded, output-error, output-denied and output-available with preliminary not true. A filtered tool part SHALL NOT invoke ToModelOutput. Without that option, preliminary available outputs SHALL convert like other available outputs. Both static and dynamic parts SHALL follow the same filtering rules; existing step-block ordering SHALL remain intact.

#### Scenario: Input streaming is excluded without an option
- **WHEN** static/dynamic input-streaming parts occur among assistant text and completed calls
- **THEN** default conversion SHALL omit the streaming tool parts entirely
- **AND** completed calls and surrounding content SHALL remain in their original blocks

#### Scenario: Preliminary filtering differs from final output
- **WHEN** equivalent available outputs carry preliminary true, false or absent
- **THEN** WithIgnoreIncompleteToolCalls SHALL exclude only the preliminary true parts
- **AND** their output callbacks SHALL NOT run
- **AND** default conversion SHALL include all three available-output cases

### Requirement: Tool conversion preserves pinned input and metadata selection

For output-error calls, conversion SHALL use non-null Input, otherwise RawInput, otherwise absent input. JSON null SHALL follow target nullish fallback. Call metadata SHALL use callProviderMetadata, falling back to resultProviderMetadata only for output-error. Provider-executed inline results SHALL use resultProviderMetadata with callProviderMetadata fallback only when result metadata is absent; a present empty object SHALL suppress fallback. Local tool-role results SHALL use callProviderMetadata. Existing provider-domain codecs' omission of empty provider-options maps and optional empty approval reason strings SHALL be an explicitly enumerated representation boundary, not generalized empty-value normalization or full model-JSON parity.

#### Scenario: Legacy failed input falls back with nullish semantics
- **WHEN** an output-error part has RawInput and absent or null Input
- **THEN** its converted tool call SHALL use RawInput
- **AND** non-null Input SHALL take precedence when supplied

#### Scenario: Error call metadata can fall back to result metadata
- **WHEN** an output-error tool lacks call metadata but has result metadata
- **THEN** the converted call SHALL use result metadata
- **AND** non-error calls SHALL NOT adopt result metadata as their call metadata

#### Scenario: Local and provider results select different metadata
- **WHEN** a tool part has distinct call and result metadata
- **THEN** its local result SHALL use call metadata
- **AND** its provider-executed inline result SHALL use result metadata, using call metadata only when result metadata is absent

#### Scenario: Present empty metadata suppresses populated fallback
- **WHEN** a provider-executed tool has populated call metadata and explicitly empty result metadata
- **THEN** conversion SHALL select the empty result metadata and SHALL NOT expose the populated call metadata on the result
- **AND** differential expectations SHALL account only for the existing provider codec's optional empty-map representation

### Requirement: Denial and output projection retain distinct target semantics

A local output-denied tool SHALL produce an error-text result using its approval reason when present, including an empty string, or exactly `Tool call execution denied.` when absent. Negative approval-responded SHALL retain the separate synthetic execution-denied result path, including provider-executed cases. Successful available outputs SHALL retain configured ToModelOutput and its errors; output-error SHALL produce local error-text or provider error-json without invoking that callback. Filtered or denied parts SHALL NOT invoke it. Custom content, provider-reference files, filename presence, tool approval placement and normal string/JSON output conversion SHALL remain unchanged. Approval RequestReason SHALL project to the provider approval-request reason using the existing provider representation.

#### Scenario: Denied reason distinguishes absent and empty
- **WHEN** output-denied parts have absent, empty and nonempty approval reasons
- **THEN** their error-text values SHALL respectively be `Tool call execution denied.`, `""`, and the supplied nonempty text
- **AND** these required error-text values SHALL NOT be normalized away in model comparisons

#### Scenario: Approval response denial is not output-denied text
- **WHEN** an approval-responded part has Approved false
- **THEN** conversion SHALL emit its approval response and synthetic execution-denied result
- **AND** provider-executed calls SHALL NOT acquire a duplicate regular local result

#### Scenario: Error modes bypass ToModelOutput
- **WHEN** local and provider-executed output-error parts are converted with a configured ToModelOutput callback
- **THEN** the callback SHALL NOT run
- **AND** the respective result kinds SHALL be error-text and error-json

#### Scenario: Successful output callback retains context and errors
- **WHEN** a successful local or provider-executed available output uses ToModelOutput
- **THEN** the callback SHALL receive the tool call ID, original Input and Output
- **AND** returned model output SHALL preserve its placement and callback errors SHALL abort conversion

### Requirement: Provider prompts coalesce consecutive tool messages

Provider prompt preparation SHALL coalesce consecutive tool-role messages after approval bookkeeping removal, preserving content order. Before appending the next message, the preceding message's provider options SHALL be deeply merged into its last content part, with part options taking precedence. The combined message SHALL retain the final message's provider options. Empty tool messages SHALL participate before final removal. Caller history SHALL NOT be mutated, and direct ConvertToModelMessages grouping SHALL remain unchanged.

#### Scenario: Resumed approval results share a tool message
- **WHEN** approval resumption appends results immediately after an existing tool message
- **THEN** the provider SHALL receive one combined tool message with the existing results followed by resumed results
- **AND** metadata precedence SHALL match the registered upstream prompt preparation

### Requirement: Data parts support opt-in text or file conversion

The root package SHALL expose `WithConvertDataPart(fn func(DataPart) (*provider.ContentPart, error)) ConvertOption`. Conversion SHALL invoke a non-nil callback on user and assistant data parts in part order, retaining assistant step boundaries. Each assistant block SHALL complete assistant/data processing before projecting local tool results; provider-executed outputs SHALL remain inline. Nil callbacks/results SHALL skip data parts. Only text and file content discriminators SHALL be accepted; all other returned kinds and callback errors SHALL yield contextual conversion errors. Returned text/file content SHALL be preserved directly. System parts SHALL NOT invoke the hook. Existing Agent helper options SHALL NOT gain an implicit data converter.

#### Scenario: User and assistant data preserve converter order
- **WHEN** user/assistant messages interleave ordinary parts, data parts and assistant step starts
- **THEN** returned text/file parts SHALL appear at the data positions in their respective blocks
- **AND** IDs, payloads and names SHALL be available in each callback DataPart

#### Scenario: Data conversion precedes local output callbacks within a step
- **WHEN** an assistant block contains a local available tool followed by a data part
- **THEN** the data callback SHALL run before the local ToModelOutput callback, and a data failure SHALL prevent that local callback
- **AND** provider-executed output callbacks SHALL remain inline while step boundaries SHALL flush prior local results before later blocks

#### Scenario: Nil result and default conversion skip data
- **WHEN** no converter is supplied or its result is nil
- **THEN** data parts SHALL be omitted without changing other converted content

#### Scenario: Unsupported converter results fail explicitly
- **WHEN** a converter returns a reasoning/tool/custom discriminator or an error
- **THEN** conversion SHALL return a contextual error instead of silently accepting or dropping it

#### Scenario: Required empty converted text survives model JSON
- **WHEN** a data converter returns a text content part containing an empty string
- **THEN** model JSON SHALL retain `"text": ""` using the existing text discriminator
- **AND** `provider.ContentPart.Text` SHALL remain a string and other variant encodings SHALL remain unchanged

#### Scenario: System data is not converted
- **WHEN** a system message contains text and data parts with a converter configured
- **THEN** the converter SHALL NOT run for those system data parts
- **AND** existing system text/provider-option handling SHALL remain unchanged
