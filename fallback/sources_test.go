package fallback

import (
	"context"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceCommitsFallbackCandidate(t *testing.T) {
	secondaryCalled := false
	source := provider.StreamPart{Type: provider.PartSource, Source: &provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "id", URL: "https://example.com"}}
	primary := &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		parts := make(chan provider.StreamPart, 2)
		parts <- source
		parts <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "private"})}
		close(parts)
		return &provider.StreamResult{Stream: parts}, nil
	}}
	secondary := &mockModel{doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		secondaryCalled = true
		return nil, nil
	}}
	result, err := mustNew(t, primary, secondary).DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	require.Len(t, parts, 2)
	assert.Equal(t, source, parts[0])
	assert.False(t, secondaryCalled)
}
