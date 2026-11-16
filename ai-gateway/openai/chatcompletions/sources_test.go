package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourcesAndCitations(t *testing.T) {
	source := provider.SourceInfo{SourceType: provider.SourceTypeURL, ID: "source", URL: "https://example.com", Title: "Example"}
	citations := []provider.Citation{{Source: source, StartIndex: ptr(0), EndIndex: ptr(5)}, {Source: source}}
	for _, streaming := range []bool{false, true} {
		f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			r := textResult()
			r.Content[0].Citations = citations
			r.Content = append(r.Content, provider.GenerateContentPart{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, URL: source.URL, Title: source.Title})
			return r, nil
		}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := textParts()
			parts[3].Citations = citations
			parts = append(parts[:4], append([]provider.StreamPart{{Type: provider.PartSource, Source: &source}}, parts[4:]...)...)
			return partsStream(parts...), nil
		}}
		body := basic
		if streaming {
			body = strings.TrimSuffix(body, "}") + `,"stream":true}`
		}
		req := httptest.NewRequest("POST", Path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		testHandler(t, f).ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)
		assert.NotContains(t, w.Body.String(), `"error"`)
		assert.Contains(t, w.Body.String(), `"annotations":[{"type":"url_citation"`)
		assert.Equal(t, 1, strings.Count(w.Body.String(), `"start_index":0`))
		assert.Contains(t, w.Body.String(), `"sources"`)
		assert.Contains(t, w.Body.String(), `"citations"`)
	}
}
func TestInvalidCitationRepresentation(t *testing.T) {
	r := textResult()
	r.Content[0].Citations = []provider.Citation{{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL, URL: "https://example.com"}, StartIndex: ptr(-1), EndIndex: ptr(2)}}
	_, err := mapGenerate(r, requestWith(t, ""), "id", "model", 1)
	require.Error(t, err)
}

// Each native text part has local character spans; Chat exposes one text value.
func TestCitationOffsetsAcrossTextAndSegments(t *testing.T) {
	source := provider.SourceInfo{SourceType: provider.SourceTypeURL, URL: "https://example.com"}
	citations := []provider.Citation{{Source: source, StartIndex: ptr(0), EndIndex: ptr(2)}}
	for _, streaming := range []bool{false, true} {
		for _, segments := range []bool{false, true} {
			if segments && !streaming {
				continue
			}
			t.Run(fmt.Sprintf("stream=%v/segments=%v", streaming, segments), func(t *testing.T) {
				f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					r := textResult()
					r.Content = []provider.GenerateContentPart{{Type: provider.ContentText, Text: "é😀"}, {Type: provider.ContentText, Text: "ok", Citations: citations}}
					return r, nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					first := textParts()
					first[2].Delta = "é😀"
					second := textParts()
					second[2].Delta = "ok"
					second[3].Citations = citations
					if segments {
						second[0] = provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "next"}
						return partsStream(append(first, second...)...), nil
					}
					for i := 1; i < 4; i++ {
						second[i].ID = "second"
					}
					return partsStream(append(first[:4], second[1:]...)...), nil
				}}
				body := basic
				if streaming {
					body = strings.TrimSuffix(body, "}") + `,"stream":true}`
				}
				req := httptest.NewRequest("POST", Path, strings.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				testHandler(t, f).ServeHTTP(w, req)
				require.Equal(t, 200, w.Code)
				require.NotContains(t, w.Body.String(), `"error"`)
				assert.Contains(t, w.Body.String(), `"start_index":2,"end_index":4`)
				// Extension preserves the exact native local span for provenance.
				assert.Contains(t, w.Body.String(), `"startIndex":0,"endIndex":2`)
				if streaming {
					assert.Contains(t, w.Body.String(), "[DONE]")
				} else {
					var out completion
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
					require.NotNil(t, out.Choices[0].Message.Content)
					assert.Equal(t, "é😀ok", *out.Choices[0].Message.Content)
				}
			})
		}
	}
	assert.Equal(t, 0, *citations[0].StartIndex)
}
func TestCitationOffsetOverflow(t *testing.T) {
	_, err := mapCitations([]provider.Citation{{Source: provider.SourceInfo{SourceType: provider.SourceTypeURL}, StartIndex: ptr(0), EndIndex: ptr(math.MaxInt)}}, 1)
	require.Error(t, err)
}
