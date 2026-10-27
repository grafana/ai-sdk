package anthropicmessages

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeStream_UsageAndIdentity(t *testing.T) {
	input, read, write, output := 7, 2, 1, 3
	initialUsage := provider.Usage{InputTokens: provider.InputTokenUsage{NoCache: &input, CacheRead: &read, CacheWrite: &write}}
	finalUsage := initialUsage
	finalUsage.OutputTokens.Total = &output
	finish := provider.FinishReason{Unified: provider.FinishReasonStop}
	for _, tc := range []struct {
		name       string
		metadata   bool
		outputOnly bool
	}{
		{name: "metadata after stream start", metadata: true},
		{name: "usage only at finish"},
		{name: "retain initial input usage", metadata: true, outputOnly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := []provider.StreamPart{{Type: provider.PartStreamStart}, {Type: provider.PartRaw}}
			if tc.metadata {
				parts = append(parts, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "msg_provider", Usage: &initialUsage})
			}
			completionUsage := finalUsage
			if tc.outputOnly {
				completionUsage.InputTokens = provider.InputTokenUsage{}
			}
			parts = append(parts, provider.StreamPart{Type: provider.PartTextDelta, Delta: "hello"}, provider.StreamPart{Type: provider.PartFinish, FinishReason: &finish, Usage: &completionUsage})
			response := httptest.NewRecorder()
			(&Handler{}).serveStream(response, context.Background(), streamTestModel{parts: parts}, "public", provider.CallOptions{})
			events := decodeEvents(t, response.Body.String())
			require.Len(t, events, 6)
			assert.Equal(t, "message_start", events[0]["type"])
			if tc.metadata {
				message := events[0]["message"].(map[string]any)
				assert.Equal(t, "msg_provider", message["id"])
				assert.Equal(t, float64(7), message["usage"].(map[string]any)["input_tokens"])
				assert.Equal(t, float64(2), message["usage"].(map[string]any)["cache_read_input_tokens"])
			}
			assert.Equal(t, "message_delta", events[4]["type"])
			assert.Equal(t, map[string]any{"input_tokens": float64(7), "output_tokens": float64(3), "cache_read_input_tokens": float64(2), "cache_creation_input_tokens": float64(1)}, events[4]["usage"])
			assert.Equal(t, "message_stop", events[5]["type"])
		})
	}
}

func decodeEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			var event map[string]any
			require.NoError(t, json.Unmarshal([]byte(data), &event))
			events = append(events, event)
		}
	}
	return events
}

type streamTestModel struct{ parts []provider.StreamPart }

func (streamTestModel) SpecificationVersion() string               { return "v4" }
func (streamTestModel) Provider() string                           { return "synthetic" }
func (streamTestModel) ModelID() string                            { return "synthetic" }
func (streamTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m streamTestModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, len(m.parts))
	for _, part := range m.parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}
func (streamTestModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}

var _ provider.LanguageModel = streamTestModel{}
