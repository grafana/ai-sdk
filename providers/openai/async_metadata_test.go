package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertResponse_AsyncToolCallMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, toolType, fields, want string
	}{
		{name: "function true with caller", toolType: "function_call", fields: `,"async":true,"namespace":"ns","caller":{"type":"program","caller_id":"prog_1"}`, want: `{"itemId":"item_1","async":true,"namespace":"ns","caller":{"type":"program","callerId":"prog_1"}}`},
		{name: "function false", toolType: "function_call", fields: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "function absent", toolType: "function_call", want: `{"itemId":"item_1"}`},
		{name: "custom true", toolType: "custom_tool_call", fields: `,"async":true`, want: `{"itemId":"item_1","async":true}`},
		{name: "custom false", toolType: "custom_tool_call", fields: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "custom absent", toolType: "custom_tool_call", want: `{"itemId":"item_1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := `"arguments":"{}"`
			if tc.toolType == "custom_tool_call" {
				field = `"input":"select 1"`
			}
			resp := decodeResponse(t, `{"id":"resp_1","created_at":1,"model":"gpt-6","status":"completed","output":[{"type":"`+tc.toolType+`","id":"item_1","call_id":"call_1","name":"lookup",`+field+tc.fields+`}]}`)
			result := mustConvertResponse(t, resp, buildResult{})
			require.Len(t, result.Content, 1)
			assert.Equal(t, provider.ContentToolCall, result.Content[0].Type)
			assert.JSONEq(t, tc.want, string(result.Content[0].ProviderMetadata["openai"]))
		})
	}
}

func TestStream_AsyncToolCallMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, toolType, added, done, want string
	}{
		{name: "function added fallback", toolType: "function_call", added: `,"async":true`, want: `{"itemId":"item_1","async":true}`},
		{name: "function done overrides added with namespace and caller", toolType: "function_call", added: `,"async":true`, done: `,"async":false,"namespace":"ns","caller":{"type":"program","caller_id":"prog_1"}`, want: `{"itemId":"item_1","async":false,"namespace":"ns","caller":{"type":"program","callerId":"prog_1"}}`},
		{name: "function done only", toolType: "function_call", done: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "function absent", toolType: "function_call", want: `{"itemId":"item_1"}`},
		{name: "custom added fallback", toolType: "custom_tool_call", added: `,"async":false`, want: `{"itemId":"item_1","async":false}`},
		{name: "custom done overrides added", toolType: "custom_tool_call", added: `,"async":false`, done: `,"async":true`, want: `{"itemId":"item_1","async":true}`},
		{name: "custom absent", toolType: "custom_tool_call", want: `{"itemId":"item_1"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			field := `"arguments":"{}"`
			if tc.toolType == "custom_tool_call" {
				field = `"input":"select 1"`
			}
			item := func(extra string) string {
				return `{"type":"` + tc.toolType + `","id":"item_1","call_id":"call_1","name":"lookup",` + field + extra + `}`
			}
			parts := collectParts(t,
				`{"type":"response.output_item.added","sequence_number":0,"output_index":0,"item":`+item(tc.added)+`}`,
				`{"type":"response.output_item.done","sequence_number":1,"output_index":0,"item":`+item(tc.done)+`}`,
			)
			var calls []provider.StreamPart
			for _, part := range parts {
				if part.Type == provider.PartToolCall {
					calls = append(calls, part)
				}
			}
			require.Len(t, calls, 1)
			assert.JSONEq(t, tc.want, string(calls[0].ProviderMetadata["openai"]))
		})
	}
}

func TestToolCallMeta_AzureNamespaceAndAsyncPresence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		async *bool
	}{
		{name: "absent"},
		{name: "false", async: boolPointer(false)},
		{name: "true", async: boolPointer(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta := toolCallMeta("azure", "id", "", "", "", tc.async)
			assert.NotContains(t, meta, "openai")
			var fields map[string]any
			require.NoError(t, json.Unmarshal(meta["azure"], &fields))
			if tc.async == nil {
				assert.NotContains(t, fields, "async")
			} else {
				assert.Equal(t, *tc.async, fields["async"])
			}
		})
	}
}
