package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func toolsArray(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["tools"].([]any)
	require.True(t, ok, "tools must be present")
	var out []map[string]any
	for _, e := range raw {
		out = append(out, e.(map[string]any))
	}
	return out
}

func TestPrepareTools_FunctionDeclaration(t *testing.T) {
	deferLoading := true
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type:            provider.ToolTypeFunction,
			Name:            "getWeather",
			Description:     "get weather",
			InputSchema:     json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
			ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{DeferLoading: &deferLoading}),
		}},
	})
	tools := toolsArray(t, body)
	require.Len(t, tools, 1)
	assert.Equal(t, "function", tools[0]["type"])
	assert.Equal(t, "getWeather", tools[0]["name"])
	assert.Equal(t, "get weather", tools[0]["description"])
	assert.Equal(t, true, tools[0]["defer_loading"])
	assert.NotNil(t, tools[0]["parameters"])
}

func TestPrepareTools_FunctionStrict(t *testing.T) {
	strictTrue := true
	strictFalse := false
	tests := []struct {
		name   string
		strict *bool
	}{
		{name: "absent"},
		{name: "true", strict: &strictTrue},
		{name: "false", strict: &strictFalse},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
				Tools: []provider.Tool{{
					Type:        provider.ToolTypeFunction,
					Name:        "getWeather",
					InputSchema: json.RawMessage(`{"type":"object"}`),
					Strict:      tc.strict,
				}},
			})
			tool := toolsArray(t, body)[0]
			got, ok := tool["strict"]
			require.True(t, ok)
			if tc.strict == nil {
				assert.Equal(t, false, got)
			} else {
				assert.Equal(t, *tc.strict, got)
			}
		})
	}
}

func TestPrepareTools_FunctionNamespace(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{
			{
				Type:        provider.ToolTypeFunction,
				Name:        "lookupCustomer",
				Description: "lookup customer",
				InputSchema: json.RawMessage(`{"type":"object"}`),
				ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{
					Namespace: &OpenAIToolNamespaceOptions{Name: "crm", Description: "CRM tools"},
				}),
			},
			{
				Type:        provider.ToolTypeFunction,
				Name:        "updateCustomer",
				Description: "update customer",
				InputSchema: json.RawMessage(`{"type":"object"}`),
				ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{
					Namespace: &OpenAIToolNamespaceOptions{Name: "crm", Description: "CRM tools"},
				}),
			},
		},
	})
	tools := toolsArray(t, body)
	require.Len(t, tools, 1)
	namespace := tools[0]
	assert.Equal(t, "namespace", namespace["type"])
	assert.Equal(t, "crm", namespace["name"])
	assert.Equal(t, "CRM tools", namespace["description"])
	nested := namespace["tools"].([]any)
	require.Len(t, nested, 2)
	assert.Equal(t, "lookupCustomer", nested[0].(map[string]any)["name"])
	assert.Equal(t, "updateCustomer", nested[1].(map[string]any)["name"])
}

func TestPrepareTools_FunctionNamespaceConflictingDescription(t *testing.T) {
	_, _, _, err := buildParams("gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{
			{
				Type: provider.ToolTypeFunction,
				Name: "a",
				ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{
					Namespace: &OpenAIToolNamespaceOptions{Name: "crm", Description: "CRM tools"},
				}),
			},
			{
				Type: provider.ToolTypeFunction,
				Name: "b",
				ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{
					Namespace: &OpenAIToolNamespaceOptions{Name: "crm", Description: "Other tools"},
				}),
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting descriptions")
}

func TestPrepareTools_WebSearchAutoIncludesSources(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   toolIDWebSearch,
			Name: "web_search",
		}},
	})
	tools := toolsArray(t, body)
	require.Len(t, tools, 1)
	assert.Equal(t, "web_search", tools[0]["type"])
	include := toStringSlice(body["include"])
	assert.Contains(t, include, "web_search_call.action.sources")
}

