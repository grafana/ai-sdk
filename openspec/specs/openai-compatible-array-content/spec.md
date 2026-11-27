# openai-compatible-array-content Specification

## Purpose
Define how the outbound OpenAI-compatible Chat adapter validates and
normalizes array-valued assistant content in unary and streaming responses.

## Requirements

### Requirement: Ordered assistant content arrays

The outbound OpenAI-compatible adapter SHALL accept arrays in unary assistant `message.content` and streaming `delta.content`. Each array member SHALL be an object with a string `type`. In outer array order, the adapter SHALL emit nonempty string `text` from `text` blocks and concatenate the string text of nested `text` fragments within each `thinking` block into one reasoning part or delta. Empty outputs, unsupported block types and invalid optional payloads SHALL be skipped.

#### Scenario: Array with separate reasoning, refusal and tool calls
- **WHEN** a unary message includes top-level reasoning, an array with text and thinking, a refusal and tool calls
- **THEN** selected nonempty top-level reasoning precedes the normalized array parts, followed by refusal text and tool calls
- **AND** a present non-null `reasoning_content` takes precedence over `reasoning` for array-valued content, even when empty

#### Scenario: Array changes content kind during streaming
- **WHEN** a stream emits text, reasoning, then more text
- **THEN** the adapter emits the deltas in that order and closes each active part before opening the other kind
- **AND** these boundaries apply whether the reasoning comes from a thinking block or a scalar delta

#### Scenario: Malformed outer content
- **WHEN** an array contains a non-object member or a member without a string discriminator
- **THEN** unary decoding fails or the streaming decoder reports an error for that chunk and can process later chunks
