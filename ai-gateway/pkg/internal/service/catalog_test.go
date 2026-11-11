package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildCatalog_ConfiguredCandidateFacts(t *testing.T) {
	for _, tc := range []struct{ name, providerType, providerName, expected string }{
		{"openai", "openai", "", "openai"},
		{"compatible default", "openai-compatible", "", "openai-compatible"},
		{"compatible custom", "openai-compatible", "custom.chat", "custom.chat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := testCatalogFile()
			route := file.Models["grafana/assistant"]
			route.Fallback = []config.Primary{{Provider: "backup", Model: "native-backup"}}
			file.Models["grafana/assistant"] = route
			created, err := BuildCatalog(file, map[string]config.ResolvedProvider{
				"anthropic-primary": {Type: "anthropic", APIKey: "secret-primary"},
				"backup":            {Type: tc.providerType, ProviderName: tc.providerName, APIKey: "secret-backup", BaseURL: "https://backend.invalid/v1"},
				"unrelated-account": {Type: "anthropic", APIKey: "unrelated-secret", BaseURL: "https://unrelated.invalid"},
			}, http.DefaultClient, identityModelFactory)
			require.NoError(t, err)
			rows, err := created.ListModels(context.Background())
			require.NoError(t, err)
			raw, err := json.Marshal(rows[0])
			require.NoError(t, err)
			assert.Contains(t, string(raw), `"ProviderInstance":"backup"`)
			assert.Contains(t, string(raw), `"Provider":"`+tc.expected+`"`)
			assert.Contains(t, string(raw), `"ModelID":"native-backup"`)
			for _, secret := range []string{"secret-primary", "secret-backup", "backend.invalid", "unrelated-account", "unrelated-secret", "unrelated.invalid", "ANTHROPIC_SECRET"} {
				assert.NotContains(t, string(raw), secret)
			}
		})
	}
}

func TestBuildCatalog_ListingExcludesSkillsPolicy(t *testing.T) {
	file := testCatalogFile()
	route := file.Models["grafana/assistant"]
	route.Primary.Skills = &config.SkillsPolicy{Allow: []config.AllowedSkill{{Type: config.SkillCustom, SkillID: "skill_policy_secret", Versions: []string{"policy-version"}}}}
	file.Models["grafana/assistant"] = route
	created, err := BuildCatalog(file, map[string]config.ResolvedProvider{"anthropic-primary": {Type: "anthropic", APIKey: "secret"}}, http.DefaultClient, identityModelFactory)
	require.NoError(t, err)
	rows, err := created.ListModels(context.Background())
	require.NoError(t, err)
	raw, err := json.Marshal(rows)
	require.NoError(t, err)
	for _, policy := range []string{"skill_policy_secret", "policy-version", "Skills", "skills"} {
		assert.NotContains(t, string(raw), policy)
	}
}

