// Package bedrock implements provider.LanguageModel for the AWS Bedrock
// Converse API, supporting Anthropic, Mistral, Amazon Nova, and OpenAI models
// hosted on Amazon Bedrock.
//
// The package authenticates with AWS SigV4 by default (using the standard AWS
// credentials chain) and optionally with a Bearer token via WithBearerToken
// or the AWS_BEARER_TOKEN_BEDROCK environment variable.
//
// SigV4 signatures are scoped to the "bedrock" service for standard Bedrock
// Runtime endpoints. Bedrock Mantle is a separate AWS service whose signatures
// must be scoped to "bedrock-mantle"; when WithBaseURL targets a Mantle
// endpoint (bedrock-mantle.<region>.api.aws) the signing service is inferred
// automatically, and WithSigningService overrides the inference when the host
// does not encode the service (for example, behind a proxy). Signing is
// endpoint-agnostic: routing Converse-shaped requests to Mantle's
// OpenAI-/Anthropic-compatible API surfaces is out of scope for this package.
//
// Anthropic-specific request features (thinking, effort, betas, native
// structured output) are routed through Converse's
// additionalModelRequestFields pass-through when the model ID identifies an
// Anthropic model on Bedrock.
//
// The module is independent of providers/anthropic and ships its own AWS SDK
// v2 dependency. Provider() returns "amazon-bedrock", matching the upstream
// @ai-sdk/amazon-bedrock provider key for cross-SDK metadata compatibility.
package bedrock

// providerName is the constant returned by model.Provider(), matching upstream
// @ai-sdk/amazon-bedrock so providerOptions["amazonBedrock"] and the matching
// providerMetadata namespace round-trip across both SDKs.
const providerName = "amazon-bedrock"

// specificationVersion identifies the language model spec version implemented
// by this provider. Mirrors provider.LanguageModel V4 contract.
const specificationVersion = "v4"
