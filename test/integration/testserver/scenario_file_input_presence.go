package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
)

func init() {
	registerScenario("file-input-presence", handleFileInputPresence)
}

func handleFileInputPresence(w http.ResponseWriter, r *http.Request) {
	var messages []aisdk.UIMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&messages); err != nil {
		http.Error(w, "invalid UI messages", http.StatusBadRequest)
		return
	}
	modelMessages, err := aisdk.ConvertToModelMessages(messages)
	if err != nil {
		http.Error(w, "invalid model messages", http.StatusBadRequest)
		return
	}
	captured := make(chan json.RawMessage, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		captured <- json.RawMessage(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"ok"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`)
	}))
	defer server.Close()
	client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "test-token", BaseURL: server.URL})
	if err != nil {
		http.Error(w, "client setup failed", http.StatusInternalServerError)
		return
	}
	model, err := client.LanguageModel("test-model")
	if err != nil {
		http.Error(w, "model setup failed", http.StatusInternalServerError)
		return
	}
	if _, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: modelMessages}); err != nil {
		http.Error(w, "file request failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"uiMessages":    messages,
		"modelMessages": modelMessages,
		"request":       <-captured,
	}); err != nil {
		return
	}
}
