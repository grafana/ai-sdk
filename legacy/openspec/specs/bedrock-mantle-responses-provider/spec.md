# bedrock-mantle-responses-provider Specification

## Purpose
Provide an authenticated Amazon Bedrock Mantle Responses model that preserves OpenAI Responses behavior while owning Bedrock-specific routing and attribution.

## Requirements

### Requirement: Mantle Responses construction and identity
The system SHALL expose an error-returning constructor for a Bedrock Mantle
Responses language model. The constructor SHALL accept a context, model ID,
Mantle client configuration, and transport request options. The returned model
SHALL implement the V4 language-model contract, preserve the model ID verbatim,
and report `bedrock-mantle.responses` as its provider identity. Construction
SHALL reject invalid or ambiguous authentication and routing values supplied
through Mantle configuration. Attempts to override protected authentication or
routing through generic request options SHALL fail before network transport.

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
The provider SHALL send Responses requests to the AWS OpenAI-compatible Mantle
surface. Generic models SHALL use the regional
`https://bedrock-mantle.<region>.api.aws/v1` base, while models with documented
route exceptions SHALL use the required model-specific base. The provider SHALL
maintain exact exceptions for every currently documented `/openai/v1` model ID
rather than inferring support from an unrelated model family. Inference requests
SHALL use `/responses` beneath the selected base. A valid custom base URL SHALL
be preserved. Explicit AWS region and profile values SHALL be trimmed before
endpoint resolution. The request body SHALL carry the caller's model ID
verbatim and SHALL NOT use Bedrock Converse request paths or shapes.

#### Scenario: Default generic regional route
- **WHEN** `openai.gpt-oss-20b` configured for `us-east-1` sends a Responses request without a custom base URL
- **THEN** it sends `POST https://bedrock-mantle.us-east-1.api.aws/v1/responses`

#### Scenario: Documented regional route exceptions
- **WHEN** any documented GPT-5.4, GPT-5.5, GPT-5.6 variant, Grok 4.3/4.6, or Gemma 4 model sends a Responses request without a custom base URL
- **THEN** it sends `POST https://bedrock-mantle.<region>.api.aws/openai/v1/responses`

#### Scenario: Normalize explicit AWS settings
- **WHEN** an explicit AWS region or profile has surrounding whitespace
- **THEN** endpoint and credential resolution use the trimmed value

#### Scenario: Custom route
- **WHEN** a model is constructed with a custom base URL ending in `/openai/v1`
- **THEN** its Responses request uses `/responses` beneath that exact base

### Requirement: Mantle authentication policy
The provider SHALL support both Bedrock bearer credentials and AWS SigV4. An
explicit bearer credential or bearer token provider SHALL select bearer mode.
Explicit AWS credentials SHALL select SigV4 mode. When neither mode is explicit,
a non-empty `AWS_BEARER_TOKEN_BEDROCK` SHALL select bearer mode; otherwise the
standard AWS credential chain SHALL be used. Bearer values SHALL be trimmed and
empty values SHALL NOT become authorization headers. SigV4 requests SHALL be
signed for service `bedrock-mantle`, the resolved region, and the final request
body on every attempt. Authenticated clients SHALL reject redirects that could
bypass authentication finalization.

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
The provider SHALL preserve the OpenAI Responses request, response, streaming,
tool, and continuation semantics supplied by the shared OpenAI model adapter.
Model and response attribution SHALL use `bedrock-mantle.responses`, while
provider options and response metadata SHALL remain under the resolved `openai`
or `azure` namespace. Per-call headers SHALL reach the final authenticated
request.

#### Scenario: Stored response continuation
- **WHEN** a Mantle response emits an OpenAI-namespaced assistant item ID that is included in a later prompt
- **THEN** the next request emits an item reference instead of resending stored assistant content

#### Scenario: Streaming attribution and metadata
- **WHEN** a Mantle Responses stream emits response and content metadata
- **THEN** response attribution reports `bedrock-mantle.responses`
- **AND** continuation metadata remains under the OpenAI options namespace

### Requirement: Responses-only parity scope
The provider SHALL expose only the Mantle Responses surface until an equivalent
shared OpenAI Chat implementation exists. It SHALL NOT present Responses as the
upstream default or Chat model factory. Documentation and parity records SHALL
continue to identify Mantle Chat, including safeguard models, as unsupported.

#### Scenario: Reviewer assesses Mantle parity
- **WHEN** a reviewer inspects the parity records after this change
- **THEN** Responses is classified by its focused routing and authentication evidence
- **AND** the upstream Mantle Chat/default provider surface remains an explicit gap
