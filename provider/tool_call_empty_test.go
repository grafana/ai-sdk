package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateContentPart_EmptyToolInput(t *testing.T) {
	value, err := json.Marshal(GenerateContentPart{Type: ContentToolCall, ToolCallID: "call", ToolName: "tool", Input: json.RawMessage{}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"tool-call","toolCallId":"call","toolName":"tool","input":""}`, string(value))
}
