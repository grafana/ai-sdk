package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
)

func TestChat_TenantDefaults(t *testing.T) {
	h := newHarnessWithOptions(t, platformhost.TopologySelfHosted, shippedOptions())
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	code, document := sendObject(t, context.Background(), http.MethodPost, catalogURL(h.server.URL, ""), h.serviceToken(t, testInternalService), teamCatalog(t, "claude-sonnet-4-6", map[string]any{"claude": map[string]any{"create": routeCatalogKey}}))
	require.Equal(t, http.StatusCreated, code, document)
	storeChatMiddleware(t, h, "default-instructions", defaultInstructionsMember)
	storeChatMiddleware(t, h, "default-settings", defaultSettingsMember)
	auth := bearer(h.host.AddServiceAccount("chat-defaults", testNamespace))
	for _, route := range []struct {
		name, url, model string
		provider         *fakeAnthropic
	}{
		{name: "namespace", url: h.url("v1/chat/completions"), model: "public", provider: h.anthropic},
		{name: "catalog", url: h.catalogURL("v1/chat/completions"), model: "team", provider: native},
	} {
		t.Run(route.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, settings, instruction string
				wantInstruction             []string
				wantTemperature             float64
				wantTokens                  int
				wantStop                    []string
			}{
				{name: "omitted", wantInstruction: []string{"Be brief.", "Cite sources."}, wantTemperature: 0.25, wantTokens: 48, wantStop: []string{"END"}},
				{name: "null", settings: `,"temperature":null,"stop":null,"max_completion_tokens":null`, wantInstruction: []string{"Be brief.", "Cite sources."}, wantTemperature: 0.25, wantTokens: 48, wantStop: []string{"END"}},
				{name: "legacy caller values", settings: `,"temperature":0,"max_tokens":16,"stop":"DONE"`, instruction: `{"role":"system","content":"Client."},`, wantInstruction: []string{"Client."}, wantTokens: 16, wantStop: []string{"DONE"}},
				{name: "caller values", settings: `,"temperature":0,"max_completion_tokens":32,"stop":["FIRST","LAST"]`, instruction: `{"role":"system","content":"Client."},`, wantInstruction: []string{"Client."}, wantTokens: 32, wantStop: []string{"FIRST", "LAST"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					for _, streaming := range []bool{false, true} {
						body := fmt.Sprintf(`{"model":%q,"messages":[%s{"role":"user","content":"hi"}],"stream":%t%s}`, route.model, tc.instruction, streaming, tc.settings)
						before := route.provider.requests.Load()
						status, response := do(t, chatRequest(t, route.url, body, auth))
						require.Equal(t, http.StatusOK, status, response)
						if streaming {
							assert.Contains(t, response, "[DONE]")
						}
						require.Equal(t, before+1, route.provider.requests.Load())
						assertAnthropicChatDefaults(t, route.provider, tc.wantInstruction, tc.wantTemperature, tc.wantTokens, tc.wantStop)
					}
				})
			}
			before := route.provider.requests.Load()
			body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}],"stop":[]}`, route.model)
			status, response := do(t, chatRequest(t, route.url, body, auth))
			assert.Equal(t, http.StatusBadRequest, status, response)
			assert.Equal(t, before, route.provider.requests.Load())
		})
	}
}

func assertAnthropicChatDefaults(t *testing.T, native *fakeAnthropic, instructions []string, temperature float64, maxTokens int, stop []string) {
	t.Helper()
	_, requests := native.nativeCalls()
	require.NotEmpty(t, requests)
	var call anthropicRequest
	require.NoError(t, json.Unmarshal([]byte(requests[len(requests)-1]), &call))
	assert.Equal(t, instructions, systemTexts(call))
	assert.Equal(t, &temperature, call.Temperature)
	assert.Equal(t, maxTokens, call.MaxTokens)
	assert.Equal(t, stop, call.StopSequences)
}

func TestChat_DefaultsSurviveHeterogeneousFallback(t *testing.T) {
	attempts := make(chan []byte, 8)
	responses := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		attempts <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":{"message":"fixture unavailable","type":"server_error"}}`)
	}))
	t.Cleanup(responses.Close)
	options := shippedOptions()
	options.env = map[string]string{"OPENAI_SECRET": "fixture-openai-key", "GF_PLUGIN_MODEL_CATALOG": fmt.Sprintf(`providers:
  responses: {type: openai, apiKeyEnv: OPENAI_SECRET, baseURL: %q}
  messages: {type: anthropic, apiKeyEnv: ANTHROPIC_SECRET}
models:
  public:
    name: Public
    primary: {provider: responses, model: gpt-4.1}
    fallback:
      - {provider: messages, model: claude-sonnet-4-6}
`, responses.URL+"/v1")}
	h := newHarnessWithOptions(t, platformhost.TopologySelfHosted, options)
	native := newFakeAnthropic(t)
	useNativeAnthropic(t, native)
	storeChatMiddleware(t, h, "default-instructions", defaultInstructionsMember)
	storeChatMiddleware(t, h, "default-settings", defaultSettingsMember)
	auth := bearer(h.host.AddServiceAccount("chat-defaults-fallback", testNamespace))
	for _, streaming := range []bool{false, true} {
		body := fmt.Sprintf(`{"model":"public","messages":[{"role":"user","content":"hi"}],"temperature":0,"tools":[{"type":"function","function":{"name":"lookup"}}],"parallel_tool_calls":false,"stream":%t}`, streaming)
		before := native.requests.Load()
		status, document := do(t, chatRequest(t, h.url("v1/chat/completions"), body, auth))
		require.Equal(t, http.StatusOK, status, document)
		if streaming {
			assert.Contains(t, document, "[DONE]")
		}
		assert.NotContains(t, document, `"execution"`)
		assert.NotContains(t, document, `"routing"`)
		require.Len(t, attempts, 1)
		var responsesCall struct {
			Input       json.RawMessage `json:"input"`
			MaxTokens   int             `json:"max_output_tokens"`
			Temperature *float64        `json:"temperature"`
			Store       *bool           `json:"store"`
			Parallel    *bool           `json:"parallel_tool_calls"`
			Tools       []struct {
				Strict *bool `json:"strict"`
			} `json:"tools"`
		}
		require.NoError(t, json.Unmarshal(<-attempts, &responsesCall))
		for _, instruction := range []string{"Be brief.", "Cite sources."} {
			assert.Contains(t, string(responsesCall.Input), instruction)
		}
		assert.Equal(t, 48, responsesCall.MaxTokens)
		assert.Equal(t, new(0.0), responsesCall.Temperature)
		assert.Equal(t, new(false), responsesCall.Store)
		assert.Equal(t, new(false), responsesCall.Parallel)
		require.Len(t, responsesCall.Tools, 1)
		assert.Equal(t, new(false), responsesCall.Tools[0].Strict)

		require.Equal(t, before+1, native.requests.Load())
		assertAnthropicChatDefaults(t, native, []string{"Be brief.", "Cite sources."}, 0, 48, []string{"END"})
		_, messages := native.nativeCalls()
		var messagesCall map[string]json.RawMessage
		require.NoError(t, json.Unmarshal([]byte(messages[len(messages)-1]), &messagesCall))
		assert.NotContains(t, messagesCall, "store")
		assert.NotContains(t, messagesCall, "parallel_tool_calls")
		assert.JSONEq(t, `{"type":"auto","disable_parallel_tool_use":true}`, string(messagesCall["tool_choice"]))
	}
}
