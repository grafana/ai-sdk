package grafana

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcesDecode(t *testing.T) {
	for _, raw := range []string{
		`{"type":"source","sourceType":"url","id":"u","url":"","title":""}`,
		`{"type":"source","sourceType":"url","id":"u","url":"https://example.com"}`,
		`{"type":"source","sourceType":"document","id":"d","mediaType":"text/plain","title":"","filename":"","providerMetadata":{"citation":{"index":0}}}`,
		`{"type":"source","sourceType":"document","id":"d","mediaType":"text/plain","title":"Title","filename":"file.txt"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			source, err := decodeSource([]byte(raw))
			require.NoError(t, err)
			stream, err := decodeStreamPart([]byte(raw))
			require.NoError(t, err)
			assert.Equal(t, source, stream.Source)
			result, err := decodeGenerate([]byte(`{"content":[` + raw + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
			require.NoError(t, err)
			assert.Equal(t, provider.ContentSource, result.Content[0].Type)
			assert.Equal(t, source.Title, result.Content[0].Title)
			assert.Equal(t, source.Title, result.Content[0].Text)
			assert.Equal(t, source.ProviderMetadata, result.Content[0].ProviderMetadata)
		})
	}
	for _, raw := range []string{
		`{"type":"source","sourceType":"other","id":"x"}`,
		`{"type":"source","sourceType":"url","id":"x"}`,
		`{"type":"source","sourceType":"url","id":"x","url":null}`,
		`{"type":"source","sourceType":"document","id":"x","mediaType":"text/plain"}`,
		`{"type":"source","sourceType":"document","id":"x","mediaType":"text/plain","title":null}`,
		`{"type":"source","sourceType":"url","id":"x","url":"x","providerMetadata":{"citation":null}}`,
		`{"type":"source","sourceType":"url","id":"u","url":"https://example.com","title":null}`,
		`{"type":"source","sourceType":"document","id":"d","mediaType":"text/plain","title":"Document","filename":null}`,
		`{"type":"source","sourceType":"url","id":"u","url":"https://example.com","providerMetadata":null}`,
	} {
		t.Run("invalid/"+raw, func(t *testing.T) {
			_, err := decodeSource([]byte(raw))
			assert.Error(t, err)
			_, err = decodeStreamPart([]byte(raw))
			assert.Error(t, err)
			_, err = decodeGenerate([]byte(`{"content":[` + raw + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
			assert.Error(t, err)
		})
	}
}
