package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCatalog_ConstructsImmutableCanonicalAndAliasModelsOnce(t *testing.T) {
	file := testCatalogFile()
	resolved := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "secret", BaseURL: "https://provider.example"},
	}
	calls := 0
	models := make(map[string]*catalogTestModel)
	created, err := buildCatalog(file, resolved, http.DefaultClient, func(apiKey, modelID string, _ ...anthropicprovider.Option) provider.LanguageModel {
		calls++
		assert.Equal(t, "secret", apiKey)
		model := &catalogTestModel{id: modelID}
		models[modelID] = model
		return model
	}, identityModelFactory)
	require.NoError(t, err)
	assert.Equal(t, 2, calls)

	canonical, err := created.ResolveModel(context.Background(), "grafana/assistant")
	require.NoError(t, err)
	alias, err := created.ResolveModel(context.Background(), "assistant")
	require.NoError(t, err)
	again, err := created.ResolveModel(context.Background(), "grafana/assistant")
	require.NoError(t, err)
	assert.Equal(t, "grafana/assistant", canonical.ID)
	assert.Equal(t, "grafana/assistant", alias.ID)
	assert.Same(t, canonical.Model, alias.Model)
	assert.Same(t, canonical.Model, again.Model)
	assert.NotSame(t, models["claude-assistant"], canonical.Model)
	assert.Equal(t, "grafana", canonical.Model.Provider())
	assert.Equal(t, "grafana/assistant", canonical.Model.ModelID())
}

func TestBuildCatalog_ModelFactoryReceivesCanonicalAndUnchangedLowerOnce(t *testing.T) {
	file := testCatalogFile()
	resolved := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "secret"},
	}
	direct := make(map[string]provider.LanguageModel)
	constructed := 0
	factoryCalls := 0
	created, err := buildCatalog(file, resolved, http.DefaultClient, func(_ string, modelID string, _ ...anthropicprovider.Option) provider.LanguageModel {
		constructed++
		model := &catalogTestModel{id: modelID}
		direct[modelID] = model
		return model
	}, func(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error) {
		factoryCalls++
		switch canonicalID {
		case "grafana/assistant":
			assert.Same(t, direct["claude-assistant"], lower)
		case "grafana/other":
			assert.Same(t, direct["claude-other"], lower)
		default:
			t.Fatalf("unexpected canonical ID %q", canonicalID)
		}
		return identityModelFactory(canonicalID, lower)
	})
	require.NoError(t, err)
	assert.Equal(t, 2, constructed)
	assert.Equal(t, 2, factoryCalls)

	canonical, err := created.ResolveModel(context.Background(), "grafana/assistant")
	require.NoError(t, err)
	alias, err := created.ResolveModel(context.Background(), "assistant")
	require.NoError(t, err)
	assert.Same(t, canonical.Model, alias.Model)
	assert.Equal(t, "grafana", canonical.Model.Provider())
	assert.Equal(t, "grafana/assistant", canonical.Model.ModelID())
}

func TestBuildCatalog_ModelFactoryFailureIsFailFast(t *testing.T) {
	file := testCatalogFile()
	resolved := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "secret"},
	}
	for _, factory := range []ModelFactory{
		nil,
		func(string, provider.LanguageModel) (provider.LanguageModel, error) { return nil, assert.AnError },
		func(string, provider.LanguageModel) (provider.LanguageModel, error) { return nil, nil },
	} {
		created, err := buildCatalog(file, resolved, http.DefaultClient, func(_ string, modelID string, _ ...anthropicprovider.Option) provider.LanguageModel {
			return &catalogTestModel{id: modelID}
		}, factory)
		require.Error(t, err)
		assert.Nil(t, created)
	}
}

