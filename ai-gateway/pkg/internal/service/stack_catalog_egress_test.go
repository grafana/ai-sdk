package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"sync"
	"testing"
	"time"

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
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

var addressLiteral = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

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

func TestSelectStackCatalog_CustomEndpointsUseTheEgressClient(t *testing.T) {
	shared, egress := &recordingClient{}, &recordingClient{}
	options := StackCatalogOptions{Clients: outbound.ModelClients{Shared: shared.client(), Egress: egress.client()}}
	for _, id := range []string{"balanced", "local-only"} {
		selected, err := SelectStackCatalog(customEndpointCatalog(), id, nil, options)
		require.NoError(t, err)
		_, err = selected.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
		require.Error(t, err)
	}

	sent := egress.requests()
	require.Len(t, sent, 3)
	assert.Equal(t, "https://anthropic-proxy.example.com/v1/messages", sent[0].URL.String())
	assert.Equal(t, "claude-catalog-key", sent[0].Header.Get("X-Api-Key"))
	assert.Equal(t, "https://llm.example.com/v1/chat/completions", sent[1].URL.String())
	assert.Equal(t, "Bearer local-catalog-key", sent[1].Header.Get("Authorization"))
	assert.Equal(t, sent[1].URL.String(), sent[2].URL.String())
	native := shared.requests()
	require.Len(t, native, 1)
	assert.Equal(t, "https://api.openai.com/v1/responses", native[0].URL.String())
	assert.Equal(t, "Bearer gpt-catalog-key", native[0].Header.Get("Authorization"))
}

func TestSelectStackCatalog_NativeDefaultUsesTheSharedClient(t *testing.T) {
	shared, egress := &recordingClient{}, &recordingClient{}
	native := "https://api.anthropic.com"
	stack := customEndpointCatalog()
	stack.Spec.Providers["claude"] = v1alpha1.Provider{Type: "anthropic", BaseURL: &native}
	selected, err := SelectStackCatalog(stack, "balanced", stackGateway(`{"only":["anthropic"]}`), StackCatalogOptions{Clients: outbound.ModelClients{Shared: shared.client(), Egress: egress.client()}})
	require.NoError(t, err)
	_, err = selected.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
	require.Error(t, err)
	require.Len(t, shared.requests(), 1)
	assert.Equal(t, "https://api.anthropic.com/v1/messages", shared.requests()[0].URL.String())
	assert.Empty(t, egress.requests())
}

func TestSelectStackCatalog_CompatibleProviderFields(t *testing.T) {
	builds := &recordedConfigs{}
	selected, err := selectStackCatalog(customEndpointCatalog(), "balanced", stackGateway(`{"only":["acme"]}`), outbound.EgressPolicy{}, builds.build)
	require.NoError(t, err)
	assert.Equal(t, "balanced", selected.ID)
	assert.Equal(t, []config.ResolvedProvider{{Type: "openai-compatible", APIKey: "local-catalog-key", BaseURL: "https://llm.example.com/v1", ProviderName: "acme", SupportsStructuredOutputs: true}}, builds.configs)

	_, err = selectStackCatalog(customEndpointCatalog(), "local-only", stackGateway(`{"only":["openai-compatible"]}`), outbound.EgressPolicy{}, builds.build)
	require.ErrorIs(t, err, execution.ErrNoRoutingCandidates, "the slug of a compatible provider with a name is its name")

	builds = &recordedConfigs{}
	_, err = selectStackCatalog(customEndpointCatalog(), "balanced", stackGateway(`{"order":["acme"]}`), outbound.EgressPolicy{}, builds.build)
	require.NoError(t, err)
	require.Len(t, builds.configs, 3)
	assert.Equal(t, "openai-compatible", builds.configs[0].Type)
}

func TestSelectStackCatalog_StricterPolicy(t *testing.T) {
	deny, err := outbound.ParseEgressList("*.example.com")
	require.NoError(t, err)
	builds := &recordedConfigs{}
	_, err = selectStackCatalog(customEndpointCatalog(), "balanced", nil, outbound.EgressPolicy{Deny: deny}, builds.build)
	require.ErrorIs(t, err, execution.ErrCatalogConfiguration)
	assert.Equal(t, execution.ErrCatalogConfiguration.Error(), err.Error())
	assert.Empty(t, builds.configs)
}

