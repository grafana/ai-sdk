package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func settingsHandler(t *testing.T, model provider.LanguageModel) http.Handler {
	t.Helper()
	entries, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "alias"}, Model: model}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(entries), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	return h
}

func settingsRequest(t *testing.T, h http.Handler, settings string, streaming bool) {
	t.Helper()
	body := fmt.Sprintf(`{"model":"alias","max_tokens":8192,"messages":[{"role":"user","content":"hi"}],"stream":%t%s}`, streaming, settings)
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), `"type":"error"`)
}

func TestHandler_Settings(t *testing.T) {
	for _, tc := range []struct {
		name      string
		target    execution.Target
		namespace string
		effortKey string
		parallel  string
		inverse   bool
	}{
		{"anthropic", execution.Target{Type: execution.TargetAnthropic}, "anthropic", "effort", "disableParallelToolUse", false},
		{"openai", execution.Target{Type: execution.TargetOpenAI}, "openai", "reasoningEffort", "parallelToolCalls", true},
		{"compatible", execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: " vendor.chat "}, "vendor", "reasoningEffort", "parallel_tool_calls", true},
		{"compatible default", execution.Target{Type: execution.TargetOpenAICompatible}, "openai-compatible", "reasoningEffort", "parallel_tool_calls", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, streaming := range []bool{false, true} {
				for _, disabled := range []bool{false, true} {
					t.Run(fmt.Sprintf("stream=%t/disabled=%t", streaming, disabled), func(t *testing.T) {
						var seen provider.CallOptions
						model := &fakeModel{
							generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
								seen = o
								return textResult(), nil
							},
							stream: func(_ context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
								seen = o
								return streamOf(finish(provider.FinishReasonStop)), nil
							},
						}
						h := settingsHandler(t, execution.Translate(model, tc.target))
						settingsRequest(t, h, fmt.Sprintf(`,"output_config":{"effort":"high"},"tool_choice":{"type":"auto","disable_parallel_tool_use":%t}`, disabled), streaming)
						encoded, err := json.Marshal(seen.ProviderOptions)
						require.NoError(t, err)
						assert.JSONEq(t, fmt.Sprintf(`{"%s":{"%s":"high","%s":%t}}`, tc.namespace, tc.effortKey, tc.parallel, disabled != tc.inverse), string(encoded))
						assert.Equal(t, ptr(8192), seen.MaxOutputTokens)
						assert.Equal(t, provider.ReasoningProviderDefault, seen.Reasoning)
					})
				}
			}
		})
	}
	for _, settings := range []string{"", `,"output_config":{},"tool_choice":{"type":"auto"}`, `,"output_config":null`, `,"output_config":{"effort":null},"tool_choice":{"type":"auto","disable_parallel_tool_use":null}`} {
		t.Run("omitted "+settings, func(t *testing.T) {
			h := testHandler(t, &fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
				assert.Empty(t, o.ProviderOptions)
				return textResult(), nil
			}})
			settingsRequest(t, h, settings, false)
		})
	}
}

func TestHandler_SettingsFallback(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", streaming), func(t *testing.T) {
			var attempts []provider.ProviderOptions
			unavailable := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, IsRetryable: ptr(true)})
			primary := execution.Translate(&fakeModel{
				generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
					attempts = append(attempts, o.ProviderOptions)
					return nil, unavailable
				},
				stream: func(_ context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
					attempts = append(attempts, o.ProviderOptions)
					return nil, unavailable
				},
			}, execution.Target{Type: execution.TargetAnthropic})
			secondary := execution.Translate(&fakeModel{
				generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
					attempts = append(attempts, o.ProviderOptions)
					return textResult(), nil
				},
				stream: func(_ context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
					attempts = append(attempts, o.ProviderOptions)
					return streamOf(finish(provider.FinishReasonStop)), nil
				},
			}, execution.Target{Type: execution.TargetOpenAI})
			chain, err := fallback.New(primary, secondary)
			require.NoError(t, err)
			settingsRequest(t, settingsHandler(t, chain.WithDecider(execution.FallbackDecider)), `,"output_config":{"effort":"low"},"tool_choice":{"type":"auto","disable_parallel_tool_use":false}`, streaming)
			require.Len(t, attempts, 2)
			encoded, err := json.Marshal(attempts)
			require.NoError(t, err)
			assert.JSONEq(t, `[{"anthropic":{"effort":"low","disableParallelToolUse":false}},{"openai":{"reasoningEffort":"low","parallelToolCalls":true}}]`, string(encoded))
		})
	}
}

func TestMapRequest_Effort(t *testing.T) {
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{`"low"`, true}, {`"medium"`, true}, {`"high"`, true},
		{`"xhigh"`, true}, {`"max"`, true},
		{`""`, false}, {`"HIGH"`, false}, {`"unknown"`, false}, {`1`, false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			in, options, err := mapRequest([]byte(fmt.Sprintf(`{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":%s}}`, tc.value)))
			if !tc.valid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			translated, err := messagesTranslator(in)(execution.Target{Type: execution.TargetAnthropic}, options)
			require.NoError(t, err)
			encoded, err := json.Marshal(translated.ProviderOptions)
			require.NoError(t, err)
			assert.JSONEq(t, `{"anthropic":{"effort":`+tc.value+`}}`, string(encoded))
		})
	}
}
