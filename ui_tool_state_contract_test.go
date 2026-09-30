package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIMessageChunk_ToolScalarConstruction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		chunk UIMessageChunk
		want  string
	}{
		{"input literal", UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c", ToolName: "lookup", Input: json.RawMessage(`{}`)}, `{"type":"tool-input-available","toolCallId":"c","toolName":"lookup","input":{}}`},
		{"output literal", UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c", Output: json.RawMessage(`null`)}, `{"type":"tool-output-available","toolCallId":"c","output":null}`},
		{"required empty error", UIMessageChunk{Type: ChunkToolOutputError, ToolCallID: "c"}, `{"type":"tool-output-error","toolCallId":"c","errorText":""}`},
		{"approval reason omitted", UIMessageChunk{Type: ChunkToolApprovalResponse, ApprovalID: "a"}, `{"type":"tool-approval-response","approvalId":"a","approved":false}`},
		{"source URL unchanged", UIMessageChunk{Type: ChunkSourceURL, SourceID: "s", URL: "https://example.test"}, `{"type":"source-url","sourceId":"s","url":"https://example.test"}`},
		{"source document required title", UIMessageChunk{Type: ChunkSourceDocument, SourceID: "s", MediaType: "text/plain"}, `{"type":"source-document","sourceId":"s","mediaType":"text/plain","title":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.chunk)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(got))
			event, err := FormatSSEEvent(tc.chunk)
			require.NoError(t, err)
			assert.Equal(t, "data: "+string(got)+"\n\n", string(event))
		})
	}
}

func TestUIMessageChunk_ApprovalResponseMetadataPresence(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
	}{
		{"absent", ""},
		{"empty", `,"providerMetadata":{}`},
		{"populated", `,"providerMetadata":{"test":{"phase":"decision"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"type":"tool-approval-response","approvalId":"a","approved":true` + tc.fields + `}`
			var chunk UIMessageChunk
			require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
			encoded, err := json.Marshal(chunk)
			require.NoError(t, err)
			assert.JSONEq(t, raw, string(encoded))
			event, err := FormatSSEEvent(chunk)
			require.NoError(t, err)
			assert.Equal(t, "data: "+string(encoded)+"\n\n", string(event))
		})
	}
}

func TestUIMessageReader_ToolSnapshotIsolation(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(map[bool]string{false: "static", true: "dynamic"}[dynamic], func(t *testing.T) {
			fields := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, Title: new("title"), Input: json.RawMessage(`{"q":"test"}`), RawInput: json.RawMessage(`"raw"`), Output: json.RawMessage(`null`), ErrorText: new(""), Preliminary: new(false), ToolMetadata: map[string]json.RawMessage{"key": json.RawMessage(`1`)}, CallProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"call":1}`)}, ResultProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"result":1}`)}, Approval: &ToolApproval{ID: "a", Approved: new(true), Descriptor: json.RawMessage(`{"scope":1}`), RequestReason: new("request"), Reason: new("reason")}}
			var part Part = ToolInvocationPart(fields)
			if dynamic {
				part = DynamicToolUIPart(fields)
			}
			state := newUIMessageReaderState(buildUIMessageReaderConfig([]UIMessageReaderOption{WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{part}})}))
			first := state.snapshot()
			before, err := json.Marshal(first)
			require.NoError(t, err)
			var mutated toolPartFields
			switch p := first.Parts[0].(type) {
			case ToolInvocationPart:
				mutated = toolPartFields(p)
			case DynamicToolUIPart:
				mutated = toolPartFields(p)
			}
			*mutated.Title = "changed"
			*mutated.ErrorText = "changed"
			*mutated.Preliminary = true
			for _, raw := range []json.RawMessage{mutated.Input, mutated.RawInput, mutated.Output, mutated.ToolMetadata["key"], mutated.CallProviderMetadata["test"], mutated.ResultProviderMetadata["test"], mutated.Approval.Descriptor} {
				raw[0] = 'x'
			}
			*mutated.Approval.Approved = false
			*mutated.Approval.RequestReason = "changed"
			*mutated.Approval.Reason = "changed"
			after, err := json.Marshal(state.snapshot())
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			write, err := state.apply(UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c", Output: json.RawMessage(`"final"`)})
			require.NoError(t, err)
			assert.True(t, write)
			later := state.snapshot()
			encoded, err := json.Marshal(later)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"title":"title"`)
			assert.Contains(t, string(encoded), `"requestReason":"request"`)
		})
	}
}
