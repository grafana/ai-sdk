package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allowedFunction(name string) provider.Tool {
	return provider.Tool{Type: provider.ToolTypeFunction, Name: name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func allowedProvider(id, name string) provider.Tool {
	return provider.Tool{Type: provider.ToolTypeProvider, ID: id, Name: name}
}

func allowedChoice(t *testing.T, body map[string]any) (map[string]any, []any) {
	t.Helper()
	choice, ok := body["tool_choice"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "allowed_tools", choice["type"])
	entries, ok := choice["tools"].([]any)
	require.True(t, ok)
	return choice, entries
}

func TestPrepareTools_AllowedToolsKinds(t *testing.T) {
	tests := []struct {
		name string
		tool provider.Tool
		want map[string]any
	}{
		{"function", allowedFunction("lookup"), map[string]any{"type": "function", "name": "lookup"}},
		{"file search", allowedProvider(toolIDFileSearch, "lookup"), map[string]any{"type": "file_search"}},
		{"web search", allowedProvider(toolIDWebSearch, "lookup"), map[string]any{"type": "web_search"}},
		{"web search preview", allowedProvider(toolIDWebSearchPreview, "lookup"), map[string]any{"type": "web_search_preview"}},
		{"code interpreter", allowedProvider(toolIDCodeInterpreter, "lookup"), map[string]any{"type": "code_interpreter"}},
		{"image generation", allowedProvider(toolIDImageGeneration, "lookup"), map[string]any{"type": "image_generation"}},
		{"local shell", allowedProvider(toolIDLocalShell, "lookup"), map[string]any{"type": "local_shell"}},
		{"shell", allowedProvider(toolIDShell, "lookup"), map[string]any{"type": "shell"}},
		{"apply patch", allowedProvider(toolIDApplyPatch, "lookup"), map[string]any{"type": "apply_patch"}},
		{"computer", allowedProvider(toolIDComputer, "lookup"), map[string]any{"type": "computer"}},
		{"programmatic tool calling", allowedProvider(toolIDProgrammatic, "lookup"), map[string]any{"type": "programmatic_tool_calling"}},
		{"custom", allowedProvider(toolIDCustom, "lookup"), map[string]any{"type": "custom", "name": "lookup"}},
		{"mcp", provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "lookup", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"srv"`)}}, map[string]any{"type": "mcp", "server_label": "srv"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Tools: []provider.Tool{tc.tool},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{"lookup"}}}),
			})
			assert.Empty(t, warnings)
			choice, entries := allowedChoice(t, body)
			assert.Equal(t, "auto", choice["mode"])
			assert.Equal(t, []any{tc.want}, entries)
		})
	}
}

func TestPrepareTools_AllowedToolsMixedAndOverride(t *testing.T) {
	tools := []provider.Tool{
		allowedFunction("weather"),
		allowedProvider(toolIDWebSearch, "search"),
		allowedProvider(toolIDCustom, "freeform"),
		{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "docs", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"srv"`)}},
	}
	selection := []string{"docs", "weather", "search", "freeform", "weather"}
	opts := provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, Tools: tools,
		ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceNone},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: selection, Mode: "required"}}),
	}
	before, err := json.Marshal(opts)
	require.NoError(t, err)
	for range 2 {
		body, warnings := buildBody(t, "gpt-4o", opts)
		assert.Empty(t, warnings)
		choice, entries := allowedChoice(t, body)
		assert.Equal(t, "required", choice["mode"])
		assert.Equal(t, []any{
			map[string]any{"type": "mcp", "server_label": "srv"},
			map[string]any{"type": "function", "name": "weather"},
			map[string]any{"type": "web_search"},
			map[string]any{"type": "custom", "name": "freeform"},
			map[string]any{"type": "function", "name": "weather"},
		}, entries)
	}
	after, err := json.Marshal(opts)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestPrepareTools_AllowedToolsAliases(t *testing.T) {
	tests := []struct {
		name     string
		tools    []provider.Tool
		selected string
		want     map[string]any
		warnings []provider.Warning
	}{
		{
			name: "canonical hosted alias", tools: []provider.Tool{allowedProvider(toolIDWebSearch, "search")},
			selected: "web_search", want: map[string]any{"type": "web_search"},
		},
		{
			name: "direct name shadows canonical alias", tools: []provider.Tool{allowedProvider(toolIDWebSearch, "search"), allowedFunction("web_search")},
			selected: "web_search", want: map[string]any{"type": "function", "name": "web_search"},
			warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: `allowedTools entry "web_search"`, Details: "this name is both a tool name and the provider tool name of another tool in this request; the tool with this name is allowed"}},
		},
		{
			name: "equivalent canonical aliases", tools: []provider.Tool{allowedProvider(toolIDWebSearch, "first_search"), allowedProvider(toolIDWebSearch, "second_search")},
			selected: "web_search", want: map[string]any{"type": "web_search"},
		},
		{
			name: "direct mcp name", tools: []provider.Tool{
				{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "alpha", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"one"`)}},
				{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "beta", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"two"`)}},
			}, selected: "beta", want: map[string]any{"type": "mcp", "server_label": "two"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Tools: tc.tools,
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{tc.selected}}}),
			})
			_, entries := allowedChoice(t, body)
			assert.Equal(t, []any{tc.want}, entries)
			assert.Equal(t, tc.warnings, warnings)
		})
	}
}

