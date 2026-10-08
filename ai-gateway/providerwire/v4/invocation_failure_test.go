package v4

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvocation_CommittedErrorEnrichmentLimits(t *testing.T) {
	native := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: strings.Repeat("detail", 128)})
	first := &recordingModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(
			provider.StreamPart{Type: provider.PartError, APICallError: native},
			provider.StreamPart{Type: provider.PartTextStart, ID: "a"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "a", Delta: "after"},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "a"},
			finishPart(),
		)}, nil
	}}
	full := invocationHarness(t, testLimits(), first).serve(streamRequest(`{"prompt":[]}`)).Body.String()
	fullFrames := strings.Split(strings.TrimSuffix(full, "\n\n"), "\n\n")
	require.Len(t, fullFrames, 6)
	completeSize := len(fullFrames[1]) + len("\n\n")
	for _, tc := range []struct {
		name  string
		limit int
	}{
		{"exact", completeSize},
		{"one byte short", completeSize - 1},
		{"canonical only", minimumStreamFrameBytes()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.StreamFrameBytes = int64(tc.limit)
			before := first.callCount()
			response := invocationHarness(t, limits, first).serve(streamRequest(`{"prompt":[]}`))
			require.Equal(t, 200, response.Code)
			requireStreamBodyMatchesSchema(t, response.Body.String())
			frames := strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n")
			require.Len(t, frames, 6)
			var envelope invocationErrorDocument
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &envelope))
			assert.Equal(t, 503, envelope.Error.StatusCode)
			assert.Equal(t, errorCode("overloaded"), envelope.Error.Code)
			require.NotNil(t, envelope.Error.Retryable)
			assert.True(t, *envelope.Error.Retryable)
			for _, frame := range frames {
				assert.LessOrEqual(t, len(frame)+len("\n\n"), tc.limit)
			}
			switch tc.name {
			case "exact":
				assert.Equal(t, fullFrames[1], frames[1])
			case "one byte short":
				assert.NotEqual(t, fullFrames[1], frames[1])
				require.NotNil(t, envelope.Error.Data)
				assert.Empty(t, envelope.Error.Data.Metadata)
				require.NotNil(t, envelope.Error.Data.NativeError)
				assert.Equal(t, native.Message, envelope.Error.Data.NativeError.Message)
			case "canonical only":
				assert.Equal(t, strings.TrimSuffix(string(canonicalOverloadStreamErrorFrame), "\n\n"), frames[1])
			}
			assert.Contains(t, frames[3], `"delta":"after"`)
			assert.Contains(t, frames[5], `"type":"finish"`)
			assert.Equal(t, before+1, first.callCount())
		})
	}
}

func TestInvocation_DirectInvalidStreamSetup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result *provider.StreamResult
		status int
	}{
		{"nil result", nil, 500},
		{"nil channel", &provider.StreamResult{}, 500},
		{"EOF before first part", &provider.StreamResult{Stream: makeStream()}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := &recordingModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return tc.result, nil }}
			response := invocationHarness(t, testLimits(), first).serve(streamRequest(`{"prompt":[]}`))
			require.Equal(t, tc.status, response.Code)
			var metadata provider.ProviderMetadata
			if tc.status == 200 {
				assert.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
				requireStreamBodyMatchesSchema(t, response.Body.String())
				frames := strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n")
				require.Len(t, frames, 2)
				var envelope invocationErrorDocument
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &envelope))
				require.NotNil(t, envelope.Error.Data)
				metadata = envelope.Error.Data.Metadata
			} else {
				assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
				var envelope invocationErrorDocument
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
				metadata = envelope.Metadata
			}
			var namespace struct {
				Overview *execution.Overview `json:"execution"`
			}
			require.NoError(t, json.Unmarshal(metadata["gateway"], &namespace))
			require.NotNil(t, namespace.Overview)
			require.Len(t, namespace.Overview.Attempts, 1)
			assert.Equal(t, fallback.AttemptFailed, namespace.Overview.Attempts[0].Outcome)
			assert.Equal(t, "A", namespace.Overview.Attempts[0].ModelID)
			assert.Equal(t, 1, first.callCount())
		})
	}
}
