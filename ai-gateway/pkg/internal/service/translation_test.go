package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/config"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/internal/nativeoptions"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
)

const (
	translationAnthropicHost = "anthropic.translation.test"
	translationOpenAIHost    = "openai.translation.test"
	translationCompatHost    = "compatible.translation.test"
	translationCompatBody    = `{"id":"chat_1","object":"chat.completion","created":1,"model":"local","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`
	translationOpenAIBody    = `{"id":"resp_1","object":"response","status":"completed","model":"gpt","output":[{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"output_text","text":"answer","annotations":[]}]}],"usage":{"input_tokens":1,"output_tokens":1}}`
	translationAnthropicBody = `{"id":"msg_1","type":"message","role":"assistant","model":"claude","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
)

type nativeRequest struct {
	host, key, marker, body string
}

// nativeServer answers Anthropic and OpenAI requests by host, and fails the
// accounts or hosts listed in failing with a retryable 503.
type nativeServer struct {
	failing map[string]bool
	mu      sync.Mutex
	seen    []nativeRequest
}

func (s *nativeServer) client() *http.Client {
	return &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		key := r.Header.Get("X-Api-Key")
		if key == "" {
			key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		s.mu.Lock()
		s.seen = append(s.seen, nativeRequest{host: r.URL.Host, key: key, marker: r.Header.Get("X-Translated-For"), body: string(body)})
		s.mu.Unlock()
		status, response := http.StatusOK, translationOpenAIBody
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			response = translationAnthropicBody
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			response = translationCompatBody
		}
		if s.failing[r.URL.Host] || s.failing[key] {
			status, response = http.StatusServiceUnavailable, `{"type":"error","error":{"type":"overloaded_error","message":"busy"}}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: r}, nil
	})}
}

func (s *nativeServer) requests() []nativeRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]nativeRequest(nil), s.seen...)
}

// markTranslation writes a target-specific native option and header.
func markTranslation(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
	if options.ProviderOptions == nil {
		options.ProviderOptions = provider.ProviderOptions{}
	}
	if options.Headers == nil {
		options.Headers = map[string]string{}
	}
	options.Headers["X-Translated-For"] = string(target.Type) + "/" + target.ModelID
	switch target.Type {
	case execution.TargetAnthropic:
		options.ProviderOptions["anthropic"] = provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"container":{"id":"translated-anthropic"}}`)}
	case execution.TargetOpenAI:
		options.ProviderOptions["openai"] = provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"user":"translated-openai"}`)}
	}
	return options, nil
}

func translationCatalog(t *testing.T, client *http.Client) *Catalog {
	t.Helper()
	file := config.File{
		Providers: map[string]config.Provider{"anthropic-primary": {Type: "anthropic"}, "openai-backup": {Type: "openai"}, "local": {Type: "openai-compatible"}, "unnamed": {Type: "openai-compatible"}},
		Models: map[string]config.Model{
			"grafana/local":   {Name: "Local", Primary: config.Primary{Provider: "local", Model: "local"}},
			"grafana/unnamed": {Name: "Unnamed", Primary: config.Primary{Provider: "unnamed", Model: "unnamed"}},
			"grafana/chain":   {Name: "Chain", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude"}, Fallback: []config.Primary{{Provider: "openai-backup", Model: "gpt"}}},
			"grafana/claude":  {Name: "Claude", Primary: config.Primary{Provider: "anthropic-primary", Model: "claude"}},
		},
	}
	providers := map[string]config.ResolvedProvider{
		"anthropic-primary": {Type: "anthropic", APIKey: "configured-anthropic", BaseURL: "https://" + translationAnthropicHost},
		"openai-backup":     {Type: "openai", APIKey: "configured-openai", BaseURL: "https://" + translationOpenAIHost + "/v1"},
		"local":             {Type: "openai-compatible", APIKey: "configured-local", BaseURL: "https://" + translationCompatHost + "/v1", ProviderName: "my-vllm.chat"},
		"unnamed":           {Type: "openai-compatible", APIKey: "configured-unnamed", BaseURL: "https://" + translationCompatHost + "/v1"},
	}
	created, err := BuildCatalog(file, providers, client, identityModelFactory)
	require.NoError(t, err)
	return created
}

// callerContext returns a context carrying a verified caller with access.
func callerContext(t *testing.T, access gatewayauth.AccountAccess) context.Context {
	t.Helper()
	var captured context.Context
	handler, _ := withAccess(t, gatewayauth.AccessModeCloud, access, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { captured = r.Context() }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	require.NotNil(t, captured)
	return captured
}

type translationCall struct {
	access     gatewayauth.AccountAccess
	modelID    string
	options    provider.CallOptions
	translator execution.Translator
}

func runTranslation(t *testing.T, selector execution.RequestSelector, call translationCall) error {
	t.Helper()
	runtime, err := execution.New(execution.Config{Selector: selector, Limits: serviceTestLimits()})
	require.NoError(t, err)
	selected, err := runtime.Call(callerContext(t, call.access), call.modelID, call.options, execution.WithTranslator(call.translator))
	if err != nil {
		return err
	}
	defer selected.Close()
	_, err = selected.Generate()
	return err
}

func byokControls(name string, keys ...string) provider.CallOptions {
	accounts := make([]string, len(keys))
	for i, key := range keys {
		accounts[i] = fmt.Sprintf(`{"apiKey":%q}`, key)
	}
	raw := fmt.Sprintf(`{"byok":{%q:[%s]}}`, name, strings.Join(accounts, ","))
	return providerOptionsCall(execution.ControlNamespace, raw)
}

func providerOptionsCall(namespace, raw string) provider.CallOptions {
	return provider.CallOptions{ProviderOptions: provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(raw)}}}
}

