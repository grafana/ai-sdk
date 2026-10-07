package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// annotatedResponse carries one of each annotation the Responses API attaches
// to output text, with distinct titles, filenames and ids.
const annotatedResponse = `{
	"id":"resp_1","created_at":1700000000,"model":"gpt-4o","object":"response","status":"completed",
	"output":[{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[
		{"type":"output_text","text":"see here","annotations":[
			{"type":"url_citation","url":"https://example.com","title":"Example","start_index":0,"end_index":3},
			{"type":"file_citation","file_id":"file-1","filename":"notes.txt","index":4},
			{"type":"container_file_citation","container_id":"cnt-1","file_id":"file-2","filename":"chart.csv","start_index":5,"end_index":6},
			{"type":"file_path","file_id":"file-3","index":7}
		]}
	]}],
	"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
}`

func TestConvertAnnotations_SourceTitles(t *testing.T) {
	res := mustConvertResponse(t, decodeResponse(t, annotatedResponse), buildResult{})

	var sources []provider.GenerateContentPart
	for _, part := range res.Content {
		if part.Type == provider.ContentSource {
			sources = append(sources, part)
		}
	}
	require.Len(t, sources, 4)

	for _, tc := range []struct {
		name       string
		part       provider.GenerateContentPart
		sourceType provider.SourceType
		title      string
		mediaType  string
		filename   string
		url        string
		metadata   string
	}{
		{
			name: "url citation", part: sources[0], sourceType: provider.SourceTypeURL,
			title: "Example", url: "https://example.com",
		},
		{
			name: "file citation", part: sources[1], sourceType: provider.SourceTypeDocument,
			title: "notes.txt", mediaType: "text/plain", filename: "notes.txt",
			metadata: `{"fileId":"file-1","index":4,"type":"file_citation"}`,
		},
		{
			name: "container file citation", part: sources[2], sourceType: provider.SourceTypeDocument,
			title: "chart.csv", mediaType: "text/plain", filename: "chart.csv",
			metadata: `{"containerId":"cnt-1","fileId":"file-2","type":"container_file_citation"}`,
		},
		{
			// A file path has no filename of its own, so the file id is both the
			// display title and the filename, as the registered upstream does.
			name: "file path", part: sources[3], sourceType: provider.SourceTypeDocument,
			title: "file-3", mediaType: "application/octet-stream", filename: "file-3",
			metadata: `{"fileId":"file-3","index":7,"type":"file_path"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.sourceType, tc.part.SourceType)
			assert.Equal(t, tc.title, tc.part.Title)
			assert.Empty(t, tc.part.Text, "a source carries its title in Title, not in the text field")
			assert.Equal(t, tc.mediaType, tc.part.MediaType)
			assert.Equal(t, tc.filename, tc.part.Filename)
			assert.Equal(t, tc.url, tc.part.URL)
			assert.NotEmpty(t, tc.part.ID)
			if tc.metadata == "" {
				assert.Empty(t, tc.part.ProviderMetadata)
				return
			}
			require.Contains(t, tc.part.ProviderMetadata, "openai")
			assert.JSONEq(t, tc.metadata, string(tc.part.ProviderMetadata["openai"]))
		})
	}

	ids := map[string]bool{}
	for _, source := range sources {
		assert.False(t, ids[source.ID], "source ids must be distinct")
		ids[source.ID] = true
	}
}

func TestSourceTitles_SurviveSimulatedStreaming(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(annotatedResponse)),
		}, nil
	})
	model := NewResponses("test-key", "gpt-4o", WithRequestOptions(
		option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: transport}),
	))
	wrapped := middleware.Wrap(middleware.WrapOptions{
		Model:      model,
		Middleware: []middleware.Middleware{middleware.SimulateStreaming()},
	})

	result, err := wrapped.DoStream(t.Context(), provider.CallOptions{
		Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: "hi"}}}},
	})
	require.NoError(t, err)

	var titles []string
	for part := range result.Stream {
		if part.Type == provider.PartSource {
			require.NotNil(t, part.Source)
			titles = append(titles, part.Source.Title)
		}
	}
	assert.Equal(t, []string{"Example", "notes.txt", "chart.csv", "file-3"}, titles,
		"a simulated stream reads Source.Title, so a unary conversion that leaves it empty loses every title")
}
