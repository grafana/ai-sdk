package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestAudioStreamCompleteness(t *testing.T) {
	fragments := []string{`{"id":"audio-1"}`, `{"data":"YQ=="}`, `{"transcript":""}`, `{"expires_at":123}`}
	request, err := mapRequest([]byte(`{"model":"m","messages":[{"role":"user","content":"speak"}]}`))
	require.NoError(t, err)
	for omitted := -1; omitted < len(fragments); omitted++ {
		state := streamState{}
		for i, fragment := range fragments {
			if i == omitted {
				continue
			}
			_, _, err := state.nativeDelta(request, provider.ProviderMetadata{"openaiCompatible": json.RawMessage(`{"chatCompletion":{"audio":` + fragment + `}}`)})
			require.NoError(t, err)
		}
		_, _, err := state.finish(provider.StreamPart{FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}, request)
		if omitted == -1 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
		_, _, err = state.finish(provider.StreamPart{FinishReason: &provider.FinishReason{Unified: provider.FinishReasonLength}}, request)
		require.NoError(t, err)
	}
	for _, conflict := range []string{`{"id":"audio-2"}`, `{"expires_at":124}`} {
		state := streamState{}
		for _, fragment := range append(fragments, conflict) {
			_, _, err := state.nativeDelta(request, provider.ProviderMetadata{"openaiCompatible": json.RawMessage(`{"chatCompletion":{"audio":` + fragment + `}}`)})
			if fragment == conflict {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		}
	}
}
