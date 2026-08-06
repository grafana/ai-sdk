package grafana

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/gateway/providerwire"
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictProviderWire_DefaultsAndValidation(t *testing.T) {
	legacy := newAccessTokenProvider(t, "https://example.com")
	assert.IsType(t, legacyWireCodec{}, legacy.wireCodec)
	assert.False(t, legacy.strictProviderWire)
	assert.Equal(t, DefaultMaxUnaryResponseBytes, legacy.maxUnaryResponseBytes)
	assert.Equal(t, DefaultMaxErrorResponseBytes, legacy.maxErrorResponseBytes)
	assert.Equal(t, DefaultMaxSSEEventBytes, legacy.maxSSEEventBytes)

	strict := newAccessTokenProvider(t, "https://example.com", WithStrictProviderWire(), WithMaxUnaryResponseBytes(10), WithMaxErrorResponseBytes(11), WithMaxSSEEventBytes(12))
	assert.IsType(t, strictWireCodec{}, strict.wireCodec)
	assert.True(t, strict.strictProviderWire)
	assert.Equal(t, int64(10), strict.maxUnaryResponseBytes)
	assert.Equal(t, int64(11), strict.maxErrorResponseBytes)
	assert.Equal(t, int64(12), strict.maxSSEEventBytes)

	cloud, err := NewWithCloudAuth(CloudAuthConfig{CAPToken: "cap", TokenExchangeURL: "https://example.com/exchange", Namespace: "stack", BaseURL: "https://example.com"}, WithStrictProviderWire(), WithMaxUnaryResponseBytes(10), WithMaxErrorResponseBytes(11), WithMaxSSEEventBytes(12))
	require.NoError(t, err)
	assert.IsType(t, strictWireCodec{}, cloud.wireCodec)
	assert.True(t, cloud.strictProviderWire)
	assert.Equal(t, int64(10), cloud.maxUnaryResponseBytes)
	assert.Equal(t, int64(11), cloud.maxErrorResponseBytes)
	assert.Equal(t, int64(12), cloud.maxSSEEventBytes)

	invalidOptions := []Option{
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
	endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, string(strictGenerateBytes(t, result))))
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

func TestStrictResponseLimitsDoNotChangeLegacyMode(t *testing.T) {
	result := &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "bounded"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings:     []provider.Warning{},
	}
	strictEncoded := strictGenerateBytes(t, result)

	for _, tc := range []struct {
		name  string
		limit int64
		ok    bool
	}{
		{name: "strict unary exact limit", limit: int64(len(strictEncoded)), ok: true},
		{name: "strict unary limit plus one", limit: int64(len(strictEncoded) - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMEJSON, string(strictEncoded)))
			client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxUnaryResponseBytes(tc.limit))
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			got, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			if tc.ok {
				require.NoError(t, err)
				assert.Equal(t, "bounded", got.Content[0].Text)
				return
			}
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.False(t, apiErr.IsRetryable)
			assert.ErrorIs(t, apiErr, errResponseTooLarge)
		})
	}

	errorBody := strictErrorBody(t, http.StatusBadGateway, "failed_dependency", "upstream dependency failed", true, nil)
	for _, tc := range []struct {
		name  string
		limit int64
		ok    bool
	}{
		{name: "strict error exact limit", limit: int64(len(errorBody)), ok: true},
		{name: "strict error limit plus one", limit: int64(len(errorBody) - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusBadGateway, providerwirev4.MIMEJSON, string(errorBody)))
			client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxErrorResponseBytes(tc.limit))
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			if tc.ok {
				assert.NotErrorIs(t, apiErr, errResponseTooLarge)
				assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
				return
			}
			assert.False(t, apiErr.IsRetryable)
			assert.Len(t, apiErr.ResponseBody, int(tc.limit))
			assert.ErrorIs(t, apiErr, errResponseTooLarge)
		})
	}

	t.Run("legacy ignores new unary limit", func(t *testing.T) {
		legacyEncoded, err := providerwire.EncodeGenerateResult(result)
		require.NoError(t, err)
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwire.MIMEJSON, string(legacyEncoded)))
		client := newAccessTokenProvider(t, endpoint.URL(), WithMaxUnaryResponseBytes(1))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		got, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		assert.Equal(t, "bounded", got.Content[0].Text)
	})
}

func TestStrictSSEEventLimitDoesNotChangeLegacyMode(t *testing.T) {
	part := provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"}
	event := strictStreamBytes(t, part)

	for _, tc := range []struct {
		name  string
		limit int64
		ok    bool
	}{
		{name: "strict event exact limit", limit: int64(len(event)), ok: true},
		{name: "strict event limit plus one", limit: int64(len(event) - 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwirev4.MIMESSE, string(event)))
			client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire(), WithMaxSSEEventBytes(tc.limit))
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.NoError(t, err)
			parts := collectStream(stream.Stream)
			require.Len(t, parts, 1)
			if tc.ok {
				assert.Equal(t, part, parts[0])
				return
			}
			assert.Equal(t, provider.PartError, parts[0].Type)
			assert.False(t, parts[0].APICallError.IsRetryable)
			assert.ErrorIs(t, parts[0].APICallError, errProtocolResponse)
		})
	}

	t.Run("legacy ignores new event limit", func(t *testing.T) {
		endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusOK, providerwire.MIMESSE, string(event)))
		client := newAccessTokenProvider(t, endpoint.URL(), WithMaxSSEEventBytes(1))
		model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
		stream, err := model.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		assert.Equal(t, []provider.StreamPart{part}, collectStream(stream.Stream))
	})
}

