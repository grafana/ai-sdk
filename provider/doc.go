// Package provider defines the interface and types that LLM provider
// implementations depend on. It is aligned with the Vercel AI SDK's
// LanguageModelV4 specification
// (https://github.com/vercel/ai/tree/main/packages/provider/src/language-model/v4).
//
// This is the leaf package in the aisdk module: it has no dependencies on
// the root aisdk package. Provider implementers import only this package.
//
// # LanguageModel interface
//
// [LanguageModel] mirrors LanguageModelV4 from the AI SDK provider spec.
// It has two call methods: [LanguageModel.DoStream] for streaming and
// [LanguageModel.DoGenerate] for non-streaming calls. Both accept
// [CallOptions] (LanguageModelV4CallOptions) and return typed results
// ([StreamResult] / [GenerateResult]).
//
//	type LanguageModel interface {
//	    SpecificationVersion() string
//	    Provider() string
//	    ModelID() string
//	    SupportedURLs() map[string][]*regexp.Regexp
//	    DoStream(ctx context.Context, params CallOptions) (*StreamResult, error)
//	    DoGenerate(ctx context.Context, params CallOptions) (*GenerateResult, error)
//	}
//
// # Messages
//
// [Message] is a flat discriminated struct with a [Role] field selecting
// the variant. Content is `[]ContentPart`; each [ContentPart] is itself
// a flat discriminated struct keyed by [ContentPartType]. Constructor
// helpers ([NewSystemMessage], [NewUserMessage], [NewAssistantMessage],
// [NewToolMessage]) produce well-formed messages for each role.
//
// This shape mirrors LanguageModelV4Message / V4Content from upstream;
// runtime types serialize directly to JSON with no DTO layer.
//
// # Streaming
//
// [StreamResult] carries a channel of [StreamPart] events. The [StreamPartType]
// constants enumerate all possible event types emitted by a provider.
//
// # Shared types
//
// [FinishReason], [Usage], [ToolChoice], [Warning], and [ProviderMetadata] are shared
// between this package and the root aisdk package.
package provider
