package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
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

func TestOpenAICompatibleOptionPolicy_NativeScopesAndPrecedence(t *testing.T) {
	for _, streaming := range []string{"false", "true"} {
		t.Run(streaming, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"message":"native scope audit","type":"invalid_request_error"}}`))
			}))
			defer server.Close()
			created, err := buildCatalog(config.File{Models: map[string]config.Model{"local": {Name: "Local", Primary: config.Primary{Provider: "backend", Model: "model"}}}}, map[string]config.ResolvedProvider{"backend": {Type: "openai-compatible", APIKey: "key", BaseURL: server.URL, ProviderName: "my-vllm.chat"}}, server.Client(), anthropicprovider.New, identityModelFactory)
			require.NoError(t, err)
			handler, err := providerv4.New(providerv4.Config{Resolver: created, Limits: serviceTestLimits()})
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"hello","providerOptions":{"openaiCompatible":{"partMarker":"part"},"myVllm":{"ignoredPart":"ignored"}}},{"type":"text","text":"second"}],"providerOptions":{"openaiCompatible":{"messageMarker":"message"},"myVllm":{"ignoredMessage":"ignored"}}}],"providerOptions":{"openai-compatible":{"user":"deprecated","ignoredCall":"ignored"},"openaiCompatible":{"user":"fixed"},"my-vllm":{"user":"raw","extension":"raw"},"myVllm":{"user":"camel","extension":"camel"}}}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
			request.Header.Set(providerv4.HeaderModelID, "local")
			request.Header.Set(providerv4.HeaderStreaming, streaming)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			assert.Equal(t, "camel", body["user"])
			assert.Equal(t, "camel", body["extension"])
			assert.NotContains(t, body, "ignoredCall")
			messages := body["messages"].([]any)
			message := messages[0].(map[string]any)
			assert.Equal(t, "message", message["messageMarker"])
			assert.NotContains(t, message, "ignoredMessage")
			part := message["content"].([]any)[0].(map[string]any)
			assert.Equal(t, "part", part["partMarker"])
			assert.NotContains(t, part, "ignoredPart")
		})
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
		"claude": anthropicOptionPolicy,
		"local":  {Namespaces: []string{"openai-compatible", "openaiCompatible", "ollama"}},
	} {
		resolved, err := created.ResolveModel(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, want, resolved.ProviderOptions, id)
		expectedSchema := catalog.OpenAIErrorSchema
		if id == "claude" {
			expectedSchema = catalog.AnthropicErrorSchema
		}
		assert.Equal(t, expectedSchema, resolved.ErrorSchema)
	}
}

func TestBuildCatalog_MixedProviderFallbackRefusesConsumedOptions(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"type":"api_error","message":"private physical failure"}}`))
	}))
	defer server.Close()
	file := config.File{Models: map[string]config.Model{
		"mixed": {
			Name:     "Mixed",
			Primary:  config.Primary{Provider: "anthropic-primary", Model: "claude-backend"},
			Fallback: []config.Primary{{Provider: "compatible", Model: "local-backend"}},
		},
		"single": {Name: "Single", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude-backend"}},
	}}
	created, err := buildCatalog(file, map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "key", BaseURL: server.URL},
		"compatible":        {Type: "openai-compatible", APIKey: "key", BaseURL: server.URL + "/v1", ProviderName: "ollama"},
	}, server.Client(), anthropicprovider.New, identityModelFactory)
	require.NoError(t, err)

	mixed, err := created.ResolveModel(context.Background(), "mixed")
	require.NoError(t, err)
	assert.Equal(t, []string{"anthropic", "openai-compatible", "openaiCompatible", "ollama"}, mixed.ProviderOptions.Namespaces)
	assert.Equal(t, anthropicOptionPolicy.Fields["anthropic"], mixed.ProviderOptions.Fields["anthropic"])
	assert.NotContains(t, mixed.ProviderOptions.Fields, "ollama")
	assert.Empty(t, mixed.ErrorSchema)
	handler, err := providerv4.New(providerv4.Config{Resolver: created, Limits: serviceTestLimits()})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, options string
		prompt        string
		status        int
		calls         int32
	}{
		{name: "consumed primary", options: `{"anthropic":{"thinking":{"type":"enabled","budgetTokens":1024}}}`, status: http.StatusBadRequest},
		{name: "consumed fallback", options: `{"ollama":{"user":"caller"}}`, status: http.StatusBadRequest},
		{name: "all candidates ignore", options: `{"ignored":{"marker":"caller"}}`, status: http.StatusServiceUnavailable, calls: 2},
		{name: "semantic empties", options: `{}`, prompt: `[{"role":"user","content":[{"type":"text","text":"hello"}],"providerOptions":{"anthropic":{},"ollama":{}}}]`, status: http.StatusServiceUnavailable, calls: 2},
	} {
		for _, streaming := range []string{"false", "true"} {
			t.Run(tc.name+"/"+streaming, func(t *testing.T) {
				calls.Store(0)
				prompt := tc.prompt
				if prompt == "" {
					prompt = `[{"role":"user","content":[{"type":"text","text":"hello"}]}]`
				}
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":`+prompt+`,"maxOutputTokens":32,"providerOptions":`+tc.options+`}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
				request.Header.Set(providerv4.HeaderModelID, "mixed")
				request.Header.Set(providerv4.HeaderStreaming, streaming)
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				assert.Equal(t, tc.status, response.Code, response.Body.String())
				assert.NotContains(t, response.Body.String(), "caller")
				assert.Equal(t, tc.calls, calls.Load(), "consumed options must be refused; ignored and empty controls retain fallback eligibility")
				assert.NotContains(t, response.Body.String(), "private physical failure")
			})
		}
	}

	single, err := created.ResolveModel(context.Background(), "single")
	require.NoError(t, err)
	assert.Equal(t, anthropicOptionPolicy, single.ProviderOptions, "a single-provider model keeps its policy")
}

