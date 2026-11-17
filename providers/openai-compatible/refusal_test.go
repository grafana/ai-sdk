package openaicompatible

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/provider"
)

func TestDoGenerate_Refusal(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		msg    string
		finish string
		texts  []string
		want   provider.UnifiedFinishReason
	}{
		{name: "refusal replaces null content", msg: `{"role":"assistant","content":null,"refusal":"I can't help."}`, finish: "stop", texts: []string{"I can't help."}, want: provider.FinishReasonContentFilter},
		{name: "content and refusal are both kept", msg: `{"role":"assistant","content":"Partial.","refusal":"I can't continue."}`, finish: "stop", texts: []string{"Partial.", "I can't continue."}, want: provider.FinishReasonContentFilter},
		{name: "non-string refusal is ignored", msg: `{"role":"assistant","content":"Hello","refusal":false}`, finish: "stop", texts: []string{"Hello"}, want: provider.FinishReasonStop},
		{name: "empty refusal is ignored", msg: `{"role":"assistant","content":"Hello","refusal":""}`, finish: "stop", texts: []string{"Hello"}, want: provider.FinishReasonStop},
		{name: "only stop becomes content-filter", msg: `{"role":"assistant","content":null,"refusal":"I can't"}`, finish: "length", texts: []string{"I can't"}, want: provider.FinishReasonLength},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"chatcmpl_1","created":1,"model":"gpt-4o","choices":[{"index":0,"message":` + tc.msg + `,"finish_reason":"` + tc.finish + `"}]}`))
			}))
			defer server.Close()

			result, err := New("gpt-4o", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)
			var texts []string
			for _, part := range result.Content {
				require.Equal(t, provider.ContentText, part.Type)
				texts = append(texts, part.Text)
			}
			assert.Equal(t, tc.texts, texts)
			assert.Equal(t, tc.want, result.FinishReason.Unified)
			assert.Equal(t, tc.finish, result.FinishReason.Raw)
		})
	}
}

func TestDoStream_Refusal(t *testing.T) {
	t.Parallel()

	chunk := func(delta, finish string) string {
		reason := "null"
		if finish != "" {
			reason = `"` + finish + `"`
		}
		return `data: {"id":"chatcmpl_1","created":1,"model":"gpt-4o","choices":[{"index":0,"delta":` + delta + `,"finish_reason":` + reason + `}]}` + "\n\n"
	}
	for _, tc := range []struct {
		name   string
		chunks []string
		text   string
		want   provider.UnifiedFinishReason
	}{
		{
			name:   "refusal deltas stream as text",
			chunks: []string{chunk(`{"role":"assistant","content":null,"refusal":""}`, ""), chunk(`{"refusal":"I can't"}`, ""), chunk(`{"refusal":" help."}`, ""), chunk(`{}`, "stop")},
			text:   "I can't help.",
			want:   provider.FinishReasonContentFilter,
		},
		{
			name:   "a refusal in the finish chunk turns stop into content-filter",
			chunks: []string{chunk(`{"content":"Partial. "}`, ""), chunk(`{"refusal":"I can't continue."}`, "stop")},
			text:   "Partial. I can't continue.",
			want:   provider.FinishReasonContentFilter,
		},
		{
			name:   "non-string refusal is ignored",
			chunks: []string{chunk(`{"content":"Hello","refusal":false}`, "stop")},
			text:   "Hello",
			want:   provider.FinishReasonStop,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(strings.Join(tc.chunks, "") + "data: [DONE]\n\n"))
			}))
			defer server.Close()

			result, err := New("gpt-4o", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{provider.UserText("hi")}})
			require.NoError(t, err)
			var text strings.Builder
			textStarts := 0
			var finish *provider.FinishReason
			for _, part := range collectStreamParts(result) {
				switch part.Type {
				case provider.PartTextStart:
					textStarts++
				case provider.PartTextDelta:
					text.WriteString(part.Delta)
				case provider.PartFinish:
					finish = part.FinishReason
				}
			}
			assert.Equal(t, 1, textStarts, "content and refusal share one text part")
			assert.Equal(t, tc.text, text.String())
			require.NotNil(t, finish)
			assert.Equal(t, tc.want, finish.Unified)
			assert.Equal(t, "stop", finish.Raw)
		})
	}
}
