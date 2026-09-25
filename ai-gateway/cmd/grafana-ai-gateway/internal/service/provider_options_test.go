package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/provider"
	anthropicprovider "github.com/grafana/ai-sdk/providers/anthropic"
	openaiprovider "github.com/grafana/ai-sdk/providers/openai"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every typed field providers/anthropic reads must be classified: forwarded by
// anthropicOptionPolicy, or refused by the runtime before resolution. A field
// the provider adds fails here until someone decides which it is.
func TestAnthropicOptionPolicy_ClassifiesEveryTypedField(t *testing.T) {
	refusedByRuntime := []string{"mcpServers", "container", "fallbacks"}
	allowed := anthropicOptionPolicy.Fields["anthropic"]
	for _, typed := range []any{anthropicprovider.AnthropicOptions{}, anthropicprovider.AnthropicSystemMessageOptions{}} {
		kind := reflect.TypeOf(typed)
		for i := range kind.NumField() {
			name, _, _ := strings.Cut(kind.Field(i).Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			assert.True(t, slices.Contains(allowed, name) || slices.Contains(refusedByRuntime, name),
				"%s.%s is read by providers/anthropic but neither forwarded nor refused", kind.Name(), name)
		}
	}
	for _, refused := range refusedByRuntime {
		assert.NotContains(t, allowed, refused, "a field the runtime refuses is not also forwarded")
	}
}

// The policy mirrors unexported provider helpers, so check it against the
// provider itself. Candidates are listed independently of the policy, so a
// namespace the provider reads but the policy omits fails, as does the reverse.
func TestOpenAICompatibleOptionPolicy_MatchesTheNamespacesTheProviderReads(t *testing.T) {
	for providerName, candidates := range map[string][]string{
		"":             {"openai-compatible", "openaiCompatible", "openai", "anthropic"},
		"ollama":       {"openai-compatible", "openaiCompatible", "ollama", "Ollama", "openai"},
		"my-vllm.chat": {"openai-compatible", "openaiCompatible", "my-vllm", "myVllm", "my-vllm.chat", "chat", "openai"},
		" acme_ai ":    {"openai-compatible", "openaiCompatible", "acme_ai", "acmeAi", " acme_ai ", "openai"},
	} {
		policy := openAICompatibleOptionPolicy(providerName)
		for _, namespace := range candidates {
			forwarded := slices.Contains(policy.Namespaces, namespace)
			t.Run(providerName+"/"+namespace, func(t *testing.T) {
				var body map[string]any
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":"1","model":"m","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
				}))
				defer server.Close()
				model := openaicompatible.New("m", openaicompatible.WithBaseURL(server.URL), openaicompatible.WithProviderName(providerName))
				_, err := model.DoGenerate(context.Background(), provider.CallOptions{
					Prompt: []provider.Message{provider.UserText("hi")},
					ProviderOptions: provider.ProviderOptions{
						namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(`{"user":"marker"}`)},
					},
				})
				require.NoError(t, err)
				assert.Equal(t, forwarded, body["user"] == "marker",
					"policy forwards %q: %v; provider reads it: %v", namespace, forwarded, body["user"] == "marker")
			})
		}
	}
}

func TestBuildCatalog_SetsTheProviderOptionPolicyOfEachBackend(t *testing.T) {
	file := config.File{Models: map[string]config.Model{
		"claude": {Name: "Claude", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude-backend"}},
		"local":  {Name: "Local", Primary: config.Primary{Provider: "compatible", Model: "local-backend"}},
	}}
	created, err := buildCatalog(file, map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "key"},
		"compatible":        {Type: "openai-compatible", APIKey: "key", BaseURL: "http://127.0.0.1:1/v1", ProviderName: "ollama"},
	}, http.DefaultClient, anthropicprovider.New, identityModelFactory)
	require.NoError(t, err)

	for id, want := range map[string]catalog.ProviderOptionPolicy{
		"claude": directAnthropicOptionPolicy(),
		"local":  {Namespaces: []string{"openai-compatible", "openaiCompatible", "ollama"}},
	} {
		resolved, err := created.ResolveModel(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, want, resolved.ProviderOptions, id)
	}
}

func TestBuildCatalog_MixedProviderFallbackForwardsNoOptions(t *testing.T) {
	// One model, two provider types: no single policy is safe for both attempts,
	// so the model forwards no caller provider options.
	file := config.File{Models: map[string]config.Model{
		"mixed": {
			Name:     "Mixed",
			Primary:  config.Primary{Provider: "anthropic-primary", Model: "claude-backend"},
			Fallback: []config.Primary{{Provider: "compatible", Model: "local-backend"}},
		},
		"single": {Name: "Single", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude-backend"}},
	}}
	created, err := buildCatalog(file, map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "key"},
		"compatible":        {Type: "openai-compatible", APIKey: "key", BaseURL: "http://127.0.0.1:1/v1", ProviderName: "ollama"},
	}, http.DefaultClient, anthropicprovider.New, identityModelFactory)
	require.NoError(t, err)

	mixed, err := created.ResolveModel(context.Background(), "mixed")
	require.NoError(t, err)
	assert.Equal(t, catalog.ProviderOptionPolicy{}, mixed.ProviderOptions, "a mixed-provider model forwards nothing")

	single, err := created.ResolveModel(context.Background(), "single")
	require.NoError(t, err)
	assert.Equal(t, directAnthropicOptionPolicy(), single.ProviderOptions, "a single-provider model keeps its policy")
}

func TestOpenAIOptionPolicy_ClassifiesEveryTypedField(t *testing.T) {
	allowed := openAIOptionPolicy.Fields["openai"]
	for _, typed := range []any{openaiprovider.OpenAIResponsesOptions{}, openaiprovider.OpenAIPartOptions{}} {
		kind := reflect.TypeOf(typed)
		for i := range kind.NumField() {
			name, _, _ := strings.Cut(kind.Field(i).Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			assert.Contains(t, allowed, name, "%s.%s is read by providers/openai but not forwarded", kind.Name(), name)
		}
	}
}
