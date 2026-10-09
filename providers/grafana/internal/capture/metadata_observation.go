package main

import (
	"context"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

func captureMetadataObservation(ctx context.Context, model provider.LanguageModel, options provider.CallOptions) error {
	var unary []provider.ProviderMetadata
	var streaming []provider.ProviderMetadata
	wrapped := middleware.WrapLanguageModel(model, middleware.Middleware{
		WrapGenerate: func(ctx context.Context, params middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			result, err := params.DoGenerate(ctx)
			if err != nil {
				return nil, err
			}
			unary = append(unary, result.ProviderMetadata)
			for _, part := range result.Content {
				unary = append(unary, part.ProviderMetadata)
			}
			return result, nil
		},
		WrapStream: func(ctx context.Context, params middleware.WrapStreamParams) (*provider.StreamResult, error) {
			result, err := params.DoStream(ctx)
			if err != nil {
				return nil, err
			}
			input := result.Stream
			output := make(chan provider.StreamPart, 64)
			go func() {
				defer close(output)
				for part := range input {
					metadata := part.ProviderMetadata
					if part.Source != nil {
						metadata = part.Source.ProviderMetadata
					}
					streaming = append(streaming, metadata)
					select {
					case output <- part:
					case <-ctx.Done():
						return
					}
				}
			}()
			result.Stream = output
			return result, nil
		},
	})
	result, err := wrapped.DoGenerate(ctx, options)
	if err != nil {
		return err
	}
	stream, err := wrapped.DoStream(ctx, options)
	if err != nil {
		return err
	}
	var returned []provider.ProviderMetadata
	for part := range stream.Stream {
		if part.Type == provider.PartError {
			return part.APICallError
		}
		metadata := part.ProviderMetadata
		if part.Source != nil {
			metadata = part.Source.ProviderMetadata
		}
		returned = append(returned, metadata)
	}
	emit(map[string]any{"observedUnary": unary, "observedStream": streaming, "returnedStream": returned, "resultMetadata": result.ProviderMetadata})
	return nil
}
