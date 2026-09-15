package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamingFunctionTools_Lifecycle(t *testing.T) {
	start := provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence"}
	end := provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call-1"}
	call := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: `{}`}
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
		valid bool
	}{
		{name: "standalone call", parts: []provider.StreamPart{call}, valid: true},
		{name: "no argument deltas", parts: []provider.StreamPart{start, end, call}, valid: true},
		{name: "parallel calls interleaved with text", parts: []provider.StreamPart{
			start,
			{Type: provider.PartTextStart, ID: "text-1"},
			{Type: provider.PartToolInputStart, ID: "call-2", ToolName: "read_baseline"},
			{Type: provider.PartToolInputDelta, ID: "call-1", Delta: ""},
			{Type: provider.PartToolInputEnd, ID: "call-2"},
			{Type: provider.PartToolCall, ToolCallID: "call-2", ToolName: "read_baseline", Input: `{}`},
			{Type: provider.PartTextDelta, ID: "text-1", Delta: "Checking"},
			{Type: provider.PartToolInputDelta, ID: "call-1", Delta: `{}`},
			end, call,
			{Type: provider.PartTextEnd, ID: "text-1"},
		}, valid: true},
		{name: "text and tool ids use separate namespaces", parts: []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "call-1"}, start, end, call,
			{Type: provider.PartTextEnd, ID: "call-1"},
		}, valid: true},
		{name: "text id after standalone call", parts: []provider.StreamPart{
			call, {Type: provider.PartTextStart, ID: "call-1"}, {Type: provider.PartTextEnd, ID: "call-1"},
		}, valid: true},
		{name: "delta without start", parts: []provider.StreamPart{{Type: provider.PartToolInputDelta, ID: "call-1", Delta: `{}`}}},
		{name: "end without start", parts: []provider.StreamPart{end}},
		{name: "duplicate start", parts: []provider.StreamPart{start, start}},
		{name: "duplicate end", parts: []provider.StreamPart{start, end, end}},
		{name: "duplicate completed call", parts: []provider.StreamPart{start, end, call, call}},
		{name: "duplicate standalone call", parts: []provider.StreamPart{call, call}},
		{name: "input after completed call", parts: []provider.StreamPart{call, start}},
		{name: "call before input end", parts: []provider.StreamPart{start, call}},
		{name: "call changes tool name", parts: []provider.StreamPart{start, end, {Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "different", Input: `{}`}}},
		{name: "delta after input end", parts: []provider.StreamPart{start, end, {Type: provider.PartToolInputDelta, ID: "call-1", Delta: `{}`}}},
		{name: "finish before input end", parts: []provider.StreamPart{start}},
		{name: "finish before completed call", parts: []provider.StreamPart{start, end}},
		{name: "response metadata after tool input", parts: []provider.StreamPart{start, {Type: provider.PartResponseMeta}, end, call}},
		{name: "response metadata after standalone call", parts: []provider.StreamPart{call, {Type: provider.PartResponseMeta}}},
		{name: "provider executed start", parts: []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence", ProviderExecuted: true}}},
		{name: "provider executed call", parts: []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: `{}`, ProviderExecuted: true}}},
		{name: "empty tool input id", parts: []provider.StreamPart{{Type: provider.PartToolInputStart, ToolName: "read_evidence"}}},
		{name: "empty tool input name", parts: []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "call-1"}}},
		{name: "empty completed call id", parts: []provider.StreamPart{{Type: provider.PartToolCall, ToolName: "read_evidence", Input: `{}`}}},
		{name: "empty completed call name", parts: []provider.StreamPart{{Type: provider.PartToolCall, ToolCallID: "call-1", Input: `{}`}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(append(tc.parts, finishPart())...)}, nil
			}
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			require.Equal(t, http.StatusOK, response.Code)
			body := response.Body.String()
			requireStreamBodyMatchesSchema(t, body)
			if tc.valid {
				assert.NotContains(t, body, `"type":"error"`)
				assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
				frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
				require.Len(t, frames, len(tc.parts)+2)
				for i, part := range tc.parts {
					var event struct {
						Type provider.StreamPartType `json:"type"`
					}
					require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[i+1], "data: ")), &event))
					assert.Equal(t, part.Type, event.Type)
				}
			} else {
				assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
				assert.NotContains(t, body, `"type":"finish"`)
			}
		})
	}
}