func TestPrepareTools_AllowedToolsWarningsAndDrops(t *testing.T) {
	deferLoading := true
	tests := []struct {
		name     string
		tools    []provider.Tool
		names    []string
		want     []any
		warnings []provider.Warning
	}{
		{
			name: "ambiguous mcp alias", tools: []provider.Tool{
				allowedFunction("weather"),
				{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "alpha", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"one"`)}},
				{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "beta", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"two"`)}},
			}, names: []string{"mcp", "beta", "weather"}, want: []any{map[string]any{"type": "mcp", "server_label": "two"}, map[string]any{"type": "function", "name": "weather"}},
			warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: `allowedTools entry "mcp"`, Details: "several tools in this request share this provider tool name; use the tool name from the tools for this request instead"}},
		},
		{
			name: "ineligible declarations", tools: []provider.Tool{
				allowedFunction("weather"),
				{Type: provider.ToolTypeFunction, Name: "deferred", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{DeferLoading: &deferLoading})},
				{Type: provider.ToolTypeFunction, Name: "nested", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{Namespace: &OpenAIToolNamespaceOptions{Name: "crm"}})},
				allowedProvider(toolIDToolSearch, "finder"),
			}, names: []string{"deferred", "nested", "finder", "tool_search", "weather"}, want: []any{map[string]any{"type": "function", "name": "weather"}},
			warnings: []provider.Warning{
				{Type: provider.WarnUnsupported, Feature: `allowedTools entry "deferred"`, Details: "deferred tools are not visible to tool_choice.allowed_tools; the tool is removed from the allowed tools"},
				{Type: provider.WarnUnsupported, Feature: `allowedTools entry "nested"`, Details: "tools inside an OpenAI tool namespace are not visible to tool_choice.allowed_tools; the tool is removed from the allowed tools"},
				{Type: provider.WarnUnsupported, Feature: `allowedTools entry "finder"`, Details: "OpenAI does not support tool_search tools in tool_choice.allowed_tools; the tool is removed from the allowed tools"},
				{Type: provider.WarnUnsupported, Feature: `allowedTools entry "tool_search"`, Details: "OpenAI does not support tool_search tools in tool_choice.allowed_tools; the tool is removed from the allowed tools"},
			},
		},
		{
			name: "unrecognized provider declaration is not selectable", tools: []provider.Tool{allowedProvider("openai.unrecognized", "mystery"), allowedFunction("weather")}, names: []string{"mystery"},
			want: []any{map[string]any{"type": "function", "name": "mystery"}},
			warnings: []provider.Warning{
				{Type: provider.WarnUnsupported, Feature: "tool", Details: "unsupported provider tool: openai.unrecognized"},
				{Type: provider.WarnUnsupported, Feature: `allowedTools entry "mystery"`, Details: "the tool is not part of the tools for this request and is sent as a function tool"},
			},
		},
		{
			name: "unknown name is sent", tools: []provider.Tool{allowedFunction("weather")}, names: []string{"absent"},
			want:     []any{map[string]any{"type": "function", "name": "absent"}},
			warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: `allowedTools entry "absent"`, Details: "the tool is not part of the tools for this request and is sent as a function tool"}},
		},
		{
			name: "unknown warning preserves name", tools: []provider.Tool{allowedFunction("weather")}, names: []string{`missing"tool`},
			want:     []any{map[string]any{"type": "function", "name": `missing"tool`}},
			warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: `allowedTools entry "missing"tool"`, Details: "the tool is not part of the tools for this request and is sent as a function tool"}},
		},
		{
			name: "unknown with provider declarations", tools: []provider.Tool{allowedProvider(toolIDWebSearch, "search"), allowedFunction("weather")}, names: []string{"unknown", "weather"},
			want:     []any{map[string]any{"type": "function", "name": "unknown"}, map[string]any{"type": "function", "name": "weather"}},
			warnings: []provider.Warning{{Type: provider.WarnUnsupported, Feature: `allowedTools entry "unknown"`, Details: "the tool is not part of the tools for this request and is sent as a function tool"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")}, Tools: tc.tools,
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: tc.names}}),
			})
			_, entries := allowedChoice(t, body)
			assert.Equal(t, tc.want, entries)
			assert.Equal(t, tc.warnings, warnings)
		})
	}
}

