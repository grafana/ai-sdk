package openaicompatible

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

const (
	specificationVersion = "v4"
	defaultBaseURL       = "https://api.openai.com/v1"
	defaultProviderName  = "openai-compatible"
	userAgent            = "ai-sdk-go/openai-compatible"
	streamBufferSize     = 64
)

// model implements provider.LanguageModel for OpenAI-compatible chat
// completions.
type model struct {
	modelID                   string
	providerName              string
	apiKey                    string
	baseURL                   string
	headers                   map[string]string
	queryParams               map[string]string
	httpClient                *http.Client
	includeUsage              bool
	supportsStructuredOutputs bool
	transformRequestBody      func(map[string]any) (map[string]any, error)
	generateID                func() string
}

var _ provider.LanguageModel = (*model)(nil)

// New constructs an OpenAI-compatible chat completions language model.
//
// When WithBaseURL is not supplied, requests go to https://api.openai.com/v1.
// WithAPIKey is optional so local OpenAI-compatible servers that do not enforce
// authentication can be used without dummy credentials.
func New(modelID string, opts ...Option) provider.LanguageModel {
	m := &model{
		modelID:      modelID,
		providerName: defaultProviderName,
		baseURL:      defaultBaseURL,
		httpClient:   http.DefaultClient,
		generateID:   defaultGenerateID,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// SpecificationVersion implements provider.LanguageModel.
func (m *model) SpecificationVersion() string { return specificationVersion }

// Provider implements provider.LanguageModel.
func (m *model) Provider() string { return m.providerName }

// ModelID implements provider.LanguageModel.
func (m *model) ModelID() string { return m.modelID }

// SupportedURLs implements provider.LanguageModel. URL file-part support varies
// across compatible servers, so the provider does not advertise a fast path.
func (m *model) SupportedURLs() map[string][]*regexp.Regexp { return nil }

// DoGenerate performs a non-streaming chat completions call.
func (m *model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	return m.doGenerate(ctx, params)
}

// DoStream performs a streaming chat completions call.
func (m *model) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	return m.doStream(ctx, params)
}

func (m *model) endpoint(path string) (string, error) {
	base := strings.TrimRight(m.baseURL, "/")
	u, err := url.Parse(base + path)
	if err != nil {
		return "", err
	}
	if len(m.queryParams) > 0 {
		q := u.Query()
		for k, v := range m.queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func defaultGenerateID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "call_fallback"
	}
	return "call_" + hex.EncodeToString(b[:])
}
