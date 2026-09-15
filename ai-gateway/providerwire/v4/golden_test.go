package v4

import (
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeGoldenReplay_FunctionTools(t *testing.T) {
	records := loadGolden(t, "function-tools.json")
	require.Len(t, records, 2)
	for index, record := range records {
		t.Run(record.Headers[HeaderStreaming], func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(requestFromGolden(t, record))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, 1, harness.model.callCount())
			options := harness.model.receivedOptions()
			require.Len(t, options.Tools, 1)
			tool := options.Tools[0]
			assert.Equal(t, provider.ToolTypeFunction, tool.Type)
			assert.Equal(t, "lookup", tool.Name)
			assert.Equal(t, "Read service evidence", tool.Description)
			assert.JSONEq(t, `{"type":"object","properties":{"service":{"type":"string"}},"required":["service"],"additionalProperties":false}`, string(tool.InputSchema))
			require.Len(t, tool.InputExamples, 1)
			assert.JSONEq(t, `{"service":"checkout"}`, string(tool.InputExamples[0].Input))
			require.NotNil(t, tool.Strict)
			assert.False(t, *tool.Strict)
			assert.Empty(t, tool.ProviderOptions)
			require.NotNil(t, options.ToolChoice)
			if index == 0 {
				assert.Equal(t, provider.ToolChoiceRequired, options.ToolChoice.Type)
				require.Len(t, options.Prompt, 1)
				return
			}
			assert.Equal(t, provider.ToolChoiceNone, options.ToolChoice.Type)
			require.Len(t, options.Prompt, 3)
			assert.Equal(t, provider.RoleAssistant, options.Prompt[1].Role)
			assert.Equal(t, provider.RoleTool, options.Prompt[2].Role)
			require.Len(t, options.Prompt[1].Content, 5)
			require.Len(t, options.Prompt[2].Content, 5)
			for resultIndex, result := range options.Prompt[2].Content {
				call := options.Prompt[1].Content[resultIndex]
				assert.Equal(t, provider.ContentPartTypeToolCall, call.Type)
				assert.Equal(t, provider.ContentPartTypeToolResult, result.Type)
				assert.Equal(t, call.ToolCallID, result.ToolCallID)
				assert.Equal(t, "lookup", result.ToolName)
				assert.JSONEq(t, `{"service":"checkout"}`, string(call.Input))
				assert.False(t, call.ProviderExecuted)
				require.NotNil(t, result.Output)
			}
			outputs := options.Prompt[2].Content
			assert.Equal(t, provider.ToolOutputText, outputs[0].Output.Type)
			assert.Empty(t, outputs[0].Output.Text)
			assert.Equal(t, provider.ToolOutputJSON, outputs[1].Output.Type)
			assert.JSONEq(t, `{"errorRate":4.2,"missing":null}`, string(outputs[1].Output.JSON))
			assert.Equal(t, provider.ToolOutputErrorText, outputs[2].Output.Type)
			assert.Equal(t, "query failed", outputs[2].Output.Text)
			assert.Equal(t, provider.ToolOutputErrorJSON, outputs[3].Output.Type)
			assert.JSONEq(t, `{"retryable":false}`, string(outputs[3].Output.JSON))
			assert.Equal(t, provider.ToolOutputExecutionDenied, outputs[4].Output.Type)
			assert.Equal(t, "permission denied", outputs[4].Output.Reason)
		})
	}
}