func TestBuildCatalog_CandidateSkillsPolicy(t *testing.T) {
	pptxPolicy := &config.SkillsPolicy{Allow: []config.AllowedSkill{{Type: config.SkillAnthropic, SkillID: "pptx"}}}
	pptx := provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"container":{"skills":[{"type":"anthropic","skillId":"pptx"}]}}`)}}
	pptxWithID := provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"container":{"id":"container_01","skills":[{"type":"anthropic","skillId":"pptx"}]}}`)}}
	file := config.File{
		Providers: map[string]config.Provider{"anthropic-primary": {Type: "anthropic", APIKeyEnv: "KEY"}, "anthropic-backup": {Type: "anthropic", APIKeyEnv: "KEY"}},
		Models: map[string]config.Model{
			"grafana/plain":    {Name: "Plain", Primary: config.Primary{Provider: "anthropic-primary", Model: "plain"}, Fallback: []config.Primary{{Provider: "anthropic-backup", Model: "plain"}}},
			"grafana/policied": {Name: "Policied", Primary: config.Primary{Provider: "anthropic-primary", Model: "policied", Skills: pptxPolicy}},
			"grafana/mixed":    {Name: "Mixed", Primary: config.Primary{Provider: "anthropic-primary", Model: "mixed"}, Fallback: []config.Primary{{Provider: "anthropic-backup", Model: "mixed", Skills: pptxPolicy}}},
			"grafana/empty":    {Name: "Empty", Primary: config.Primary{Provider: "anthropic-primary", Model: "empty", Skills: &config.SkillsPolicy{}}},
			"grafana/busy":     {Name: "Busy", Primary: config.Primary{Provider: "anthropic-primary", Model: "busy"}, Fallback: []config.Primary{{Provider: "anthropic-backup", Model: "busy", Skills: pptxPolicy}}},
		},
	}
	type call struct{ account, model, options string }
	build := func(t *testing.T) (*Catalog, *[]call) {
		t.Helper()
		var calls []call
		created, err := buildCatalog(file, map[string]config.ResolvedProvider{
			"anthropic-primary": {Type: "anthropic", APIKey: "primary-key"},
			"anthropic-backup":  {Type: "anthropic", APIKey: "backup-key"},
		}, http.DefaultClient, func(account nativemodel.Config, modelID string, _ *http.Client) provider.LanguageModel {
			return &observabilityTestModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
				raw, _ := options.ProviderOptions["anthropic"].(provider.RawProviderOption)
				calls = append(calls, call{account: account.APIKey, model: modelID, options: string(raw.Raw)})
				if modelID == "busy" {
					return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "busy", StatusCode: http.StatusServiceUnavailable, IsRetryable: new(true)})
				}
				return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
			}}
		}, identityModelFactory)
		require.NoError(t, err)
		return created, &calls
	}
	generate := func(t *testing.T, created *Catalog, id string, options provider.ProviderOptions) error {
		t.Helper()
		resolved, err := created.ResolveModel(context.Background(), id)
		require.NoError(t, err)
		_, err = resolved.Model.DoGenerate(context.Background(), provider.CallOptions{ProviderOptions: options})
		return err
	}

	t.Run("policied route reaches the backend with the skills", func(t *testing.T) {
		created, calls := build(t)
		require.NoError(t, generate(t, created, "grafana/policied", pptx))
		require.Len(t, *calls, 1)
		assert.Contains(t, (*calls)[0].options, `"skillId":"pptx"`)
	})

	t.Run("unpolicied primary refuses and the policied fallback serves", func(t *testing.T) {
		created, calls := build(t)
		require.NoError(t, generate(t, created, "grafana/mixed", pptx))
		require.Len(t, *calls, 1)
		assert.Equal(t, "backup-key", (*calls)[0].account)
		assert.Contains(t, (*calls)[0].options, `"skillId":"pptx"`)
	})

	t.Run("no candidate allows the skills", func(t *testing.T) {
		created, calls := build(t)
		err := generate(t, created, "grafana/plain", pptx)
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		assert.Equal(t, execution.CategoryInvalidRequest, execution.ClassifyProvider(err))
		assert.Empty(t, *calls)
	})

	t.Run("a policied fallback outage after a refusal is the real failure", func(t *testing.T) {
		created, calls := build(t)
		err := generate(t, created, "grafana/busy", pptx)
		require.Error(t, err)
		assert.Equal(t, execution.CategoryOverload, execution.ClassifyProvider(err))
		require.Len(t, *calls, 1)
		assert.Equal(t, "backup-key", (*calls)[0].account)
	})

	t.Run("an empty policy refuses every skill", func(t *testing.T) {
		created, calls := build(t)
		require.ErrorIs(t, generate(t, created, "grafana/empty", pptx), catalog.ErrUnsupportedRequest)
		assert.Empty(t, *calls)
	})

	t.Run("allowed skills with a container ID are refused", func(t *testing.T) {
		created, calls := build(t)
		require.ErrorIs(t, generate(t, created, "grafana/policied", pptxWithID), catalog.ErrUnsupportedRequest)
		assert.Empty(t, *calls)
	})
}

