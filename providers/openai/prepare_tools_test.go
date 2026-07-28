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
			if tc.strict == nil {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, *tc.strict, got)
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
					"filters":           json.RawMessage(`{"allowedDomains":["grafana.com"]}`),
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
	tc := body["tool_choice"].(map[string]any)
	assert.Equal(t, "file_search", tc["type"])
}

func TestPrepareTools_ProviderToolChoiceVariants(t *testing.T) {
	tests := []struct {
		name     string
		tool     provider.Tool
		choice   string
		wantType string
	}{
		{
			name:     "custom provider tool",
			tool:     provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "freeform"},
			choice:   "freeform",
			wantType: "custom",
		},
		{
			name:     "shell provider tool",
			tool:     provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDShell, Name: "terminal"},
			choice:   "terminal",
			wantType: "shell",
		},
		{
			name:     "apply patch provider tool",
			tool:     provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDApplyPatch, Name: "patch"},
			choice:   "patch",
			wantType: "apply_patch",
		},
		{
			name:     "computer provider tool",
			tool:     provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"},
			choice:   "browser",
			wantType: "computer",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt:     []provider.Message{provider.UserText("hi")},
				ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: tc.choice},
				Tools:      []provider.Tool{tc.tool},
			})
			toolChoice := body["tool_choice"].(map[string]any)
			assert.Equal(t, tc.wantType, toolChoice["type"])
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
	assert.Equal(t, "file_search", tools[0].(map[string]any)["name"])
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
	t.Run("required", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceRequired})
		assert.Equal(t, "required", body["tool_choice"])
	})
	t.Run("specific function tool", func(t *testing.T) {
		body := mk(&provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "getWeather"})
		tc := body["tool_choice"].(map[string]any)
		assert.Equal(t, "function", tc["type"])
		assert.Equal(t, "getWeather", tc["name"])
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
