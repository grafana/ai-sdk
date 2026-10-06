## MODIFIED Requirements

### Requirement: Constructor and options

The provider SHALL expose a single constructor `New(modelID string, opts ...Option) provider.LanguageModel` that accepts functional options for region, credentials, base URL, SigV4 signing service, HTTP client, request headers, and ID generation.

#### Scenario: Basic construction

- **WHEN** a consumer calls `bedrock.New("anthropic.claude-sonnet-4-5-20250929-v1:0", bedrock.WithRegion("us-east-1"))`
- **THEN** the call returns a `provider.LanguageModel` whose `ModelID()` equals the supplied model ID

#### Scenario: Bearer token takes precedence over SigV4

- **WHEN** a consumer constructs the provider with both `WithBearerToken("token")` and credentials configured
- **THEN** outgoing requests carry `Authorization: Bearer token` and no SigV4 signature headers

#### Scenario: Custom HTTP client

- **WHEN** a consumer supplies `WithHTTPClient(client)` and makes a call
- **THEN** the request is dispatched through the supplied client

#### Scenario: Custom base URL

- **WHEN** a consumer supplies `WithBaseURL("https://custom.example.com")`
- **THEN** the provider issues requests against `https://custom.example.com/model/{modelID}/converse[-stream]` instead of the default AWS endpoint

#### Scenario: Custom signing service

- **WHEN** a consumer supplies `WithSigningService("bedrock-mantle")`
- **THEN** SigV4 signatures use `bedrock-mantle` as the credential-scope service name regardless of the endpoint host

#### Scenario: Application inference-profile ARN

- **WHEN** the model ID is an application inference-profile ARN
- **THEN** the Converse and ConverseStream request paths preserve the ARN `:` and `/` delimiters while ordinary model IDs remain URL-segment escaped

### Requirement: AWS authentication

When no bearer token is configured, the provider SHALL sign outbound `POST` requests using AWS Signature Version 4 with credentials obtained from a configured `aws.CredentialsProvider` or, if absent, the AWS SDK v2 default credential chain. Requests without a body or non-`POST` requests MUST be sent without signing.

The provider SHALL resolve the SigV4 credential-scope service name per request as follows: an explicit `WithSigningService` value takes precedence; otherwise, when the endpoint host is a Bedrock Mantle host (`bedrock-mantle.<region>.api.aws`) the service name SHALL be `bedrock-mantle`; otherwise the service name SHALL be `bedrock`. The resolved service name MUST NOT affect bearer-token authentication.

#### Scenario: Default credential chain

- **WHEN** the consumer constructs the provider without `WithCredentials` or `WithBearerToken`
- **THEN** the provider uses AWS SDK v2's default credential resolution (env vars, shared config, EC2/IRSA, etc.) at request time

#### Scenario: Explicit credentials provider

- **WHEN** the consumer passes `WithCredentials(cp)` where `cp` is an `aws.CredentialsProvider`
- **THEN** the provider signs requests using credentials returned by `cp.Retrieve(ctx)` for the configured region

#### Scenario: Bearer token via environment

- **WHEN** the env var `AWS_BEARER_TOKEN_BEDROCK` is set and `WithBearerToken` is not used
- **THEN** the provider sends `Authorization: Bearer <env value>` and skips SigV4

#### Scenario: SigV4 service and region

- **WHEN** the provider signs a request for the default Bedrock Runtime endpoint in region `us-east-1` without a signing-service override
- **THEN** the signature uses service name `bedrock` and the configured region

#### Scenario: Mantle endpoint infers bedrock-mantle service

- **WHEN** `WithBaseURL` targets a Bedrock Mantle host (`bedrock-mantle.<region>.api.aws`) and no signing-service override is configured
- **THEN** the signature uses service name `bedrock-mantle` and the configured region

#### Scenario: Explicit signing service overrides host inference

- **WHEN** a signing-service override is configured via `WithSigningService`
- **THEN** the signature uses the overriding service name even when the endpoint host would otherwise infer a different service (for example, a Mantle host forced to `bedrock`, or a non-Mantle proxy host forced to `bedrock-mantle`)
