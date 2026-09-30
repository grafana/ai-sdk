package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForcedToolChoice_RequestSnapshots(t *testing.T) {
	for _, tc := range []struct {
		id        string
		canonical string
		alias     string
	}{
		{toolIDShell, "shell", "terminal"},
		{toolIDLocalShell, "local_shell", "localTerminal"},
		{toolIDToolSearch, "tool_search", "discover"},
	} {
		t.Run(tc.canonical, func(t *testing.T) {
			for _, selection := range []struct {
				name string
				tool string
			}{
				{"canonical", tc.canonical},
				{"alias", tc.alias},
			} {
				for _, mode := range []string{"generate", "stream"} {
					t.Run(selection.name+"/"+mode, func(t *testing.T) {
						golden, err := os.ReadFile(filepath.Join("testdata", "forced_tool_choice", tc.canonical+"-"+selection.name+"-"+mode+".json"))
						require.NoError(t, err)
						var expected map[string]any
						require.NoError(t, json.Unmarshal(golden, &expected))

						var body map[string]any
						requests := 0
						client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
							requests++
							encoded, err := io.ReadAll(req.Body)
							require.NoError(t, err)
							require.NoError(t, json.Unmarshal(encoded, &body))
							response := `{"id":"resp_test","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}`
							contentType := "application/json"
							if mode == "stream" {
								contentType = "text/event-stream"
								response = "event: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":0,\"response\":" + response + "}\n\n"
							}
							return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
						})}
						model := NewResponses("test-key", "gpt-4o", WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0)))
						call := provider.CallOptions{
							Prompt:     []provider.Message{provider.UserText("hi")},
							Tools:      []provider.Tool{{Type: provider.ToolTypeProvider, ID: tc.id, Name: tc.alias}},
							ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: selection.tool},
						}
						if mode == "generate" {
							result, err := model.DoGenerate(t.Context(), call)
							require.NoError(t, err)
							assert.Empty(t, result.Warnings)
						} else {
							result, err := model.DoStream(t.Context(), call)
							require.NoError(t, err)
							finished := false
							for part := range result.Stream {
								assert.NotEqual(t, provider.PartError, part.Type, "%v", part.APICallError)
								if part.Type == provider.PartFinish {
									finished = true
								}
							}
							assert.True(t, finished)
						}
						require.Equal(t, 1, requests)
						projection := map[string]any{"tools": body["tools"], "tool_choice": body["tool_choice"]}
						if stream, ok := body["stream"]; ok {
							projection["stream"] = stream
						}
						assert.Equal(t, expected, projection)
					})
				}
			}
		})
	}
}

func TestForcedToolChoice_AllowedToolsOverride(t *testing.T) {
	for _, tc := range []struct {
		id        string
		canonical string
		alias     string
	}{
		{toolIDShell, "shell", "terminal"},
		{toolIDLocalShell, "local_shell", "localTerminal"},
		{toolIDToolSearch, "tool_search", "discover"},
	} {
		for _, selected := range []string{tc.canonical, tc.alias} {
			t.Run(selected, func(t *testing.T) {
				body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
					Prompt:     []provider.Message{provider.UserText("hi")},
					Tools:      []provider.Tool{allowedProvider(tc.id, tc.alias), allowedFunction("weather")},
					ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: selected},
					ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
						AllowedTools: &AllowedToolsOption{ToolNames: []string{selected, "weather"}},
					}),
				})
				entries := []any{map[string]any{"type": tc.canonical}, map[string]any{"type": "function", "name": "weather"}}
				if tc.id == toolIDToolSearch {
					entries = entries[1:]
					assert.Equal(t, []provider.Warning{{
						Type:    provider.WarnUnsupported,
						Feature: `allowedTools entry "` + selected + `"`,
						Details: "OpenAI does not support tool_search tools in tool_choice.allowed_tools; the tool is removed from the allowed tools",
					}}, warnings)
				} else {
					assert.Empty(t, warnings)
				}
				assert.Equal(t, map[string]any{"type": "allowed_tools", "mode": "auto", "tools": entries}, body["tool_choice"])
			})
		}
	}
}