func TestTranslation_HeterogeneousConfiguredChain(t *testing.T) {
	server := &nativeServer{failing: map[string]bool{translationAnthropicHost: true}}
	client := server.client()
	selector := NewSelector(translationCatalog(t, client), nil)
	require.NoError(t, runTranslation(t, selector, translationCall{access: gatewayauth.ConfiguredAccounts, modelID: "grafana/chain", translator: markTranslation}))

	requests := server.requests()
	require.Len(t, requests, 2)
	assert.Equal(t, translationAnthropicHost, requests[0].host)
	assert.Equal(t, "anthropic/claude", requests[0].marker)
	assert.Contains(t, requests[0].body, "translated-anthropic")
	assert.NotContains(t, requests[0].body, "translated-openai")
	assert.Equal(t, translationOpenAIHost, requests[1].host)
	assert.Equal(t, "openai/gpt", requests[1].marker)
	assert.Contains(t, requests[1].body, "translated-openai")
	assert.NotContains(t, requests[1].body, "translated-anthropic")
}

func TestTranslation_BYOKAccountsAreTranslatedPerAttempt(t *testing.T) {
	server := &nativeServer{failing: map[string]bool{"dummy-first": true}}
	selector := byokSelector(server.client(), func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil })
	var attempts int
	translator := func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		attempts++
		options.Headers = map[string]string{"X-Translated-For": fmt.Sprintf("%s/%s/%d", target.Type, target.ModelID, attempts)}
		return options, nil
	}
	require.NoError(t, runTranslation(t, selector, translationCall{access: gatewayauth.RequestBYOK, modelID: "openai/gpt", options: byokControls("openai", "dummy-first", "dummy-second"), translator: translator}))

	requests := server.requests()
	require.Len(t, requests, 2)
	assert.Equal(t, "dummy-first", requests[0].key)
	assert.Equal(t, "openai/gpt/1", requests[0].marker)
	assert.Equal(t, "dummy-second", requests[1].key)
	assert.Equal(t, "openai/gpt/2", requests[1].marker)
}

