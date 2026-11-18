package bedrock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConverse_NamedToolChoice(t *testing.T) {
	allTools := []string{"unused", "lookup"}
	onlyNamed := []string{"lookup"}
	const standardChoice = `{"tool":{"name":"lookup"}}`
	const anthropicChoice = `{"type":"tool","name":"lookup","disable_parallel_tool_use":true}`
	streamBody := encodeFixtures(t, `{"messageStart":{"role":"assistant"}}`, `{"messageStop":{"stopReason":"end_turn"}}`, `{"metadata":{"usage":{"inputTokens":1,"outputTokens":1}}}`)

	for _, tc := range []struct {
		name             string
		modelID          string
		parallelDisabled string
		providerTool     bool
		wantTools        []string
		wantStandard     string
		wantAnthropic    string
		wantWarning      bool
	}{
		{name: "Anthropic default", modelID: testAnthropicModel, wantTools: allTools, wantAnthropic: anthropicChoice},
		{name: "Anthropic parallel allowed", modelID: testAnthropicModel, parallelDisabled: "false", wantTools: allTools, wantStandard: standardChoice},
		{name: "Anthropic parallel disabled", modelID: testAnthropicModel, parallelDisabled: "true", wantTools: allTools, wantAnthropic: anthropicChoice},
		{name: "Anthropic with provider tool", modelID: testAnthropicModel, providerTool: true, wantTools: []string{"code_execution", "unused", "lookup"}, wantAnthropic: anthropicChoice},
		{name: "Anthropic with provider tool and parallel allowed", modelID: testAnthropicModel, parallelDisabled: "false", providerTool: true, wantTools: []string{"code_execution", "unused", "lookup"}, wantAnthropic: `{"type":"tool","name":"lookup","disable_parallel_tool_use":false}`},
		{name: "non-Anthropic", modelID: testOpenAIModel, wantTools: onlyNamed, wantStandard: standardChoice},
		{name: "Sonnet 5.5", modelID: testSonnet55Model, wantTools: onlyNamed, wantStandard: `{"auto":{}}`, wantWarning: true},
		{name: "Sonnet 5.5 parallel disabled", modelID: testSonnet55Model, parallelDisabled: "true", wantTools: onlyNamed, wantAnthropic: `{"type":"auto","disable_parallel_tool_use":true}`, wantWarning: true},
	} {
		for _, endpoint := range []string{"converse", "converse-stream"} {
			t.Run(tc.name+"/"+endpoint, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/model/"+tc.modelID+"/"+endpoint, r.URL.Path)
					var body struct {
						ToolConfig struct {
							Tools      []toolDefinition `json:"tools"`
							ToolChoice json.RawMessage  `json:"toolChoice"`
						} `json:"toolConfig"`
						AdditionalModelRequestFields map[string]json.RawMessage `json:"additionalModelRequestFields"`
					}
					if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
						http.Error(w, "invalid request body", http.StatusBadRequest)
						return
					}
					var names []string
					for _, tool := range body.ToolConfig.Tools {
						if assert.NotNil(t, tool.ToolSpec) {
							names = append(names, tool.ToolSpec.Name)
						}
					}
					assert.Equal(t, tc.wantTools, names, "tool order")
					if tc.wantStandard == "" {
						assert.Empty(t, body.ToolConfig.ToolChoice)
					} else {
						assert.JSONEq(t, tc.wantStandard, string(body.ToolConfig.ToolChoice))
					}
					if tc.wantAnthropic == "" {
						assert.NotContains(t, body.AdditionalModelRequestFields, "tool_choice")
					} else {
						assert.JSONEq(t, tc.wantAnthropic, string(body.AdditionalModelRequestFields["tool_choice"]))
					}
					if endpoint == "converse-stream" {
						w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
						_, err := w.Write(streamBody)
						assert.NoError(t, err)
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, err := w.Write([]byte(`{"output":{"message":{"role":"assistant","content":[{"text":"ok"}]}},"stopReason":"end_turn","usage":{"inputTokens":1,"outputTokens":1}}`))
						assert.NoError(t, err)
					}
				}))
				defer server.Close()

				model := New(tc.modelID, WithBaseURL(server.URL), WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")))
				opts := provider.CallOptions{
					Prompt: []provider.Message{provider.UserText("hi")},
					Tools: []provider.Tool{
						{Type: provider.ToolTypeFunction, Name: "unused", InputSchema: json.RawMessage(`{"type":"object"}`)},
						{Type: provider.ToolTypeFunction, Name: "lookup", InputSchema: json.RawMessage(`{"type":"object"}`)},
					},
					ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"},
				}
				if tc.providerTool {
					opts.Tools = append(opts.Tools, provider.Tool{Type: provider.ToolTypeProvider, ID: "anthropic.code_execution_20250522", Name: "code_execution"})
				}
				if tc.parallelDisabled != "" {
					opts.ProviderOptions = provider.ProviderOptions{
						"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"disableParallelToolUse":` + tc.parallelDisabled + `}`)},
					}
				}

				var warnings []provider.Warning
				if endpoint == "converse-stream" {
					result, err := model.DoStream(t.Context(), opts)
					require.NoError(t, err)
					for part := range result.Stream {
						if part.Type == provider.PartStreamStart {
							warnings = part.Warnings
						}
					}
				} else {
					result, err := model.DoGenerate(t.Context(), opts)
					require.NoError(t, err)
					warnings = result.Warnings
				}
				if tc.wantWarning {
					require.Len(t, warnings, 1)
					assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
					assert.Equal(t, "toolChoice", warnings[0].Feature)
					assert.Contains(t, warnings[0].Details, "Only the 'lookup' tool is sent with 'auto' tool choice.")
				} else {
					assert.Empty(t, warnings)
				}
			})
		}
	}
}
