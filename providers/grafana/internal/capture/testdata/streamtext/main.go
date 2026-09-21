package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
)

func run() (string, error) {
	var input struct {
		BaseURL         string      `json:"baseURL"`
		AccessToken     string      `json:"accessToken"`
		Headers         http.Header `json:"headers"`
		ModelID         string      `json:"modelID"`
		Prompt          string      `json:"prompt"`
		MaxOutputTokens int         `json:"maxOutputTokens"`
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input); err != nil {
		return "", err
	}
	client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: input.AccessToken, BaseURL: input.BaseURL, Headers: input.Headers})
	if err != nil {
		return "", err
	}
	model, err := client.LanguageModel(input.ModelID)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := aisdk.StreamText(ctx, model,
		aisdk.WithModelMessages(provider.UserText(input.Prompt)),
		aisdk.WithMaxOutputTokens(input.MaxOutputTokens),
		aisdk.WithMaxRetries(0),
	)
	for range result.FullStream() {
	}
	return result.Text(), result.Err()
}

func main() {
	text, err := run()
	result := map[string]any{"text": text}
	if err != nil {
		result["error"] = err.Error()
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(1)
	}
}
