// Package bedrock implements provider.LanguageModel for the AWS Bedrock
// Converse API, supporting Anthropic, Mistral, Amazon Nova, and OpenAI models
// hosted on Amazon Bedrock.
//
// The package authenticates with AWS SigV4 by default (using the standard AWS
// credentials chain) and optionally with a Bearer token via WithBearerToken
// or the AWS_BEARER_TOKEN_BEDROCK environment variable.
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
