package grafana

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolMetadata_OpaquePresence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata string
	}{
		{name: "omitted"},
		{name: "empty", metadata: `{}`},
		{name: "extensions", metadata: `{"anthropic":{"type":"mcp-tool-use","serverName":"echo","caller":{"type":"future","extension":[null,false,{}]},"authorizationToken":"application-data"},"future":{"nested":{"values":[0,"",null]},"empty":{}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want map[string]json.RawMessage
			field := ""
			if tc.metadata != "" {
				require.NoError(t, json.Unmarshal([]byte(tc.metadata), &want))
				field = `,"providerMetadata":` + tc.metadata
			}
			for _, content := range []string{
				`{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}"}`,
				`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":false}`,
			} {
				body := `{"content":[` + strings.TrimSuffix(content, "}") + field + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`
				result, err := decodeGenerate([]byte(body))
				require.NoError(t, err)
				require.Len(t, result.Content, 1)
				assert.Equal(t, want == nil, result.Content[0].ProviderMetadata == nil)
				assert.Len(t, result.Content[0].ProviderMetadata, len(want))
				for namespace, raw := range want {
					assert.JSONEq(t, string(raw), string(result.Content[0].ProviderMetadata[namespace]))
				}
			}
			for _, event := range []string{
				`{"type":"tool-input-start","id":"call","toolName":"echo"}`,
				`{"type":"tool-input-delta","id":"call","delta":""}`,
				`{"type":"tool-input-end","id":"call"}`,
				`{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}"}`,
				`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":false}`,
			} {
				part, err := decodeStreamPart([]byte(strings.TrimSuffix(event, "}") + field + "}"))
				require.NoError(t, err)
				assert.Equal(t, want == nil, part.ProviderMetadata == nil)
				assert.Len(t, part.ProviderMetadata, len(want))
				for namespace, raw := range want {
					assert.JSONEq(t, string(raw), string(part.ProviderMetadata[namespace]))
				}
			}
		})
	}
}