func TestTranslation_FailuresStopBeforeProviderContact(t *testing.T) {
	for _, tc := range []struct {
		name       string
		translator execution.Translator
	}{
		{"panic", func(execution.Target, provider.CallOptions) (provider.CallOptions, error) { panic("private") }},
		{"protected header on the first candidate", func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
			if target.Type == execution.TargetAnthropic {
				options.Headers = map[string]string{"X-Api-Key": "injected"}
			}
			return options, nil
		}},
		{"control namespace", func(_ execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
			options.ProviderOptions = provider.ProviderOptions{execution.ControlNamespace: provider.RawProviderOption{Key: execution.ControlNamespace, Raw: json.RawMessage(`{}`)}}
			return options, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &nativeServer{}
			configured := NewSelector(translationCatalog(t, server.client()), nil)
			err := runTranslation(t, configured, translationCall{access: gatewayauth.ConfiguredAccounts, modelID: "grafana/chain", translator: tc.translator})
			require.ErrorIs(t, err, execution.ErrTranslation)
			assert.Equal(t, execution.CategoryInternal, execution.ClassifyProvider(err))

			byok := byokSelector(server.client(), func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil })
			err = runTranslation(t, byok, translationCall{access: gatewayauth.RequestBYOK, modelID: "anthropic/claude", options: byokControls("anthropic", "dummy-first", "dummy-second"), translator: tc.translator})
			require.ErrorIs(t, err, execution.ErrTranslation)
			assert.Empty(t, server.requests())
		})
	}
}

func TestTranslation_AnthropicNativeValidation(t *testing.T) {
	skills := func(_ execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		options.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"container":{"skills":[{"type":"custom","skillId":"external"}]}}`)}}
		return options, nil
	}
	for _, route := range []struct {
		name string
		call translationCall
	}{
		{"configured", translationCall{access: gatewayauth.ConfiguredAccounts, modelID: "grafana/claude"}},
		{"BYOK", translationCall{access: gatewayauth.RequestBYOK, modelID: "anthropic/claude", options: byokControls("anthropic", "dummy-first")}},
	} {
		t.Run(route.name, func(t *testing.T) {
			server := &nativeServer{}
			factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil }
			selector := NewSelector(translationCatalog(t, server.client()), &BYOKOptions{Client: server.client(), Factory: factory})

			valid := route.call
			valid.translator = markTranslation
			require.NoError(t, runTranslation(t, selector, valid))
			requests := server.requests()
			require.Len(t, requests, 1)
			assert.Contains(t, requests[0].body, "translated-anthropic")

			refused := route.call
			refused.translator = skills
			err := runTranslation(t, selector, refused)
			require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
			assert.Equal(t, execution.CategoryInvalidRequest, execution.ClassifyProvider(err))
			assert.Len(t, server.requests(), 1)
		})
	}
}

// byokChain returns the fallback chain inside a BYOK model's request-level
// native option validation.
func byokChain(model provider.LanguageModel) provider.LanguageModel {
	if validated, ok := model.(nativeoptions.Model); ok {
		return validated.LanguageModel
	}
	return model
}

func TestTranslation_BYOKAnthropicValidation(t *testing.T) {
	skills := json.RawMessage(`{"container":{"skills":[{"type":"custom","skillId":"external"}]}}`)
	for _, tc := range []struct {
		name       string
		options    bool
		translator execution.Translator
		outcomes   []fallback.AttemptOutcome
	}{
		{name: "request options refused before any account", options: true},
		{name: "translated options refused on each account", outcomes: []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptFailed},
			translator: func(_ execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
				options.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: skills}}
				return options, nil
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := &nativeServer{}
			var outcomes []fallback.AttemptOutcome
			factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) {
				chain, ok := byokChain(model).(*fallback.Model)
				require.True(t, ok)
				chain.WithAttemptObserver(func(_ context.Context, attempt fallback.Attempt) { outcomes = append(outcomes, attempt.Outcome) })
				return model, nil
			}
			options := byokControls("anthropic", "dummy-first", "dummy-second")
			if tc.options {
				options.ProviderOptions["anthropic"] = provider.RawProviderOption{Key: "anthropic", Raw: skills}
			}
			err := runTranslation(t, byokSelector(server.client(), factory), translationCall{access: gatewayauth.RequestBYOK, modelID: "anthropic/claude", options: options, translator: tc.translator})
			require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
			assert.Equal(t, execution.CategoryInvalidRequest, execution.ClassifyProvider(err))
			assert.Equal(t, tc.outcomes, outcomes)
			assert.Empty(t, server.requests())
		})
	}
}

func TestTranslation_OpenAICompatibleTarget(t *testing.T) {
	write := func(fields string) execution.Translator {
		return func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
			assert.Equal(t, execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "my-vllm.chat", ModelID: "local"}, target)
			namespace, _, _ := strings.Cut(target.ProviderName, ".")
			options.ProviderOptions = provider.ProviderOptions{namespace: provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(fields)}}
			return options, nil
		}
	}
	server := &nativeServer{}
	selector := NewSelector(translationCatalog(t, server.client()), nil)
	require.NoError(t, runTranslation(t, selector, translationCall{access: gatewayauth.ConfiguredAccounts, modelID: "grafana/local", translator: write(`{"translatedField":"compatible"}`)}))
	requests := server.requests()
	require.Len(t, requests, 1)
	assert.Equal(t, translationCompatHost, requests[0].host)
	assert.Contains(t, requests[0].body, `"translatedField":"compatible"`)

	err := runTranslation(t, selector, translationCall{access: gatewayauth.ConfiguredAccounts, modelID: "grafana/local", translator: write(`{"model":"other"}`)})
	require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
	assert.Len(t, server.requests(), 1)

	t.Run("default provider name", func(t *testing.T) {
		server := &nativeServer{}
		selector := NewSelector(translationCatalog(t, server.client()), nil)
		var seen execution.Target
		translator := func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
			seen = target
			options.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"model":"other"}`)}}
			return options, nil
		}
		err := runTranslation(t, selector, translationCall{access: gatewayauth.ConfiguredAccounts, modelID: "grafana/unnamed", translator: translator})
		assert.Equal(t, execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "openai-compatible", ModelID: "unnamed"}, seen)
		require.ErrorIs(t, err, catalog.ErrUnsupportedRequest)
		assert.Empty(t, server.requests())
	})
}

