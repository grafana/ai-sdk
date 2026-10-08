package v4

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
	//go:embed schema/gateway_execution.json
	executionSchemaJSON []byte
)

var (
	canonicalInvalidRequestError = []byte(`{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	canonicalAuthenticationError = []byte(`{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`)
	canonicalPermissionError     = []byte(`{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`)
	canonicalModelNotFoundError  = []byte(`{"error":{"message":"model not found","type":"model_not_found","param":null,"code":"model_not_found"}}`)
	canonicalRateLimitError      = []byte(`{"error":{"message":"rate limit exceeded","type":"rate_limit_exceeded","param":null,"code":"rate_limit_exceeded"}}`)
	canonicalOverloadError       = []byte(`{"error":{"message":"service overloaded","type":"internal_server_error","param":null,"code":"overloaded"}}`)
	canonicalDependencyError     = []byte(`{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`)
	canonicalUpstreamError       = []byte(`{"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error"}}`)
	canonicalTimeoutError        = []byte(`{"error":{"message":"request timed out","type":"internal_server_error","param":null,"code":"timeout"}}`)
	canonicalCancellationError   = []byte(`{"error":{"message":"request canceled","type":"internal_server_error","param":null,"code":"canceled"}}`)
	canonicalInternalError       = []byte(`{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`)

	unsupportedCustomContentError    = []byte(`{"error":{"message":"unsupported capability: custom-content","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedToolsError            = []byte(`{"error":{"message":"unsupported capability: tools","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedToolApprovalsError    = []byte(`{"error":{"message":"unsupported capability: tool-approvals","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedStructuredOutputError = []byte(`{"error":{"message":"unsupported capability: structured-output","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedRawOutputError        = []byte(`{"error":{"message":"unsupported capability: raw-output","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	unsupportedProviderOptionsError  = []byte(`{"error":{"message":"unsupported capability: provider-options","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)

	reservedProviderOptionsError = []byte(`{"error":{"message":"reserved provider option namespace","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
	protectedCallHeaderError     = []byte(`{"error":{"message":"protected call header","type":"invalid_request_error","param":null,"code":"invalid_request"}}`)
)

var (
	canonicalRateLimitStreamErrorFrame    = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"rate limit exceeded\",\"type\":\"rate_limit_exceeded\",\"param\":null,\"code\":\"rate_limit_exceeded\",\"statusCode\":429,\"retryable\":true}}\n\n")
	canonicalOverloadStreamErrorFrame     = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"service overloaded\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"overloaded\",\"statusCode\":503,\"retryable\":true}}\n\n")
	canonicalDependencyStreamErrorFrame   = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"failed dependency\",\"type\":\"failed_dependency\",\"param\":null,\"code\":\"failed_dependency\",\"statusCode\":424,\"retryable\":false}}\n\n")
	canonicalUpstreamStreamErrorFrame     = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"upstream failure\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"upstream_error\",\"statusCode\":502,\"retryable\":true}}\n\n")
	canonicalTimeoutStreamErrorFrame      = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"request timed out\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"timeout\",\"statusCode\":504,\"retryable\":true}}\n\n")
	canonicalCancellationStreamErrorFrame = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"request canceled\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"canceled\",\"statusCode\":499,\"retryable\":false}}\n\n")
	canonicalInternalStreamErrorFrame     = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"internal error\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"internal_error\",\"statusCode\":500,\"retryable\":true}}\n\n")
)

type wireSchemaValidator struct {
	schema *jsonschema.Schema
}

func compileWireSchema(t *testing.T, data []byte) *wireSchemaValidator {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	require.NoError(t, err)
	require.NoError(t, compiler.AddResource("schema.json", document))
	compiled, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	return &wireSchemaValidator{schema: compiled}
}

func (v *wireSchemaValidator) Validate(data json.RawMessage) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return v.schema.Validate(document)
}

func TestUnarySuccessSchema(t *testing.T) {
	compiled := compileWireSchema(t, unarySuccessSchemaJSON)

	valid := []byte(`{"content":[{"type":"text","text":""}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`)
	require.NoError(t, compiled.Validate(json.RawMessage(valid)))
	require.NoError(t, compiled.Validate(json.RawMessage(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{},"raw":{"native":{"tokens":[1,null,true]}}},"warnings":[],"response":{}}`)))

	invalid := [][]byte{
		[]byte(`{"content":[],"finishReason":{"unified":"stop"}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"future"},"usage":{"inputTokens":{},"outputTokens":{}}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{"total":-1},"outputTokens":{}}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"response":{}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{},"raw":null}}`),
		[]byte(`{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{},"raw":[],"private":1}}`),
	}
	for _, document := range invalid {
		require.Error(t, compiled.Validate(json.RawMessage(document)))
	}
}

func TestStreamEventSchema(t *testing.T) {
	compiled := compileWireSchema(t, streamEventSchemaJSON)
	valid := []string{
		`{"type":"stream-start","warnings":[]}`,
		`{"type":"stream-start","warnings":[{"type":"unsupported","feature":"model capability","details":"a requested model capability is unsupported"},{"type":"compatibility","feature":"model compatibility","details":"a requested setting was adjusted for model compatibility"},{"type":"deprecated","setting":"model setting","message":"a requested model setting is deprecated"},{"type":"other","message":"the model reported a warning"}]}`,
		`{"type":"response-metadata","id":"","modelId":"public/model","timestamp":"2026-08-22T00:00:00Z"}`,
		`{"type":"text-start","id":"a"}`,
		`{"type":"text-delta","id":"a","delta":""}`,
		`{"type":"text-end","id":"a"}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"stop"}}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{},"raw":{"native":{"tokens":[1,null,true]}}},"finishReason":{"unified":"stop"}}`,
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
		`{"type":"stream-start","warnings":[{"type":"other"}]}`,
		`{"type":"response-metadata","modelId":null}`,
		`{"type":"response-metadata","modelId":"public","provider":"private"}`,
		`{"type":"text-start","id":""}`,
		`{"type":"text-delta","id":"a"}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{}},"finishReason":{"unified":"future"}}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{},"raw":null},"finishReason":{"unified":"stop"}}`,
		`{"type":"finish","usage":{"inputTokens":{},"outputTokens":{},"raw":[],"private":1},"finishReason":{"unified":"stop"}}`,
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
	compiled := compileWireSchema(t, errorSchemaJSON)

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
		unsupportedCustomContentError,
		unsupportedToolsError,
		unsupportedToolApprovalsError,
		unsupportedStructuredOutputError,
		reservedProviderOptionsError,
		protectedCallHeaderError,
		unsupportedRawOutputError,
	}
	for _, document := range documents {
		require.NoError(t, compiled.Validate(json.RawMessage(document)), string(document))
	}

	require.Error(t, compiled.Validate(json.RawMessage(`{"error":{"message":"private","type":"internal_server_error","param":null,"code":"internal_error","extra":true}}`)))
}

func TestExecutionSchema_ProducedCarriers(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			t.Run(map[bool]string{false: "unary", true: "stream"}[streaming]+"/"+map[bool]string{false: "success", true: "error"}[failure], func(t *testing.T) {
				nativeError := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "upstream busy"})
				first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if failure {
						return nil, nativeError
					}
					return validGenerateResult(), nil
				}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					if failure {
						return nil, nativeError
					}
					return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: nativeError}, finishPart())}, nil
				}}
				h := invocationHarness(t, testLimits(), first)
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request = streamRequest(`{"prompt":[]}`)
				}
				response := h.serve(request)
				if failure {
					require.NoError(t, compileWireSchema(t, errorSchemaJSON).Validate(response.Body.Bytes()))
					return
				}
				if !streaming {
					require.NoError(t, compileWireSchema(t, unarySuccessSchemaJSON).Validate(response.Body.Bytes()))
					return
				}
				for _, frame := range strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n") {
					require.NoError(t, compileWireSchema(t, streamEventSchemaJSON).Validate([]byte(strings.TrimPrefix(frame, "data: "))))
				}
			})
		}
	}
}

func TestExecutionSchema_ProducedNamespace(t *testing.T) {
	compiled := compileWireSchema(t, executionSchemaJSON)
	for _, outcome := range []fallback.AttemptOutcome{fallback.AttemptSelected, fallback.AttemptFailed, fallback.AttemptCanceled} {
		t.Run(string(outcome), func(t *testing.T) {
			failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Data: json.RawMessage(`{"error":{"message":"overloaded","code":900719925474099312345}}`)})
			overview := execution.Project("alias", "public", []fallback.Attempt{
				{Index: 1, Provider: "native", ModelID: "first", Outcome: fallback.AttemptFailed, SourceErr: failure, WillFallback: true},
				{Index: 2, Provider: "native", ModelID: "second", Outcome: outcome},
			}, nil, nil, 4096)
			require.NotNil(t, overview)
			metadata := execution.Metadata(overview, provider.ProviderMetadata{"gateway": json.RawMessage(`{"execution":{"native":true},"null":null}`)})
			require.NoError(t, compiled.Validate(metadata["gateway"]))
		})
	}
}
