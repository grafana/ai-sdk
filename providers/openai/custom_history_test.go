package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomHistoryWithoutDeclaration(t *testing.T) {
	marker := provider.BuildProviderOptions(OpenAIPartOptions{CustomTool: true})
	call := provider.ToolCallPart("c", "code", json.RawMessage(`"print('hello')"`))
	call.ProviderOptions = marker
	result := provider.ToolResultPart("c", "code", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "hello"})
	result.ProviderOptions = marker
	ctx := inputConversionContext{providerOptionsName: "openai"}
	native, err := convertAssistantToolCall(call, ctx)
	require.NoError(t, err)
	encoded, err := json.Marshal(native)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"type":"custom_tool_call"`)
	assert.Contains(t, string(encoded), `"input":"print('hello')"`)
	output, _, err := convertProviderToolResult(result, ctx)
	require.NoError(t, err)
	encoded, err = json.Marshal(output)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"type":"custom_tool_call_output"`)
}