func TestAnthropicOptionPolicy_ForwardsSafeguards(t *testing.T) {
	var captured provider.CallOptions
	file := config.File{Models: map[string]config.Model{
		"claude": {Name: "Claude", Primary: config.Primary{Provider: "backend", Model: "claude-backend"}},
	}}
	created, err := buildCatalog(file, map[string]config.ResolvedProvider{
		"backend": {Type: "anthropic", APIKey: "key"},
	}, http.DefaultClient, anthropicprovider.New, func(_ string, lower provider.LanguageModel) (provider.LanguageModel, error) {
		return &optionCaptureModel{LanguageModel: lower, captured: &captured}, nil
	})
	require.NoError(t, err)
	handler, err := providerv4.New(providerv4.Config{Resolver: created, Limits: serviceTestLimits()})
	require.NoError(t, err)

	body := `{"prompt":[],"providerOptions":{"anthropic":{"safeguards":[{"type":"dangerous_tool_use"}],"unknown":true}}}`
	request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
	request.Header.Set(providerv4.HeaderModelID, "claude")
	request.Header.Set(providerv4.HeaderStreaming, "false")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	option, ok := captured.ProviderOptions["anthropic"].(provider.RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, `{"safeguards":[{"type":"dangerous_tool_use"}]}`, string(option.Raw))
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

func TestOpenAIOptionPolicy_ForwardsCapabilityControls(t *testing.T) {
	for _, namespace := range []string{"openai", "azure"} {
		t.Run(namespace, func(t *testing.T) {
			var captured provider.CallOptions
			file := config.File{Models: map[string]config.Model{
				"public": {Name: "OpenAI", Primary: config.Primary{Provider: "backend", Model: "gpt-4o"}},
			}}
			created, err := buildCatalog(file, map[string]config.ResolvedProvider{
				"backend": {Type: "openai", APIKey: "key"},
			}, http.DefaultClient, anthropicprovider.New, func(_ string, lower provider.LanguageModel) (provider.LanguageModel, error) {
				return &optionCaptureModel{LanguageModel: lower, captured: &captured}, nil
			})
			require.NoError(t, err)
			handler, err := providerv4.New(providerv4.Config{Resolver: created, Limits: serviceTestLimits()})
			require.NoError(t, err)

			body := `{"prompt":[],"providerOptions":{"` + namespace + `":{"includeWebSearchSources":false,"reasoningEffortUpdate":"high","compactionTrigger":true,"async":false,"encryptedContent":"blob","unknown":true}}}`
			request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
			request.Header.Set(providerv4.HeaderModelID, "public")
			request.Header.Set(providerv4.HeaderStreaming, "false")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())

			option, ok := captured.ProviderOptions[namespace].(provider.RawProviderOption)
			require.True(t, ok)
			assert.JSONEq(t, `{"includeWebSearchSources":false,"reasoningEffortUpdate":"high","compactionTrigger":true,"async":false,"encryptedContent":"blob"}`, string(option.Raw))
		})
	}
}

