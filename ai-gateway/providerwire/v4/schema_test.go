package v4

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	//go:embed schema/unary_success.json
	unarySuccessSchemaJSON []byte
	//go:embed schema/error.json
	errorSchemaJSON []byte
	//go:embed schema/stream_event.json
	streamEventSchemaJSON []byte
)

func TestUnarySuccessSchema(t *testing.T) {
	compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
	require.NoError(t, err)

	valid := []byte(`{"content":[{"type":"text","text":""}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`)
	require.NoError(t, compiled.Validate(json.RawMessage(valid)))

	invalid := [][]byte{
		[]byte(`{"content":[],"finishReason":{"unified":"stop"}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"future"},"usage":{"inputTokens":{},"outputTokens":{}}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{"total":-1},"outputTokens":{}}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"response":{}}`),
	}
	for _, document := range invalid {
		require.Error(t, compiled.Validate(json.RawMessage(document)))
	}
}

func TestStreamEventSchema(t *testing.T) {
	compiled, err := schema.CompileSchema(streamEventSchemaJSON)
	require.NoError(t, err)
	valid := []string{
		`{"type":"stream-start","warnings":[]}`,
		`{"type":"stream-start","warnings":[{"type":"unsupported","feature":"model capability","details":"a requested model capability is unsupported"},{"type":"compatibility","feature":"model compatibility","details":"a requested setting was adjusted for model compatibility"},{"type":"deprecated","setting":"model setting","message":"a requested model setting is deprecated"},{"type":"other","message":"the model reported a warning"}]}`,
		`{"type":"response-metadata","id":"","modelId":"public/model","timestamp":"2026-08-22T00:00:00Z"}`,
		`{"type":"text-start","id":"a"}`,
		`{"type":"text-delta","id":"a","delta":""}`,
		`{"type":"text-end","id":"a"}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"stop"}}`,
	}
	for _, frame := range [][]byte{
		canonicalRateLimitStreamErrorFrame,
		canonicalOverloadStreamErrorFrame,
		canonicalDependencyStreamErrorFrame,
		canonicalUpstreamStreamErrorFrame,
		canonicalTimeoutStreamErrorFrame,
		canonicalCancellationStreamErrorFrame,
		canonicalInternalStreamErrorFrame,
	} {
		valid = append(valid, string(frame[len("data: "):len(frame)-len("\n\n")]))
	}
	for _, document := range valid {
		require.NoError(t, compiled.Validate(json.RawMessage(document)), document)
	}
	invalid := []string{
		`{"type":"stream-start"}`,
		`{"type":"stream-start","warnings":[{"type":"other","message":"private"}]}`,
		`{"type":"response-metadata","modelId":""}`,
		`{"type":"response-metadata","modelId":"public","provider":"private"}`,
		`{"type":"text-start","id":""}`,
		`{"type":"text-delta","id":"a"}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"future"}}`,
		`{"type":"error","error":{"message":"private","type":"internal_server_error","param":null,"code":"internal_error","statusCode":500,"retryable":true}}`,
		`{"type":"error","error":{"message":"internal error","type":"rate_limit_exceeded","param":null,"code":"rate_limit_exceeded","statusCode":429,"retryable":true}}`,
		`{"type":"error","error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error","statusCode":500,"retryable":true,"details":"private"}}`,
		`{"type":"raw","rawValue":{}}`,
	}
	for _, document := range invalid {
		assert.Error(t, compiled.Validate(json.RawMessage(document)), document)
	}
}

func TestErrorSchema(t *testing.T) {
	compiled, err := schema.CompileSchema(errorSchemaJSON)
	require.NoError(t, err)

	documents := [][]byte{
		canonicalInvalidRequestError,
		canonicalModelNotFoundError,
		canonicalRateLimitError,
		canonicalOverloadError,
		canonicalDependencyError,
		canonicalUpstreamError,
		canonicalTimeoutError,
		canonicalCancellationError,
		canonicalInternalError,
		unsupportedFilesError,
		unsupportedReasoningContentError,
		unsupportedCustomContentError,
		unsupportedToolsError,
		unsupportedToolApprovalsError,
		unsupportedStructuredOutputError,
		unsupportedProviderOptionsError,
		unsupportedBodyHeadersError,
		unsupportedRawOutputError,
	}
	for _, document := range documents {
		require.NoError(t, compiled.Validate(json.RawMessage(document)), string(document))
	}

	require.Error(t, compiled.Validate(json.RawMessage(`{"error":{"message":"private","type":"internal_server_error","param":null,"code":"internal_error","extra":true}}`)))
}
