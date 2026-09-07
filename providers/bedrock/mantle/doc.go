// Package mantle implements Bedrock Mantle's OpenAI-compatible Responses API.
//
// Models authenticate with Bedrock bearer credentials or AWS SigV4 through the
// official openai-go Bedrock client. SigV4 requests use the bedrock-mantle
// service and the standard AWS credential chain by default. Bearer credentials
// can be supplied explicitly or through AWS_BEARER_TOKEN_BEDROCK for rollback.
//
// Requests use the regional /v1 endpoint by default. GPT-5.6 Luna uses its
// model-specific /openai/v1 endpoint. The model reports
// "bedrock-mantle.responses" for attribution while Responses provider options
// and continuation metadata remain under the "openai" namespace.
//
// This package intentionally exposes Responses only. Bedrock Mantle Chat
// Completions, including safeguard models, remain unsupported until the Go
// OpenAI provider exposes an equivalent shared Chat implementation.
package mantle
