# Spec Delta

## MODIFIED Requirements

### Requirement: Assistant compaction text round-trips without trimming

Assistant text with `ProviderOptions["anthropic"].type == "compaction"` SHALL serialize as `compaction`, not `text`, preserving bytes even in the final assistant block. Empty compaction parts SHALL be omitted, and assistant messages left without content SHALL also be omitted. Part-level and message-level cache control SHALL follow ordinary assistant-text rules. A string signature SHALL be preserved on retained compaction and activate the September compaction beta.

#### Scenario: Final compaction content preserves whitespace and block type
- **WHEN** the prompt ends with an assistant text part containing `"Compaction summary  \n"` and Anthropic provider option `type == "compaction"`
- **THEN** the resulting content block SHALL be `{type: "compaction", content: "Compaction summary  \n"}`

#### Scenario: Empty compaction does not produce an invalid block or message
- **WHEN** an assistant message contains only an empty compaction part
- **THEN** neither the compaction block nor the empty assistant message is sent to Anthropic
- **AND** an empty compaction part alongside ordinary text is omitted without dropping that text

#### Scenario: Signed continuation preserves authenticated content
- **WHEN** retained assistant compaction history carries a string signature
- **THEN** the native compaction block SHALL carry the exact content and signature, including a present empty string
- **AND** the native request SHALL carry `compact-2026-09-04` once even without a new on-demand compaction option

#### Scenario: Unsigned continuation remains legacy compaction
- **WHEN** a retained compaction part has an absent, null or non-string signature
- **THEN** the native compaction block SHALL omit the signature and SHALL NOT activate `compact-2026-09-04` on its own

#### Scenario: Case variants are not compaction replay metadata
- **WHEN** assistant text options contain case variants such as `Type` or `Signature`
- **THEN** those keys SHALL NOT supply or override the lowercase `type` and `signature` keys for compaction replay