func TestPrepareTools_WebSearchSourcesIncludePrecedence(t *testing.T) {
	yes, no := true, false
	const sources = "web_search_call.action.sources"
	tests := []struct {
		name     string
		toolID   string
		support  *bool
		perCall  *bool
		explicit bool
		want     bool
	}{
		{name: "default web search", toolID: toolIDWebSearch, want: true},
		{name: "default preview", toolID: toolIDWebSearchPreview, want: true},
		{name: "explicit true", toolID: toolIDWebSearch, perCall: &yes, want: true},
		{name: "explicit false", toolID: toolIDWebSearch, perCall: &no},
		{name: "enabled capability", toolID: toolIDWebSearch, support: &yes, want: true},
		{name: "disabled capability", toolID: toolIDWebSearch, support: &no},
		{name: "disabled capability with true", toolID: toolIDWebSearch, support: &no, perCall: &yes},
		{name: "disabled capability with false", toolID: toolIDWebSearch, support: &no, perCall: &no},
		{name: "no web tool", perCall: &yes},
		{name: "explicit source with per-call false", toolID: toolIDWebSearch, perCall: &no, explicit: true, want: true},
		{name: "explicit source with disabled capability", toolID: toolIDWebSearchPreview, support: &no, explicit: true, want: true},
		{name: "explicit source with both disabled", toolID: toolIDWebSearch, support: &no, perCall: &no, explicit: true, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var options []Option
			if tc.support != nil {
				options = append(options, WithWebSearchSourcesIncludeSupport(*tc.support))
			}
			call := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}}
			if tc.toolID != "" {
				call.Tools = []provider.Tool{{Type: provider.ToolTypeProvider, ID: tc.toolID, Name: "search"}}
			}
			popts := OpenAIResponsesOptions{IncludeWebSearchSources: tc.perCall}
			if tc.explicit {
				popts.Include = []string{sources, sources}
			}
			call.ProviderOptions = withOpenAIOptions(popts)

			params, warnings, _, err := newModel("gpt-4o", options...).buildParams(call)
			require.NoError(t, err)
			require.Empty(t, warnings)
			encoded, err := json.Marshal(params)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(encoded, &body))
			if tc.toolID != "" {
				tools := toolsArray(t, body)
				require.Len(t, tools, 1)
				wantType := "web_search"
				if tc.toolID == toolIDWebSearchPreview {
					wantType = "web_search_preview"
				}
				assert.Equal(t, wantType, tools[0]["type"])
			}
			if tc.want {
				assert.Equal(t, []string{sources}, toStringSlice(body["include"]))
			} else {
				assert.NotContains(t, toStringSlice(body["include"]), sources)
			}
		})
	}
}

func TestPrepareTools_WebSearchOptOutPreservesOtherIncludes(t *testing.T) {
	no := false
	logprobs := int64(3)
	params, warnings, _, err := newModel("gpt-5", WithWebSearchSourcesIncludeSupport(false)).buildParams(provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeProvider, ID: toolIDWebSearch, Name: "search"},
			{Type: provider.ToolTypeProvider, ID: toolIDCodeInterpreter, Name: "python"},
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
			Store: &no, Logprobs: &LogprobsOption{Int: &logprobs},
		}),
	})
	require.NoError(t, err)
	require.Empty(t, warnings)
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(encoded, &body))
	assert.Equal(t, []string{
		"reasoning.encrypted_content", "code_interpreter_call.outputs", "message.output_text.logprobs",
	}, toStringSlice(body["include"]))
	assert.EqualValues(t, 3, body["top_logprobs"])
	assert.Len(t, toolsArray(t, body), 2)
}

func TestPrepareTools_WebSearchBlockedDomainsOnly(t *testing.T) {
	body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   toolIDWebSearch,
			Name: "web_search",
			Args: map[string]json.RawMessage{
				"filters": json.RawMessage(`{"blockedDomains":["example.com"]}`),
			},
		}},
	})
	require.Empty(t, warnings)

	tools := toolsArray(t, body)
	require.Len(t, tools, 1)
	filters := tools[0]["filters"].(map[string]any)
	assert.NotContains(t, filters, "allowed_domains")
	assert.Equal(t, []any{"example.com"}, filters["blocked_domains"])
}

