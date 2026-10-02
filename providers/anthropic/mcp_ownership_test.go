package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_MCPMarkerRequiresProviderOwnership(t *testing.T) {
	for _, owned := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "provider"}[owned], func(t *testing.T) {
			call := provider.ToolCallPart("call", "echo", json.RawMessage(`{}`))
			call.ProviderExecuted = owned
			call.ProviderOptions = makeProviderOpts(`{"type":"mcp-tool-use","serverName":"echo"}`)
			params, _, _, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(call)}}, false)
			require.NoError(t, err)
			require.Len(t, params.Messages, 1)
			require.Len(t, params.Messages[0].Content, 1)
			block := params.Messages[0].Content[0]
			if owned {
				require.NotNil(t, block.OfMCPToolUse)
				assert.Equal(t, "echo", block.OfMCPToolUse.ServerName)
			} else {
				require.NotNil(t, block.OfToolUse)
				assert.Nil(t, block.OfMCPToolUse)
			}
		})
	}
}
