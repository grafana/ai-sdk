package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativemodel"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativeoptions"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
	"github.com/grafana/ai-sdk/provider"
)

// customEndpointCatalog has an Anthropic provider with a custom endpoint, an
// OpenAI provider on its native endpoint, and an openai-compatible provider.
func customEndpointCatalog() StackCatalog {
	anthropicURL, compatibleURL := "https://anthropic-proxy.example.com", "https://llm.example.com/v1"
	name, structured := "acme", true
	return StackCatalog{
		Name: "team-catalog",
		Spec: v1alpha1.Spec{
			Providers: map[string]v1alpha1.Provider{
				"claude": {Type: "anthropic", BaseURL: &anthropicURL},
				"gpt":    {Type: "openai"},
				"local":  {Type: "openai-compatible", BaseURL: &compatibleURL, ProviderName: &name, SupportsStructuredOutputs: &structured},
			},
			Models: map[string]v1alpha1.Model{
				"balanced": {
					Name:     "Balanced",
					Primary:  v1alpha1.Target{Provider: "claude", Model: "claude-sonnet"},
					Fallback: []v1alpha1.Target{{Provider: "gpt", Model: "gpt-large"}, {Provider: "local", Model: "llama"}},
				},
				"local-only": {Name: "Local", Primary: v1alpha1.Target{Provider: "local", Model: "llama"}},
			},
		},
		Keys: resource.DecryptedSecureValues{"claude": "claude-catalog-key", "gpt": "gpt-catalog-key", "local": "local-catalog-key"},
	}
}

// listedEndpoints lists the custom endpoints of customEndpointCatalog.
func listedEndpoints(t *testing.T) outbound.CustomEndpoints {
	t.Helper()
	endpoints, err := outbound.ParseCustomEndpoints("https://anthropic-proxy.example.com,https://llm.example.com/v1")
	require.NoError(t, err)
	return endpoints
}

// recordingClient records each request it receives and sends none.
type recordingClient struct {
	mu   sync.Mutex
	sent []*http.Request
}

func (recorder *recordingClient) client() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		recorder.mu.Lock()
		recorder.sent = append(recorder.sent, request)
		recorder.mu.Unlock()
		return nil, fmt.Errorf("request not sent")
	})}
}

func (recorder *recordingClient) requests() []*http.Request {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]*http.Request(nil), recorder.sent...)
}

func TestSelectStackCatalog_ListedEndpointsUseTheSharedClient(t *testing.T) {
	shared := &recordingClient{}
	options := StackCatalogOptions{Client: shared.client(), Endpoints: listedEndpoints(t)}
	for _, id := range []string{"balanced", "local-only"} {
		selected, err := SelectStackCatalog(customEndpointCatalog(), id, nil, options)
		require.NoError(t, err)
		_, err = selected.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
		require.Error(t, err)
	}

	sent := shared.requests()
	require.Len(t, sent, 4)
	assert.Equal(t, "https://anthropic-proxy.example.com/v1/messages", sent[0].URL.String())
	assert.Equal(t, "claude-catalog-key", sent[0].Header.Get("X-Api-Key"))
	assert.Equal(t, "https://api.openai.com/v1/responses", sent[1].URL.String())
	assert.Equal(t, "Bearer gpt-catalog-key", sent[1].Header.Get("Authorization"))
	assert.Equal(t, "https://llm.example.com/v1/chat/completions", sent[2].URL.String())
	assert.Equal(t, "Bearer local-catalog-key", sent[2].Header.Get("Authorization"))
	assert.Equal(t, sent[2].URL.String(), sent[3].URL.String())
}

func TestSelectStackCatalog_CompatibleProviderFields(t *testing.T) {
	builds := &recordedConfigs{}
	selected, err := selectStackCatalog(customEndpointCatalog(), "balanced", stackGateway(`{"only":["acme"]}`), listedEndpoints(t), builds.build)
	require.NoError(t, err)
	assert.Equal(t, "balanced", selected.ID)
	assert.Equal(t, []config.ResolvedProvider{{Type: "openai-compatible", APIKey: "local-catalog-key", BaseURL: "https://llm.example.com/v1", ProviderName: "acme", SupportsStructuredOutputs: true}}, builds.configs)

	_, err = selectStackCatalog(customEndpointCatalog(), "local-only", stackGateway(`{"only":["openai-compatible"]}`), listedEndpoints(t), builds.build)
	require.ErrorIs(t, err, execution.ErrNoRoutingCandidates, "the slug of a compatible provider with a name is its name")

	builds = &recordedConfigs{}
	_, err = selectStackCatalog(customEndpointCatalog(), "balanced", stackGateway(`{"order":["acme"]}`), listedEndpoints(t), builds.build)
	require.NoError(t, err)
	require.Len(t, builds.configs, 3)
	assert.Equal(t, "openai-compatible", builds.configs[0].Type)
}

