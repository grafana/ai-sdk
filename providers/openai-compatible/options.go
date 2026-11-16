package openaicompatible

import (
	"net/http"

	"github.com/grafana/ai-sdk/provider"
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
// It preserves request-conversion warnings. Use WithRequestTransformAndWarnings
// to also report or change warnings. The last transform option wins; nil disables
// transformation. The callback may run concurrently for calls on the same model.
func WithRequestTransform(transform func(map[string]any) (map[string]any, error)) Option {
	if transform == nil {
		return WithRequestTransformAndWarnings(nil)
	}
	return WithRequestTransformAndWarnings(func(body map[string]any, warnings []provider.Warning) (map[string]any, []provider.Warning, error) {
		body, err := transform(body)
		return body, warnings, err
	})
}

// WithRequestTransformAndWarnings rewrites the completed JSON body and warnings
// before sending a generation or streaming request. The callback receives the
// current call's conversion warnings and returns the complete warning slice;
// append to the input slice to retain existing warnings. Returned warnings appear
// in GenerateResult.Warnings or the streaming PartStreamStart event.
//
// An error or nil body prevents the HTTP request. The last of this option and
// WithRequestTransform wins; nil disables transformation. Bodies and conversion
// warnings are call-local. The callback may run concurrently and must synchronize
// any captured mutable state; it must not mutate returned values after returning.
func WithRequestTransformAndWarnings(transform func(map[string]any, []provider.Warning) (map[string]any, []provider.Warning, error)) Option {
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

	extraFields map[string]any
}

// ProviderKey returns the provider option namespace.
func (OpenAIOptions) ProviderKey() string { return "openaiCompatible" }
