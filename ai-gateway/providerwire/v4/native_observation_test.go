package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/middleware/logger"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/grafana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeResponse_ConsumerObservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	harness := newRuntimeHarness(t, testLimits())
	source := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "native-source", Title: "file-native", Filename: "file-native", MediaType: "text/plain"}
	identity := provider.ResponseMetadata{ID: "native-response", ModelID: "native model ☃", Timestamp: time.Date(2026, 9, 30, 12, 0, 0, 123000000, time.UTC)}
	harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, Title: source.Title, Filename: source.Filename, MediaType: source.MediaType}}, Warnings: nativeWarnings(), Response: &provider.GenerateResponse{ResponseMetadata: identity}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
	}
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: nativeWarnings()}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: identity.ID, ModelID: identity.ModelID, Timestamp: identity.Timestamp}, provider.StreamPart{Type: provider.PartSource, Source: &source}, finishPart())}, nil
	}
	server := httptest.NewServer(harness.handler)
	defer server.Close()
	client, err := grafana.NewWithAccessToken(grafana.AccessTokenConfig{AccessToken: "dummy-client-token", BaseURL: server.URL})
	require.NoError(t, err)
	model, err := client.LanguageModel("requested-alias")
	require.NoError(t, err)
	var seen *provider.GenerateResult
	observed := make(chan provider.StreamPart, 4)
	observer := middleware.Middleware{
		WrapGenerate: func(ctx context.Context, params middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			result, err := params.DoGenerate(ctx)
			if err == nil {
				seen = result
			}
			return result, err
		},
		WrapStream: func(ctx context.Context, params middleware.WrapStreamParams) (*provider.StreamResult, error) {
			result, err := params.DoStream(ctx)
			if err != nil {
				return nil, err
			}
			forwarded := make(chan provider.StreamPart)
			go func() {
				defer close(forwarded)
				defer close(observed)
				for {
					select {
					case part, ok := <-result.Stream:
						if !ok {
							return
						}
						select {
						case observed <- part:
						case <-ctx.Done():
							return
						}
						select {
						case forwarded <- part:
						case <-ctx.Done():
							return
						}
					case <-ctx.Done():
						return
					}
				}
			}()
			copy := *result
			copy.Stream = forwarded
			return &copy, nil
		},
	}
	var consumerLogs bytes.Buffer
	wrapped := middleware.WrapLanguageModel(model, observer, logger.Middleware(logger.Options{Logger: slog.New(slog.NewJSONHandler(&consumerLogs, nil)), Capture: logger.CaptureOptions{ResponseBody: true, MaxJSONBytes: 16384}}))
	result, err := wrapped.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	require.Same(t, seen, result)
	require.NotNil(t, result.Response)
	assert.Empty(t, result.Response.ID)
	assert.Empty(t, result.Response.ModelID)
	assert.True(t, result.Response.Timestamp.IsZero())
	assert.Equal(t, source.ID, result.Content[0].ID)
	assert.Equal(t, source.Title, result.Content[0].Title)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(result.Response.Body, &body))
	assert.JSONEq(t, nativeWarningsJSON, string(body["warnings"]))
	assert.JSONEq(t, `{"id":"native-response","modelId":"native model ☃","timestamp":"2026-09-30T12:00:00.123Z"}`, string(body["response"]))
	var captured map[string]json.RawMessage
	for _, line := range bytes.Split(bytes.TrimSpace(consumerLogs.Bytes()), []byte{'\n'}) {
		var record map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(line, &record))
		if value, ok := record["ai_sdk.response.body"]; ok {
			require.NoError(t, json.Unmarshal(value, &captured))
		}
	}
	require.NotNil(t, captured, "actual consumer log destination must contain the response body")
	for _, field := range []string{"warnings", "response", "content"} {
		assert.JSONEq(t, string(body[field]), string(captured[field]))
	}
	stream, err := wrapped.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	var returned []provider.StreamPart
	for part := range stream.Stream {
		returned = append(returned, part)
	}
	var received []provider.StreamPart
	for part := range observed {
		received = append(received, part)
	}
	require.Len(t, returned, 4)
	assert.Equal(t, returned, received)
	assert.Equal(t, identity.ID, returned[1].ResponseID)
	assert.Equal(t, identity.ModelID, returned[1].ModelID)
	assert.Equal(t, identity.Timestamp, returned[1].Timestamp)
	assert.Equal(t, source, *returned[2].Source)
	assert.Equal(t, "requested-alias", harness.resolver.requestedModelID())
	generate, streaming := harness.model.invocationCounts()
	assert.Equal(t, 1, generate)
	assert.Equal(t, 1, streaming)
	assert.NotContains(t, consumerLogs.String(), "dummy-client-token")
}
