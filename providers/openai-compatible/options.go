package openaicompatible

import (
	"github.com/grafana/ai-sdk/provider"
	"net/http"
)

// Option configures an OpenAI-compatible model instance.
type Option func(*model)

// WithAPIKey sets the bearer token used for Authorization.
func WithAPIKey(apiKey string) Option {
	return func(m *model) { m.apiKey = apiKey }
}

// WithBaseURL sets the OpenAI-compatible API base URL, usually ending in /v1.
func WithBaseURL(baseURL string) Option {
	return func(m *model) { m.baseURL = baseURL }
}

// WithProviderName sets the provider identifier returned by Provider.
func WithProviderName(name string) Option {
	return func(m *model) {
		if name != "" {
			m.providerName = name
		}
	}
}

// WithHeaders attaches static headers to every outbound request. Per-call
// CallOptions.Headers override these values.
func WithHeaders(headers map[string]string) Option {
	return func(m *model) {
		if m.headers == nil {
			m.headers = make(map[string]string, len(headers))
		}
		for k, v := range headers {
			m.headers[k] = v
		}
	}
}

// WithQueryParams attaches static query parameters to every request URL.
func WithQueryParams(params map[string]string) Option {
	return func(m *model) {
		if m.queryParams == nil {
			m.queryParams = make(map[string]string, len(params))
		}
		for k, v := range params {
			m.queryParams[k] = v
		}
	}
}

// WithHTTPClient sets the HTTP client used for outbound requests.
func WithHTTPClient(client *http.Client) Option {
	return func(m *model) {
		if client != nil {
			m.httpClient = client
		}
	}
}

// WithIncludeUsage controls whether streaming requests send
// stream_options.include_usage. Some compatible servers reject stream_options,
// so the default is false.
func WithIncludeUsage(include bool) Option {
	return func(m *model) { m.includeUsage = include }
}

// WithStructuredOutputs enables JSON-schema response_format requests.
func WithStructuredOutputs(enabled bool) Option {
	return func(m *model) { m.supportsStructuredOutputs = enabled }
}

// WithRequestTransform rewrites the JSON body before it is sent. It is useful
// for compatible providers that need a small deviation from OpenAI's shape.
func WithRequestTransform(transform func(map[string]any) (map[string]any, error)) Option {
	return func(m *model) { m.transformRequestBody = transform }
}

// WithGenerateID overrides the fallback tool-call ID generator. Intended for
// tests and deterministic fixture generation.
func WithGenerateID(gen func() string) Option {
	return func(m *model) {
		if gen != nil {
			m.generateID = gen
		}
	}
}

// OpenAIOptions carries OpenAI-compatible provider-specific request options
// from CallOptions.ProviderOptions["openaiCompatible"].
type OpenAIOptions struct {
	// MaxTokensField selects the native name for CallOptions.MaxOutputTokens.
	// It is a local selector, never a second token budget. Empty uses max_tokens.
	MaxTokensField MaxTokensField `json:"maxTokensField,omitempty"`

	// AllowedTools restricts the common tool list without replacing declarations.
	AllowedTools *AllowedToolsOption `json:"allowedTools,omitempty"`
	// User is forwarded as the OpenAI-compatible `user` request field.
	User string `json:"user,omitempty"`
	// ReasoningEffort overrides CallOptions.Reasoning and is sent as
	// `reasoning_effort`.
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// TextVerbosity is forwarded as the OpenAI-compatible `verbosity` request
	// field.
	TextVerbosity string `json:"textVerbosity,omitempty"`
	// StrictJSONSchema controls response_format.json_schema.strict. Defaults to
	// true when structured outputs are enabled and a schema is provided.
	StrictJSONSchema *bool `json:"strictJsonSchema,omitempty"`

	// PreserveChatCompletionMetadata retains validated Chat-only output fields.
	// This local codec option is not sent to the provider.
	PreserveChatCompletionMetadata *bool `json:"preserveChatCompletionMetadata,omitempty"`

	// Modalities and Audio request native Chat audio generation.
	Modalities []string                    `json:"modalities,omitempty"`
	Audio      *ChatCompletionAudioOptions `json:"audio,omitempty"`

	// Prediction and WebSearchOptions configure native Chat generation.
	Prediction       *ChatPrediction       `json:"prediction,omitempty"`
	WebSearchOptions *ChatWebSearchOptions `json:"web_search_options,omitempty"`

	Moderation *ChatModerationOptions `json:"moderation,omitempty"`

	PromptCacheOptions *ChatPromptCacheOptions `json:"prompt_cache_options,omitempty"`

	extraFields map[string]any
}

// ProviderKey returns the provider option namespace.
func (OpenAIOptions) ProviderKey() string { return "openaiCompatible" }

// ImageDetail selects how a compatible provider processes an input image.
type ImageDetail string

const (
	// ImageDetailAuto lets the native provider choose image resolution.
	ImageDetailAuto ImageDetail = "auto"
	// ImageDetailOriginal requests original-resolution image processing.
	ImageDetailOriginal ImageDetail = "original"
	// ImageDetailLow requests low-resolution image processing.
	ImageDetailLow ImageDetail = "low"
	// ImageDetailHigh requests high-resolution image processing.
	ImageDetailHigh ImageDetail = "high"
)

// ImageOptions configures a file part without replacing its builder-owned URL.
// This typed Go extension maps imageDetail to image_url.detail and does not
// forward imageDetail as a top-level native content-part field.
type ImageOptions struct {
	Detail ImageDetail `json:"imageDetail,omitempty"`
}

// ProviderKey returns the image option namespace.
func (ImageOptions) ProviderKey() string { return "openaiCompatible" }

// MaxTokensField selects the native Chat token-limit field.
type MaxTokensField string

const (
	// MaxTokensFieldLegacy preserves the pinned max_tokens default.
	MaxTokensFieldLegacy MaxTokensField = "max_tokens"
	// MaxTokensFieldCompletion selects max_completion_tokens for reasoning APIs.
	MaxTokensFieldCompletion MaxTokensField = "max_completion_tokens"
)

// AllowedToolsOption selects declared tool names under the common auto/required mode.
type AllowedToolsOption struct {
	ToolNames []string                `json:"toolNames"`
	Mode      provider.ToolChoiceType `json:"mode,omitempty"`
}
