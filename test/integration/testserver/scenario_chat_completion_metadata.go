package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func init() { registerScenario("chat-completion-metadata", handleChatCompletionMetadata) }

func chatCompletionMetadataServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunks := []string{
			`{"choices":[{"delta":{"role":"assistant"}}]}`,
			`{"choices":[{"delta":{"content":"hello ","refusal":"Cannot "},"logprobs":{"content":[{"token":"hello","logprob":-0.1}]}}]}`,
			`{"choices":[{"delta":{"content":"world","refusal":"help"},"logprobs":{"content":[{"token":"world","logprob":-0.2}]},"finish_reason":"stop"}]}`,
			`[DONE]`,
		}
		for _, chunk := range chunks {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
				return
			}
		}
	}))
}

func handleChatCompletionMetadata(w http.ResponseWriter, r *http.Request) {
	server := chatCompletionMetadataServer()
	defer server.Close()
	model := openaicompatible.New("test", openaicompatible.WithBaseURL(server.URL), openaicompatible.WithChatCompletionMetadata())
	result := aisdk.StreamText(r.Context(), model, aisdk.WithModelMessages(provider.UserText("hi")))
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