func TestPrepareTools_WebSearchPreservesEmptyFilters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filters string
		check   func(*testing.T, map[string]any)
	}{
		{name: "empty object", filters: `{}`, check: func(t *testing.T, filters map[string]any) {
			assert.Empty(t, filters)
		}},
		{name: "empty arrays", filters: `{"allowedDomains":[],"blockedDomains":[]}`, check: func(t *testing.T, filters map[string]any) {
			assert.Equal(t, []any{}, filters["allowed_domains"])
			assert.Equal(t, []any{}, filters["blocked_domains"])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
				Tools: []provider.Tool{{
					Type: provider.ToolTypeProvider,
					ID:   toolIDWebSearch,
					Name: "web_search",
					Args: map[string]json.RawMessage{"filters": json.RawMessage(tc.filters)},
				}},
			})
			require.Empty(t, warnings)
			tools := toolsArray(t, body)
			require.Len(t, tools, 1)
			filters, ok := tools[0]["filters"].(map[string]any)
			require.True(t, ok)
			tc.check(t, filters)
		})
	}
}

func TestPrepareTools_CodeInterpreterAutoIncludesOutputs(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   toolIDCodeInterpreter,
			Name: "code_interpreter",
		}},
	})
	include := toStringSlice(body["include"])
	assert.Contains(t, include, "code_interpreter_call.outputs")
}

func TestPrepareTools_RareProviderTools(t *testing.T) {
	cases := []struct {
		id       string
		wantType string
	}{
		{toolIDImageGeneration, "image_generation"},
		{toolIDLocalShell, "local_shell"},
		{toolIDShell, "shell"},
		{toolIDApplyPatch, "apply_patch"},
		{toolIDComputer, "computer"},
		{toolIDToolSearch, "tool_search"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
				Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: tc.id, Name: tc.wantType}},
			})
			tools := toolsArray(t, body)
			require.Len(t, tools, 1)
			assert.Equal(t, tc.wantType, tools[0]["type"])
			assert.Empty(t, warnings)
		})
	}
}

func TestPrepareTools_MCPRequireApprovalDefault(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   toolIDMCP,
			Name: "mcp",
			Args: map[string]json.RawMessage{"serverLabel": json.RawMessage(`"srv"`)},
		}},
	})
	tools := toolsArray(t, body)
	require.Len(t, tools, 1)
	assert.Equal(t, "mcp", tools[0]["type"])
	assert.Equal(t, "srv", tools[0]["server_label"])
	assert.Equal(t, "never", tools[0]["require_approval"])
}

func TestPrepareTools_MCPConnector(t *testing.T) {
	for _, connectorID := range []string{"", "connector_googledrive"} {
		t.Run(connectorID, func(t *testing.T) {
			body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
				Tools: []provider.Tool{{
					Type: provider.ToolTypeProvider,
					ID:   toolIDMCP,
					Name: "mcp",
					Args: map[string]json.RawMessage{
						"serverLabel": json.RawMessage(`"srv"`),
						"connectorId": json.RawMessage(`"` + connectorID + `"`),
					},
				}},
			})
			assert.Empty(t, warnings)
			tools := toolsArray(t, body)
			require.Len(t, tools, 1)
			if connectorID == "" {
				assert.NotContains(t, tools[0], "connector_id")
			} else {
				assert.Equal(t, connectorID, tools[0]["connector_id"])
			}
			assert.Equal(t, "srv", tools[0]["server_label"])
			assert.Equal(t, "never", tools[0]["require_approval"])
		})
	}
}