func TestBuildCatalog_ContainerMetadataIsReturned(t *testing.T) {
	var requests atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		body := `{"id":"msg_1","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1},"container":{"id":"container_from_response","expires_at":"2026-11-10T00:00:00Z"}}`
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
	})}
	file := testCatalogFile()
	created, err := BuildCatalog(file, map[string]config.ResolvedProvider{"anthropic-primary": {Type: "anthropic", APIKey: "secret", BaseURL: "https://anthropic.invalid"}}, client, identityModelFactory)
	require.NoError(t, err)
	resolved, err := created.ResolveModel(context.Background(), "grafana/assistant")
	require.NoError(t, err)
	result, err := resolved.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(64)})
	require.NoError(t, err)
	assert.Contains(t, string(result.ProviderMetadata["anthropic"]), `"container_from_response"`)
	require.Equal(t, int32(1), requests.Load())

	followUp := provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"container":{"id":"container_from_response"}}`)}}
	_, err = resolved.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("again")}, MaxOutputTokens: new(64), ProviderOptions: followUp})
	require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
	assert.Equal(t, int32(1), requests.Load())
}

func TestBuildCatalog_ConstructsImmutableCanonicalAndAliasModelsOnce(t *testing.T) {
	file := testCatalogFile()
	resolved := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "secret", BaseURL: "https://provider.example"},
	}
	calls := 0
	models := make(map[string]*catalogTestModel)
	created, err := buildCatalog(file, resolved, http.DefaultClient, func(account nativemodel.Config, modelID string, client *http.Client) provider.LanguageModel {
		calls++
		assert.Equal(t, "secret", account.APIKey)
		assert.Equal(t, "https://provider.example", account.BaseURL)
		assert.Same(t, http.DefaultClient, client)
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

func TestBuildCatalog_ModelFactoryReceivesCanonicalAndProtectedLowerOnce(t *testing.T) {
	file := testCatalogFile()
	resolved := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "secret"},
	}
	direct := make(map[string]provider.LanguageModel)
	constructed := 0
	factoryCalls := 0
	created, err := buildCatalog(file, resolved, http.DefaultClient, func(_ nativemodel.Config, modelID string, _ *http.Client) provider.LanguageModel {
		constructed++
		model := &catalogTestModel{id: modelID}
		direct[modelID] = model
		return model
	}, func(canonicalID string, lower provider.LanguageModel) (provider.LanguageModel, error) {
		factoryCalls++
		switch canonicalID {
		case "grafana/assistant":
			assert.NotEqual(t, direct["claude-assistant"], lower)
			assert.Equal(t, "claude-assistant", lower.ModelID())
		case "grafana/other":
			assert.NotEqual(t, direct["claude-other"], lower)
			assert.Equal(t, "claude-other", lower.ModelID())
		default:
			t.Fatalf("unexpected canonical ID %q", canonicalID)
		}
		skills := provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"container":{"skills":[{"type":"anthropic","skillId":"pptx"}]}}`)}}
		_, err := lower.DoGenerate(context.Background(), provider.CallOptions{ProviderOptions: skills})
		assert.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
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
		created, err := buildCatalog(file, resolved, http.DefaultClient, func(_ nativemodel.Config, modelID string, _ *http.Client) provider.LanguageModel {
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
		{"anthropic-primary": {Type: "unsupported", APIKey: "secret"}},
		{"anthropic-primary": {Type: "anthropic"}},
		{"anthropic-primary": {Type: "openai-compatible", APIKey: "secret"}},
	} {
		created, err := BuildCatalog(file, providers, http.DefaultClient, identityModelFactory)
		require.Error(t, err)
		assert.Nil(t, created)
	}
}

func TestBuildCatalog_FallbackConstructionFailsOpenWithoutPrivateSink(t *testing.T) {
	file := testCatalogFile()
	model := file.Models["grafana/assistant"]
	model.Fallback = []config.Primary{{Provider: "anthropic-primary", Model: "backup"}}
	file.Models["grafana/assistant"] = model
	calls := 0
	_, err := buildCatalog(file, map[string]config.ResolvedProvider{"anthropic-primary": {Type: "anthropic", APIKey: "secret"}}, http.DefaultClient, func(nativemodel.Config, string, *http.Client) provider.LanguageModel {
		calls++
		return &catalogTestModel{}
	}, identityModelFactory)
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestBuildCatalog_OpenAIUsesExplicitTransportAndIgnoresEnvironment(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "hostile-key")
	t.Setenv("OPENAI_BASE_URL", "https://hostile.example.test/v1")
	t.Setenv("OPENAI_ORG_ID", "hostile-org")
	t.Setenv("OPENAI_PROJECT_ID", "hostile-project")
	t.Setenv("OPENAI_CUSTOM_HEADERS", "X-Hostile: 1")
	var sent *http.Request
	var body []byte
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		sent = request
		body, _ = io.ReadAll(request.Body)
		return nil, fmt.Errorf("request not sent")
	})}
	file := config.File{
		Models: map[string]config.Model{"public": {Name: "Public", Primary: config.Primary{Provider: "openai-primary", Model: "backend-private"}}},
	}
	created, err := BuildCatalog(file, map[string]config.ResolvedProvider{
		"openai-primary": {Type: "openai", APIKey: "explicit-key"},
	}, client, identityModelFactory)
	require.NoError(t, err)
	resolved, err := created.ResolveModel(context.Background(), "public")
	require.NoError(t, err)
	_, err = resolved.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.Error(t, err)
	require.NotNil(t, sent)
	assert.Equal(t, "https://api.openai.com/v1/responses", sent.URL.String())
	assert.Equal(t, "Bearer explicit-key", sent.Header.Get("Authorization"))
	for _, header := range []string{"OpenAI-Organization", "OpenAI-Project", "X-Hostile"} {
		assert.Empty(t, sent.Header.Get(header), header)
	}
	assert.Contains(t, string(body), `"model":"backend-private"`)
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
	}, server.Client(), nativemodel.NewAnthropic, func(canonicalID string, model provider.LanguageModel) (provider.LanguageModel, error) {
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
