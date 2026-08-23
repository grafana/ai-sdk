package v4

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnarySuccessSchema(t *testing.T) {
	compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
	require.NoError(t, err)

	valid := []string{
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"length"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"content-filter"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"tool-calls"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"error"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[{"type":"text","text":""}],"finishReason":{"unified":"other","raw":""},"usage":{"inputTokens":{"total":9007199254740991,"noCache":0,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":0,"text":0,"reasoning":0}},"warnings":[{"type":"unsupported","feature":"","details":""},{"type":"compatibility","feature":""},{"type":"deprecated","setting":"","message":""},{"type":"other","message":""}],"response":{"id":"","modelId":"public/model","timestamp":"2026-08-22T00:00:00.123456789+02:30"}}`,
	}
	for _, document := range valid {
		require.NoError(t, compiled.Validate(json.RawMessage(document)), document)
	}

	invalid := []string{
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`,
		`{"content":[{"type":"reasoning","text":"private"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[{"type":"text"}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"future"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{"total":-1},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{"total":1.5},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{"total":9007199254740992}},"warnings":[],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"unsupported"}],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"compatibility"}],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"deprecated","setting":""}],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"deprecated","message":""}],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"other"}],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"other","message":"","details":"extra"}],"response":{"modelId":"public/model"}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":""}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model","headers":{}}}`,
		`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"modelId":"public/model","timestamp":"not-a-date"}}`,
	}
	for _, document := range invalid {
		assert.Error(t, compiled.Validate(json.RawMessage(document)), document)
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
		`{"type":"error","error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error","statusCode":500,"retryable":true}}`,
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

	require.NoError(t, compiled.Validate(json.RawMessage(canonicalInternalError)))
	require.NoError(t, compiled.Validate(json.RawMessage(canonicalInvalidRequestError)))

	invalid := []string{
		`{"message":"invalid request"}`,
		`{"error":{"type":"invalid_request_error","param":null,"code":"invalid_request"}}`,
		`{"error":{"message":"invalid request","param":null,"code":"invalid_request"}}`,
		`{"error":{"message":"invalid request","type":"invalid_request_error","code":"invalid_request"}}`,
		`{"error":{"message":"invalid request","type":"invalid_request_error","param":null}}`,
		`{"error":{"message":"invalid request","type":"unknown","param":null,"code":"invalid_request"}}`,
		`{"error":{"message":"invalid request","type":"invalid_request_error","param":"field","code":"invalid_request"}}`,
		`{"error":{"message":"invalid request","type":"authentication_error","param":null,"code":"timeout"}}`,
		`{"error":{"message":"https://provider.invalid secret","type":"internal_server_error","param":null,"code":"internal_error"}}`,
		`{"error":{"message":"unsupported capability: unknown","type":"invalid_request_error","param":null,"code":"invalid_request"}}`,
		`{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request","retryable":false}}`,
		`{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"},"generationId":"private"}`,
	}
	for _, document := range invalid {
		assert.Error(t, compiled.Validate(json.RawMessage(document)), document)
	}
}