func TestPrepareTools_ProviderToolArgs(t *testing.T) {
	body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDFileSearch,
				Name: "docs",
				Args: map[string]json.RawMessage{
					"vectorStoreIds": json.RawMessage(`["vs_1"]`),
					"maxNumResults":  json.RawMessage(`7`),
					"ranking":        json.RawMessage(`{"ranker":"auto","scoreThreshold":0.25}`),
					"filters":        json.RawMessage(`{"type":"eq","key":"kind","value":"runbook"}`),
				},
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDWebSearch,
				Name: "search",
				Args: map[string]json.RawMessage{
					"externalWebAccess": json.RawMessage(`true`),
					"filters":           json.RawMessage(`{"allowedDomains":["grafana.com"],"blockedDomains":["example.com"]}`),
					"searchContextSize": json.RawMessage(`"high"`),
					"userLocation":      json.RawMessage(`{"type":"approximate","country":"US","city":"New York"}`),
				},
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDCodeInterpreter,
				Name: "python",
				Args: map[string]json.RawMessage{
					"container": json.RawMessage(`{"fileIds":["file_1"]}`),
				},
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDImageGeneration,
				Name: "draw",
				Args: map[string]json.RawMessage{
					"background":        json.RawMessage(`"transparent"`),
					"inputFidelity":     json.RawMessage(`"high"`),
					"inputImageMask":    json.RawMessage(`{"fileId":"file_mask","imageUrl":"data:image/png;base64,abc"}`),
					"model":             json.RawMessage(`"gpt-image-1"`),
					"outputCompression": json.RawMessage(`80`),
					"outputFormat":      json.RawMessage(`"webp"`),
					"partialImages":     json.RawMessage(`2`),
					"quality":           json.RawMessage(`"high"`),
					"size":              json.RawMessage(`"1024x1024"`),
				},
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDMCP,
				Name: "mcp",
				Args: map[string]json.RawMessage{
					"serverLabel":       json.RawMessage(`"srv"`),
					"serverUrl":         json.RawMessage(`"https://mcp.example.com"`),
					"serverDescription": json.RawMessage(`"docs server"`),
					"headers":           json.RawMessage(`{"x-test":"1"}`),
					"allowedTools":      json.RawMessage(`{"readOnly":true,"toolNames":["lookup"]}`),
					"requireApproval":   json.RawMessage(`{"never":{"toolNames":["lookup"]}}`),
				},
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDToolSearch,
				Name: "toolSearch",
				Args: map[string]json.RawMessage{
					"description": json.RawMessage(`"find tools"`),
					"execution":   json.RawMessage(`"client"`),
					"parameters":  json.RawMessage(`{"type":"object"}`),
				},
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   toolIDCustom,
				Name: "freeform",
				Args: map[string]json.RawMessage{
					"description": json.RawMessage(`"freeform input"`),
					"format":      json.RawMessage(`{"type":"grammar","syntax":"lark","definition":"start: /.+/"}`),
				},
			},
		},
	})
	require.Empty(t, warnings)

	tools := toolsArray(t, body)
	require.Len(t, tools, 7)

	fileSearch := tools[0]
	assert.Equal(t, "file_search", fileSearch["type"])
	assert.Equal(t, []any{"vs_1"}, fileSearch["vector_store_ids"])
	assert.Equal(t, float64(7), fileSearch["max_num_results"])
	assert.Equal(t, "auto", fileSearch["ranking_options"].(map[string]any)["ranker"])
	assert.Equal(t, 0.25, fileSearch["ranking_options"].(map[string]any)["score_threshold"])
	assert.Equal(t, "eq", fileSearch["filters"].(map[string]any)["type"])

	webSearch := tools[1]
	assert.Equal(t, "web_search", webSearch["type"])
	assert.Equal(t, true, webSearch["external_web_access"])
	assert.Equal(t, "high", webSearch["search_context_size"])
	assert.Equal(t, []any{"grafana.com"}, webSearch["filters"].(map[string]any)["allowed_domains"])
	assert.Equal(t, []any{"example.com"}, webSearch["filters"].(map[string]any)["blocked_domains"])
	assert.Equal(t, "US", webSearch["user_location"].(map[string]any)["country"])

	codeInterpreter := tools[2]
	assert.Equal(t, "code_interpreter", codeInterpreter["type"])
	assert.Equal(t, []any{"file_1"}, codeInterpreter["container"].(map[string]any)["file_ids"])

	imageGeneration := tools[3]
	assert.Equal(t, "image_generation", imageGeneration["type"])
	assert.Equal(t, "transparent", imageGeneration["background"])
	assert.Equal(t, "high", imageGeneration["input_fidelity"])
	assert.Equal(t, "file_mask", imageGeneration["input_image_mask"].(map[string]any)["file_id"])
	assert.Equal(t, float64(80), imageGeneration["output_compression"])
	assert.Equal(t, float64(2), imageGeneration["partial_images"])

	mcp := tools[4]
	assert.Equal(t, "mcp", mcp["type"])
	assert.Equal(t, "srv", mcp["server_label"])
	assert.Equal(t, "https://mcp.example.com", mcp["server_url"])
	assert.Equal(t, "docs server", mcp["server_description"])
	assert.Equal(t, "1", mcp["headers"].(map[string]any)["x-test"])
	assert.Equal(t, true, mcp["allowed_tools"].(map[string]any)["read_only"])
	assert.NotNil(t, mcp["require_approval"])

	toolSearch := tools[5]
	assert.Equal(t, "tool_search", toolSearch["type"])
	assert.Equal(t, "find tools", toolSearch["description"])
	assert.Equal(t, "client", toolSearch["execution"])
	assert.Equal(t, "object", toolSearch["parameters"].(map[string]any)["type"])

	custom := tools[6]
	assert.Equal(t, "custom", custom["type"])
	assert.Equal(t, "freeform", custom["name"])
	assert.Equal(t, "freeform input", custom["description"])
	assert.Equal(t, "grammar", custom["format"].(map[string]any)["type"])
}

