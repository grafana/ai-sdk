package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func TestFallbackAcceptance_MetadataFailureDoesNotReplay(t *testing.T) {
	for _, selectedSecondary := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("secondary=%v/stream=%v", selectedSecondary, streaming), func(t *testing.T) {
				metadata := provider.ProviderMetadata{"future": json.RawMessage(`null`)}
				invalid := &observabilityTestModel{
					generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
						return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "paid"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}, ProviderMetadata: metadata}, nil
					},
					stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						return fallbackParts(provider.StreamPart{Type: provider.PartTextStart, ID: "text", ProviderMetadata: metadata}), nil
					},
				}
				primary, secondary := invalid, &observabilityTestModel{}
				outcomes := []fallback.AttemptOutcome{fallback.AttemptSelected}
				if selectedSecondary {
					primary = &observabilityTestModel{
						generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
							return nil, hostileFallbackError(503)
						},
						stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
							return nil, hostileFallbackError(503)
						},
					}
					secondary = invalid
					outcomes = []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}
				}
				h := newFallbackAcceptance(t, primary, secondary)
				w := httptest.NewRecorder()
				h.handler.ServeHTTP(w, h.request(t.Context(), streaming))
				mode := "generate"
				if streaming {
					mode = "stream"
					assert.Equal(t, http.StatusOK, w.Code)
					assert.NotContains(t, w.Body.String(), `"type":"text-start"`)
					assert.Contains(t, w.Body.String(), `"type":"error"`)
				} else {
					assert.Equal(t, http.StatusInternalServerError, w.Code)
				}
				assert.NotContains(t, w.Body.String(), "future")
				h.verify(t, mode, w.Body.String(), outcomes)
			})
		}
	}
}
