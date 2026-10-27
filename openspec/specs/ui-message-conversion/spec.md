# UI Message Conversion Specification

## Purpose

Define conversion of persisted UI messages into provider V4 model messages, including lossless provider-reference file handling.

## Requirements

### Requirement: Later user messages supersede unresolved approvals

ConvertToModelMessages SHALL omit approval-requested tool parts before the last user message without mutating UI history. It SHALL preserve approval requests after that boundary and preserve responded approvals regardless of the boundary.

#### Scenario: Abandoned approval before a new user turn

- **WHEN** an assistant has an unresolved approval request followed by a later user message
- **THEN** model history omits that tool call and approval request while retaining unrelated assistant content and the user turn

#### Scenario: Pending approval in the current turn

- **WHEN** an unresolved approval request appears after the last user message
- **THEN** model history retains that call and approval request


### Requirement: UI file parts represent provider references

The root package SHALL define `FilePart.ProviderReference` as an optional `map[string]string` serialized as `providerReference`, where keys are provider names and values are provider-specific file identifiers. JSON encoding and decoding SHALL preserve both populated references and an explicitly present empty reference object.

#### Scenario: Provider reference JSON round-trip

- **WHEN** a UI file part carries `ProviderReference: map[string]string{"openai": "file-abc123"}`
- **THEN** it SHALL serialize with `"providerReference":{"openai":"file-abc123"}`
- **AND** decoding SHALL restore the same provider reference map

#### Scenario: Empty provider reference remains present

- **WHEN** a UI file part carries a non-nil empty `ProviderReference`
- **THEN** it SHALL serialize with `"providerReference":{}`
- **AND** decoding SHALL restore a non-nil empty map rather than treating the field as absent

### Requirement: UI file conversion preserves provider references

`ConvertToModelMessages` SHALL convert user and assistant UI file parts with a present `ProviderReference` into provider file content whose `DataContent.Reference` carries the canonical provider reference object. A present provider reference SHALL take precedence over the UI file part URL, including when the reference object is empty. Media type, filename, and provider metadata SHALL remain associated with the converted file part.

#### Scenario: User file reference takes precedence over URL

- **WHEN** a user UI file part carries both a URL and an OpenAI provider reference
- **THEN** the converted provider file data SHALL contain the provider reference
- **AND** it SHALL NOT contain URL or inline base64 data

#### Scenario: Assistant file reference is preserved

- **WHEN** an assistant UI file part carries a provider reference
- **THEN** the converted assistant message SHALL contain a provider file part with the same reference object

#### Scenario: Empty reference does not fall back to URL

- **WHEN** a UI file part carries a non-nil empty provider reference and a URL
- **THEN** conversion SHALL preserve the empty reference object
- **AND** it SHALL NOT substitute the URL as provider file data

#### Scenario: Assistant file without reference preserves URL data

- **WHEN** an assistant UI file part has no provider reference
- **THEN** conversion SHALL preserve its URL as provider URL data
- **AND** a data URL SHALL remain URL data rather than being normalized to base64 data

### Requirement: Ordinary UI file inputs preserve filename presence

Ordinary UI FilePart.Filename SHALL be *string: nil absent, pointer to empty explicitly present empty. UI JSON round-trip, ConvertToModelMessages, provider-domain files and Go Gateway projection SHALL retain absent/empty/nonempty filenames for user/assistant files. Provider-reference precedence, media type and provider metadata association SHALL remain unchanged.

#### Scenario: Filename presence survives the UI input path
- **WHEN** otherwise equivalent user or assistant UI file JSON contains an absent, empty, or non-empty filename
- **THEN** Go decoding and re-encoding SHALL preserve all three states
- **AND** conversion to provider messages and Gateway request projection SHALL preserve the same distinction

#### Scenario: Reference input retains filename presence
- **WHEN** an ordinary UI file has a present provider reference and an explicit empty filename
- **THEN** conversion SHALL retain the reference arm and present empty filename without falling back to URL data or omitting the filename

#### Scenario: Cross-language input conversion proves the distinction
- **WHEN** the deterministic integration scenario validates and converts each filename state with the pinned TypeScript UI API and the Go input path
- **THEN** filename presence SHALL agree in both roles through the resulting model/client request, not merely in an intermediate Go struct
- **AND** any SSE consumed by the scenario SHALL use the registered chunk schema without adding an unregistered filename field to file chunks

### Requirement: Descriptive and generated filename boundaries remain unchanged

UI SourceDocumentPart and generated/source-only descriptive filenames SHALL retain existing normalization. Filename presence work SHALL NOT add filenames to generated file SSE chunks or enable generated-media Gateway output.

#### Scenario: Source filenames retain normalization
- **WHEN** source-document or generated/source-only descriptive filenames are serialized
- **THEN** existing normalization SHALL remain; generated file SSE chunks SHALL NOT gain filenames and Gateway generated-media output SHALL NOT be enabled.

### Requirement: Tool conversion respects incomplete and preliminary states

ConvertToModelMessages SHALL omit input-streaming parts by default and with WithIgnoreIncompleteToolCalls, emitting no calls/approvals/results. That option SHALL retain only approval-responded, output-error, output-denied and output-available with preliminary not true. Filtered parts SHALL NOT invoke ToModelOutput. Without the option, preliminary outputs SHALL convert like available outputs. Static/dynamic filtering and existing step-block order SHALL remain identical.

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

