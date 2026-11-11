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
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/modelcatalog/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/outbound"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

func testStackCatalog() StackCatalog {
	return StackCatalog{
		Name: "team-catalog",
		Spec: v1alpha1.Spec{
			Providers: map[string]v1alpha1.Provider{
				"claude": {Type: "anthropic"},
				"gpt":    {Type: "openai"},
			},
			Models: map[string]v1alpha1.Model{
				"balanced": {
					Name:     "Balanced",
					Aliases:  []string{"default"},
					Primary:  v1alpha1.Target{Provider: "claude", Model: "claude-sonnet"},
					Fallback: []v1alpha1.Target{{Provider: "gpt", Model: "gpt-large"}},
				},
				"fast": {Name: "Fast", Primary: v1alpha1.Target{Provider: "gpt", Model: "gpt-small"}},
				"same": {Name: "Same", Primary: v1alpha1.Target{Provider: "claude", Model: "claude-sonnet"}},
			},
		},
		Keys: resource.DecryptedSecureValues{"claude": "claude-catalog-key", "gpt": "gpt-catalog-key"},
	}
}

// stackBuilds is a candidate builder that records each built candidate and
// returns a fake model for it, failing for the destinations in fail.
type stackBuilds struct {
	mu    sync.Mutex
	built []string
	fail  map[string]bool
	keys  map[string]string
}

func (builds *stackBuilds) build(providerConfig config.ResolvedProvider, descriptor config.Primary) (provider.LanguageModel, error) {
	destination := descriptor.Provider + "/" + descriptor.Model
	builds.mu.Lock()
	builds.built = append(builds.built, destination)
	if builds.keys == nil {
		builds.keys = map[string]string{}
	}
	builds.keys[destination] = providerConfig.APIKey
	builds.mu.Unlock()
	return &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		if builds.fail[destination] {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "unavailable " + providerConfig.APIKey, IsRetryable: new(true)})
		}
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}}, nil
}

// withEndpoint returns the test catalog with provider name of type
// providerType at endpoint.
func withEndpoint(name, providerType, endpoint string) StackCatalog {
	stack := testStackCatalog()
	stack.Spec.Providers[name] = v1alpha1.Provider{Type: providerType, BaseURL: &endpoint}
	return stack
}

func stackGateway(controls string) json.RawMessage {
	if controls == "" {
		return nil
	}
	return json.RawMessage(controls)
}

func TestSelectStackCatalog_BuildsOnlyTheChain(t *testing.T) {
	many := testStackCatalog()
	for i := range 50 {
		many.Spec.Models[fmt.Sprintf("model-%d", i)] = v1alpha1.Model{Name: "Other", Primary: v1alpha1.Target{Provider: "claude", Model: fmt.Sprintf("other-%d", i)}}
	}
	for _, tc := range []struct {
		name      string
		catalog   StackCatalog
		modelID   string
		gateway   string
		canonical string
		built     []string
	}{
		{name: "requested model", modelID: "balanced", canonical: "balanced", built: []string{"claude/claude-sonnet", "gpt/gpt-large"}},
		{name: "alias", modelID: "default", canonical: "balanced", built: []string{"claude/claude-sonnet", "gpt/gpt-large"}},
		{name: "models entry", modelID: "fast", gateway: `{"models":["balanced"]}`, canonical: "fast", built: []string{"gpt/gpt-small", "claude/claude-sonnet", "gpt/gpt-large"}},
		{name: "only", modelID: "balanced", gateway: `{"only":["openai"]}`, canonical: "balanced", built: []string{"gpt/gpt-large"}},
		{name: "order", modelID: "balanced", gateway: `{"order":["openai"]}`, canonical: "balanced", built: []string{"gpt/gpt-large", "claude/claude-sonnet"}},
		{name: "repeated destination", modelID: "balanced", gateway: `{"models":["same","default"]}`, canonical: "balanced", built: []string{"claude/claude-sonnet", "gpt/gpt-large"}},
		{name: "large catalog", catalog: many, modelID: "balanced", canonical: "balanced", built: []string{"claude/claude-sonnet", "gpt/gpt-large"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stack := tc.catalog
			if stack.Name == "" {
				stack = testStackCatalog()
			}
			builds := &stackBuilds{}
			selected, err := selectStackCatalog(stack, tc.modelID, stackGateway(tc.gateway), outbound.EgressPolicy{}, builds.build)
			require.NoError(t, err)
			assert.Equal(t, tc.built, builds.built)
			assert.Equal(t, tc.canonical, selected.ID)
			assert.Equal(t, execution.SourceStackCatalog, selected.Source)
			assert.Equal(t, "team-catalog", selected.Catalog)
			assert.Equal(t, "grafana", selected.Model.Provider())
			assert.Equal(t, tc.canonical, selected.Model.ModelID())
			for destination, key := range builds.keys {
				assert.Equal(t, strings.SplitN(destination, "/", 2)[0]+"-catalog-key", key)
			}
		})
	}
}

