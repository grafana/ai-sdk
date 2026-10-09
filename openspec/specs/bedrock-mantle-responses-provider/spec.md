# bedrock-mantle-responses-provider Specification

## Purpose
Provide an authenticated Amazon Bedrock Mantle Responses model that preserves OpenAI Responses behavior while owning Bedrock-specific routing and attribution.

## Requirements

### Requirement: Mantle Responses construction and identity

The system SHALL expose an error-returning Mantle Responses constructor accepting context, model ID, Mantle client configuration and transport request options. The model SHALL implement V4, preserve the model ID verbatim and report `bedrock-mantle.responses`. Construction SHALL reject invalid/ambiguous configured authentication or routing. Generic request options overriding protected authentication/routing SHALL fail before transport.

#### Scenario: Construct a Luna Responses model
- **WHEN** a caller constructs a Responses model for `openai.gpt-5.6-luna` with valid Mantle configuration
- **THEN** the model reports V4, model ID `openai.gpt-5.6-luna`, and provider `bedrock-mantle.responses`

#### Scenario: Reject ambiguous explicit authentication
- **WHEN** bearer and AWS credential modes are configured together
- **THEN** construction returns an error instead of silently selecting one mode

#### Scenario: Reject nil context
- **WHEN** a caller constructs a model with a nil context and default configuration
- **THEN** construction returns an error without panicking

#### Scenario: Reject a protected request-option override
- **WHEN** a caller supplies a generic request option that overrides the configured Mantle route
- **THEN** the first model call returns a routing error without reaching network transport

### Requirement: OpenAI-compatible Mantle routing

Mantle SHALL use regional `https://bedrock-mantle.<region>.api.aws/v1` for generic models and exact documented model-specific `/openai/v1` exceptions, not unrelated-family inference. Requests SHALL use `/responses` beneath the selected base, preserve valid custom bases and carry the model ID verbatim, never Converse paths/shapes. Explicit region/profile SHALL be trimmed before endpoint resolution.

#### Scenario: Default generic regional route
- **WHEN** `openai.gpt-oss-20b` configured for `us-east-1` sends a Responses request without a custom base URL
- **THEN** it sends `POST https://bedrock-mantle.us-east-1.api.aws/v1/responses`

#### Scenario: Documented regional route exceptions
- **WHEN** any documented GPT-5.4, GPT-5.5, GPT-5.6 variant, GPT-6 Astra/Sol/Luna, Grok 4.3/4.6, or Gemma 4 model sends a Responses request without a custom base URL
- **THEN** it sends `POST https://bedrock-mantle.<region>.api.aws/openai/v1/responses`

#### Scenario: Normalize explicit AWS settings
- **WHEN** an explicit AWS region or profile has surrounding whitespace
- **THEN** endpoint and credential resolution use the trimmed value

#### Scenario: Custom route
- **WHEN** a model is constructed with a custom base URL ending in `/openai/v1`
- **THEN** its Responses request uses `/responses` beneath that exact base

### Requirement: Mantle authentication policy

Explicit bearer credential/token provider SHALL select bearer mode; explicit AWS credentials SHALL select SigV4. Without either, non-empty `AWS_BEARER_TOKEN_BEDROCK` SHALL select bearer, else the standard AWS chain. Bearer values SHALL be trimmed; empty values SHALL NOT become headers. Each SigV4 attempt SHALL sign the final body for `bedrock-mantle` and resolved region. Authenticated clients SHALL reject redirects bypassing authentication finalization.

#### Scenario: Bearer rollback mode
- **WHEN** a non-empty bearer credential is configured
- **THEN** requests carry `Authorization: Bearer <credential>` and do not carry a SigV4 authorization value

#### Scenario: SigV4 mode
- **WHEN** valid AWS credentials and a region are configured without bearer authentication
- **THEN** the final request authorization scope contains `<region>/bedrock-mantle/aws4_request`
- **AND** the request includes the signed payload hash and any session token

#### Scenario: Retry authentication
- **WHEN** the HTTP client retries a Mantle request
- **THEN** the request is authenticated again using current credentials and its final replayed body

### Requirement: Responses delegation and continuation metadata

Mantle SHALL preserve shared OpenAI adapter request, response, stream, tool and continuation semantics. Model/response attribution SHALL be `bedrock-mantle.responses`; options/metadata SHALL remain under resolved `openai`/`azure`. Per-call headers SHALL reach the final authenticated request.

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

