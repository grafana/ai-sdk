package nativeoptions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestCustomHistoryCannotOverrideNativeIdentity(t *testing.T) {
	for _, field := range []string{"id", "type", "function", "custom"} {
		call := provider.ToolCallPart("c", "safe", json.RawMessage(`"text"`))
		raw, _ := json.Marshal(map[string]any{"customTool": true, field: map[string]string{"name": "unsafe"}})
		call.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: raw}}
		require.Error(t, Compatible(provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(call)}}, "custom.chat"))
	}
}
