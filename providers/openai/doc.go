// Package openai implements a [provider.LanguageModel] backed by the OpenAI
// Responses API (POST /v1/responses) using the official
// github.com/openai/openai-go/v3 SDK for transport, request params, and SSE
// event types.
//
// The provider mirrors the behavior of Vercel AI SDK's OpenAI Responses
// implementation: it converts a [provider.CallOptions] prompt into the
// Responses input item array, prepares function and built-in (server-executed)
// tools, maps the rich Responses event protocol to [provider.StreamPart] for
// streaming and to [provider.GenerateContentPart] for non-streaming calls, and
// surfaces reasoning, sources/citations, structured output, and OpenAI-specific
// provider options.
//
// Construct a model with [NewResponses]:
//
//	model := openai.NewResponses(apiKey, "gpt-4o")
//
// Provider integrations that supply their own official SDK client can reuse
// the Responses implementation with [NewResponsesWithClient] and set the
// identity reported by the model with [WithProviderName]. Response metadata
// remains in the OpenAI or Azure options namespace for multi-step round trips.
//
// Per-call OpenAI options are passed via
// CallOptions.ProviderOptions["openai"] as an [OpenAIResponsesOptions] value.
package openai
