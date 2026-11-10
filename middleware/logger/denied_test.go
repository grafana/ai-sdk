package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errPolicyDenied = errors.New("policy denied")

func isPolicyDenied(err error) bool { return errors.Is(err, errPolicyDenied) }

func TestMiddleware_DeniedCalls(t *testing.T) {
	deniedPart := provider.NewAPICallError(provider.APICallErrorOptions{Message: "denied", Cause: errPolicyDenied})
	for _, tc := range []struct {
		name      string
		isDenied  func(error) bool
		model     *mockModel
		stream    bool
		event     EventKind
		level     slog.Level
		outcome   string
		errorType string
	}{
		{
			name:     "generate error",
			isDenied: isPolicyDenied,
			model: &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, fmt.Errorf("wrapped: %w", errPolicyDenied)
			}},
			event: EventGenerateError, level: slog.LevelInfo, outcome: outcomeDenied, errorType: errorClassDenied,
		},
		{
			name:     "stream setup error",
			isDenied: isPolicyDenied,
			model: &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return nil, errPolicyDenied
			}},
			stream: true, event: EventStreamError, level: slog.LevelInfo, outcome: outcomeDenied, errorType: errorClassDenied,
		},
		{
			name:     "stream error part",
			isDenied: isPolicyDenied,
			model:    streamingModel(provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "a"}, provider.StreamPart{Type: provider.PartError, APICallError: deniedPart}),
			stream:   true, event: EventStreamError, level: slog.LevelInfo, outcome: outcomeDenied, errorType: errorClassDenied,
		},
		{
			name:   "nil classifier",
			model:  streamingModel(provider.StreamPart{Type: provider.PartError, APICallError: deniedPart}),
			stream: true, event: EventStreamError, level: slog.LevelError, outcome: outcomeError, errorType: "api_call_error",
		},
		{
			name:     "panicking classifier",
			isDenied: func(error) bool { panic("classifier") },
			model: &mockModel{generateFunc: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errPolicyDenied
			}},
			event: EventGenerateError, level: slog.LevelError, outcome: outcomeError, errorType: "unknown",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := newTestHandler()
			wrapped := Wrap(tc.model, Options{Logger: slog.New(handler), IsDenied: tc.isDenied})
			if tc.stream {
				result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
				if err == nil {
					parts := drainParts(result.Stream)
					assert.Equal(t, provider.PartError, parts[len(parts)-1].Type)
				} else {
					assert.ErrorIs(t, err, errPolicyDenied)
				}
			} else {
				_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{})
				assert.ErrorIs(t, err, errPolicyDenied)
			}

			records := handler.Records()
			require.Len(t, records, 2)
			terminal := records[1]
			assert.Equal(t, string(tc.event), terminal.Message)
			assert.Equal(t, tc.level, terminal.Level)
			attrs := terminal.AttrsMap()
			assert.Equal(t, tc.outcome, attrs["ai_sdk.outcome"])
			assert.Equal(t, false, attrs["ai_sdk.success"])
			assert.Equal(t, tc.errorType, attrs["ai_sdk.error.type"])
		})
	}
}

func streamingModel(parts ...provider.StreamPart) *mockModel {
	return &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, len(parts))
		for _, part := range parts {
			stream <- part
		}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
}

func drainParts(stream <-chan provider.StreamPart) []provider.StreamPart {
	var parts []provider.StreamPart
	for part := range stream {
		parts = append(parts, part)
	}
	return parts
}

func TestMiddleware_DeniedStreamPartRecord(t *testing.T) {
	handler := newTestHandler()
	deniedPart := provider.NewAPICallError(provider.APICallErrorOptions{Message: "denied", Cause: errPolicyDenied})
	model := streamingModel(provider.StreamPart{Type: provider.PartError, APICallError: deniedPart})
	wrapped := Wrap(model, Options{Logger: slog.New(handler), IsDenied: isPolicyDenied, LogStreamParts: true})

	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	drainParts(result.Stream)

	var partRecords []capturedRecord
	for _, record := range handler.Records() {
		if record.Message == string(EventStreamPart) {
			partRecords = append(partRecords, record)
		}
	}
	require.Len(t, partRecords, 1)
	assert.Equal(t, slog.LevelDebug, partRecords[0].Level)
	assert.Equal(t, errorClassDenied, partRecords[0].AttrsMap()["ai_sdk.error.type"])
}
