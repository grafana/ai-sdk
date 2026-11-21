package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
	vertexsdk "github.com/anthropics/anthropic-sdk-go/v2/vertex"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func TestModel_AutomaticStructuredOutput(t *testing.T) {
	const openObject = `{"type":"object","additionalProperties":true}`
	const closedObject = `{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}`
	manualThinking := &ThinkingConfig{Type: ThinkingEnabled, BudgetTokens: 1024}
	cases := []struct {
		name, schema, output, toolChoice string
		mode                             StructuredOutputMode
		modelID                          string
		thinking                         *ThinkingConfig
		textFallback                     bool
	}{
		{
			name: "closed", schema: closedObject, output: `{"answer":"42"}`,
		},
		{
			name: "open", schema: openObject, output: `{"answer":42}`, toolChoice: "any",
		},
		{
			name: "map", schema: `{"type":"object","properties":{"scores":{"type":"object","additionalProperties":{"type":"number"}}},"required":["scores"],"additionalProperties":false}`,
			output: `{"scores":{"alice":42}}`, toolChoice: "any",
		},
		{
			name: "recursive", schema: `{"$ref":"#/$defs/node","$defs":{"node":{"type":"object","properties":{"next":{"$ref":"#/$defs/node"}},"additionalProperties":false}}}`,
			output: `{"next":{}}`, toolChoice: "any",
		},
		{
			name: "11_optional", schema: `{"type":"object","properties":{
				"a":{"type":"string"},"b":{"type":"string"},"c":{"type":"string"},"d":{"type":"string"},
				"e":{"type":"string"},"f":{"type":"string"},"g":{"type":"string"},"h":{"type":"string"},
				"i":{"type":"string"},"j":{"type":"string"},"k":{"type":"string"}},"additionalProperties":false}`,
			output: `{"k":"present"}`, toolChoice: "any",
		},
		{
			name: "sonnet_5_5", schema: openObject, output: `{"answer":42}`, toolChoice: "auto",
			mode: StructuredOutputAuto, modelID: "claude-sonnet-5-5",
		},
		{
			name: "explicit_output_format", schema: openObject, output: `{}`, mode: StructuredOutputFormat,
		},
		{
			name: "manual_thinking_text_fallback", modelID: "claude-3-haiku", textFallback: true,
			schema: closedObject, output: `{"answer":"42"}`, toolChoice: "auto", thinking: manualThinking,
		},
		{
			name: "manual_thinking_keeps_native", schema: closedObject, output: `{"answer":"42"}`, thinking: manualThinking,
		},
		{
			name: "manual_thinking_keeps_explicit_json_tool", schema: closedObject, output: `{"answer":"42"}`,
			mode: StructuredOutputJSONTool, toolChoice: "auto", thinking: manualThinking,
		},
		{
			name: "adaptive_thinking_keeps_any", modelID: "claude-sonnet-4-6", schema: openObject,
			output: `{"answer":42}`, toolChoice: "any", thinking: &ThinkingConfig{Type: ThinkingAdaptive},
		},
	}

	original := vertexGoogleAuth
	t.Cleanup(func() { vertexGoogleAuth = original })
	vertexGoogleAuth = func(ctx context.Context, region, project string, _ ...string) option.RequestOption {
		creds := &google.Credentials{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "vertex-token"})}
		return vertexsdk.WithCredentials(ctx, region, project, creds)
	}
	for _, route := range []struct {
		name           string
		vertex, stream bool
	}{
		{name: "direct/generate"},
		{name: "direct/stream", stream: true},
		{name: "vertex/generate", vertex: true},
		{name: "vertex/stream", vertex: true, stream: true},
	} {
		for _, tc := range cases {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				modelID := tc.modelID
				if modelID == "" {
					modelID = "claude-sonnet-4-5"
				}
				var wire map[string]json.RawMessage
				client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
					if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
						return nil, err
					}
					return structuredOutputResponse(r, modelID, tc.output, tc.toolChoice != "" && !tc.textFallback, route.stream), nil
				})}
				requestOptions := WithRequestOptions(option.WithHTTPClient(client), option.WithMaxRetries(0))
				var m provider.LanguageModel
				if route.vertex {
					var err error
					m, err = NewVertex(t.Context(), "us-east5", "project", modelID, requestOptions)
					require.NoError(t, err)
				} else {
					m = New("test-key", modelID, requestOptions)
				}

				opts := provider.CallOptions{
					MaxOutputTokens: new(64),
					Prompt:          []provider.Message{provider.UserText("Return JSON.")},
					ResponseFormat:  &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(tc.schema)},
					ProviderOptions: provider.BuildProviderOptions(AnthropicOptions{StructuredOutputMode: tc.mode, Thinking: tc.thinking}),
				}
				var output string
				var finish provider.UnifiedFinishReason
				var warnings []provider.Warning
				if route.stream {
					result, err := m.DoStream(t.Context(), opts)
					require.NoError(t, err)
					var text strings.Builder
					for part := range result.Stream {
						switch part.Type {
						case provider.PartStreamStart:
							warnings = append(warnings, part.Warnings...)
						case provider.PartTextDelta:
							text.WriteString(part.Delta)
						case provider.PartFinish:
							finish = part.FinishReason.Unified
						case provider.PartError, provider.PartToolCall, provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd:
							assert.Failf(t, "unexpected stream part for structured output", "part type: %s", part.Type)
						}
					}
					output = text.String()
				} else {
					result, err := m.DoGenerate(t.Context(), opts)
					require.NoError(t, err)
					finish = result.FinishReason.Unified
					warnings = result.Warnings
					assert.Len(t, result.Content, 1)
					for _, part := range result.Content {
						assert.Equal(t, provider.ContentText, part.Type)
						output += part.Text
					}
				}

				if tc.toolChoice == "" {
					assert.NotContains(t, wire, "tools")
					assert.NotContains(t, wire, "tool_choice")
					wantSchema := tc.schema
					if tc.mode == StructuredOutputFormat {
						wantSchema = `{"type":"object","additionalProperties":false}`
					}
					assert.JSONEq(t, `{"format":{"type":"json_schema","schema":`+wantSchema+`}}`, string(wire["output_config"]))
				} else {
					require.Empty(t, string(wire["output_config"]), "the original schema needs the JSON tool")
					assert.JSONEq(t, fmt.Sprintf(`{"type":%q,"disable_parallel_tool_use":true}`, tc.toolChoice), string(wire["tool_choice"]))
					var tools []struct {
						Name   string          `json:"name"`
						Schema json.RawMessage `json:"input_schema"`
					}
					require.NoError(t, json.Unmarshal(wire["tools"], &tools))
					require.Len(t, tools, 1)
					assert.Equal(t, "json", tools[0].Name)
					assert.JSONEq(t, tc.schema, string(tools[0].Schema))
				}
				if tc.toolChoice == "auto" {
					require.Len(t, warnings, 1)
					assert.Equal(t, provider.WarnUnsupported, warnings[0].Type)
					assert.Equal(t, "toolChoice", warnings[0].Feature)
					assert.Contains(t, warnings[0].Details, "'required' is not supported with this model or thinking configuration")
				} else {
					assert.Empty(t, warnings)
				}
				if tc.thinking != nil && tc.thinking.Type == ThinkingEnabled {
					assert.JSONEq(t, `{"type":"enabled","budget_tokens":1024}`, string(wire["thinking"]))
				}
				assert.Equal(t, provider.FinishReasonStop, finish)
				assert.JSONEq(t, tc.output, output)
			})
		}
	}
}

