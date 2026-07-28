//go:build conformance

package conformance

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamTextAbortUIConformance(t *testing.T) {
	tests := []struct {
		name            string
		providerParts   []provider.StreamPart
		waitForDelta    bool
		pendingAtCancel bool
	}{
		{name: "no-output"},
		{
			name: "partial-output",
			providerParts: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "t1"},
				{Type: provider.PartTextDelta, ID: "t1", Delta: "partial"},
			},
			waitForDelta: true,
		},
		{
			name: "pending-output",
			providerParts: []provider.StreamPart{
				{Type: provider.PartTextStart, ID: "t1"},
				{Type: provider.PartTextDelta, ID: "t1", Delta: "discarded"},
			},
			pendingAtCancel: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(t.Context())
			streamBuffer := 0
			if tc.pendingAtCancel {
				streamBuffer = len(tc.providerParts)
			}
			stream := make(chan provider.StreamPart, streamBuffer)
			started := make(chan struct{})
			published := make(chan struct{})
			release := make(chan struct{})
			model := &configCaptureModel{
				streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					close(started)
					if tc.pendingAtCancel {
						<-release
					} else {
						go func() {
							for _, part := range tc.providerParts {
								stream <- part
							}
							close(published)
						}()
					}
					return &provider.StreamResult{Stream: stream}, nil
				},
			}

			var finishCalls int
			var finish aisdk.UIMessageStreamOnFinishState
			deltaEmitted := make(chan struct{})
			result := aisdk.StreamText(ctx, model,
				aisdk.WithModelMessages(provider.UserText("hi")),
				aisdk.OnChunk(func(state aisdk.OnChunkState) {
					if _, ok := state.Chunk.(aisdk.StreamTextDelta); ok && tc.waitForDelta {
						close(deltaEmitted)
					}
				}),
			)
			uiStream := result.ToUIMessageStream(
				aisdk.OnUIMessageStreamFinish(func(state aisdk.UIMessageStreamOnFinishState) {
					finishCalls++
					finish = state
				}),
			)

			<-started
			if tc.pendingAtCancel {
				for _, part := range tc.providerParts {
					stream <- part
				}
				cancel(errors.New("manual abort"))
				close(release)
			} else {
				<-published
				if tc.waitForDelta {
					<-deltaEmitted
				}
				cancel(errors.New("manual abort"))
			}

			var actual []map[string]any
			for chunk := range uiStream {
				data, err := json.Marshal(chunk)
				require.NoError(t, err)
				var parsed map[string]any
				require.NoError(t, json.Unmarshal(data, &parsed))
				actual = append(actual, parsed)
			}

			expected, err := LoadExpected(filepath.Join("ui", "stream-abort", tc.name, "expected.jsonl"))
			require.NoError(t, err)
			CompareChunks(t, expected, actual)
			assert.Equal(t, 1, finishCalls)
			assert.True(t, finish.IsAborted)
			assert.NoError(t, result.Err())
		})
	}
}
