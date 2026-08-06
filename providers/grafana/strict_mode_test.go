package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/gateway/providerwire"
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderWireMode_DefaultsAndValidation(t *testing.T) {
	legacy := newAccessTokenProvider(t, "https://example.com")
	assert.IsType(t, legacyWireCodec{}, legacy.wireCodec)
	assert.Equal(t, DefaultMaxUnaryResponseBytes, legacy.maxUnaryResponseBytes)
	assert.Equal(t, DefaultMaxErrorResponseBytes, legacy.maxErrorResponseBytes)
	assert.Equal(t, DefaultMaxSSEEventBytes, legacy.maxSSEEventBytes)

	strict := newAccessTokenProvider(t, "https://example.com", WithStrictProviderWire(), WithMaxUnaryResponseBytes(10), WithMaxErrorResponseBytes(11), WithMaxSSEEventBytes(12))
	assert.IsType(t, strictWireCodec{}, strict.wireCodec)
	assert.Equal(t, int64(10), strict.maxUnaryResponseBytes)
	assert.Equal(t, int64(11), strict.maxErrorResponseBytes)
	assert.Equal(t, int64(12), strict.maxSSEEventBytes)

	cloud, err := NewWithCloudAuth(CloudAuthConfig{CAPToken: "cap", TokenExchangeURL: "https://example.com/exchange", Namespace: "stack", BaseURL: "https://example.com"}, WithStrictProviderWire(), WithMaxUnaryResponseBytes(10), WithMaxErrorResponseBytes(11), WithMaxSSEEventBytes(12))
	require.NoError(t, err)
	assert.IsType(t, strictWireCodec{}, cloud.wireCodec)
	assert.Equal(t, int64(10), cloud.maxUnaryResponseBytes)
	assert.Equal(t, int64(11), cloud.maxErrorResponseBytes)
	assert.Equal(t, int64(12), cloud.maxSSEEventBytes)

	invalidOptions := []Option{
		WithProviderWireMode("future"),
		WithMaxUnaryResponseBytes(0),
		WithMaxErrorResponseBytes(-1),
		WithMaxSSEEventBytes(0),
		nil,
	}
	for _, option := range invalidOptions {
		_, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.com"}, option)
		require.Error(t, err)
		_, err = NewWithCloudAuth(CloudAuthConfig{CAPToken: "cap", TokenExchangeURL: "https://example.com/exchange", Namespace: "stack", BaseURL: "https://example.com"}, option)
		require.Error(t, err)
	}
}

func TestStrictMode_UsesCanonicalRequestAndResponseCodecs(t *testing.T) {
	timestamp := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "strict"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings:     []provider.Warning{{Type: provider.WarnOther, Message: "public"}},
		Response:     &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "response", Timestamp: timestamp}},
	}
	endpoint := newFakeHostedEndpoint(t, strictGenerateSuccess(result))
	providerClient := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
	model, err := providerClient.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)

	got, err := model.DoGenerate(context.Background(), testCallOptions())
	require.NoError(t, err)
	assert.Equal(t, "strict", got.Content[0].Text)
	assert.Equal(t, result.Warnings, got.Warnings)
	require.NotNil(t, got.Request)
	require.NotNil(t, got.Response)
	assert.Equal(t, "response", got.Response.ID)
	assert.Equal(t, timestamp, got.Response.Timestamp)
	assert.Empty(t, got.Response.ModelID)
	assert.Empty(t, got.Response.Provider)
	assert.NotEmpty(t, got.Response.Headers)
	assert.NotEmpty(t, got.Response.Body)

	requests := endpoint.Requests()
	require.Len(t, requests, 1)
	var requestObject map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(requests[0].Body, &requestObject))
	assert.Contains(t, requestObject, "prompt")
}

