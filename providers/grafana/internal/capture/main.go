package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
)

func main() {
	var input struct {
		BaseURL         string                   `json:"baseURL"`
		AccessToken     string                   `json:"accessToken"`
		UserIDToken     string                   `json:"userIDToken"`
		Cloud           *grafana.CloudAuthConfig `json:"cloud"`
		Mode            string                   `json:"mode"`
		ModelID         string                   `json:"modelID"`
		Options         provider.CallOptions     `json:"options"`
		Headers         map[string][]string      `json:"headers"`
		AbortAfterParts int                      `json:"abortAfterParts"`
		AbortBefore     bool                     `json:"abortBefore"`
		CancelAfterMS   int                      `json:"cancelAfterMs"`
		PreferBytes     bool                     `json:"preferBytes"`
	}
	if err := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20)).Decode(&input); err != nil {
		emit(map[string]any{"error": map[string]any{"message": "invalid capture input"}})
		return
	}
	if input.PreferBytes {
		for messageIndex := range input.Options.Prompt {
			for partIndex := range input.Options.Prompt[messageIndex].Content {
				data := input.Options.Prompt[messageIndex].Content[partIndex].Data
				if data != nil && data.Base64 != "" {
					decoded, err := base64.StdEncoding.DecodeString(data.Base64)
					if err != nil {
						emit(map[string]any{"error": map[string]any{"message": "invalid capture binary input"}})
						return
					}
					data.Bytes, data.Base64 = decoded, ""
				}
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if input.AbortBefore {
		cancel()
	}
	if input.CancelAfterMS > 0 {
		stop := time.AfterFunc(time.Duration(input.CancelAfterMS)*time.Millisecond, cancel)
		defer stop.Stop()
	}
	if input.UserIDToken != "" {
		ctx = grafana.WithUserIDToken(ctx, input.UserIDToken)
	}
	var client *grafana.Provider
	var err error
	if input.Cloud != nil {
		client, err = grafana.NewWithCloudAuth(*input.Cloud)
	} else {
		client, err = grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: input.AccessToken, BaseURL: input.BaseURL, Headers: input.Headers})
	}
	if err != nil {
		emitError(err)
		return
	}
	if input.Mode == "discovery" {
		models, err := client.ListModels(ctx)
		if err != nil {
			emitError(err)
		} else {
			emit(map[string]any{"models": models})
		}
		return
	}
	model, err := client.LanguageModel(input.ModelID)
	if err != nil {
		emitError(err)
		return
	}
	if input.Mode == "stream" {
		result, err := model.DoStream(ctx, input.Options)
		if err != nil {
			emitError(err)
			return
		}
		parts := make([]provider.StreamPart, 0)
		for part := range result.Stream {
			parts = append(parts, part)
			if input.AbortAfterParts > 0 && len(parts) >= input.AbortAfterParts {
				cancel()
			}
		}
		emit(map[string]any{"parts": parts, "request": result.Request, "response": result.Response, "canceled": ctx.Err() != nil})
		return
	}
	result, err := model.DoGenerate(ctx, input.Options)
	if err != nil {
		emitError(err)
		return
	}
	emit(map[string]any{"result": result})
}

func emitError(err error) {
	value := map[string]any{"message": err.Error()}
	var gateway *grafana.GatewayError
	if errors.As(err, &gateway) {
		value["category"] = gateway.Category
		value["code"] = gateway.Code
		value["message"] = gateway.Message
	}
	var api *provider.APICallError
	if errors.As(err, &api) {
		value["statusCode"] = api.StatusCode
		value["isRetryable"] = api.IsRetryable
		value["apiError"] = api
	}
	causes := make([]string, 0)
	for cause := errors.Unwrap(err); cause != nil && len(causes) < 16; cause = errors.Unwrap(cause) {
		causes = append(causes, cause.Error())
	}
	value["causes"] = causes
	value["canceled"] = errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
	emit(map[string]any{"error": value})
}

func emit(value any) {
	if json.NewEncoder(os.Stdout).Encode(value) != nil {
		os.Exit(1)
	}
}
