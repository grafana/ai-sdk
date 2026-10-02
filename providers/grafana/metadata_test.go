package grafana

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderMetadata_DecodeScopes(t *testing.T) {
	for _, metadata := range []string{"", `{}`, `{"future":{"nested":[null,false,0,"",[],{}],"token":"sk-application-data"}}`} {
		suffix := ""
		if metadata != "" {
			suffix = `,"providerMetadata":` + metadata
		}
		for _, content := range []string{
			`"type":"text","text":"hello"`,
			`"type":"reasoning","text":""`,
			`"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}`,
			`"type":"source","sourceType":"url","id":"native","url":"https://example.test"`,
			`"type":"tool-call","toolCallId":"call","toolName":"weather","input":"{}"`,
		} {
			result, err := decodeGenerate([]byte(`{"content":[{` + content + suffix + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}` + suffix + `}`))
			require.NoError(t, err)
			require.Len(t, result.Content, 1)
			assertDecodedMetadata(t, metadata, result.ProviderMetadata)
			assertDecodedMetadata(t, metadata, result.Content[0].ProviderMetadata)
		}
		for _, event := range metadataStreamEvents() {
			part, err := decodeStreamPart([]byte(`{` + event + suffix + `}`))
			require.NoError(t, err)
			if part.Type == provider.PartSource {
				assertDecodedMetadata(t, metadata, part.Source.ProviderMetadata)
			} else {
				assertDecodedMetadata(t, metadata, part.ProviderMetadata)
			}
		}
	}
}

func metadataStreamEvents() []string {
	return []string{
		`"type":"text-start","id":"text"`, `"type":"text-delta","id":"text","delta":""`, `"type":"text-end","id":"text"`,
		`"type":"reasoning-start","id":"r"`, `"type":"reasoning-delta","id":"r","delta":""`, `"type":"reasoning-end","id":"r"`,
		`"type":"reasoning-file","mediaType":"image/png","data":{"type":"data","data":""}`,
		`"type":"source","sourceType":"url","id":"native","url":"https://example.test"`,
		`"type":"tool-input-start","id":"call","toolName":"weather"`, `"type":"tool-input-delta","id":"call","delta":""`, `"type":"tool-input-end","id":"call"`,
		`"type":"tool-call","toolCallId":"call","toolName":"weather","input":"{}"`,
		`"type":"tool-result","toolCallId":"call","toolName":"weather","result":{}`,
		`"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}`,
	}
}

func assertDecodedMetadata(t *testing.T, want string, metadata provider.ProviderMetadata) {
	t.Helper()
	if want == "" {
		assert.Nil(t, metadata)
		return
	}
	require.NotNil(t, metadata)
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	assert.JSONEq(t, want, string(raw))
}

func TestProviderMetadata_MalformedScopes(t *testing.T) {
	for _, metadata := range []string{`null`, `[]`, `false`, `{"future":null}`, `{"future":[]}`, `{"future":0}`} {
		t.Run(metadata, func(t *testing.T) {
			_, err := decodeGenerate([]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"providerMetadata":` + metadata + `}`))
			require.Error(t, err)
			_, err = decodeGenerate([]byte(`{"content":[{"type":"text","text":"hello","providerMetadata":` + metadata + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
			require.Error(t, err)
			for i, event := range metadataStreamEvents() {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					_, err := decodeStreamPart([]byte(`{` + event + `,"providerMetadata":` + metadata + `}`))
					require.Error(t, err)
				})
			}
		})
	}
}
