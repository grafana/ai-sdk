package providerwire

// PathLanguageModel is the HTTP path served by both DoGenerate (unary) and
// DoStream (server-streaming). The streaming mode is selected by the
// [HeaderStreaming] header rather than a separate path, mirroring upstream.
const PathLanguageModel = "/language-model"

// HTTP header names used by the wire. They match upstream Vercel AI SDK
// gateway conventions exactly.
const (
	// HeaderModelID carries the model id, e.g. "claude-sonnet-4-5-20250929".
	HeaderModelID = "ai-language-model-id"

	// HeaderStreaming is "true" for DoStream and "false" for DoGenerate.
	HeaderStreaming = "ai-language-model-streaming"

	// HeaderSpecVersion is the LanguageModelV* spec version. Always "4" today.
	HeaderSpecVersion = "ai-language-model-specification-version"
)

// SpecVersionV4 is the value sent in [HeaderSpecVersion] for V4-compatible
// providers (the only value currently supported).
const SpecVersionV4 = "4"

// MIME types used on the wire.
const (
	MIMEJSON = "application/json"
	MIMESSE  = "text/event-stream"
)
