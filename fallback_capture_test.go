package aisdk

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackCapture_HighLevelAccess(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			failure := provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "overloaded",
				StatusCode: 503,
				Data:       json.RawMessage(`{"error":{"type":"overloaded","message":"try another route"}}`),
			})
			primary := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, failure
			}}
			metadata := provider.ProviderMetadata{"native": json.RawMessage(`{"opaque":true}`)}
			secondary := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				parts := make(chan provider.StreamPart, 4)
				parts <- provider.StreamPart{Type: provider.PartTextStart, ID: "t1"}
				parts <- provider.StreamPart{Type: provider.PartTextDelta, ID: "t1", Delta: "answer"}
				parts <- provider.StreamPart{Type: provider.PartTextEnd, ID: "t1"}
				parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, ProviderMetadata: metadata}
				close(parts)
				return &provider.StreamResult{Stream: parts}, nil
			}}
			model, err := fallback.New(primary, secondary)
			require.NoError(t, err)
			model.WithAttemptObserver(func(context.Context, fallback.Attempt) { panic("operator observer failed") })
			var mu sync.Mutex
			var attempts []fallback.Attempt
			ctx := fallback.WithAttemptObserver(t.Context(), func(_ context.Context, attempt fallback.Attempt) {
				mu.Lock()
				defer mu.Unlock()
				attempts = append(attempts, attempt)
			})
			if mode == "generate" {
				result, err := GenerateText(ctx, model, WithModelMessages(provider.UserText("hello")), WithMaxRetries(0))
				require.NoError(t, err)
				assert.Equal(t, "answer", result.Text)
				assert.Equal(t, metadata, result.ProviderMetadata)
			} else {
				result := StreamText(ctx, model, WithModelMessages(provider.UserText("hello")), WithMaxRetries(0))
				for range result.FullStream() {
				}
				require.NoError(t, result.Err())
				assert.Equal(t, "answer", result.Text())
				assert.Equal(t, metadata, result.ProviderMetadata())
			}
			mu.Lock()
			defer mu.Unlock()
			require.Len(t, attempts, 2)
			assert.Equal(t, fallback.AttemptFailed, attempts[0].Outcome)
			assert.Same(t, failure, attempts[0].SourceErr)
			assert.Equal(t, fallback.AttemptSelected, attempts[1].Outcome)
			assert.Nil(t, attempts[1].SourceErr)
			assert.Equal(t, 1, primary.callCount)
			assert.Equal(t, 1, secondary.callCount)
		})
	}
}
