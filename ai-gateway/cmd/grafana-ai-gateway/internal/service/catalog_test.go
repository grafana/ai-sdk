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
	})
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
	assert.Same(t, models["claude-assistant"], canonical.Model)
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
	}, server.Client())
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
	} {
		created, err := BuildCatalog(file, providers, http.DefaultClient)
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
