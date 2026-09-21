package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrepareTools_AllowedToolsResolution(t *testing.T) {
	function := provider.Tool{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}
	web := provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDWebSearch, Name: "search"}
	mcp := func(name, label string) provider.Tool {
		return provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: name, Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"` + label + `"`), "serverUrl": json.RawMessage(`"https://example.com/mcp"`)}}
	}
	deferred := function
	deferred.Name = "deferred"
	deferred.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"deferLoading":true}`)}}
	namespaced := function
	namespaced.Name = "namespaced"
	namespaced.ProviderOptions = provider.ProviderOptions{"openai": provider.RawProviderOption{Key: "openai", Raw: json.RawMessage(`{"namespace":{"name":"ns","description":"tools"}}`)}}
	for _, tc := range []struct {
		name     string
		tools    []provider.Tool
		allowed  []string
		mode     string
		want     string
		warnings []string
		wantErr  bool
	}{
		{name: "function", tools: []provider.Tool{function}, allowed: []string{"lookup"}, want: `[{"type":"function","name":"lookup"}]`},
		{name: "mixed ordered duplicates", tools: []provider.Tool{function, web}, allowed: []string{"search", "lookup", "search"}, mode: "required", want: `[{"type":"web_search"},{"type":"function","name":"lookup"},{"type":"web_search"}]`},
		{name: "canonical", tools: []provider.Tool{web}, allowed: []string{"web_search"}, want: `[{"type":"web_search"}]`},
		{name: "mcp", tools: []provider.Tool{mcp("alpha", "server-a"), mcp("beta", "server-b")}, allowed: []string{"beta"}, want: `[{"type":"mcp","server_label":"server-b"}]`},
		{name: "equivalent aliases", tools: []provider.Tool{mcp("alpha", "shared"), mcp("beta", "shared")}, allowed: []string{"mcp"}, want: `[{"type":"mcp","server_label":"shared"}]`},
		{name: "ambiguous alias", tools: []provider.Tool{function, mcp("alpha", "a"), mcp("beta", "b")}, allowed: []string{"lookup", "mcp"}, want: `[{"type":"function","name":"lookup"}]`, warnings: []string{"several tools"}},
		{name: "direct wins", tools: []provider.Tool{web, {Type: provider.ToolTypeFunction, Name: "web_search", InputSchema: function.InputSchema}}, allowed: []string{"web_search"}, want: `[{"type":"function","name":"web_search"}]`, warnings: []string{"both a tool name"}},
		{name: "unknown", tools: []provider.Tool{function}, allowed: []string{"typo", "function"}, want: `[{"type":"function","name":"typo"},{"type":"function","name":"function"}]`, warnings: []string{"not part of the tools", "not part of the tools"}},
		{name: "drop unsupported", tools: []provider.Tool{function, deferred, namespaced, {Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "discovery"}}, allowed: []string{"lookup", "deferred", "namespaced", "tool_search"}, want: `[{"type":"function","name":"lookup"}]`, warnings: []string{"deferred tools", "namespace", "tool_search tools"}},
		{name: "empty", tools: []provider.Tool{function}, allowed: []string{}, wantErr: true},
		{name: "fully dropped", tools: []provider.Tool{deferred, namespaced}, allowed: []string{"deferred", "namespaced"}, wantErr: true},
		{name: "ambiguous only", tools: []provider.Tool{mcp("alpha", "a"), mcp("beta", "b")}, allowed: []string{"mcp"}, wantErr: true},
		{name: "no tools", allowed: []string{"lookup"}},
		{name: "empty with no tools", allowed: []string{}, wantErr: true},
		{name: "invalid mode", tools: []provider.Tool{function}, allowed: []string{"lookup"}, mode: "none", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}, Tools: tc.tools, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceNone}, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: tc.allowed, Mode: tc.mode}})}
			params, warnings, _, err := buildParams("gpt-4o", opts)
			if tc.wantErr {
				require.ErrorContains(t, err, "allowedTools")
				return
			}
			require.NoError(t, err)
			raw, err := json.Marshal(params)
			require.NoError(t, err)
			var body map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &body))
			if len(tc.tools) == 0 {
				assert.NotContains(t, body, "tools")
				assert.NotContains(t, body, "tool_choice")
				return
			}
			mode := tc.mode
			if mode == "" {
				mode = "auto"
			}
			assert.JSONEq(t, `{"type":"allowed_tools","mode":"`+mode+`","tools":`+tc.want+`}`, string(body["tool_choice"]))
			require.Len(t, warnings, len(tc.warnings))
			for i, detail := range tc.warnings {
				assert.Equal(t, provider.WarnUnsupported, warnings[i].Type)
				assert.Contains(t, warnings[i].Feature, "allowedTools entry")
				assert.Contains(t, warnings[i].Details, detail)
			}
			opts.ProviderOptions = nil
			control, _ := buildBody(t, "gpt-4o", opts)
			controlTools, err := json.Marshal(control["tools"])
			require.NoError(t, err)
			assert.JSONEq(t, string(controlTools), string(body["tools"]))
		})
	}
}

func TestPrepareTools_AllowedToolsProviderShapes(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want string
	}{
		{toolIDWebSearch, `{"type":"web_search"}`},
		{toolIDWebSearchPreview, `{"type":"web_search_preview"}`},
		{toolIDCodeInterpreter, `{"type":"code_interpreter"}`},
		{toolIDFileSearch, `{"type":"file_search"}`},
		{toolIDImageGeneration, `{"type":"image_generation"}`},
		{toolIDLocalShell, `{"type":"local_shell"}`},
		{toolIDShell, `{"type":"shell"}`},
		{toolIDApplyPatch, `{"type":"apply_patch"}`},
		{toolIDComputer, `{"type":"computer"}`},
		{toolIDProgrammatic, `{"type":"programmatic_tool_calling"}`},
		{toolIDCustom, `{"type":"custom","name":"selected"}`},
	} {
		t.Run(tc.id, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{Prompt: []provider.Message{provider.UserText("Hi")}, Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: tc.id, Name: "selected"}}, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{"selected"}}})})
			raw, err := json.Marshal(body["tool_choice"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"type":"allowed_tools","mode":"auto","tools":[`+tc.want+`]}`, string(raw))
			assert.Empty(t, warnings)
		})
	}
}

func TestModel_AllowedToolsRejectsBeforeHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	m := NewResponses("test", "gpt-4o", WithRequestOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)))
	opts := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)}}, ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{}}})}
	_, err := m.DoGenerate(context.Background(), opts)
	require.ErrorContains(t, err, "allowedTools")
	_, err = m.DoStream(context.Background(), opts)
	require.ErrorContains(t, err, "allowedTools")
	assert.Zero(t, requests.Load())
}
