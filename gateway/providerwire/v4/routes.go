package providerwirev4

const (
	// PathLanguageModel is the relative LanguageModelV4 service route.
	PathLanguageModel = "/language-model"
	// HeaderModelID carries the requested public model ID.
	HeaderModelID = "ai-language-model-id"
	// HeaderStreaming selects unary or streaming execution.
	HeaderStreaming = "ai-language-model-streaming"
	// HeaderSpecVersion carries the LanguageModel specification version.
	HeaderSpecVersion = "ai-language-model-specification-version"
	// SpecVersionV4 is the supported LanguageModel specification version.
	SpecVersionV4 = "4"
	// MIMEJSON is the unary JSON media type.
	MIMEJSON = "application/json"
	// MIMESSE is the streaming event media type.
	MIMESSE = "text/event-stream"
)
