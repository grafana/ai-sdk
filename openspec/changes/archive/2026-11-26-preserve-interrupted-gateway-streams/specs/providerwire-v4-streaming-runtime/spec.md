## MODIFIED Requirements

### Requirement: Finish validation and terminal authority

A finish part SHALL be valid only when no tool preview series lacks its final result, the current segment has not already finished, its warnings are empty, and it contains a non-nil registered finish reason and non-nil usage. No text, reasoning or tool-input block SHALL be active unless the unified reason is `interrupted`. A written finish SHALL never be retracted or followed by a synthetic finish.

#### Scenario: Finish closes a valid stream
- **WHEN** a valid finish is emitted with no active text, reasoning or tool-input block
- **THEN** the finish SHALL preserve normalized usage, finish reason and supplied ordinary providerMetadata, unrelated transport fields SHALL remain omitted, the handler SHALL keep reading the provider stream under the sequential response segment rules, and the response SHALL end at clean EOF without `[DONE]` when the provider then closes

#### Scenario: Interrupted finish preserves partial blocks
- **WHEN** an otherwise valid `interrupted` finish arrives with open text, reasoning or tool input
- **THEN** the handler SHALL forward that finish after the partial blocks as emitted, without synthesizing block ends, tool calls or results, and SHALL end at clean EOF without `[DONE]` when the provider then closes

#### Scenario: Finish is invalid
- **WHEN** a non-interrupted finish arrives with an active block, or any finish arrives with an unfinished tool preview series, after a finish without intervening response-metadata, with warnings, nil or invalid usage, nil or invalid finish reason, unsafe token counts, invalid supplied raw usage, or invalid finish metadata
- **THEN** finish SHALL not be written and the handler SHALL attempt at most one synthetic terminal internal error

#### Scenario: Provider emits after finish
- **WHEN** any provider part is available after a valid finish has been written
- **THEN** response-metadata, raw and provider error parts SHALL be processed under their rules, any other part SHALL fail safely under the parts-after-a-finish rule, and the written finish SHALL remain in place

#### Scenario: Native raw object survives finish
- **WHEN** a valid finish contains normalized usage and a supplied raw JSON object of any size, including nested provider-native keys or `{}`
- **THEN** its complete `usage.raw` object SHALL be emitted with the normalized counts, and a distinct `type: "raw"` stream part SHALL NOT be synthesized

#### Scenario: No raw value is supplied
- **WHEN** a valid finish has absent provider `Usage.Raw`
- **THEN** the finish SHALL omit `usage.raw` and retain its normalized counts

#### Scenario: Raw usage fails after stream commitment
- **WHEN** finish usage supplies malformed, null, scalar, array or invalid-UTF-8 raw JSON
- **THEN** the finish SHALL NOT be written and the handler SHALL attempt at most one fixed complete terminal internal-error SSE frame, without reflecting the offending value or issuing normalized-only finish

#### Scenario: Finish metadata is ordinary output
- **WHEN** valid finish metadata contains unknown namespaces or an explicit empty object and IncludeRawChunks is false
- **THEN** both clients SHALL receive that metadata on finish before clean EOF without any synthesized raw event

### Requirement: Sequential response segments
The public start SHALL open the first response segment, and each accepted response-metadata part SHALL open a new one. Response-metadata SHALL be accepted only when no text, reasoning or tool-input block is open and no tool preview is pending. Response-metadata and finish parts SHALL be written in provider order.

#### Scenario: Metadata after an interrupted segment
- **WHEN** response-metadata follows a valid `interrupted` finish that ends a segment with open blocks
- **THEN** the open blocks SHALL be treated as abandoned, and the metadata SHALL open a new segment

#### Scenario: Message-shaped segments
- **WHEN** a provider emits metadata, content and a valid finish, then a raw part, then metadata, content and a valid finish again, and then closes its stream
- **THEN** every part SHALL be written in order, including both metadata parts and both finish parts, and the response SHALL end at clean EOF

#### Scenario: Response-shaped segments
- **WHEN** a provider emits metadata, reasoning and a complete tool call, then metadata, a complete tool call, then metadata, text and one valid finish, and then closes its stream
- **THEN** every part SHALL be written in order without a terminal error, and the response SHALL end at clean EOF

#### Scenario: Metadata while a block is open
- **WHEN** response-metadata arrives while a text, reasoning or tool-input block is open, or while a tool preview series lacks its final result
- **THEN** the metadata SHALL NOT be written, provider work SHALL be canceled and at most one synthetic terminal internal error SHALL be attempted

#### Scenario: Overlapping message start
- **WHEN** a second message's response-metadata arrives before the previous message's text block has ended
- **THEN** the handler SHALL fail safely as for metadata while a block is open, without closing the open block or writing a finish

#### Scenario: Consecutive metadata parts
- **WHEN** two valid response-metadata parts arrive with nothing open between them
- **THEN** both SHALL be written, each opening a segment
