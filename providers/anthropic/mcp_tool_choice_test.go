package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_MCPToolsRespectEffectiveToolChoice(t *testing.T) {
	const (
		openObject     = `{"type":"object","additionalProperties":true}`
		closedObject   = `{"type":"object","properties":{},"additionalProperties":false}`
		autoChoice     = `{"type":"auto"}`
		jsonAutoChoice = `{"type":"auto","disable_parallel_tool_use":true}`
		anyChoice      = `{"type":"any","disable_parallel_tool_use":true}`
		namedChoice    = `{"type":"tool","name":"lookup"}`
		noneChoice     = `{"type":"none"}`
		preservedMCP   = `[
			{"type":"url","name":"default","url":"https://default.invalid/mcp"},
			{"type":"url","name":"enabled","url":"https://enabled.invalid/mcp","authorization_token":"test-token","tool_configuration":{"enabled":true,"allowed_tools":["lookup","other"]}},
			{"type":"url","name":"empty","url":"https://empty.invalid/mcp","tool_configuration":{"allowed_tools":[]}},
			{"type":"url","name":"disabled","url":"https://disabled.invalid/mcp","tool_configuration":{"enabled":false}}
		]`
		disabledMCP = `[
			{"type":"url","name":"default","url":"https://default.invalid/mcp","tool_configuration":{"enabled":false}},
			{"type":"url","name":"enabled","url":"https://enabled.invalid/mcp","authorization_token":"test-token","tool_configuration":{"enabled":false,"allowed_tools":["lookup","other"]}},
			{"type":"url","name":"empty","url":"https://empty.invalid/mcp","tool_configuration":{"enabled":false,"allowed_tools":[]}},
			{"type":"url","name":"disabled","url":"https://disabled.invalid/mcp","tool_configuration":{"enabled":false}}
		]`
	)
	newServers := func() []MCPServer {
		return []MCPServer{
			{Name: "default", URL: "https://default.invalid/mcp"},
			{Name: "enabled", URL: "https://enabled.invalid/mcp", AuthorizationToken: new("test-token"), ToolConfiguration: &MCPToolConfiguration{Enabled: new(true), AllowedTools: []string{"lookup", "other"}}},
			{Name: "empty", URL: "https://empty.invalid/mcp", ToolConfiguration: &MCPToolConfiguration{AllowedTools: []string{}}},
			{Name: "disabled", URL: "https://disabled.invalid/mcp", ToolConfiguration: &MCPToolConfiguration{Enabled: new(false)}},
		}
	}
	manual := &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 1024}
	cases := []struct {
		name, modelID, schema string
		mode                  StructuredOutputMode
		choice                provider.ToolChoiceType
		thinking              *ThinkingConfig
		wantChoice, wantMCP   string
		jsonTool              bool
	}{
		{name: "named_on_rejecting_model", modelID: "claude-sonnet-5-5", choice: provider.ToolChoiceTool, wantChoice: autoChoice, wantMCP: disabledMCP},
		{name: "named_with_manual_thinking", choice: provider.ToolChoiceTool, thinking: manual, wantChoice: autoChoice, wantMCP: disabledMCP},
		{name: "named_with_automatic_json_tool", choice: provider.ToolChoiceTool, schema: openObject, wantChoice: anyChoice, wantMCP: disabledMCP, jsonTool: true},
		{name: "none_with_automatic_json_tool", choice: provider.ToolChoiceNone, schema: openObject, wantChoice: anyChoice, wantMCP: disabledMCP, jsonTool: true},
		{name: "named_with_explicit_json_tool", choice: provider.ToolChoiceTool, schema: closedObject, mode: StructuredOutputJSONTool, wantChoice: anyChoice, wantMCP: disabledMCP, jsonTool: true},
		{name: "none_with_explicit_json_tool", choice: provider.ToolChoiceNone, schema: closedObject, mode: StructuredOutputJSONTool, wantChoice: anyChoice, wantMCP: disabledMCP, jsonTool: true},
		{name: "none_keeps_json_tool_on_rejecting_model", modelID: "claude-sonnet-5-5", choice: provider.ToolChoiceNone, schema: openObject, mode: StructuredOutputJSONTool, wantChoice: jsonAutoChoice, wantMCP: disabledMCP, jsonTool: true},
		{name: "none_kept_when_json_tool_switches_to_native_output", modelID: "claude-sonnet-5-5", choice: provider.ToolChoiceNone, schema: closedObject, mode: StructuredOutputJSONTool, wantChoice: noneChoice, wantMCP: preservedMCP},
		{name: "default_with_json_tool", schema: openObject, wantChoice: anyChoice, wantMCP: preservedMCP, jsonTool: true},
		{name: "auto_with_json_tool", choice: provider.ToolChoiceAuto, schema: openObject, wantChoice: anyChoice, wantMCP: preservedMCP, jsonTool: true},
		{name: "required_with_json_tool", choice: provider.ToolChoiceRequired, schema: openObject, wantChoice: anyChoice, wantMCP: preservedMCP, jsonTool: true},
		{name: "required_on_rejecting_model", modelID: "claude-sonnet-5-5", choice: provider.ToolChoiceRequired, wantChoice: autoChoice, wantMCP: preservedMCP},
		{name: "enforced_named", choice: provider.ToolChoiceTool, wantChoice: namedChoice, wantMCP: preservedMCP},
		{name: "enforced_none", choice: provider.ToolChoiceNone, wantChoice: noneChoice, wantMCP: preservedMCP},
		{name: "enforced_named_with_native_output", choice: provider.ToolChoiceTool, schema: closedObject, wantChoice: namedChoice, wantMCP: preservedMCP},
		{name: "enforced_none_with_native_output", choice: provider.ToolChoiceNone, schema: closedObject, wantChoice: noneChoice, wantMCP: preservedMCP},
	}
	for _, stream := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("stream=%t/%s", stream, tc.name), func(t *testing.T) {
				modelID := tc.modelID
				if modelID == "" {
					modelID = "claude-sonnet-4-5"
				}
				var wire struct {
					ToolChoice   json.RawMessage `json:"tool_choice"`
					MCPServers   json.RawMessage `json:"mcp_servers"`
					OutputConfig struct {
						Format json.RawMessage `json:"format"`
					} `json:"output_config"`
				}
				client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
						return nil, err
					}
					assert.Contains(t, r.Header.Get("Anthropic-Beta"), "mcp-client-2025-04-04")
					return structuredOutputResponse(r, modelID, `{}`, tc.jsonTool, stream), nil
				})}
				m := New("test-key", modelID, WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
				servers := newServers()
				opts := provider.CallOptions{
					MaxOutputTokens: new(2048),
					Prompt:          []provider.Message{provider.UserText("Look up the answer.")},
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{MCPServers: servers, Thinking: tc.thinking, StructuredOutputMode: tc.mode}),
					Tools:           []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(closedObject)}},
				}
				if tc.choice != "" {
					opts.ToolChoice = &provider.ToolChoice{Type: tc.choice}
					if tc.choice == provider.ToolChoiceTool {
						opts.ToolChoice.ToolName = "lookup"
					}
				}
				if tc.schema != "" {
					opts.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(tc.schema)}
				}
				if stream {
					result, err := m.DoStream(t.Context(), opts)
					require.NoError(t, err)
					for part := range result.Stream {
						assert.NotEqual(t, provider.PartError, part.Type)
					}
				} else {
					_, err := m.DoGenerate(t.Context(), opts)
					require.NoError(t, err)
				}
				assert.JSONEq(t, tc.wantChoice, string(wire.ToolChoice))
				assert.JSONEq(t, tc.wantMCP, string(wire.MCPServers))
				assert.Equal(t, tc.schema != "" && !tc.jsonTool, len(wire.OutputConfig.Format) != 0)
				assert.Equal(t, newServers(), servers, "caller MCP configuration changed")
			})
		}
	}
}