func TestBuildCatalog_InjectsExplicitClientBaseURLAndBackendModel(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		assert.Equal(t, "/v1/messages", request.URL.Path)
		assert.Equal(t, "explicit-key", request.Header.Get("X-Api-Key"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"model":"backend-private"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"backend-private","stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	file := config.File{
		Providers: map[string]config.Provider{"anthropic-primary": {Type: "anthropic", APIKeyEnv: "KEY", BaseURL: server.URL}},
		Models:    map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "anthropic-primary", Model: "backend-private"}}},
	}
	created, err := BuildCatalog(file, map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "explicit-key", BaseURL: server.URL},
	}, server.Client(), identityModelFactory)
	require.NoError(t, err)
	resolved, err := created.ResolveModel(context.Background(), "public")
	require.NoError(t, err)
	maxTokens := 64
	result, err := resolved.Model.DoGenerate(context.Background(), provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hello")},
		MaxOutputTokens: &maxTokens,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, requests)
	assert.Equal(t, "public", resolved.ID)
}

func TestBuildCatalog_RejectsMissingOrInvalidReferences(t *testing.T) {
	file := testCatalogFile()
	for _, providers := range []map[string]config.ResolvedProvider{
		{},
		{"anthropic-primary": {Type: "openai", APIKey: "secret"}},
		{"anthropic-primary": {Type: "anthropic"}},
		{"anthropic-primary": {Type: "openai-compatible", APIKey: "secret"}},
	} {
		created, err := BuildCatalog(file, providers, http.DefaultClient, identityModelFactory)
		require.Error(t, err)
		assert.Nil(t, created)
	}
}

func testCatalogFile() config.File {
	return config.File{
		Providers: map[string]config.Provider{"anthropic-primary": {Type: "anthropic", APIKeyEnv: "KEY"}},
		Models: map[string]config.Model{
			"grafana/assistant": {Name: "Assistant", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude-assistant"}, Aliases: []string{"assistant"}},
			"grafana/other":     {Name: "Other", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude-other"}},
		},
	}
}

type catalogTestModel struct {
	id string
}

func (model *catalogTestModel) SpecificationVersion() string { return "v4" }
func (model *catalogTestModel) Provider() string             { return "anthropic" }
func (model *catalogTestModel) ModelID() string              { return model.id }
func (model *catalogTestModel) SupportedURLs() map[string][]*regexp.Regexp {
	return nil
}
func (model *catalogTestModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	return nil, nil
}
func (model *catalogTestModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

func TestBuildCatalog_InjectsOpenAICompatibleClientBaseURLAndBackendModel(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		assert.Equal(t, "/v1/chat/completions", request.URL.Path)
		assert.Equal(t, "Bearer explicit-key", request.Header.Get("Authorization"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"model":"backend-private"`)
		assert.Contains(t, string(body), `"stream_options":{"include_usage":true}`)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	file := config.File{
		Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "local", Model: "backend-private"}}},
	}
	var lower provider.LanguageModel
	created, err := buildCatalog(file, map[string]config.ResolvedProvider{
		"local": {Type: "openai-compatible", APIKey: "explicit-key", BaseURL: server.URL + "/v1", ProviderName: "ollama"},
	}, server.Client(), anthropicprovider.New, func(canonicalID string, model provider.LanguageModel) (provider.LanguageModel, error) {
		lower = model
		return identityModelFactory(canonicalID, model)
	})
	require.NoError(t, err)
	resolved, err := created.ResolveModel(context.Background(), "public")
	require.NoError(t, err)
	// The configured provider name reaches the constructed model; the logical
	// wrapper publishes "grafana" so the backend identity stays private.
	require.NotNil(t, lower)
	assert.Equal(t, "ollama", lower.Provider())
	assert.Equal(t, "grafana", resolved.Model.Provider())
	result, err := resolved.Model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	finished := false
	for part := range result.Stream {
		if part.Type == provider.PartFinish {
			finished = true
		}
	}
	assert.True(t, finished, "a stream without a usage chunk still finishes")
	assert.Equal(t, 1, requests)
}