func TestProviderOptionPolicies_AllClassifiedFieldsAtMappedScopes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy catalog.ProviderOptionPolicy
	}{
		{name: "anthropic", policy: anthropicOptionPolicy},
		{name: "openai", policy: openAIOptionPolicy},
	} {
		for _, namespace := range tc.policy.Namespaces {
			t.Run(tc.name+"/"+namespace, func(t *testing.T) {
				var captured provider.CallOptions
				fields := make(map[string]any)
				for _, field := range tc.policy.Fields[namespace] {
					fields[field] = map[string]any{"marker": field, "empty": "", "zero": 0, "false": false, "null": nil}
				}
				expected, err := json.Marshal(fields)
				require.NoError(t, err)
				fields["unclassified"] = "excluded"
				raw, err := json.Marshal(map[string]any{namespace: fields})
				require.NoError(t, err)
				options := string(raw)
				model := &optionCaptureModel{LanguageModel: &observabilityTestModel{}, captured: &captured}
				resolver, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "route"}, Model: model, ProviderOptions: tc.policy}})
				require.NoError(t, err)
				handler, err := providerv4.New(providerv4.Config{Resolver: resolver, Limits: serviceTestLimits()})
				require.NoError(t, err)
				body := `{"providerOptions":` + options + `,"tools":[{"type":"function","name":"lookup","inputSchema":{},"providerOptions":` + options + `}],"prompt":[{"role":"system","content":"system","providerOptions":` + options + `},{"role":"user","providerOptions":` + options + `,"content":[{"type":"text","text":"text","providerOptions":` + options + `},{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""},"providerOptions":` + options + `}]},{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":` + options + `}]},{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"lookup","output":{"type":"content","value":[{"type":"file","mediaType":"application/pdf","data":{"type":"data","data":""},"providerOptions":` + options + `}]}}]}]}`
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
				request.Header.Set(providerv4.HeaderModelID, "route")
				request.Header.Set(providerv4.HeaderStreaming, "false")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				for _, scoped := range []provider.ProviderOptions{captured.ProviderOptions, captured.Tools[0].ProviderOptions, captured.Prompt[0].ProviderOptions, captured.Prompt[1].ProviderOptions, captured.Prompt[1].Content[0].ProviderOptions, captured.Prompt[1].Content[1].ProviderOptions, captured.Prompt[2].Content[0].ProviderOptions, captured.Prompt[3].Content[0].Output.Content[0].ProviderOptions} {
					value, ok := scoped[namespace].(provider.RawProviderOption)
					require.True(t, ok)
					assert.JSONEq(t, string(expected), string(value.Raw))
				}
			})
		}
	}
}

type optionCaptureModel struct {
	provider.LanguageModel
	captured *provider.CallOptions
}

func (model *optionCaptureModel) DoGenerate(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
	*model.captured = opts
	return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
}
