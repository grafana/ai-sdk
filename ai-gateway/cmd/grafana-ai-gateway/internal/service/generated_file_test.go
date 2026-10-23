package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestGeneratedFileFallbackCommitment(t *testing.T) {
	file := provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: &provider.StreamFileData{Bytes: []byte{1, 2, 3}}}
	finish := provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
	for _, tc := range []struct {
		name     string
		first    *provider.StreamResult
		err      error
		types    []string
		outcomes []fallback.AttemptOutcome
	}{
		{"preselection", nil, hostileFallbackError(503), []string{"stream-start", "file", "finish"}, []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{"file only", fallbackParts(file, finish), nil, []string{"stream-start", "file", "finish"}, []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{"file then error", fallbackParts(file, provider.StreamPart{Type: provider.PartError, APICallError: hostileFallbackError(503)}), nil, []string{"stream-start", "file", "error", "error"}, []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{"selected encoding", fallbackParts(provider.StreamPart{Type: provider.PartFile, MediaType: "image/png", Data: &provider.StreamFileData{Bytes: make([]byte, 1<<20)}}), nil, []string{"stream-start", "error"}, []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{"text file ordering", fallbackParts(provider.StreamPart{Type: provider.PartTextStart, ID: "t"}, provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "hello"}, file, provider.StreamPart{Type: provider.PartTextEnd, ID: "t"}, finish), nil, []string{"stream-start", "text-start", "text-delta", "file", "text-end", "finish"}, []fallback.AttemptOutcome{fallback.AttemptSelected}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newFallbackAcceptance(t, &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return tc.first, tc.err }}, &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				require.Len(t, tc.outcomes, 2, "must not replay selected output")
				return fallbackParts(file, finish), nil
			}})
			w := httptest.NewRecorder()
			h.handler.ServeHTTP(w, h.request(t.Context(), true, mappedFallbackBody))
			require.Equal(t, http.StatusOK, w.Code)
			var types []string
			for _, line := range strings.Split(w.Body.String(), "\n") {
				if strings.HasPrefix(line, "data: ") {
					var event struct {
						Type string `json:"type"`
					}
					require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
					types = append(types, event.Type)
				}
			}
			require.Equal(t, tc.types, types)
			h.verify(t, "stream", w.Body.String(), tc.outcomes)
		})
	}
}

func TestGeneratedFileUnaryAnySize(t *testing.T) {
	for _, size := range []int{0, 7 << 20} {
		t.Run(fmt.Sprintf("bytes=%d", size), func(t *testing.T) {
			data := provider.BytesDataContent(make([]byte, size))
			h := newFallbackAcceptance(t, &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "image/png", Data: &data}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
			}}, &observabilityTestModel{})
			w := httptest.NewRecorder()
			h.handler.ServeHTTP(w, h.request(t.Context(), false, mappedFallbackBody))
			require.Equal(t, 200, w.Code)
			require.Contains(t, w.Body.String(), `"type":"file"`)
			h.verify(t, "generate", w.Body.String(), []fallback.AttemptOutcome{fallback.AttemptSelected})
		})
	}
}
