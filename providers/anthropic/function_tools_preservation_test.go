package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionTools_NativeSelectedValues(t *testing.T) {
	requests := make(chan map[string]json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"response","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
	maxTokens := 64
	strict := false
	invoke := func(t *testing.T, schema json.RawMessage, output *provider.ToolResultOutput) map[string]json.RawMessage {
		t.Helper()
		result, err := model.DoGenerate(context.Background(), provider.CallOptions{
			MaxOutputTokens: &maxTokens,
			Tools:           []provider.Tool{{Type: provider.ToolTypeFunction, Name: "weather", InputSchema: schema, Strict: &strict, InputExamples: []provider.InputExample{{Input: json.RawMessage(`{"city":"Rio"}`)}}}},
			ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "weather"},
			Prompt: []provider.Message{
				provider.NewAssistantMessage(provider.ToolCallPart("call", "weather", json.RawMessage(`{"city":"Rio"}`))),
				provider.NewToolMessage(provider.ToolResultPart("call", "weather", output)),
			},
		})
		require.NoError(t, err)
		require.NotNil(t, result)
		return <-requests
	}
	for _, tc := range []struct {
		name, schema, want string
	}{
		{name: "absent schema remains omitted"},
		{name: "selected empty schema", schema: `{}`, want: `{}`},
		{name: "schema without root type", schema: `{"properties":{"city":{"type":"string"}},"additionalProperties":false}`, want: `{"properties":{"city":{"type":"string"}},"additionalProperties":false}`},
		{name: "typed object schema", schema: `{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`, want: `{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var schema json.RawMessage
			if tc.schema != "" {
				schema = json.RawMessage(tc.schema)
			}
			body := invoke(t, schema, &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "sunny"})
			var tools []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body["tools"], &tools))
			require.Len(t, tools, 1)
			if tc.want == "" {
				assert.NotContains(t, tools[0], "input_schema")
			} else {
				assert.JSONEq(t, tc.want, string(tools[0]["input_schema"]))
			}
			assert.JSONEq(t, `false`, string(tools[0]["strict"]))
			assert.JSONEq(t, `[{"city":"Rio"}]`, string(tools[0]["input_examples"]))
			assert.JSONEq(t, `"weather"`, string(tools[0]["name"]))
			assert.NotContains(t, tools[0], "description")
			var choice map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body["tool_choice"], &choice))
			assert.JSONEq(t, `"tool"`, string(choice["type"]))
			assert.JSONEq(t, `"weather"`, string(choice["name"]))
		})
	}
	for _, tc := range []struct {
		name    string
		output  *provider.ToolResultOutput
		want    string
		isError bool
	}{
		{name: "absent output keeps existing default", want: `[{"type":"text","text":""}]`},
		{name: "absent content keeps existing default", output: &provider.ToolResultOutput{Type: provider.ToolOutputContent}, want: `[{"type":"text","text":""}]`},
		{name: "selected empty content", output: &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{}}, want: `[]`},
		{name: "selected empty text block", output: &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: ""}}}, want: `[{"type":"text","text":""}]`},
		{name: "ordered text content", output: &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "first"}, {Type: provider.ToolContentText, Text: ""}, {Type: provider.ToolContentText, Text: "last"}}}, want: `[{"type":"text","text":"first"},{"type":"text","text":""},{"type":"text","text":"last"}]`},
		{name: "empty text", output: &provider.ToolResultOutput{Type: provider.ToolOutputText}, want: `[{"type":"text","text":""}]`},
		{name: "empty error text", output: &provider.ToolResultOutput{Type: provider.ToolOutputErrorText}, want: `[{"type":"text","text":""}]`, isError: true},
		{name: "JSON null", output: &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`null`)}, want: `[{"type":"text","text":"null"}]`},
		{name: "error JSON null", output: &provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`null`)}, want: `[{"type":"text","text":"null"}]`, isError: true},
		{name: "error JSON false", output: &provider.ToolResultOutput{Type: provider.ToolOutputErrorJSON, JSON: json.RawMessage(`false`)}, want: `[{"type":"text","text":"false"}]`, isError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := invoke(t, json.RawMessage(`{"type":"object"}`), tc.output)
			var messages []struct {
				Content []map[string]json.RawMessage `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body["messages"], &messages))
			require.Len(t, messages, 2)
			require.Len(t, messages[1].Content, 1)
			block := messages[1].Content[0]
			assert.JSONEq(t, tc.want, string(block["content"]))
			assert.JSONEq(t, `"tool_result"`, string(block["type"]))
			assert.JSONEq(t, `"call"`, string(block["tool_use_id"]))
			if tc.isError {
				assert.JSONEq(t, `true`, string(block["is_error"]))
			} else {
				assert.NotContains(t, block, "is_error")
			}
		})
	}
}
