package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStructuredOutput_NativeSchemaNumbers(t *testing.T) {
	for _, mode := range []StructuredOutputMode{StructuredOutputModeOutputFormat, StructuredOutputModeJSONTool} {
		opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("json")}, ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(`{"type":"object","properties":{"value":{"type":"integer","const":9007199254740993,"enum":[9007199254740993,-9007199254740993,1.234567890123456789,1e100],"minimum":9007199254740993,"default":9007199254740993}}}`)}, ProviderOptions: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: mode})}
		request, _, _, err := buildRequest(testAnthropicModel, opts)
		require.NoError(t, err)
		body, err := json.Marshal(request)
		require.NoError(t, err)
		for _, token := range []string{`"const":9007199254740993`, `"enum":[9007199254740993,-9007199254740993,1.234567890123456789,1e100]`, `"default":9007199254740993`} {
			assert.Contains(t, string(body), token)
		}
		if mode == StructuredOutputModeOutputFormat {
			assert.Contains(t, string(body), `"description":"minimum: 9007199254740993."`)
		} else {
			assert.Contains(t, string(body), `"minimum":9007199254740993`)
		}
		assert.NotContains(t, string(body), "9007199254740992")
	}
}

func TestStructuredOutput_RejectsTrailingSchemaJSON(t *testing.T) {
	for _, raw := range []string{`{} {}`, `{} true`, `{} garbage`, `null`} {
		_, _, _, err := buildRequest(testAnthropicModel, provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Schema: json.RawMessage(raw)}, ProviderOptions: provider.BuildProviderOptions(BedrockOptions{StructuredOutputMode: StructuredOutputModeOutputFormat})})
		require.Error(t, err)
	}
}
