## ADDED Requirements

### Requirement: Filter inactive tool history before Converse message grouping

The provider SHALL prepare the effective tools before converting the prompt.
When no tools remain active, it SHALL remove tool-call and tool-result parts from
non-system source messages, discard messages made empty by that filtering, and
then group the remaining messages. It SHALL emit the existing `toolContent`
warning once when tool content is removed and SHALL NOT mutate the caller's
prompt. A synthetic JSON response tool counts as active; native JSON output
without active tools does not.

#### Scenario: Final response after a disabled tool loop

- **WHEN** tool calls and a cached tool-result message occur between user messages and no prepared tools remain active
- **THEN** the request SHALL combine the surviving adjacent user messages
- **AND** the cache point owned by the removed tool-result message SHALL be omitted
- **AND** cache points on surviving source content SHALL preserve their placement and TTL

#### Scenario: Content is ignored during conversion

- **WHEN** an assistant source contains only unsigned reasoning or approval bookkeeping, or a user/tool source contains only approval bookkeeping
- **THEN** that source SHALL contribute neither native content nor a cache point
- **AND** an unsupported system-only source SHALL likewise contribute no cache point while retaining its unsupported-content warning

#### Scenario: Preserve conversion boundaries

- **WHEN** inactive tool filtering removes the last part or message of the prompt
- **THEN** assistant prefill trimming SHALL use the resulting source positions
- **AND** other ignored content SHALL retain the existing source-position trimming rules
- **AND** provider options on a wholly removed tool message SHALL NOT be parsed, while retained messages SHALL retain existing option validation
