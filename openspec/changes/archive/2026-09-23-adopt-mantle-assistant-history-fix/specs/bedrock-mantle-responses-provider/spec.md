## MODIFIED Requirements

### Requirement: Responses delegation and continuation metadata
The provider SHALL preserve the OpenAI Responses request, response, streaming,
tool, and continuation semantics supplied by the shared OpenAI model adapter.
Model and response attribution SHALL use `bedrock-mantle.responses`, while
provider options and response metadata SHALL remain under the resolved `openai`
or `azure` namespace. Per-call headers SHALL reach the final authenticated
request.

For unary and streaming calls through `mantle.NewResponses`, reconstructed
assistant text SHALL use an easy-input message with `role: "assistant"` and
string `content`, including explicitly empty text. Without an active
conversation, reconstruction SHALL apply when `store` is false or the assistant
text has no stored item ID. It SHALL preserve supplied phase metadata and omit
stale item IDs, without emitting an incomplete output message containing an
`output_text` content array. When storage is enabled and a stored assistant
item ID is present without an active conversation, the provider SHALL retain
item-reference continuation instead of resending the text. With an active
conversation, assistant text with an existing item ID SHALL be skipped to avoid
duplicating an item already in the conversation context. These behaviors SHALL
work with the Bedrock module's published dependencies without workspace
substitutions or production replacement directives.

#### Scenario: Stored response continuation
- **WHEN** a Mantle response emits an OpenAI-namespaced assistant item ID that is included in a later prompt with storage enabled and no active conversation
- **THEN** the next request emits an item reference instead of resending stored assistant content

#### Scenario: Streaming attribution and metadata
- **WHEN** a Mantle Responses stream emits response and content metadata
- **THEN** response attribution reports `bedrock-mantle.responses`
- **AND** continuation metadata remains under the OpenAI options namespace

#### Scenario: Reconstructed continuation with storage disabled
- **WHEN** a unary or streaming Mantle call sends user → assistant → user history with `store: false` and no active conversation
- **THEN** the assistant input has string content containing the supplied text
- **AND** surrounding user messages retain their content and order
- **AND** the assistant input contains neither a stale item ID nor an incomplete output-message content array

#### Scenario: Reconstructed continuation without an item ID
- **WHEN** a unary or streaming Mantle call includes assistant text without an item ID while storage is enabled
- **THEN** the assistant input has string content rather than an item reference or output-message content array

#### Scenario: Phase-bearing reconstructed continuation
- **WHEN** a unary or streaming Mantle call includes assistant text with an OpenAI-namespaced item ID and `commentary` or `final_answer` phase while `store: false` and no conversation is active
- **THEN** the assistant input preserves the text as a string and the supplied phase
- **AND** the stale item ID is omitted

#### Scenario: Conversation reuses assistant items
- **WHEN** a Mantle call supplies a conversation ID and assistant text with an existing OpenAI-namespaced item ID
- **THEN** the request omits that assistant item rather than resending text or emitting an item reference

#### Scenario: Empty reconstructed assistant text
- **WHEN** a unary or streaming Mantle call includes an empty assistant text part with `store: false`
- **THEN** the assistant input contains `content: ""` rather than omitting content or converting it to an output-message array

#### Scenario: Standalone consumer adoption
- **WHEN** the Bedrock module runs unary and streaming continuation calls without an active conversation with `GOWORK=off` using its publicly resolvable declared dependencies
- **THEN** reconstructed assistant text uses the same string-content encoding as workspace calls
- **AND** storage-enabled assistant items with IDs still use item references
