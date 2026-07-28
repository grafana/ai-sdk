package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/providers/anthropic"
)

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required")
	}

	model := anthropic.New(apiKey, "claude-sonnet-5")
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []aisdk.UIMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		result := aisdk.StreamText(r.Context(), model,
			aisdk.WithSystem("You are a helpful assistant."),
			aisdk.WithMessages(body.Messages...),
		)
		if err := aisdk.WriteUIMessageStream(
			w,
			result,
			aisdk.WithUIMessageStreamReasoning(false),
		); err != nil {
			log.Printf("streaming chat response: %v", err)
		}
	})

	server := http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("listening on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
