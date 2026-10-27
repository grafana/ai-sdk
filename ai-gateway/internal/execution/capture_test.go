package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProject_SharedFallbackCapture(t *testing.T) {
	for _, operator := range []string{"disabled", "drops records", "panics"} {
		t.Run(operator, func(t *testing.T) {
			failure := provider.NewAPICallError(provider.APICallErrorOptions{Message: "capacity exhausted", StatusCode: 503})
			streamError := provider.NewAPICallError(provider.APICallErrorOptions{Message: "current stream error"})
			parts := []provider.StreamPart{
				{Type: provider.PartError, APICallError: streamError},
				{Type: provider.PartTextDelta, Delta: "later content"},
				{Type: provider.PartFinish},
			}
			model, err := fallback.New(&captureModel{modelID: "primary", setupErr: failure}, &captureModel{modelID: "secondary", parts: parts})
			require.NoError(t, err)
			if operator != "disabled" {
				model.WithAttemptObserver(func(context.Context, fallback.Attempt) {
					if operator == "panics" {
						panic("operator callback failed")
					}
				})
			}
			var decisions []fallback.Attempt
			ctx := fallback.WithAttemptObserver(t.Context(), func(_ context.Context, attempt fallback.Attempt) { decisions = append(decisions, attempt) })
			result, err := model.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			var received []provider.StreamPart
			for part := range result.Stream {
				received = append(received, part)
			}
			assert.Equal(t, parts, received)
			overview := Project("alias", "public", decisions, nil, 4096)
			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, 2)
			require.NotNil(t, overview.Attempts[0].Error)
			assert.Equal(t, "capacity exhausted", overview.Attempts[0].Error.Message)
			assert.Equal(t, fallback.AttemptSelected, overview.Attempts[1].Outcome)
			assert.Nil(t, overview.Attempts[1].Error)
			encoded, err := json.Marshal(overview)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "current stream error")
		})
	}
}

type captureModel struct {
	provider.LanguageModel
	modelID  string
	setupErr error
	parts    []provider.StreamPart
}

func (m *captureModel) SpecificationVersion() string { return "v4" }
func (m *captureModel) Provider() string             { return "native" }
func (m *captureModel) ModelID() string              { return m.modelID }
func (m *captureModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	if m.setupErr != nil {
		return nil, m.setupErr
	}
	stream := make(chan provider.StreamPart, len(m.parts))
	for _, part := range m.parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}
