package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIMessageReader_PartialInputResume(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(map[bool]string{false: "static", true: "dynamic"}[dynamic], func(t *testing.T) {
			fields := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: ToolStateInputStreaming, Input: json.RawMessage(`{"q":"par"}`), RawInput: json.RawMessage(`"{\"q\":\"par"`), Title: new(""), ToolMetadata: map[string]json.RawMessage{}}
			var part Part = ToolInvocationPart(fields)
			if dynamic {
				part = DynamicToolUIPart(fields)
			}
			initial := UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "old", ToolName: "lookup", State: ToolStateInputStreaming}, StepStartPart{}, part}}
			message, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "c", InputTextDelta: `tial"}`}), WithUIMessageReaderInitialMessage(initial))
			require.NoError(t, err)
			require.Len(t, message.Parts, 3)
			var got toolPartFields
			switch p := message.Parts[2].(type) {
			case ToolInvocationPart:
				got = toolPartFields(p)
			case DynamicToolUIPart:
				got = toolPartFields(p)
			}
			assert.JSONEq(t, `{"q":"partial"}`, string(got.Input))
			assert.JSONEq(t, `"{\"q\":\"partial\"}"`, string(got.RawInput))
			assert.Equal(t, new(""), got.Title)
			assert.NotNil(t, got.ToolMetadata)
			finished, err := assembleResponseMessageWithInitial("m", []UIMessageChunk{{Type: ChunkToolInputDelta, ToolCallID: "c", InputTextDelta: `tial"}`}}, &initial)
			require.NoError(t, err)
			assert.Equal(t, message, finished)
			_, err = AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "old", InputTextDelta: `{}`}), WithUIMessageReaderInitialMessage(initial))
			require.Error(t, err)
		})
	}
}

func TestUIMessageReader_InvalidPersistedPartialInput(t *testing.T) {
	for _, raw := range []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`{}`), json.RawMessage(`42`)} {
		t.Run(string(raw), func(t *testing.T) {
			initial := UIMessage{Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c", State: ToolStateInputStreaming, RawInput: raw}}}
			_, err := AssembleUIMessage(chunks(), WithUIMessageReaderInitialMessage(initial))
			require.ErrorContains(t, err, "non-string streaming raw input")
			assert.Empty(t, collectMessages(StreamUIMessage(chunks(), WithUIMessageReaderInitialMessage(initial))))
		})
	}
}

func TestUIMessage_StreamingRawInputValidation(t *testing.T) {
	for _, kind := range []string{"tool-lookup", "dynamic-tool"} {
		for _, raw := range []string{`null`, `{}`, `42`, `"{\"q\":"`} {
			t.Run(kind+"/"+raw, func(t *testing.T) {
				var message UIMessage
				err := json.Unmarshal([]byte(`{"id":"m","role":"assistant","parts":[{"type":"`+kind+`","toolName":"lookup","toolCallId":"c","state":"input-streaming","rawInput":`+raw+`}]}`), &message)
				if raw[0] == '"' {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			})
		}
	}
}