func TestSelectStackCatalog_Refusals(t *testing.T) {
	withSpec := func(mutate func(*v1alpha1.Spec)) StackCatalog {
		stack := testStackCatalog()
		mutate(&stack.Spec)
		return stack
	}
	withKeys := func(keys resource.DecryptedSecureValues) StackCatalog {
		stack := testStackCatalog()
		stack.Keys = keys
		return stack
	}
	nineCandidates := withSpec(func(spec *v1alpha1.Spec) {
		for i := range 9 {
			spec.Models[fmt.Sprintf("single-%d", i)] = v1alpha1.Model{Name: "Single", Primary: v1alpha1.Target{Provider: "claude", Model: fmt.Sprintf("single-%d", i)}}
		}
	})
	tooManyFallbacks := withSpec(func(spec *v1alpha1.Spec) {
		model := spec.Models["balanced"]
		for i := range 8 {
			model.Fallback = append(model.Fallback, v1alpha1.Target{Provider: "claude", Model: fmt.Sprintf("fallback-%d", i)})
		}
		spec.Models["balanced"] = model
	})
	for _, tc := range []struct {
		name    string
		catalog StackCatalog
		modelID string
		gateway string
		err     error
	}{
		{name: "byok accounts", gateway: `{"byok":{"anthropic":[{"apiKey":"request-key"}]}}`, err: execution.ErrUnsupportedGatewayControl},
		{name: "unknown control", gateway: `{"sort":"cost"}`, err: execution.ErrUnsupportedGatewayControl},
		{name: "byok before an invalid catalog", catalog: withKeys(nil), gateway: `{"byok":{}}`, err: execution.ErrUnsupportedGatewayControl},
		{name: "malformed controls", gateway: `{"models":"balanced"}`, err: execution.ErrRoutingControls},
		{name: "unknown model", modelID: "missing", err: catalog.ErrUnknownModel},
		{name: "unknown models entry", gateway: `{"models":["missing"]}`, err: execution.ErrUnknownFallbackModel},
		{name: "only excludes every candidate", gateway: `{"only":["other"]}`, err: execution.ErrNoRoutingCandidates},
		{name: "composed chain above the bound", catalog: nineCandidates, modelID: "single-0", gateway: `{"models":["single-1","single-2","single-3","single-4","single-5","single-6","single-7","single-8"]}`, err: execution.ErrRoutingAttemptBound},
		{name: "unknown provider type", catalog: withSpec(func(spec *v1alpha1.Spec) { spec.Providers["gpt"] = v1alpha1.Provider{Type: "bedrock"} }), err: execution.ErrCatalogConfiguration},
		{name: "compatible provider without an endpoint", catalog: withSpec(func(spec *v1alpha1.Spec) { spec.Providers["gpt"] = v1alpha1.Provider{Type: "openai-compatible"} }), err: execution.ErrCatalogConfiguration},
		{name: "plain HTTP endpoint", catalog: withEndpoint("gpt", "openai", "http://llm.example.com"), err: execution.ErrCatalogConfiguration},
		{name: "endpoint the policy refuses", catalog: withEndpoint("gpt", "openai", "https://169.254.169.254"), err: execution.ErrCatalogConfiguration},
		{name: "alias collision", catalog: withSpec(func(spec *v1alpha1.Spec) {
			model := spec.Models["fast"]
			model.Aliases = []string{"default"}
			spec.Models["fast"] = model
		}), err: execution.ErrCatalogConfiguration},
		{name: "model above the attempt bound", catalog: tooManyFallbacks, err: execution.ErrCatalogConfiguration},
		{name: "missing key", catalog: withKeys(resource.DecryptedSecureValues{"claude": "claude-catalog-key"}), err: execution.ErrCatalogConfiguration},
		{name: "empty key", catalog: withKeys(resource.DecryptedSecureValues{"claude": "claude-catalog-key", "gpt": ""}), err: execution.ErrCatalogConfiguration},
		{name: "key without provider", catalog: withKeys(resource.DecryptedSecureValues{"claude": "claude-catalog-key", "gpt": "gpt-catalog-key", "other": "other-key"}), err: execution.ErrCatalogConfiguration},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stack := tc.catalog
			if stack.Name == "" {
				stack = testStackCatalog()
			}
			modelID := tc.modelID
			if modelID == "" {
				modelID = "balanced"
			}
			builds := &stackBuilds{}
			_, err := selectStackCatalog(stack, modelID, stackGateway(tc.gateway), outbound.EgressPolicy{}, builds.build)
			require.ErrorIs(t, err, tc.err)
			assert.Empty(t, builds.built)
			if tc.err == execution.ErrCatalogConfiguration {
				assert.Equal(t, execution.ErrCatalogConfiguration.Error(), err.Error())
			}
		})
	}
}

