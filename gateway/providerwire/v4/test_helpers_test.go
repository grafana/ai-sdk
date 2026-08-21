package v4

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

type testModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
	mu       sync.Mutex
	calls    int
	options  provider.CallOptions
}

func (m *testModel) SpecificationVersion() string               { return "v4" }
func (m *testModel) Provider() string                           { return "private-provider" }
func (m *testModel) ModelID() string                            { return "backend-model" }
func (m *testModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *testModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	m.mu.Lock()
	m.calls++
	m.options = options
	m.mu.Unlock()
	if m.generate == nil {
		return validGenerateResult(), nil
	}
	return m.generate(ctx, options)
}
func (m *testModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.mu.Lock()
	m.calls++
	m.options = options
	m.mu.Unlock()
	if m.stream == nil {
		parts := make(chan provider.StreamPart)
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}
	return m.stream(ctx, options)
}

type testResolver struct {
	resolve func(context.Context, string) (catalog.ResolvedModel, error)
	calls   int
	modelID string
}

func (r *testResolver) ResolveModel(ctx context.Context, modelID string) (catalog.ResolvedModel, error) {
	r.calls++
	r.modelID = modelID
	if r.resolve != nil {
		return r.resolve(ctx, modelID)
	}
	return catalog.ResolvedModel{}, nil
}

func resolverFor(model provider.LanguageModel) *testResolver {
	return &testResolver{resolve: func(context.Context, string) (catalog.ResolvedModel, error) {
		return catalog.ResolvedModel{ID: "public/canonical", Model: model}, nil
	}}
}

func validGenerateResult() *provider.GenerateResult {
	one := 1
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "ok"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop"},
		Usage: provider.Usage{
			InputTokens:  provider.InputTokenUsage{Total: &one},
			OutputTokens: provider.OutputTokenUsage{Total: &one},
		},
	}
}

func validFinishPart() provider.StreamPart {
	one := 1
	finish := provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop"}
	usage := provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: &one},
		OutputTokens: provider.OutputTokenUsage{Total: &one},
	}
	return provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &usage, Warnings: []provider.Warning{}}
}

func newTestHandler(t *testing.T, model provider.LanguageModel, options ...Option) *Handler {
	t.Helper()
	handler, err := NewHandler(resolverFor(model), options...)
	require.NoError(t, err)
	return handler
}

func validRequest(body string, streaming bool) *http.Request {
	request := httptest.NewRequest(http.MethodPost, PathLanguageModel, io.NopCloser(strings.NewReader(body)))
	request.Header[HeaderModelID] = []string{"public/alias"}
	request.Header[HeaderSpecVersion] = []string{SpecVersionV4}
	request.Header[HeaderStreaming] = []string{strconv.FormatBool(streaming)}
	request.Header["Content-Type"] = []string{MIMEJSON}
	return request
}