func TestStreamingFunctionTools_ArgumentsAndPrivacy(t *testing.T) {
	for _, input := range []string{"", "{", `{"service":"checkout"}`, "\"\\\n\t<>&\u2028\u2029"} {
		t.Run(input, func(t *testing.T) {
			dynamic := false
			private := provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"metadata-sentinel"}`)}
			parts := []provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence", Dynamic: &dynamic, Title: "Read evidence", ProviderMetadata: private},
				{Type: provider.PartToolInputDelta, ID: "call-1", Delta: input, ProviderMetadata: private},
				{Type: provider.PartToolInputEnd, ID: "call-1", ProviderMetadata: private},
				{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: input, Dynamic: &dynamic, ProviderMetadata: private},
				{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}},
			}
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(parts...)}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			requireStreamBodyMatchesSchema(t, body)
			frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
			require.Len(t, frames, 6)
			assert.JSONEq(t, `{"type":"tool-input-start","id":"call-1","toolName":"read_evidence","dynamic":false,"title":"Read evidence"}`, strings.TrimPrefix(frames[1], "data: "))
			var delta, call map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[2], "data: ")), &delta))
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[4], "data: ")), &call))
			var gotDelta, gotInput string
			require.NoError(t, json.Unmarshal(delta["delta"], &gotDelta))
			require.NoError(t, json.Unmarshal(call["input"], &gotInput))
			assert.Equal(t, input, gotDelta)
			assert.Equal(t, input, gotInput)
			assert.Equal(t, json.RawMessage("false"), call["dynamic"])
			assert.NotContains(t, body, "metadata-sentinel")
			assert.NotContains(t, body, "providerMetadata")
			assert.NotContains(t, body, "providerExecuted")
			assert.Contains(t, frames[5], `"unified":"tool-calls"`)
		})
	}
}

func TestStreamingFunctionTools_InvalidUTF8(t *testing.T) {
	invalid := string([]byte{0xff})
	for _, tc := range []struct {
		name string
		part provider.StreamPart
	}{
		{name: "input id", part: provider.StreamPart{Type: provider.PartToolInputStart, ID: invalid, ToolName: "read_evidence"}},
		{name: "input name", part: provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: invalid}},
		{name: "title", part: provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence", Title: invalid}},
		{name: "call id", part: provider.StreamPart{Type: provider.PartToolCall, ToolCallID: invalid, ToolName: "read_evidence", Input: `{}`}},
		{name: "call name", part: provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: invalid, Input: `{}`}},
		{name: "arguments", part: provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: invalid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(tc.part, finishPart())}, nil
			}
			body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
			assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), body)
		})
	}
}

func TestStreamingFunctionTools_FrameBounds(t *testing.T) {
	large := strings.Repeat("<\"\n", 128)
	start := provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence"}
	end := provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call-1"}
	call := provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: `{}`}
	argumentParts := []provider.StreamPart{start}
	for i := 0; i < 8; i++ {
		argumentParts = append(argumentParts, provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-1", Delta: strings.Repeat("<\"\n", 16)})
	}
	argumentParts = append(argumentParts, end, provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "read_evidence", Input: large})
	for _, tc := range []struct {
		name    string
		parts   []provider.StreamPart
		payload map[string]any
	}{
		{
			name: "title",
			parts: []provider.StreamPart{
				{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "read_evidence", Title: large}, end, call,
			},
			payload: map[string]any{"type": provider.PartToolInputStart, "id": "call-1", "toolName": "read_evidence", "title": large},
		},
		{
			name: "delta",
			parts: []provider.StreamPart{
				start, {Type: provider.PartToolInputDelta, ID: "call-1", Delta: large}, end, call,
			},
			payload: map[string]any{"type": provider.PartToolInputDelta, "id": "call-1", "delta": large},
		},
		{
			name:    "arguments after bounded deltas",
			parts:   argumentParts,
			payload: map[string]any{"type": provider.PartToolCall, "toolCallId": "call-1", "toolName": "read_evidence", "input": large},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := json.Marshal(tc.payload)
			require.NoError(t, err)
			frameSize := int64(len(payload) + len("data: \n\n"))
			for _, limit := range []int64{frameSize, frameSize - 1, 256} {
				limits := testLimits()
				limits.StreamFrameBytes = limit
				harness := newRuntimeHarness(t, limits)
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(append(tc.parts, finishPart())...)}, nil
				}
				body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
				requireStreamBodyMatchesSchema(t, body)
				frames := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n\n")
				for _, frame := range frames {
					assert.LessOrEqual(t, int64(len(frame)+len("\n\n")), limit)
				}
				if tc.name == "arguments after bounded deltas" {
					assert.Equal(t, 8, strings.Count(body, `"type":"tool-input-delta"`))
					assert.Equal(t, limit == frameSize, strings.Contains(body, `"type":"tool-call"`))
				}
				if limit == frameSize {
					assert.Contains(t, body, `"type":"finish"`)
					assert.NotContains(t, body, `"type":"error"`)
				} else {
					assert.NotContains(t, body, `"type":"finish"`)
					assert.Equal(t, 1, strings.Count(body, `"code":"internal_error"`))
				}
			}
		})
	}
}

func TestStreamingFunctionTools_Schema(t *testing.T) {
	compiled, err := schema.CompileSchema(streamEventSchemaJSON)
	require.NoError(t, err)
	for _, document := range []string{
		`{"type":"tool-input-start","id":"a"}`,
		`{"type":"tool-input-start","id":"a","toolName":"f","providerExecuted":true}`,
		`{"type":"tool-input-delta","id":"a"}`,
		`{"type":"tool-input-end","id":""}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f"}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":"{}","providerMetadata":{}}`,
	} {
		assert.Error(t, compiled.Validate(json.RawMessage(document)), document)
	}
}