func TestPrepareTools_AllowedToolsEmptyAndNoDeclarations(t *testing.T) {
	deferLoading := true
	for _, tc := range []struct {
		name  string
		tools []provider.Tool
		names []string
		want  string
	}{
		{"empty", []provider.Tool{allowedFunction("weather")}, nil, "allowedTools with only tools that cannot be allow-listed ()"},
		{"all dropped in selection order", []provider.Tool{allowedProvider(toolIDToolSearch, "finder"), {Type: provider.ToolTypeFunction, Name: "deferred", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{DeferLoading: &deferLoading})}}, []string{"deferred", "finder", "deferred"}, "allowedTools with only tools that cannot be allow-listed (deferred, finder, deferred)"},
		{"ambiguous only", []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "alpha", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"one"`)}}, {Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "beta", Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"two"`)}}}, []string{"mcp"}, "allowedTools with only tools that cannot be allow-listed (mcp)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}, Tools: tc.tools,
				ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceAuto},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: tc.names}})}
			_, _, _, err := buildParams("gpt-4o", call)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
	for _, tc := range []struct {
		name string
		call provider.CallOptions
	}{
		{"allowed restriction", provider.CallOptions{ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{"absent"}}})}},
		{"forced choice", provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}}},
	} {
		t.Run("no tools "+tc.name, func(t *testing.T) {
			tc.call.Prompt = []provider.Message{provider.UserText("hi")}
			body, warnings := buildBody(t, "gpt-4o", tc.call)
			assert.Empty(t, warnings)
			assert.NotContains(t, body, "tools")
			assert.NotContains(t, body, "tool_choice")
		})
	}
}

func TestPrepareTools_AllowedToolsRequestModes(t *testing.T) {
	call := provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools:  []provider.Tool{allowedProvider(toolIDWebSearch, "search"), allowedFunction("weather")},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
			AllowedTools: &AllowedToolsOption{ToolNames: []string{"search", "weather"}},
		}),
	}
	before, err := json.Marshal(call)
	require.NoError(t, err)
	var requests []map[string]any
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		encoded, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(encoded, &body))
		requests = append(requests, body)
		response := `{"id":"resp_123","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`
		contentType := "application/json"
		if len(requests) == 2 {
			contentType = "text/event-stream"
			response = "event: response.completed\n" + `data: {"type":"response.completed","sequence_number":0,"response":{"id":"resp_123","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}` + "\n\n"
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
	})}
	model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
	generated, err := model.DoGenerate(t.Context(), call)
	require.NoError(t, err)
	assert.Empty(t, generated.Warnings)
	streamed, err := model.DoStream(t.Context(), call)
	require.NoError(t, err)
	for range streamed.Stream {
	}
	require.Len(t, requests, 2)
	for _, request := range requests {
		_, entries := allowedChoice(t, request)
		assert.Equal(t, []any{map[string]any{"type": "web_search"}, map[string]any{"type": "function", "name": "weather"}}, entries)
	}
	after, err := json.Marshal(call)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestPrepareTools_AllowedToolsRejectsBeforeHTTP(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("unexpected HTTP request")
	})}
	model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
	call := provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")}, Tools: []provider.Tool{allowedFunction("weather")},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{}}}),
	}
	_, err := model.DoGenerate(context.Background(), call)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allowedTools with only tools that cannot be allow-listed")
	_, err = model.DoStream(context.Background(), call)
	require.Error(t, err)
	assert.Zero(t, requests)
}
