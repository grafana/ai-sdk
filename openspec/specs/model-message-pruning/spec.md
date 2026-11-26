# model-message-pruning Specification

## Purpose
Let callers prune model history explicitly before submitting a continuation, without changing default generation or tool orchestration.

## Requirements

### Requirement: Explicit nonmutating pruning
The root SDK SHALL expose caller-invoked pruning of model messages. It SHALL preserve the order and values of retained parts and messages, and SHALL NOT mutate input. Normal generation and streaming SHALL NOT call the helper automatically.

#### Scenario: No content pruning enabled
- **WHEN** the caller supplies no reasoning or tool rules
- **THEN** all content parts and their options SHALL be retained, subject only to the empty-message policy

#### Scenario: Independent slices
- **WHEN** the caller replaces entries in the returned message or content slices
- **THEN** the input slices SHALL be unchanged; nested payloads and provider options need not be copied

### Requirement: Assistant reasoning pruning
Reasoning pruning SHALL remove assistant reasoning text and reasoning files together, either from all messages or from messages before the final input message. It SHALL preserve regular text and file parts.

#### Scenario: Last message has another role
- **WHEN** reasoning is pruned before the last input message and that message is a user message
- **THEN** assistant reasoning in every earlier message SHALL be removed

### Requirement: Tool rules select history by name and retained suffix
Tool pruning SHALL apply rules in caller order to assistant and tool messages. A rule with a suffix count of zero has no protected suffix; a positive count protects that many trailing input messages, and a negative count disables the rule. Omitting tool names selects all tools; an explicit empty list selects no known names.

#### Scenario: Selected tool and associated approvals
- **WHEN** history outside the suffix includes a selected call, its result, and a matching approval request and response
- **THEN** the selected exchange SHALL be removed while known unselected tool history remains in order
- **AND** unresolved approvals outside the suffix SHALL be removed even when a name list is supplied

#### Scenario: A result or approval is in the suffix
- **WHEN** the suffix contains a tool call or result
- **THEN** associated calls and results earlier in history SHALL be retained
- **AND** an approval request or response in the suffix SHALL also protect earlier parts carrying that approval ID and earlier calls and results for the originating call

### Requirement: Tool pruning preserves related reasoning runs
A tool rule SHALL prune a contiguous group of assistant reasoning text and files if at least one non-reasoning part follows it and the rule removes every part before the next reasoning group or the end of the message.

#### Scenario: A non-tool part survives
- **WHEN** a tool call after a reasoning group is pruned but later text in the same run remains
- **THEN** the entire reasoning group SHALL be retained

### Requirement: Empty-message policy applies last
After content pruning, all messages with zero parts SHALL be discarded by default, including messages that already had no parts. An explicit keep option SHALL preserve them.

#### Scenario: Empty text is a part
- **WHEN** a message contains a text part whose text is empty
- **THEN** the empty-message policy SHALL retain the message
