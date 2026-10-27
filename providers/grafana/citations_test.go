package grafana

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCitationsDecode(t *testing.T) {
	for _, raw := range []string{
		`[]`,
		`[{"source":{"type":"source","sourceType":"url","id":"","url":""},"startIndex":0,"endIndex":1,"citedText":""}]`,
		`[{"source":{"type":"source","sourceType":"document","id":"d","mediaType":"text/plain","title":"","providerMetadata":{}}}]`,
	} {
		t.Run(raw, func(t *testing.T) {
			citations, err := decodeCitations(json.RawMessage(raw))
			require.NoError(t, err)
			part, err := decodeStreamPart([]byte(`{"type":"text-end","id":"t","citations":` + raw + `}`))
			require.NoError(t, err)
			assert.Equal(t, citations, part.Citations)
			encoded, err := json.Marshal(part.Citations)
			require.NoError(t, err)
			assert.JSONEq(t, raw, string(encoded))
		})
	}
	for _, raw := range []string{`null`, `{}`, `[null]`, `[{"source":{}}]`, `[{"source":{"type":"other","sourceType":"url","id":"u","url":"url"}}]`, `[{"source":{"type":"source","sourceType":"url","id":"u","url":"url"},"startIndex":-1}]`, `[{"source":{"type":"source","sourceType":"url","id":"u","url":"url"},"citedText":null}]`} {
		t.Run("invalid/"+raw, func(t *testing.T) {
			_, err := decodeCitations(json.RawMessage(raw))
			assert.Error(t, err)
		})
	}
	part, err := decodeStreamPart([]byte(`{"type":"tool-input-start","id":"c","toolName":"write","inputEncoding":"text"}`))
	require.NoError(t, err)
	assert.Equal(t, provider.ToolInputEncodingText, part.InputEncoding)
	_, err = decodeStreamPart([]byte(`{"type":"tool-input-start","id":"c","toolName":"write","inputEncoding":"other"}`))
	assert.Error(t, err)
}