func TestPrepareTools_HostedToolChoiceUsesProviderName(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt:     []provider.Message{provider.UserText("hi")},
		ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "docs"},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   toolIDFileSearch,
			Name: "docs",
			Args: map[string]json.RawMessage{"vectorStoreIds": json.RawMessage(`["vs_1"]`)},
		}},
	})
	assert.Equal(t, map[string]any{"type": "file_search"}, body["tool_choice"])
}

func TestPrepareTools_ProviderToolChoiceVariants(t *testing.T) {
	tests := []struct {
		tool      provider.Tool
		canonical string
		want      map[string]any
	}{
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDShell, Name: "terminal"}, "shell", map[string]any{"type": "function", "name": "shell"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDLocalShell, Name: "localTerminal"}, "local_shell", map[string]any{"type": "function", "name": "local_shell"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "discover"}, "tool_search", map[string]any{"type": "function", "name": "tool_search"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDCodeInterpreter, Name: "python"}, "code_interpreter", map[string]any{"type": "code_interpreter"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDFileSearch, Name: "docs"}, "file_search", map[string]any{"type": "file_search"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDImageGeneration, Name: "draw"}, "image_generation", map[string]any{"type": "image_generation"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDWebSearchPreview, Name: "preview"}, "web_search_preview", map[string]any{"type": "web_search_preview"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDWebSearch, Name: "search"}, "web_search", map[string]any{"type": "web_search"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDMCP, Name: "server"}, "mcp", map[string]any{"type": "mcp"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDApplyPatch, Name: "patch"}, "apply_patch", map[string]any{"type": "apply_patch"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}, "computer", map[string]any{"type": "computer"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDProgrammatic, Name: "program"}, "programmatic_tool_calling", map[string]any{"type": "programmatic_tool_calling"}},
		{provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "freeform"}, "freeform", map[string]any{"type": "custom", "name": "freeform"}},
	}

	for _, tc := range tests {
		t.Run(tc.canonical, func(t *testing.T) {
			for _, selection := range []struct {
				name     string
				declared string
				selected string
			}{
				{"canonical declaration", tc.canonical, tc.canonical},
				{"canonical selection", tc.tool.Name, tc.canonical},
				{"alias selection", tc.tool.Name, tc.tool.Name},
			} {
				t.Run(selection.name, func(t *testing.T) {
					tool := tc.tool
					tool.Name = selection.declared
					body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
						Prompt:     []provider.Message{provider.UserText("hi")},
						ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: selection.selected},
						Tools:      []provider.Tool{tool},
					})
					assert.Empty(t, warnings)
					assert.Equal(t, tc.want, body["tool_choice"])
					declarations := toolsArray(t, body)
					require.Len(t, declarations, 1)
					if tool.ID == toolIDCustom {
						assert.Equal(t, "custom", declarations[0]["type"])
						assert.Equal(t, tool.Name, declarations[0]["name"])
					} else {
						assert.Equal(t, tc.canonical, declarations[0]["type"])
					}
				})
			}
		})
	}
}