func TestStrictMode_DoesNotFallBackToLegacyNormalization(t *testing.T) {
	t.Run("request rejects legacy-only system content", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t)
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{{Type: provider.ContentPartTypeReasoning, Text: "legacy would drop this"}}}}})
		require.Error(t, err)
		assert.Empty(t, endpoint.Requests())
	})

	t.Run("generate rejects legacy tool-call input object", func(t *testing.T) {
		legacyOnly := `{"content":[{"type":"tool-call","toolCallId":"call","toolName":"tool","input":{"x":1}}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, legacyOnly))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr)
		assert.False(t, apiErr.IsRetryable)
	})

	t.Run("generate rejects private provider metadata", func(t *testing.T) {
		wire := `{"content":[],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[],"response":{"provider":"private"}}`
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr)
		assert.False(t, apiErr.IsRetryable)
	})

	t.Run("generate rejects null typed field", func(t *testing.T) {
		wire := `{"content":[],"finishReason":{"unified":"stop","raw":null},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}`
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr)
		assert.False(t, apiErr.IsRetryable)
	})

	t.Run("stream rejects private field", func(t *testing.T) {
		wire := `data: {"type":"response-metadata","provider":"private"}\n\n`
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartError, parts[0].Type)
		assert.False(t, parts[0].APICallError.IsRetryable)
	})

	t.Run("stream rejects null typed field", func(t *testing.T) {
		wire := `data: {"type":"text-delta","id":"text","delta":null}\n\n`
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartError, parts[0].Type)
		assert.False(t, parts[0].APICallError.IsRetryable)
	})

	t.Run("stream rejects legacy nested source", func(t *testing.T) {
		wire := `data: {"type":"source","source":{"sourceType":"url","id":"source","url":"https://example.com"}}\n\n`
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
		model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
		require.NoError(t, err)
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartError, parts[0].Type)
		assert.False(t, parts[0].APICallError.IsRetryable)
	})
}

func TestBoundedUnaryAndErrorResponses(t *testing.T) {
	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "bounded"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings:     []provider.Warning{},
	}
	strictEncoded, err := providerwirev4.EncodeGenerateResult(result)
	require.NoError(t, err)
	legacyEncoded, err := providerwire.EncodeGenerateResult(result)
	require.NoError(t, err)

	clients := []struct {
		name    string
		strict  bool
		cloud   bool
		options []Option
	}{
		{name: "legacy access token"},
		{name: "strict access token", strict: true, options: []Option{WithStrictProviderWire()}},
		{name: "legacy cloud auth", cloud: true},
		{name: "strict cloud auth", strict: true, cloud: true, options: []Option{WithStrictProviderWire()}},
	}
	for _, tc := range clients {
		encoded := legacyEncoded
		if tc.strict {
			encoded = strictEncoded
		}
		t.Run("unary exact limit "+tc.name, func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, string(encoded)))
			options := append(append([]Option(nil), tc.options...), WithMaxUnaryResponseBytes(int64(len(encoded))))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			got, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.NoError(t, err)
			assert.Equal(t, "bounded", got.Content[0].Text)
		})

		t.Run("unary limit plus one "+tc.name, func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, string(encoded)))
			options := append(append([]Option(nil), tc.options...), WithMaxUnaryResponseBytes(int64(len(encoded)-1)))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.False(t, apiErr.IsRetryable)
			assert.ErrorIs(t, apiErr, errResponseTooLarge)
			assert.LessOrEqual(t, len(apiErr.ResponseBody), len(encoded)-1)
		})

		t.Run("error body exact limit "+tc.name, func(t *testing.T) {
			status := http.StatusForbidden
			var body []byte
			var err error
			if tc.strict {
				status, body, err = providerwirev4.EncodeFailure(failure.Classification{Kind: failure.KindForbidden})
			} else {
				retryable := false
				body, err = providerwire.EncodeAPICallError(provider.NewAPICallError(provider.APICallErrorOptions{
					Message: "request forbidden", StatusCode: status, IsRetryable: &retryable,
				}))
			}
			require.NoError(t, err)
			endpoint := newFakeHostedEndpoint(t, rawResponse(status, providerwirev4.MIMEJSON, string(body)))
			options := append(append([]Option(nil), tc.options...), WithMaxErrorResponseBytes(int64(len(body))))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.NotErrorIs(t, apiErr, errResponseTooLarge)
			assert.Equal(t, status, apiErr.StatusCode)
		})

		t.Run("error body limit plus one "+tc.name, func(t *testing.T) {
			body := strings.Repeat("x", 11)
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusBadGateway, providerwirev4.MIMEJSON, body))
			options := append(append([]Option(nil), tc.options...), WithMaxErrorResponseBytes(10))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.False(t, apiErr.IsRetryable)
			assert.Len(t, apiErr.ResponseBody, 10)
			assert.ErrorIs(t, apiErr, errResponseTooLarge)
		})
	}

	t.Run("invalid stream content type diagnostic is bounded", func(t *testing.T) {
		body := strings.Repeat("x", 11)
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, "text/plain", body))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxErrorResponseBytes(10))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		_, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr)
		assert.False(t, apiErr.IsRetryable)
		assert.Len(t, apiErr.ResponseBody, 10)
		assert.ErrorIs(t, apiErr, errResponseTooLarge)
	})

	for _, tc := range clients {
		t.Run("invalid unary content type exact limit "+tc.name, func(t *testing.T) {
			body := strings.Repeat("x", 10)
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, "text/plain; charset=utf-8", body))
			options := append(append([]Option(nil), tc.options...), WithMaxErrorResponseBytes(10))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.False(t, apiErr.IsRetryable)
			assert.Equal(t, body, apiErr.ResponseBody)
			assert.NotErrorIs(t, apiErr, errResponseTooLarge)
		})

		t.Run("invalid unary content type limit plus one "+tc.name, func(t *testing.T) {
			body := strings.Repeat("x", 11)
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, "invalid content type", body))
			options := append(append([]Option(nil), tc.options...), WithMaxErrorResponseBytes(10))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.False(t, apiErr.IsRetryable)
			assert.Len(t, apiErr.ResponseBody, 10)
			assert.ErrorIs(t, apiErr, errResponseTooLarge)
		})
	}
}

func TestBoundedSSEEvents(t *testing.T) {
	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
	event, err := providerwirev4.EncodeSSEEventWithinLimit(part, 1024)
	require.NoError(t, err)

	t.Run("exact complete event", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, string(event)))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxSSEEventBytes(int64(len(event))))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, part, parts[0])
	})

	t.Run("limit plus one", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, string(event)))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxSSEEventBytes(int64(len(event)-1)))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, provider.PartError, parts[0].Type)
		assert.False(t, parts[0].APICallError.IsRetryable)
		assert.ErrorIs(t, parts[0].APICallError, errSSEEventTooLarge)
	})

	t.Run("long unterminated line", func(t *testing.T) {
		wire := "data: " + strings.Repeat("x", 100)
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxSSEEventBytes(32))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.ErrorIs(t, parts[0].APICallError, errSSEEventTooLarge)
	})

	t.Run("multiline aggregate and final EOF", func(t *testing.T) {
		wire := "data: {\"type\":\"text-delta\",\n" + "data: \"id\":\"text\",\"delta\":\"x\"}"
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, wire))
		client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxSSEEventBytes(int64(len(wire))))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectStream(stream.Stream)
		require.Len(t, parts, 1)
		assert.Equal(t, part, parts[0])
	})
}

func TestBoundedSSEEvents_LegacyAndStrictModes(t *testing.T) {
	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
	event, err := providerwirev4.EncodeSSEEventWithinLimit(part, 1024)
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		cloud   bool
		options []Option
	}{
		{name: "legacy access token"},
		{name: "strict access token", options: []Option{WithStrictProviderWire()}},
		{name: "legacy cloud auth", cloud: true},
		{name: "strict cloud auth", cloud: true, options: []Option{WithStrictProviderWire()}},
	} {
		t.Run(tc.name+" exact", func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, string(event)))
			options := append(append([]Option(nil), tc.options...), WithMaxSSEEventBytes(int64(len(event))))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.NoError(t, err)
			assert.Equal(t, []provider.StreamPart{part}, collectStream(stream.Stream))
		})
		t.Run(tc.name+" over limit", func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, string(event)))
			options := append(append([]Option(nil), tc.options...), WithMaxSSEEventBytes(int64(len(event)-1)))
			client := newResponseLimitTestProvider(t, endpoint.URL(), tc.cloud, options...)
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.NoError(t, err)
			parts := collectStream(stream.Stream)
			require.Len(t, parts, 1)
			assert.Equal(t, provider.PartError, parts[0].Type)
			assert.ErrorIs(t, parts[0].APICallError, errSSEEventTooLarge)
		})
	}
}

func TestStrictMode_MalformedNon2xxEnvelopesRemainProtocolAPICallErrors(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"error":`},
		{name: "incomplete strict envelope", body: `{"error":{"message":"internal","type":"internal_server_error","statusCode":500}}`},
		{name: "legacy envelope", body: `{"error":{"message":"legacy","statusCode":500,"isRetryable":true}}`},
		{name: "status mismatch", body: `{"error":{"message":"internal","type":"internal_server_error","statusCode":502,"isRetryable":true}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusInternalServerError, providerwirev4.MIMEJSON, tc.body))
			client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
			model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
			require.NoError(t, err)

			_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
			assert.False(t, apiErr.IsRetryable)
			assert.ErrorIs(t, apiErr, errProtocolResponse)
			var gatewayErr *GatewayError
			assert.False(t, errors.As(err, &gatewayErr))
		})
	}
}

func TestStrictErrorsNormalizeEverySafeCategory(t *testing.T) {
	cases := []struct {
		kind      failure.Kind
		retryable bool
		want      GatewayErrorType
	}{
		{failure.KindUnauthenticated, false, GatewayErrorAuthentication},
		{failure.KindInvalidCall, false, GatewayErrorInvalidRequest},
		{failure.KindUnknownModel, false, GatewayErrorModelNotFound},
		{failure.KindForbidden, false, GatewayErrorForbidden},
		{failure.KindRateLimited, true, GatewayErrorRateLimit},
		{failure.KindTimeout, true, GatewayErrorInternalServer},
		{failure.KindCanceled, false, GatewayErrorInternalServer},
		{failure.KindFailedDependency, false, GatewayErrorFailedDependency},
		{failure.KindFailedDependency, true, GatewayErrorFailedDependency},
		{failure.KindInternal, false, GatewayErrorInternalServer},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			classification := failure.Classification{Kind: tc.kind, Retryable: tc.retryable, SafeParameters: failure.SafeParameters{RequestedModelID: "alias"}}
			status, body, err := providerwirev4.EncodeFailure(classification)
			require.NoError(t, err)
			endpoint := newFakeHostedEndpoint(t, rawResponse(status, providerwirev4.MIMEJSON, string(body)))
			client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err = model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var gatewayErr *GatewayError
			require.ErrorAs(t, err, &gatewayErr)
			assert.Equal(t, tc.want, gatewayErr.Type)
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.retryable, apiErr.IsRetryable)
			if tc.kind == failure.KindUnknownModel {
				assert.Equal(t, "alias", gatewayErr.ModelID)
			}
		})
	}
}

func TestStrictMode_StreamCancellationClosesChannel(t *testing.T) {
	started := make(chan struct{})
	endpoint := newFakeHostedEndpoint(t, blockingStream(started))
	client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
	model, err := client.LanguageModel("claude-sonnet-4-5-20250929")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := model.DoStream(ctx, provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	<-started
	cancel()
	select {
	case _, open := <-stream.Stream:
		assert.False(t, open)
	case <-time.After(time.Second):
		t.Fatal("strict stream did not close after cancellation")
	}
}

func TestStrictStreamErrorCategoryIsRecoverable(t *testing.T) {
	classification := failure.Classification{Kind: failure.KindFailedDependency, Retryable: true}
	apiErr := strictSafeAPICallError(t, classification)
	part := provider.StreamPart{Type: provider.PartError, APICallError: apiErr}
	event, err := providerwirev4.EncodeSSEEventWithinLimit(part, 1024)
	require.NoError(t, err)
	endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, string(event)))
	client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
	model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
	stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.NoError(t, err)
	parts := collectStream(stream.Stream)
	require.Len(t, parts, 1)
	normalized := NormalizeAPICallError(parts[0].APICallError)
	require.NotNil(t, normalized)
	assert.Equal(t, GatewayErrorFailedDependency, normalized.Type)
	assert.True(t, parts[0].APICallError.IsRetryable)
}

func newResponseLimitTestProvider(t *testing.T, baseURL string, cloud bool, options ...Option) *Provider {
	t.Helper()
	if !cloud {
		return newAccessTokenProvider(t, baseURL, options...)
	}
	client, err := NewWithCloudAuth(CloudAuthConfig{
		CAPToken:         "cap",
		TokenExchangeURL: baseURL + "/exchange",
		Namespace:        "stack",
		BaseURL:          baseURL,
	}, options...)
	require.NoError(t, err)
	client.tokenExchanger = &fakeTokenExchanger{token: "access-token"}
	return client
}

func strictGenerateSuccess(result *provider.GenerateResult) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		data, err := providerwirev4.EncodeGenerateResult(result)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", providerwirev4.MIMEJSON)
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(data)
	}
}

func rawResponse(status int, contentType, body string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", contentType)
		writer.WriteHeader(status)
		_, _ = io.Copy(writer, bytes.NewBufferString(body))
	}
}

func strictSafeAPICallError(t *testing.T, classification failure.Classification) *provider.APICallError {
	t.Helper()
	status, body, err := providerwirev4.EncodeFailure(classification)
	require.NoError(t, err)
	apiErr, err := providerwirev4.DecodeErrorResponse(body, status)
	require.NoError(t, err)
	return apiErr
}