func TestStrictMode_MalformedNon2xxEnvelopeRemainsProtocolAPICallError(t *testing.T) {
	endpoint := newFakeHostedEndpoint(t, rawResponse(http.StatusInternalServerError, providerwirev4.MIMEJSON, `{"error":`))
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
}

func TestStrictErrorsPreservePublicCategoryDistinctions(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		typeValue string
		retryable bool
		want      GatewayErrorType
	}{
		{"forbidden", 403, "forbidden", false, GatewayErrorForbidden},
		{"permanent dependency", 424, "failed_dependency", false, GatewayErrorFailedDependency},
		{"transient dependency", 502, "failed_dependency", true, GatewayErrorFailedDependency},
		{"internal", 500, "internal_server_error", false, GatewayErrorInternalServer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strictErrorBody(t, tc.status, tc.typeValue, "safe message", tc.retryable, nil)
			endpoint := newFakeHostedEndpoint(t, rawResponse(tc.status, providerwirev4.MIMEJSON, string(body)))
			client := newAccessTokenProvider(t, endpoint.URL(), WithStrictProviderWire())
			model, _ := client.LanguageModel("claude-sonnet-4-5-20250929")
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			var gatewayErr *GatewayError
			require.ErrorAs(t, err, &gatewayErr)
			assert.Equal(t, tc.want, gatewayErr.Type)
			var apiErr *provider.APICallError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.retryable, apiErr.IsRetryable)
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
	body := strictErrorBody(t, http.StatusBadGateway, "failed_dependency", "upstream dependency failed", true, nil)
	apiErr, err := providerwirev4.DecodeErrorResponse(body, http.StatusBadGateway)
	require.NoError(t, err)
	part := provider.StreamPart{Type: provider.PartError, APICallError: apiErr}
	event := strictStreamBytes(t, part)
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

type strictFixtureModel struct {
	generateResult *provider.GenerateResult
	streamParts    []provider.StreamPart
}

func (m *strictFixtureModel) SpecificationVersion() string               { return "v4" }
func (m *strictFixtureModel) Provider() string                           { return "fixture" }
func (m *strictFixtureModel) ModelID() string                            { return "fixture" }
func (m *strictFixtureModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *strictFixtureModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return m.generateResult, nil
}
func (m *strictFixtureModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	parts := make(chan provider.StreamPart, len(m.streamParts))
	for _, part := range m.streamParts {
		parts <- part
	}
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}

type strictFixtureResolver struct{ model provider.LanguageModel }

func (r strictFixtureResolver) ResolveModel(_ context.Context, modelID string) (catalog.ResolvedModel, error) {
	return catalog.ResolvedModel{ID: modelID, Model: r.model}, nil
}

func strictGenerateBytes(t *testing.T, result *provider.GenerateResult) []byte {
	t.Helper()
	return strictHandlerBytes(t, &strictFixtureModel{generateResult: result}, false)
}

func strictStreamBytes(t *testing.T, parts ...provider.StreamPart) []byte {
	t.Helper()
	return strictHandlerBytes(t, &strictFixtureModel{streamParts: parts}, true)
}

func strictHandlerBytes(t *testing.T, model provider.LanguageModel, streaming bool) []byte {
	t.Helper()
	handler, err := providerwirev4.NewHandler(strictFixtureResolver{model: model})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, providerwirev4.PathLanguageModel, strings.NewReader(`{"prompt":[]}`))
	request.Header.Set(providerwirev4.HeaderModelID, "fixture")
	request.Header.Set(providerwirev4.HeaderSpecVersion, providerwirev4.SpecVersionV4)
	request.Header.Set("Content-Type", providerwirev4.MIMEJSON)
	if streaming {
		request.Header.Set(providerwirev4.HeaderStreaming, "true")
		request.Header.Set("Accept", providerwirev4.MIMESSE)
	} else {
		request.Header.Set(providerwirev4.HeaderStreaming, "false")
		request.Header.Set("Accept", providerwirev4.MIMEJSON)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	return recorder.Body.Bytes()
}

func rawResponse(status int, contentType, body string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", contentType)
		writer.WriteHeader(status)
		_, _ = io.Copy(writer, bytes.NewBufferString(body))
	}
}

func strictErrorBody(t *testing.T, status int, typeValue, message string, retryable bool, param any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"error": map[string]any{
		"message": message, "type": typeValue, "statusCode": status,
		"isRetryable": retryable, "param": param,
	}})
	require.NoError(t, err)
	return body
}
