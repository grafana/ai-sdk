package v4

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/require"
)

var (
	//go:embed schema/unary_success.json
	unarySuccessSchemaJSON []byte
	//go:embed schema/error.json
	errorSchemaJSON []byte
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