func TestTranslation_ConcurrentCallsAreIsolated(t *testing.T) {
	server := &nativeServer{}
	factory := func(_ string, model provider.LanguageModel) (provider.LanguageModel, error) { return model, nil }
	selector := NewSelector(translationCatalog(t, server.client()), &BYOKOptions{Client: server.client(), Factory: factory})
	runtime, err := execution.New(execution.Config{Selector: selector, Limits: serviceTestLimits()})
	require.NoError(t, err)
	configured, byok := callerContext(t, gatewayauth.ConfiguredAccounts), callerContext(t, gatewayauth.RequestBYOK)
	const calls = 24
	var group sync.WaitGroup
	errs := make([]error, calls)
	for index := range calls {
		group.Go(func() {
			marker := fmt.Sprintf("call-%d", index)
			translator := func(_ execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
				switch index % 3 {
				case 0:
					panic("private")
				case 1:
					options.Headers = map[string]string{"X-Translated-For": marker}
				}
				return options, nil
			}
			ctx, modelID, options := configured, "grafana/claude", provider.CallOptions{}
			if index%2 == 1 {
				ctx, modelID, options = byok, "openai/gpt", byokControls("openai", "dummy-"+marker)
			}
			call, err := runtime.Call(ctx, modelID, options, execution.WithTranslator(translator))
			if err != nil {
				errs[index] = err
				return
			}
			defer call.Close()
			_, errs[index] = call.Generate()
		})
	}
	group.Wait()
	expected := 0
	for index, err := range errs {
		if index%3 == 0 {
			assert.ErrorIs(t, err, execution.ErrTranslation, "call %d", index)
			continue
		}
		assert.NoError(t, err, "call %d", index)
		expected++
	}
	requests := server.requests()
	require.Len(t, requests, expected)
	var markers, wantMarkers []string
	for index := range calls {
		if index%3 == 1 {
			wantMarkers = append(wantMarkers, fmt.Sprintf("call-%d", index))
		}
	}
	for _, request := range requests {
		if request.marker != "" {
			markers = append(markers, request.marker)
		}
		if marker, ok := strings.CutPrefix(request.key, "dummy-"); ok && request.marker != "" {
			assert.Equal(t, marker, request.marker, "a BYOK account carries only its own call's translation")
		}
	}
	assert.ElementsMatch(t, wantMarkers, markers)
}
