package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func init() { registerScenario("chat-content-array", handleChatContentArray) }

func handleChatContentArray(w http.ResponseWriter, r *http.Request) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"delta":{"content":[{"type":"text","text":"First."}]}}]}`,
			`{"choices":[{"delta":{"content":[{"type":"thinking","thinking":[{"type":"text","text":"Check "},{"type":"text","text":"the result."}]},{"type":"future-part"},{"type":"text","text":"Done."}]},"finish_reason":"stop"}]}`,
			`[DONE]`,
		}
		for _, chunk := range chunks {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	model := openaicompatible.New("test", openaicompatible.WithBaseURL(server.URL))
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("hi")),
		aisdk.WithGenerateID(func() string { return "second-text" }),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
