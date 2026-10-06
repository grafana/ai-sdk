// Package aisdk is a Go port of Vercel's AI SDK.
//
// It provides streaming LLM orchestration, tool execution, and SSE output
// compatible with the @ai-sdk/react hooks (useChat, useCompletion, useObject).
// The AI SDK docs at https://ai-sdk.dev/docs serve as the primary reference
// for naming, behavior, and protocol details.
//
// # Architecture
//
// The package has two layers:
//
//   - [aisdk/provider] (sub-package): the provider interface and types that LLM
//     provider implementations depend on. This is a leaf package with no
//     dependencies on the root.
//   - aisdk (this package): orchestration (StreamText, GenerateText), the
//     UIMessage type, tool system, SSE stream writing, and HTTP helpers.
//
// # Streaming
//
// [StreamText] is the main entry point. It calls a [provider.LanguageModel],
// runs a multi-step tool execution loop, and emits [TextStreamPart] events on
// [StreamTextResult.FullStream]. These events can be converted to SSE chunks
// via [StreamTextResult.ToUIMessageStream] and piped to an HTTP response with
// [PipeUIMessageStreamToResponse].
//
//	result := aisdk.StreamText(ctx, myProvider,
//	    aisdk.WithMessages(messages...),
//	    aisdk.WithTools(tools),
//	)
//	aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream())
//
// For non-streaming use, [GenerateText] blocks until the LLM finishes and
// returns a populated [GenerateTextResult].
//
// # Messages
//
// [UIMessage] is the canonical message type, matching the AI SDK's UIMessage.
// It is JSON-serializable for persistence. Parts are represented as an
// interface ([Part]) with concrete types ([TextPart], [ToolInvocationPart],
// etc.) for type-safe access via type switches.
//
// Provider-level messages live in the [provider] sub-package as the flat
// discriminated [provider.Message] struct (with [provider.NewSystemMessage],
// [provider.NewUserMessage], [provider.NewAssistantMessage], and
// [provider.NewToolMessage] constructors). [ConvertToModelMessages] bridges
// the UI-layer and provider-layer message types.
//
// [ToResponseMessages] converts a slice of collected response content parts
// into the next-call assistant + tool messages, preserving reasoning blocks
// (and provider signatures) across multi-step tool execution. The same output
// is surfaced on [ResponseMetadata.Messages] for the last completed step.
//
// [ExtractTextContent] and [ExtractReasoningContent] read the forward
// direction: they join text or reasoning parts from a
// [[]provider.ContentPart] slice (text concatenates with no separator,
// reasoning joins with "\n"). [ExtractTextContentGenerate] and
// [ExtractReasoningContentGenerate] are the equivalents for
// [[]provider.GenerateContentPart] returned by [provider.LanguageModel]'s
// DoGenerate.
//
// # Tools
//
// Tools are defined as a [ToolSet] (map[string][Tool]). Each tool has an
// optional Execute function: if nil, the tool call is returned to the caller
// for external execution. If set, StreamText executes it locally and feeds
// the result back to the model.
//
// # Standalone stream composition
//
// [CreateUIMessageStream] and [UIMessageStreamWriter] allow composing LLM
// output with custom data parts independently of StreamText, matching the
// AI SDK's createUIMessageStream API.
package aisdk