func TestSelectStackCatalog_FallbackAndOverview(t *testing.T) {
	builds := &stackBuilds{fail: map[string]bool{"claude/claude-sonnet": true}}
	runtime, err := execution.New(execution.Config{Limits: serviceTestLimits(), Selector: func(_ context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (execution.Selection, error) {
		return selectStackCatalog(testStackCatalog(), id, gateway, outbound.EgressPolicy{}, builds.build)
	}})
	require.NoError(t, err)
	call, err := runtime.Call(t.Context(), "default", provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}})
	require.NoError(t, err)
	_, err = call.Generate()
	require.NoError(t, err)

	overview := publishedOverview(t, call.View(nil))
	require.NotNil(t, overview)
	assert.Equal(t, "default", overview.RequestedModelID)
	assert.Equal(t, "balanced", overview.CanonicalModelID)
	require.Len(t, overview.Attempts, 2)
	assert.Equal(t, execution.Attempt{Provider: "anthropic", ModelID: "claude-sonnet", ProviderInstance: "claude", Outcome: fallback.AttemptFailed, Error: overview.Attempts[0].Error}, overview.Attempts[0])
	assert.Equal(t, "unavailable [REDACTED]", overview.Attempts[0].Error.Message)
	assert.Equal(t, execution.Attempt{Provider: "openai", ModelID: "gpt-large", ProviderInstance: "gpt", Outcome: fallback.AttemptSelected}, overview.Attempts[1])
}

func TestSelectStackCatalog_NativeEndpointsWithCatalogKeys(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "ambient-key")
	t.Setenv("ANTHROPIC_BASE_URL", "https://ambient.example.test")
	t.Setenv("OPENAI_API_KEY", "ambient-key")
	t.Setenv("OPENAI_BASE_URL", "https://ambient.example.test/v1")
	var mu sync.Mutex
	var sent []*http.Request
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		sent = append(sent, request)
		mu.Unlock()
		return nil, fmt.Errorf("request not sent")
	})}
	for _, modelID := range []string{"same", "fast"} {
		selected, err := SelectStackCatalog(testStackCatalog(), modelID, nil, StackCatalogOptions{Client: client, EgressClient: http.DefaultClient})
		require.NoError(t, err)
		_, err = selected.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
		require.ErrorContains(t, err, "request not sent")
	}
	require.Len(t, sent, 2)
	assert.Equal(t, "https://api.anthropic.com/v1/messages", sent[0].URL.String())
	assert.Equal(t, "claude-catalog-key", sent[0].Header.Get("X-Api-Key"))
	assert.Equal(t, "https://api.openai.com/v1/responses", sent[1].URL.String())
	assert.Equal(t, "Bearer gpt-catalog-key", sent[1].Header.Get("Authorization"))

	_, err := SelectStackCatalog(testStackCatalog(), "same", nil, StackCatalogOptions{})
	require.Error(t, err)
	_, err = SelectStackCatalog(testStackCatalog(), "same", nil, StackCatalogOptions{Client: client})
	require.Error(t, err)
}

func TestSelectStackCatalog_ConcurrentCatalogsKeepTheirKeys(t *testing.T) {
	var mu sync.Mutex
	sent := map[string][]string{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		sent[request.Header.Get("X-Api-Key")] = append(sent[request.Header.Get("X-Api-Key")], request.URL.Host)
		mu.Unlock()
		return nil, fmt.Errorf("request not sent")
	})}
	const calls = 16
	var wg sync.WaitGroup
	for i := range calls {
		wg.Go(func() {
			stack := testStackCatalog()
			stack.Keys = resource.DecryptedSecureValues{"claude": resource.RawSecureValue(fmt.Sprintf("catalog-%d-key", i%2)), "gpt": "gpt-catalog-key"}
			selected, err := SelectStackCatalog(stack, "same", nil, StackCatalogOptions{Client: client, EgressClient: http.DefaultClient})
			if !assert.NoError(t, err) {
				return
			}
			_, err = selected.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16)})
			assert.ErrorContains(t, err, "request not sent")
		})
	}
	wg.Wait()
	assert.Len(t, sent, 2)
	assert.Len(t, sent["catalog-0-key"], calls/2)
	assert.Len(t, sent["catalog-1-key"], calls/2)
}