### Requirement: Mantle web-search source include compatibility

Mantle SHALL disable automatic `web_search_call.action.sources` inclusion for web tools in unary/streaming shared OpenAI calls. This restriction SHALL NOT remove tools, change attribution, block unrelated automatic includes or remove explicit caller includes.

#### Scenario: Unary request accepted by strict compatibility endpoint
- **WHEN** a Mantle Responses model makes a `DoGenerate` call with a web-search tool to a synthetic endpoint rejecting `web_search_call.action.sources`
- **THEN** the request retains the web-search tool, omits the automatic web-source include, and is accepted by that endpoint

#### Scenario: Streaming request accepted by strict compatibility endpoint
- **WHEN** a Mantle Responses model makes a `DoStream` call with a web-search tool to the same strict synthetic endpoint
- **THEN** the request retains the web-search tool, omits the automatic web-source include, and its stream succeeds

#### Scenario: Caller-specified source include is preserved
- **WHEN** a Mantle caller explicitly requests `web_search_call.action.sources` via the OpenAI Responses `include` option
- **THEN** the serialized request still contains that value
- **AND** this does not assert Mantle accepts that explicitly requested value

#### Scenario: Unrelated includes and attribution survive
- **WHEN** Mantle requests code-interpreter outputs, logprobs, or stateless encrypted reasoning alongside a web-search tool
- **THEN** their existing automatic include behavior remains intact, and response attribution stays `bedrock-mantle.responses`

#### Scenario: Candidate-source consumer boundary
- **WHEN** the Bedrock module is tested in the SDK workspace with its declared OpenAI module dependency pinned to an older merged revision
- **THEN** Mantle unary and streaming web-tool requests use the candidate OpenAI source and omit the automatic source include

### Requirement: Responses-only parity scope
The provider SHALL expose only the Mantle Responses surface until an equivalent
shared OpenAI Chat implementation exists. It SHALL NOT present Responses as the
upstream default or Chat model factory. Documentation and parity records SHALL
continue to identify Mantle Chat, including safeguard models, as unsupported.

#### Scenario: Reviewer assesses Mantle parity
- **WHEN** a reviewer inspects the parity records after this change
- **THEN** Responses is classified by its focused routing and authentication evidence
- **AND** the upstream Mantle Chat/default provider surface remains an explicit gap

### Requirement: Mantle reconstructed assistant encoding

For unary/stream calls through `mantle.NewResponses` without active conversation, assistant text SHALL reconstruct when `store:false` or no stored item ID: easy-input `role:"assistant"`, string `content` including empty text, preserved phase, no stale ID or incomplete `output_text` content array.

#### Scenario: Mantle reconstructed assistant encoding

- **WHEN** store is false and assistant text has an old item ID, phase and explicitly empty content
- **THEN** the assistant input SHALL retain phase and `content:""`, omitting the old ID and output-message array

### Requirement: Mantle stored assistant and conversation reuse

Without active conversation, storage-enabled assistant text with a stored item ID SHALL retain item-reference continuation rather than resend text. With active conversation, assistant text with an existing item ID SHALL be skipped to avoid context duplication.

#### Scenario: Mantle stored assistant and conversation reuse

- **WHEN** an assistant item with an ID is continued once with storage enabled and once with an active conversation
- **THEN** the former SHALL emit an item reference; the latter SHALL omit the assistant item

### Requirement: Mantle published continuation boundary

Unary/streaming assistant reconstruction and stored continuation SHALL work with the Bedrock module's published dependencies, without workspace substitutions or production replacement directives.

#### Scenario: Mantle published continuation boundary

- **WHEN** a standalone consumer uses published declared dependencies with GOWORK=off
- **THEN** reconstruction SHALL use string-content encoding and stored assistant IDs SHALL still produce item references without dependency replacements

### Requirement: Mantle candidate-source web-include evidence

Candidate-source checks SHALL exercise web-source automatic-include suppression with shared OpenAI implementation in the SDK workspace while Bedrock's declared OpenAI dependency remains pinned to a revision already merged on canonical main.

#### Scenario: Mantle candidate-source web-include evidence

- **WHEN** workspace checks use candidate OpenAI source alongside the older merged declared module revision
- **THEN** Mantle unary and stream web-tool requests SHALL omit automatic web sources without changing the declared dependency pin