func structuredOutputResponse(r *http.Request, modelID, output string, tool, stream bool) *http.Response {
	block := fmt.Sprintf(`{"type":"text","text":%q}`, output)
	start := `{"type":"text","text":""}`
	delta := fmt.Sprintf(`{"type":"text_delta","text":%q}`, output)
	stop := "end_turn"
	if tool {
		block = `{"type":"tool_use","id":"toolu_test","name":"json","input":` + output + `}`
		start = `{"type":"tool_use","id":"toolu_test","name":"json","input":{}}`
		delta = fmt.Sprintf(`{"type":"input_json_delta","partial_json":%q}`, output)
		stop = "tool_use"
	}
	contentType := "application/json"
	payload := fmt.Sprintf(`{"id":"msg_structured","type":"message","role":"assistant","model":%q,"content":[%s],"stop_reason":%q,"usage":{"input_tokens":1,"output_tokens":1}}`, modelID, block, stop)
	if stream {
		contentType = "text/event-stream"
		payload = transportSSE([]string{
			fmt.Sprintf(`{"type":"message_start","message":{"id":"msg_structured","type":"message","role":"assistant","model":%q,"content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`, modelID),
			`{"type":"content_block_start","index":0,"content_block":` + start + `}`,
			`{"type":"content_block_delta","index":0,"delta":` + delta + `}`,
			`{"type":"content_block_stop","index":0}`,
			fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":1}}`, stop),
			`{"type":"message_stop"}`,
		})
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(payload)), Request: r}
}
