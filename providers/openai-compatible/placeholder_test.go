package openaicompatible

import (
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStream_MetadataAfterPlaceholder(t *testing.T) {
	stream := "data: {\"id\":\"\",\"model\":\"\",\"created\":0,\"choices\":[]}\n\n" +
		"data: {\"id\":\"chatcmpl-123\",\"model\":\"actual-model-name\",\"created\":1700000000,\"choices\":[{\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	parts := make(chan provider.StreamPart, 16)
	m := &model{providerName: "test"}
	m.runStream(t.Context(), "https://example.test", nil, strings.NewReader(stream), nil, nil, false, "test", parts)
	close(parts)
	var metadata []provider.StreamPart
	for part := range parts {
		if part.Type == provider.PartResponseMeta {
			metadata = append(metadata, part)
		}
	}
	require.Len(t, metadata, 1)
	assert.Equal(t, "chatcmpl-123", metadata[0].ResponseID)
	assert.Equal(t, "actual-model-name", metadata[0].ModelID)
	assert.Equal(t, time.Unix(1700000000, 0).UTC(), metadata[0].Timestamp)
}