func TestSelectStackCatalog_EndpointNoLongerListed(t *testing.T) {
	only, err := outbound.ParseCustomEndpoints("https://llm.example.com/v1")
	require.NoError(t, err)
	for name, endpoints := range map[string]outbound.CustomEndpoints{"empty list": {}, "entry removed": only} {
		t.Run(name, func(t *testing.T) {
			builds := &recordedConfigs{}
			_, err := selectStackCatalog(customEndpointCatalog(), "local-only", nil, endpoints, builds.build)
			require.ErrorIs(t, err, execution.ErrCatalogConfiguration)
			assert.Equal(t, execution.ErrCatalogConfiguration.Error(), err.Error())
			assert.Empty(t, builds.configs)
			_, err = StackCatalogLister{Endpoints: endpoints}.ListModels(WithStackCatalog(t.Context(), customEndpointCatalog()))
			require.ErrorIs(t, err, execution.ErrCatalogConfiguration)
		})
	}
}

func TestStackCatalogLister_CompatibleSlug(t *testing.T) {
	infos, err := StackCatalogLister{Endpoints: listedEndpoints(t)}.ListModels(WithStackCatalog(t.Context(), customEndpointCatalog()))
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, []catalog.ConfiguredCandidate{
		{ProviderInstance: "claude", Provider: "anthropic", ModelID: "claude-sonnet"},
		{ProviderInstance: "gpt", Provider: "openai", ModelID: "gpt-large"},
		{ProviderInstance: "local", Provider: "acme", ModelID: "llama"},
	}, infos[0].Candidates)
	encoded, err := json.Marshal(infos)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "example.com", "discovery names no endpoint")
}

// recordedConfigs is a candidate builder that records each provider
// configuration and returns a model that succeeds.
type recordedConfigs struct {
	configs []config.ResolvedProvider
}

func (builds *recordedConfigs) build(providerConfig config.ResolvedProvider, _ config.Primary) (provider.LanguageModel, error) {
	builds.configs = append(builds.configs, providerConfig)
	return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}}, nil
}

// TestCustomEndpoints_AcceptedEndpointIsTheDestination checks, for each
// provider SDK, that every endpoint the list accepts makes the SDK request a
// URL on the entry's origin and under its path, because the SDKs decode the
// base URL's path and resolve dot segments when they join their own path.
func TestCustomEndpoints_AcceptedEndpointIsTheDestination(t *testing.T) {
	const entry = "https://llm.corp.example/team"
	endpoints, err := outbound.ParseCustomEndpoints(entry)
	require.NoError(t, err)
	candidates := []string{
		"https://llm.corp.example/team", "https://LLM.corp.example/team/", "https://llm.corp.example/team/sub",
		"https://llm.corp.example/team/%252e%252e/admin", "https://llm.corp.example/team/a;b", "https://llm.corp.example/team//sub",
		"https://llm.corp.example/team/..%2fadmin", "https://llm.corp.example/team/%2e%2e%2fadmin", "https://llm.corp.example/team/..%5cadmin",
		"https://llm.corp.example/team/x%2F..%2F..%2Fadmin", "https://llm.corp.example/team/a%2Fb", "https://llm.corp.example/team/../admin",
		"https://llm.corp.example:/team", "https://llm.corp.example/team/%2E",
	}
	accepted := 0
	for _, candidate := range candidates {
		checked, err := endpoints.Check(candidate, "")
		if err != nil {
			continue
		}
		accepted++
		for _, providerType := range []string{"anthropic", "openai", "openai-compatible"} {
			t.Run(candidate+"/"+providerType, func(t *testing.T) {
				sent := &recordingClient{}
				model, _, err := newCandidate(config.ResolvedProvider{Type: providerType, APIKey: "key", BaseURL: checked}, config.Primary{Provider: "custom", Model: "model"}, nativeoptions.AnthropicPolicy{}, sent.client(), nativemodel.NewAnthropic)
				require.NoError(t, err)
				_, _ = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
				requests := sent.requests()
				require.Len(t, requests, 1)
				destination := requests[0].URL
				assert.Equal(t, "https", destination.Scheme)
				assert.Equal(t, "llm.corp.example", strings.ToLower(destination.Hostname()))
				assert.Contains(t, []string{"", "443"}, destination.Port())
				assert.True(t, strings.HasPrefix(destination.EscapedPath(), "/team/"), destination.String())
				assert.NotContains(t, destination.Path, "/admin")
			})
		}
	}
	assert.Equal(t, 6, accepted, "the accepted candidates")
}
