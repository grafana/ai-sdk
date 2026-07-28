package grafana_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/providerwire"
	"github.com/grafana/ai-sdk/provider"
	grafana "github.com/grafana/ai-sdk/providers/grafana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type serverTestModel struct {
	generate func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream   func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
}

func (m *serverTestModel) SpecificationVersion() string               { return "v4" }
func (m *serverTestModel) Provider() string                           { return "test" }
func (m *serverTestModel) ModelID() string                            { return "server-model" }
func (m *serverTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *serverTestModel) DoGenerate(ctx context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	return m.generate(ctx, opts)
}
func (m *serverTestModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	return m.stream(ctx, opts)
}

var _ provider.LanguageModel = (*serverTestModel)(nil)

func newPublicServerProvider(t *testing.T, resolver providerwire.ModelResolver) *grafana.Provider {
	t.Helper()
	handler, err := providerwire.NewHandler(resolver)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle(providerwire.PathLanguageModel, handler)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	clientProvider, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{
		AccessToken: "access-token",
		BaseURL:     server.URL,
	})
	require.NoError(t, err)
	return clientProvider
}

func newPublicServerClient(t *testing.T, model provider.LanguageModel) provider.LanguageModel {
	t.Helper()
	resolver := providerwire.ModelResolverFunc(func(r *http.Request, modelID string) (provider.LanguageModel, error) {
		assert.Equal(t, "server-model", modelID)
		assert.Equal(t, "access-token", r.Header.Get("X-Access-Token"))
		return model, nil
	})
	clientProvider := newPublicServerProvider(t, resolver)
	clientModel, err := clientProvider.LanguageModel("server-model")
	require.NoError(t, err)
	return clientModel
}

func TestPublicProviderWireServer_RealGrafanaClientUnary(t *testing.T) {
	maxTokens := 128
	temperature := 0.25
	opts := provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hello")},
		MaxOutputTokens: &maxTokens,
		Temperature:     &temperature,
		Headers:         map[string]string{"X-Test": "metadata"},
		ProviderOptions: provider.ProviderOptions{"grafana": provider.RawProviderOption{Key: "grafana", Raw: json.RawMessage(`{"trace":true}`)}},
	}
	expected := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "yes"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}
	var got provider.CallOptions
	model := &serverTestModel{generate: func(_ context.Context, actual provider.CallOptions) (*provider.GenerateResult, error) {
		got = actual
		return expected, nil
	}}
	client := newPublicServerClient(t, model)
	result, err := client.DoGenerate(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, opts, got)
	assert.Equal(t, expected.Content, result.Content)
	assert.Equal(t, expected.FinishReason, result.FinishReason)
}

func TestPublicProviderWireServer_RealGrafanaClientStreaming(t *testing.T) {
	stream := make(chan provider.StreamPart)
	var once sync.Once
	model := &serverTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: stream}, nil
	}}
	client := newPublicServerClient(t, model)
	result, err := client.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)

	parts := []provider.StreamPart{
		{Type: provider.PartTextStart, ID: "text"},
		{Type: provider.PartTextDelta, ID: "text", Delta: "hello"},
		{Type: provider.PartTextEnd, ID: "text"},
	}
	for _, part := range parts {
		stream <- part
		select {
		case got := <-result.Stream:
			assert.Equal(t, part, got)
		case <-time.After(time.Second):
			t.Fatal("stream part was not delivered immediately")
		}
	}
	once.Do(func() { close(stream) })
	select {
	case _, open := <-result.Stream:
		assert.False(t, open)
	case <-time.After(time.Second):
		t.Fatal("stream did not close cleanly")
	}
}

func TestPublicProviderWireServer_GatewayCatalog(t *testing.T) {
	expected := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "catalog response"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}
	model := &serverTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return expected, nil
	}}
	modelCatalog, err := catalog.NewStatic([]catalog.StaticEntry{{
		Info:  catalog.ModelInfo{ID: "public-model", Aliases: []string{"friendly-model"}},
		Model: model,
	}})
	require.NoError(t, err)

	var canonicalID string
	resolver := providerwire.ModelResolverFunc(func(r *http.Request, modelID string) (provider.LanguageModel, error) {
		resolved, resolveErr := modelCatalog.ResolveModel(r.Context(), modelID)
		if resolveErr != nil {
			if errors.Is(resolveErr, catalog.ErrUnknownModel) {
				retryable := false
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{
					Message:     resolveErr.Error(),
					StatusCode:  http.StatusNotFound,
					IsRetryable: &retryable,
					Cause:       resolveErr,
				})
			}
			return nil, resolveErr
		}
		canonicalID = resolved.ID
		return resolved.Model, nil
	})
	clientProvider := newPublicServerProvider(t, resolver)

	aliasModel, err := clientProvider.LanguageModel("friendly-model")
	require.NoError(t, err)
	result, err := aliasModel.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, "public-model", canonicalID)
	assert.Equal(t, expected.Content, result.Content)

	missingModel, err := clientProvider.LanguageModel("missing-model")
	require.NoError(t, err)
	_, err = missingModel.DoGenerate(context.Background(), provider.CallOptions{})
	var apiErr *provider.APICallError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusNotFound, apiErr.StatusCode)
	assert.False(t, apiErr.IsRetryable)
	assert.Contains(t, apiErr.Message, `unknown model "missing-model"`)
}

func TestPublicProviderWireServer_RealGrafanaClientPartError(t *testing.T) {
	retryable := true
	apiErr := provider.NewAPICallError(provider.APICallErrorOptions{
		Message:     "upstream failed",
		StatusCode:  http.StatusBadGateway,
		IsRetryable: &retryable,
		Data:        json.RawMessage(`{"code":"upstream"}`),
	})
	stream := make(chan provider.StreamPart, 3)
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}
	stream <- provider.StreamPart{Type: provider.PartError, APICallError: apiErr}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "not forwarded"}
	close(stream)
	model := &serverTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: stream}, nil
	}}
	client := newPublicServerClient(t, model)
	result, err := client.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	var got []provider.StreamPart
	for part := range result.Stream {
		got = append(got, part)
	}
	require.Len(t, got, 2)
	require.Equal(t, provider.PartError, got[1].Type)
	require.NotNil(t, got[1].APICallError)
	assert.Equal(t, apiErr.StatusCode, got[1].APICallError.StatusCode)
	assert.Equal(t, apiErr.IsRetryable, got[1].APICallError.IsRetryable)
	assert.JSONEq(t, string(apiErr.Data), string(got[1].APICallError.Data))
	assert.False(t, errors.Is(got[1].APICallError, apiErr))
}