func TestNewSelector_CatalogAccess(t *testing.T) {
	operator := routedFakeCatalog(t, ConfiguredIdentity, nil, map[string]*observabilityTestModel{"backend-primary": {}, "backend-secondary": {}})
	selector := NewSelector(operator, nil, &StackCatalogOptions{Client: http.DefaultClient, EgressClient: http.DefaultClient})
	ctx := callerContext(t, gatewayauth.ConfiguredAccounts)
	catalogCtx := callerContext(t, gatewayauth.CatalogAccounts)
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		modelID string
		gateway string
		err     error
		source  execution.Source
	}{
		{name: "stack catalog model", ctx: WithStackCatalog(catalogCtx, testStackCatalog()), modelID: "default", source: execution.SourceStackCatalog},
		{name: "operator model is not in the stack catalog", ctx: WithStackCatalog(catalogCtx, testStackCatalog()), modelID: "public", err: catalog.ErrUnknownModel},
		{name: "byok refused", ctx: WithStackCatalog(catalogCtx, testStackCatalog()), modelID: "default", gateway: `{"byok":{"anthropic":[{"apiKey":"request-key"}]}}`, err: execution.ErrUnsupportedGatewayControl},
		{name: "no catalog carried", ctx: catalogCtx, modelID: "default", err: execution.ErrAccountAccess},
		{name: "configured caller ignores a carried catalog", ctx: WithStackCatalog(ctx, testStackCatalog()), modelID: "public", source: execution.SourceOperatorCatalog},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := selector(tc.ctx, tc.modelID, provider.CallOptions{}, stackGateway(tc.gateway))
			if tc.err != nil {
				require.ErrorIs(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.source, selected.Source)
		})
	}
	t.Run("without stack-catalog options", func(t *testing.T) {
		_, err := NewSelector(operator, nil, nil)(WithStackCatalog(catalogCtx, testStackCatalog()), "default", provider.CallOptions{}, nil)
		require.ErrorIs(t, err, execution.ErrAccountAccess)
	})
}

func TestStackCatalogLister(t *testing.T) {
	t.Run("rows", func(t *testing.T) {
		infos, err := StackCatalogLister{}.ListModels(WithStackCatalog(t.Context(), testStackCatalog()))
		require.NoError(t, err)
		assert.Equal(t, []catalog.ModelInfo{
			{ID: "balanced", Name: "Balanced", Aliases: []string{"default"}, Candidates: []catalog.ConfiguredCandidate{
				{ProviderInstance: "claude", Provider: "anthropic", ModelID: "claude-sonnet"},
				{ProviderInstance: "gpt", Provider: "openai", ModelID: "gpt-large"},
			}},
			{ID: "fast", Name: "Fast", Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "gpt", Provider: "openai", ModelID: "gpt-small"}}},
			{ID: "same", Name: "Same", Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "claude", Provider: "anthropic", ModelID: "claude-sonnet"}}},
		}, infos)
		encoded, err := json.Marshal(infos)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "catalog-key")
	})
	t.Run("catalog that fails its checks", func(t *testing.T) {
		stack := testStackCatalog()
		stack.Keys = resource.DecryptedSecureValues{"claude": "claude-catalog-key"}
		_, err := StackCatalogLister{}.ListModels(WithStackCatalog(t.Context(), stack))
		require.ErrorIs(t, err, execution.ErrCatalogConfiguration)
		assert.Equal(t, execution.ErrCatalogConfiguration.Error(), err.Error())
	})
	t.Run("undecoded parent", func(t *testing.T) {
		_, err := StackCatalogLister{}.ListModels(WithStackCatalog(t.Context(), StackCatalog{Name: "team"}))
		require.ErrorIs(t, err, execution.ErrCatalogConfiguration)
	})
	t.Run("no carried catalog", func(t *testing.T) {
		_, err := StackCatalogLister{}.ListModels(t.Context())
		require.Error(t, err)
		assert.NotErrorIs(t, err, execution.ErrCatalogConfiguration)
	})
}

func TestSelectStackCatalog_RefusesOperatorPolicedAnthropicOptions(t *testing.T) {
	for name, raw := range map[string]string{
		"container skills":             `{"container":{"skills":[{"type":"anthropic","skillId":"pptx"}]}}`,
		"server-side fallback chain":   `{"fallbacks":[{"model":"claude-backup"}]}`,
		"default server-side fallback": `{"fallbacks":"default"}`,
	} {
		for endpoint, stack := range map[string]StackCatalog{"native endpoint": testStackCatalog(), "custom endpoint": withEndpoint("claude", "anthropic", "https://anthropic-proxy.example.com")} {
			t.Run(name+"/"+endpoint, func(t *testing.T) {
				var sent int
				client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					sent++
					return nil, fmt.Errorf("request not sent")
				})}
				selected, err := SelectStackCatalog(stack, "same", nil, StackCatalogOptions{Client: client, EgressClient: client})
				require.NoError(t, err)
				options := provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(raw)}}
				_, err = selected.Model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: new(16), ProviderOptions: options})
				require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
				assert.Zero(t, sent)
			})
		}
	}
}
