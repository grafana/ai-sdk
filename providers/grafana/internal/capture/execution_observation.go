package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

func captureExecutionObservation(ctx context.Context, model provider.LanguageModel, options provider.CallOptions, generate bool) {
	var observed []json.RawMessage
	var mutex sync.Mutex
	wrapped := middleware.WrapLanguageModel(model, middleware.Middleware{WrapStream: func(ctx context.Context, params middleware.WrapStreamParams) (*provider.StreamResult, error) {
		result, err := params.DoStream(ctx)
		if err != nil {
			return nil, err
		}
		input := result.Stream
		output := make(chan provider.StreamPart, 64)
		go func() {
			defer close(output)
			for part := range input {
				if part.Type == provider.PartError && part.APICallError != nil {
					mutex.Lock()
					observed = append(observed, part.APICallError.Data)
					mutex.Unlock()
				}
				select {
				case output <- part:
				case <-ctx.Done():
					return
				}
			}
		}()
		result.Stream = output
		return result, nil
	}})
	if generate {
		result, err := aisdk.GenerateText(ctx, wrapped, aisdk.WithModelMessages(options.Prompt...), aisdk.WithMaxRetries(0))
		if err != nil {
			emitError(err)
			return
		}
		emit(map[string]any{"text": result.Text, "metadata": result.ProviderMetadata})
		return
	}
	result := aisdk.StreamText(ctx, wrapped, aisdk.WithModelMessages(options.Prompt...), aisdk.WithMaxRetries(0))
	var normalized []json.RawMessage
	for part := range result.FullStream() {
		switch part := part.(type) {
		case aisdk.StreamError:
			var api *provider.APICallError
			if errors.As(part.Error, &api) {
				normalized = append(normalized, api.Data)
			}
		}
	}
	mutex.Lock()
	defer mutex.Unlock()
	emit(map[string]any{"observed": observed, "normalized": normalized, "text": result.Text(), "metadata": result.ProviderMetadata()})
}