func TestStackCatalogLister_CompatibleSlug(t *testing.T) {
	infos, err := StackCatalogLister{}.ListModels(WithStackCatalog(t.Context(), customEndpointCatalog()))
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

func TestSelectStackCatalog_EgressErrorsAreBare(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := "https://" + listener.Addr().String() + "/v1"
	require.NoError(t, listener.Close())
	allowLoopback, err := outbound.ParseEgressList("127.0.0.1")
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		policy outbound.EgressPolicy
		want   error
	}{
		{name: "refused", want: execution.ErrEgressRefused},
		{name: "connection failed", policy: outbound.EgressPolicy{Allow: allowLoopback}, want: execution.ErrEgressConnection},
	} {
		client, err := outbound.NewEgressClient(time.Second, 1024, tc.policy)
		require.NoError(t, err)
		for _, providerType := range []string{"anthropic", "openai", "openai-compatible"} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, providerType, streaming), func(t *testing.T) {
					providerConfig := config.ResolvedProvider{Type: providerType, APIKey: "private-key", BaseURL: closed}
					raw, _, err := newCandidate(providerConfig, config.Primary{Provider: "custom", Model: "model"}, nativeoptions.AnthropicPolicy{}, client, nativemodel.NewAnthropic)
					require.NoError(t, err)
					call := func(model provider.LanguageModel) error {
						options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)}
						if streaming {
							_, err := model.DoStream(context.Background(), options)
							return err
						}
						_, err := model.DoGenerate(context.Background(), options)
						return err
					}
					require.ErrorIs(t, call(raw), tc.want, "the provider keeps the sentinel in its error chain")
					assert.Equal(t, tc.want, call(outbound.EgressModel(raw)), "the candidate drops the request URL")
				})
			}
		}
	}
}

func TestSelectStackCatalog_RefusedPrimaryFallsBack(t *testing.T) {
	builds := &recordedConfigs{fail: map[string]error{
		"claude": fmt.Errorf(`openai: HTTP request failed: Post "https://private-host.example.com:8443/v1": %w`, execution.ErrEgressRefused),
	}}
	runtime, err := execution.New(execution.Config{Limits: serviceTestLimits(), Selector: func(_ context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		return selectStackCatalog(customEndpointCatalog(), id, gateway, outbound.EgressPolicy{}, builds.build)
	}})
	require.NoError(t, err)
	call, err := runtime.Call(t.Context(), "balanced", provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	_, err = call.Generate()
	require.NoError(t, err)

	overview := publishedOverview(t, call.View(nil))
	require.NotNil(t, overview)
	require.Len(t, overview.Attempts, 2)
	assert.Equal(t, fallback.AttemptFailed, overview.Attempts[0].Outcome)
	require.NotNil(t, overview.Attempts[0].Error)
	assert.Equal(t, execution.ErrEgressRefused.Error(), overview.Attempts[0].Error.Message)
	assert.Equal(t, fallback.AttemptSelected, overview.Attempts[1].Outcome)
	encoded, err := json.Marshal(overview)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-host")
	assert.Empty(t, addressLiteral.FindAllString(string(encoded), -1))
}

// recordedConfigs is a candidate builder that records each provider
// configuration. A provider in fail gets a model whose calls fail with that
// error, wrapped like an egress candidate when it has a custom endpoint.
type recordedConfigs struct {
	configs []config.ResolvedProvider
	fail    map[string]error
}

func (builds *recordedConfigs) build(providerConfig config.ResolvedProvider, descriptor config.Primary) (provider.LanguageModel, error) {
	builds.configs = append(builds.configs, providerConfig)
	failure := builds.fail[descriptor.Provider]
	model := &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		if failure != nil {
			return nil, failure
		}
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}}
	if providerConfig.BaseURL != "" {
		return outbound.EgressModel(model), nil
	}
	return model, nil
}