Output-error calls SHALL use non-null Input, then RawInput, then absent input; JSON null SHALL follow target nullish fallback. Call metadata SHALL use callProviderMetadata, with resultProviderMetadata fallback only on output-error. Inline provider results SHALL use resultProviderMetadata, falling back to call metadata only if result metadata is absent; present empty SHALL suppress fallback. Local tool-role results SHALL use call metadata.

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

### Requirement: Provider codec empty normalization is an enumerated boundary

Existing provider-domain omission of empty provider-options maps and optional empty approval reason strings SHALL be an explicitly enumerated representation boundary, not general empty-value normalization or full model-JSON parity.

#### Scenario: Empty selected metadata does not justify general normalization
- **WHEN** differential evidence encounters an explicitly empty selected metadata object or optional empty approval reason
- **THEN** it SHALL account only for the existing enumerated codec boundary, not claim general empty normalization or full model-JSON parity.

### Requirement: Denial and output projection retain distinct target semantics

Local output-denied SHALL emit error-text using present approval reason (even empty), otherwise exactly `Tool call execution denied.` Negative approval-responded SHALL retain separate synthetic execution-denied path, including provider-executed calls. Successful outputs SHALL retain ToModelOutput and its errors; output-error SHALL emit local error-text/provider error-json without it. Filtered/denied parts SHALL NOT invoke it.

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

### Requirement: Tool projection retains custom content and approval placement

Custom content, provider-reference files, filename presence, approval placement and normal string/JSON output conversion SHALL remain unchanged. Approval RequestReason SHALL project to provider approval-request reason through existing provider representation.

#### Scenario: Request reason projects separately from decision reason
- **WHEN** history contains a tool approval request with RequestReason and a separate completed tool returning custom content or a provider-reference file
- **THEN** RequestReason SHALL become provider request reason while existing content, filenames and approval placement remain unchanged.

### Requirement: Provider prompts coalesce consecutive tool messages

Prompt preparation SHALL coalesce consecutive tool-role messages after approval bookkeeping removal, keeping content order. Before next append, preceding message options SHALL deep-merge into its last part, with part options winning. Combined message SHALL retain final message options. Empty tool messages SHALL participate before removal. Caller history SHALL NOT mutate; direct ConvertToModelMessages grouping SHALL stay unchanged.

#### Scenario: Resumed approval results share a tool message
- **WHEN** approval resumption appends results immediately after an existing tool message
- **THEN** the provider SHALL receive one combined tool message with the existing results followed by resumed results
- **AND** metadata precedence SHALL match the registered upstream prompt preparation

### Requirement: Data parts support opt-in text or file conversion

The root SHALL expose WithConvertDataPart(fn func(DataPart) (*provider.ContentPart, error)) ConvertOption. Non-nil callbacks SHALL run on user/assistant data in part order with assistant step boundaries. Nil callbacks/results SHALL skip data. Only text/file discriminators SHALL be accepted and preserved directly; other kinds/errors SHALL yield contextual conversion errors. System data SHALL NOT invoke the hook; Agent helpers SHALL NOT gain implicit converters.

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

### Requirement: Data and tool output processing retain step-local order

Each assistant block SHALL complete assistant/data processing before projecting local tool results. Provider-executed outputs SHALL remain inline.

#### Scenario: Data errors precede local result projection
- **WHEN** a block contains a local available output then data whose converter errors
- **THEN** data conversion SHALL fail before the local output callback, while provider-executed output callbacks remain inline.

### Requirement: Supported continuation metadata presence survives UI conversion

Supported text/reasoning, tool-call/basic-result, reasoning-file and source paths SHALL preserve represented providerMetadata/callProviderMetadata/resultProviderMetadata presence independently through root chunk serialization, assembled parts and UI JSON: nil omits, non-nil empty serializes {}. Namespaces/nested JSON SHALL stay opaque. Latest non-nullish object assembly SHALL replace, not recursively merge.

#### Scenario: Empty text metadata clears frontend state
- **WHEN** schema-parsed text chunks first carry metadata and a later registered chunk supplies an explicit empty metadata object
- **THEN** the pinned frontend and Go UI reader SHALL assemble empty replacement metadata instead of retaining the prior object
- **AND** affected persisted UI JSON and applicable model-message conversion SHALL preserve that explicit empty state

#### Scenario: Call and result metadata remain separate
- **WHEN** supported tool input/call/output UI chunks carry distinct call and result metadata, including an explicit empty replacement
- **THEN** frontend/Go assembly, UI JSON and applicable model-message conversion SHALL retain the distinct scopes without dropping emptiness or merging objects

#### Scenario: Cross-language proof precedes the core fix
- **WHEN** a demonstrated metadata presence/assembly gap changes frontend-visible wire behavior
- **THEN** a deterministic Go scenario and matching Vitest test SHALL consume SSE through parseJsonEventStream and uiMessageChunkSchema, assert chunk fields and assembled messages, and compare applicable converted model history

### Requirement: Continuation metadata projection stays within represented scopes

ConvertToModelMessages SHALL retain represented continuation metadata as scoped providerOptions on applicable content; source metadata SHALL remain response-only. Changes SHALL be limited to demonstrated serialization/assembly/conversion gaps and SHALL NOT add metadata to pinned chunk variants lacking it.

#### Scenario: Source metadata remains response-only
- **WHEN** supported source chunks carry explicit empty or populated metadata
- **THEN** UI assembly/persistence SHALL preserve its presence without converting source metadata into provider prompt content or adding fields to unsupported chunk variants.
