package openaicompatible

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChatResponseFields(t *testing.T) {
	for _, stream := range []bool{false, true} {
		raw := `{"service_tier":"priority","system_fingerprint":"fp_native","choices":[{"message":{"annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":5,"url":"https://example.com","title":"Example"}}]},"delta":{"annotations":[{"type":"url_citation","url_citation":{"start_index":0,"end_index":5,"url":"https://example.com","title":"Example"}}]}}]}`
		m, err := decodeChatCompletionMetadata([]byte(raw), stream)
		require.NoError(t, err)
		assert.Equal(t, "priority", *m.ServiceTier)
		assert.Equal(t, "fp_native", *m.SystemFingerprint)
		require.Len(t, m.Annotations, 1)
		assert.Equal(t, 5, *m.Annotations[0].URLCitation.EndIndex)
	}
	_, err := decodeChatCompletionMetadata([]byte(`{"choices":[{"message":{"annotations":[{"type":"url_citation","url_citation":{"end_index":5,"url":"https://example.com"}}]}}]}`), false)
	require.Error(t, err)
}
