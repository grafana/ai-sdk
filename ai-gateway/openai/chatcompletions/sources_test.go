package chatcompletions

import (
	"context"
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