func TestPrepareTools_AllowedToolsMapsProviderToolNames(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   toolIDFileSearch,
			Name: "docs",
			Args: map[string]json.RawMessage{"vectorStoreIds": json.RawMessage(`["vs_1"]`)},
		}},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
			AllowedTools: &AllowedToolsOption{ToolNames: []string{"docs"}},
		}),
	})
	toolChoice := body["tool_choice"].(map[string]any)
	tools := toolChoice["tools"].([]any)
	require.Len(t, tools, 1)
	assert.Equal(t, map[string]any{"type": "file_search"}, tools[0])
}

func TestPrepareTools_UnknownProviderToolWarning(t *testing.T) {
	body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		Tools: []provider.Tool{{
			Type: provider.ToolTypeProvider,
			ID:   "openai.unknown_tool",
			Name: "unknown",
		}},
	})
	_, hasTools := body["tools"]
	assert.False(t, hasTools, "unknown tool should not appear in tools")
	assert.Contains(t, warningFeatures(warnings), "tool")
}

func TestPrepareTools_ToolChoiceVariants(t *testing.T) {
	mk := func(tc *provider.ToolChoice, opts ...OpenAIResponsesOptions) map[string]any {
		co := provider.CallOptions{
			Prompt:     []provider.Message{provider.UserText("hi")},
			ToolChoice: tc,
			Tools: []provider.Tool{{
				Type: provider.ToolTypeFunction, Name: "getWeather", InputSchema: json.RawMessage(`{"type":"object"}`),
			}},
		}
		if len(opts) > 0 {
			co.ProviderOptions = withOpenAIOptions(opts[0])
		}
		body, _ := buildBody(t, "gpt-4o", co)
		return body
	}

	t.Run("auto", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceAuto})
		assert.Equal(t, "auto", body["tool_choice"])
	})
	t.Run("none", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceNone})
		assert.Equal(t, "none", body["tool_choice"])
	})
	t.Run("required", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceRequired})
		assert.Equal(t, "required", body["tool_choice"])
	})
	t.Run("specific function tool", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "getWeather"})
		assert.Equal(t, map[string]any{"type": "function", "name": "getWeather"}, body["tool_choice"])
	})
	t.Run("unmapped name", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "absent"})
		assert.Equal(t, map[string]any{"type": "function", "name": "absent"}, body["tool_choice"])
	})
	t.Run("allowedTools overrides tool choice", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceAuto}, OpenAIResponsesOptions{
			AllowedTools: &AllowedToolsOption{ToolNames: []string{"getWeather"}, Mode: "required"},
		})
		tc := body["tool_choice"].(map[string]any)
		assert.Equal(t, "allowed_tools", tc["type"])
		assert.Equal(t, "required", tc["mode"])
		allowed := tc["tools"].([]any)
		require.Len(t, allowed, 1)
		assert.Equal(t, "getWeather", allowed[0].(map[string]any)["name"])
	})
}
