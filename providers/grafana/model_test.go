package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unaryFixture = `{"content":[{"type":"text","text":"hello"},{"type":"text","text":""}],"finishReason":{"unified":"stop","raw":"end_turn"},"usage":{"inputTokens":{"total":2,"noCache":2,"cacheRead":0,"cacheWrite":0},"outputTokens":{"total":1,"text":1,"reasoning":0}}}`

func TestDecodeGenerate_FunctionCalls(t *testing.T) {
	for _, input := range []string{"", "{}", "{\"city\":\"Rio\"}"} {
		encoded, err := json.Marshal(input)
		require.NoError(t, err)
		body := `{"content":[{"type":"text","text":""},{"type":"tool-call","toolCallId":"call","toolName":"weather","input":` + string(encoded) + `}],"finishReason":{"unified":"tool-calls"},"usage":{"inputTokens":{},"outputTokens":{}}}`
		result, err := decodeGenerate([]byte(body))
		require.NoError(t, err)
		require.Len(t, result.Content, 2)
		assert.Equal(t, input, string(result.Content[1].Input))
		assert.Equal(t, "call", result.Content[1].ToolCallID)
		for _, marker := range []string{"providerExecuted", "dynamic"} {
			marked := strings.Replace(body, `"toolName":"weather"`, `"toolName":"weather","`+marker+`":true`, 1)
			result, err := decodeGenerate([]byte(marked))
			require.NoError(t, err)
			if marker == "providerExecuted" {
				assert.True(t, result.Content[1].ProviderExecuted)
			} else {
				assert.True(t, result.Content[1].Dynamic)
			}
		}
	}
}

func TestDecodeGenerate_ProviderToolResults(t *testing.T) {
	for _, tc := range []struct {
		name        string
		result      string
		isError     bool
		preliminary bool
	}{
		{name: "empty object", result: `{}`},
		{name: "zero", result: `0`},
		{name: "false", result: `false`},
		{name: "empty string and error", result: `""`, isError: true},
		{name: "preliminary", result: `[]`, preliminary: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"content":[{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}","providerExecuted":true,"dynamic":true,"providerMetadata":{"anthropic":{"type":"mcp-tool-use","serverName":"echo","private":"discard"}}},{"type":"tool-result","toolCallId":"call","toolName":"echo","result":` + tc.result + `,"isError":` + strconv.FormatBool(tc.isError) + `,"preliminary":` + strconv.FormatBool(tc.preliminary) + `,"providerMetadata":{"anthropic":{"type":"mcp-tool-use","serverName":"echo","private":"discard"}}}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`
			decoded, err := decodeGenerate([]byte(body))
			require.NoError(t, err)
			require.Len(t, decoded.Content, 2)
			assert.True(t, decoded.Content[0].ProviderExecuted)
			assert.True(t, decoded.Content[0].Dynamic)
			assert.JSONEq(t, `{"type":"mcp-tool-use","serverName":"echo"}`, string(decoded.Content[0].ProviderMetadata["anthropic"]))
			assert.Len(t, decoded.Content[0].ProviderMetadata, 1)
			assert.JSONEq(t, tc.result, string(decoded.Content[1].Result))
			assert.Equal(t, tc.isError, decoded.Content[1].IsError)
			assert.Equal(t, tc.preliminary, decoded.Content[1].Preliminary)
			assert.Equal(t, decoded.Content[0].ProviderMetadata, decoded.Content[1].ProviderMetadata)
		})
	}
	for _, result := range []string{`null`, ``, `{"bad":`} {
		body := `{"content":[{"type":"tool-result","toolCallId":"call","toolName":"echo","result":` + result + `}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`
		_, err := decodeGenerate([]byte(body))
		require.Error(t, err)
	}
}

func TestDecodeGenerate_ToolMetadataAndMarkerValidation(t *testing.T) {
	body := `{"content":[{"type":"tool-call","toolCallId":"call","toolName":"search","input":"{}","providerMetadata":{"openai":{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent"},"secret":"discard"},"private":{"url":"hidden"}}},{"type":"tool-result","toolCallId":"call","toolName":"search","result":false,"providerMetadata":{"openai":{"itemId":"output-1"}}}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`
	decoded, err := decodeGenerate([]byte(body))
	require.NoError(t, err)
	assert.JSONEq(t, `{"itemId":"item-1","namespace":"tools","caller":{"type":"program","callerId":"parent"}}`, string(decoded.Content[0].ProviderMetadata["openai"]))
	assert.NotContains(t, string(decoded.Content[0].ProviderMetadata["openai"]), "secret")
	assert.NotContains(t, decoded.Content[0].ProviderMetadata, "private")
	assert.JSONEq(t, `{"itemId":"output-1"}`, string(decoded.Content[1].ProviderMetadata["openai"]))
	for _, value := range []string{
		`{"providerExecuted":null}`, `{"dynamic":"false"}`, `{"preliminary":null}`,
		`{"providerMetadata":{"anthropic":{"type":"mcp-tool-use"}}}`,
		`{"providerMetadata":{"anthropic":{"type":"mcp-tool-use","serverName":null}}}`,
		`{"providerMetadata":{"openai":{"itemId":5}}}`,
	} {
		content := `{"type":"tool-call","toolCallId":"call","toolName":"search","input":"{}",` + strings.TrimPrefix(value, "{")
		_, err := decodeGenerate([]byte(`{"content":[` + content + `],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`))
		require.Error(t, err, value)
	}
}

func TestDecodeGenerate_FunctionCallRequiredFields(t *testing.T) {
	for _, content := range []string{
		`{"type":"tool-call","toolName":"f","input":""}`,
		`{"type":"tool-call","toolCallId":"a","input":""}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f"}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":null}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":{}}`,
	} {
		body := `{"content":[` + content + `],"finishReason":{"unified":"tool-calls"},"usage":{"inputTokens":{},"outputTokens":{}}}`
		_, err := decodeGenerate([]byte(body))
		require.Error(t, err)
	}
}

func TestModel_InvalidProviderToolDoesNotSendRequest(t *testing.T) {
	var calls atomic.Int32
	p := testProvider(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) }, nil)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	options := provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: "anthropic.web_search", Name: "search", ProviderOptions: provider.ProviderOptions{}}}}
	_, err = m.DoGenerate(context.Background(), options)
	require.Error(t, err)
	_, err = m.DoStream(context.Background(), options)
	require.Error(t, err)
	assert.Zero(t, calls.Load())
}

func TestModel_GenerateRequestAndNormalization(t *testing.T) {
	var calls atomic.Int32
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/api/v1/aisdk/language-model", r.URL.Path)
		for key, want := range map[string]string{"Content-Type": "application/json", "Accept": "application/json", "X-Access-Token": "access-token", "X-Grafana-Id": "user-token", "Ai-Language-Model-Id": "assistant", "Ai-Language-Model-Specification-Version": "4", "Ai-Language-Model-Streaming": "false", "X-Custom": "call"} {
			assert.Equal(t, []string{want}, r.Header.Values(key), key)
		}
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.JSONEq(t, `[]`, string(body["prompt"]))
		assert.JSONEq(t, `0`, string(body["maxOutputTokens"]))
		var headers map[string]string
		require.NoError(t, json.Unmarshal(body["headers"], &headers))
		assert.Equal(t, "call", headers["x-custom"])
		assert.Equal(t, "evil-token", headers["X-Access-Token"])
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Server", "actual")
		w.Header().Add("X-Multi", "one")
		w.Header().Add("X-Multi", "two")
		_, _ = io.WriteString(w, strings.TrimSuffix(unaryFixture, "}")+`,"warnings":[{"type":"other","message":"private-warning"}],"request":{"body":"private-request"},"response":{"modelId":"private-backend","id":"private-id","headers":{"X-Server":"fake"}},"providerMetadata":{"private":{"value":"application-data"}}}`)
	}, nil)
	p.headers.Set("X-Custom", "configured")
	p.headers.Set("Ai-Language-Model-Id", "configured-model")
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	zero := 0
	result, err := m.DoGenerate(WithUserIDToken(context.Background(), "user-token"), provider.CallOptions{Prompt: []provider.Message{}, MaxOutputTokens: &zero, Headers: map[string]string{"x-custom": "call", "X-Access-Token": "evil-token", "X-Grafana-Id": "evil-user", "ai-language-model-id": "evil-model", "accept": "evil-accept", "content-type": "evil-type", "ai-language-model-streaming": "true", "ai-language-model-specification-version": "3"}})
	require.NoError(t, err)
	require.Len(t, result.Content, 2)
	assert.Equal(t, "hello", result.Content[0].Text)
	assert.Empty(t, result.Content[1].Text)
	assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
	assert.Equal(t, "end_turn", result.FinishReason.Raw)
	assert.Equal(t, 2, *result.Usage.InputTokens.Total)
	require.NotNil(t, result.Warnings)
	assert.Equal(t, []provider.Warning{{Type: provider.WarnOther, Message: "private-warning"}}, result.Warnings)
	assert.JSONEq(t, `{"value":"application-data"}`, string(result.ProviderMetadata["private"]))
	require.NotNil(t, result.Request)
	assert.Contains(t, string(result.Request.Body), "maxOutputTokens")
	assert.NotContains(t, string(result.Request.Body), "private-request")
	require.NotNil(t, result.Response)
	assert.Equal(t, "actual", result.Response.Headers["X-Server"])
	assert.Equal(t, "one, two", result.Response.Headers["X-Multi"])
	assert.Empty(t, result.Response.ModelID)
	assert.Empty(t, result.Response.ID)
	assert.Equal(t, int32(1), calls.Load())
}

func TestModel_UnaryRawUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "absent"},
		{name: "empty", raw: `{}`, want: `{}`},
		{name: "native", raw: `{"input_tokens":32,"service_tier":"standard","nested":[1,true]}`, want: `{"input_tokens":32,"service_tier":"standard","nested":[1,true]}`},
		{name: "valid surrogate", raw: `{"nested":{"\ud83d\ude00":1}}`, want: `{"nested":{"😀":1}}`},
		{name: "lone high key", raw: `{"nested":{"\ud800":"private"}}`, want: `{"nested":{"\ud800":"private"}}`},
		{name: "lone low nested", raw: `{"nested":["\udc00"]}`, want: `{"nested":["\udc00"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := unaryFixture
			if tc.raw != "" {
				body = strings.Replace(body, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":`+tc.raw, 1)
			}
			p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}, nil)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			assert.Equal(t, 2, *result.Usage.InputTokens.Total)
			if tc.want == "" {
				assert.Nil(t, result.Usage.Raw)
			} else {
				assert.JSONEq(t, tc.want, string(result.Usage.Raw))
				if strings.Contains(tc.name, "lone") {
					assert.Equal(t, tc.raw, string(result.Usage.Raw))
				}
			}
		})
	}
}

func TestModel_UnaryRawUsageSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantSize int
	}{
		{name: "exact", raw: `{"large":"` + strings.Repeat("x", maxRawUsageBytes-len(`{"large":""}`)) + `"}`, wantSize: maxRawUsageBytes},
		{name: "one over", raw: `{"large":"` + strings.Repeat("x", maxRawUsageBytes+1-len(`{"large":""}`)) + `"}`},
		{name: "whitespace exact", raw: strings.Repeat(" ", maxRawUsageBytes-2) + `{}`, wantSize: 2},
		{name: "leading whitespace", raw: strings.Repeat(" ", maxRawUsageBytes-1) + `{}`, wantSize: 2},
		{name: "trailing whitespace", raw: `{}` + strings.Repeat(" ", maxRawUsageBytes-1), wantSize: 2},
		{name: "interior whitespace", raw: `{"x":` + strings.Repeat(" ", maxRawUsageBytes) + `0}`, wantSize: len(`{"x":0}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":`+tc.raw, 1)
			p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}, nil)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{})
			if tc.wantSize != 0 {
				require.NoError(t, err)
				assert.Len(t, result.Usage.Raw, tc.wantSize)
				assert.JSONEq(t, tc.raw, string(result.Usage.Raw))
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
			}
		})
	}
}

func TestModel_UnaryFailures(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"missing content", `{"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`},
		{"null content", strings.Replace(unaryFixture, `[{"type":"text","text":"hello"},{"type":"text","text":""}]`, `null`, 1)},
		{"unknown family", strings.Replace(unaryFixture, `"type":"text"`, `"type":"custom"`, 1)},
		{"missing text", strings.Replace(unaryFixture, `,"text":"hello"`, "", 1)},
		{"null text", strings.Replace(unaryFixture, `"text":"hello"`, `"text":null`, 1)},
		{"unknown finish", strings.Replace(unaryFixture, `"unified":"stop"`, `"unified":"unknown"`, 1)},
		{"unknown warning", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"unknown","message":"invalid"}]}`},
		{"missing warning message", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"other"}]}`},
		{"missing warning feature", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"unsupported"}]}`},
		{"missing deprecated setting", strings.TrimSuffix(unaryFixture, "}") + `,"warnings":[{"type":"deprecated","message":"invalid"}]}`},
		{"null finish", strings.Replace(unaryFixture, `{"unified":"stop","raw":"end_turn"}`, `null`, 1)},
		{"negative usage", strings.Replace(unaryFixture, `"total":2`, `"total":-1`, 1)},
		{"null usage", strings.Replace(unaryFixture, `"total":2`, `"total":null`, 1)},
		{"unsafe usage", strings.Replace(unaryFixture, `"total":2`, `"total":9007199254740992`, 1)},
		{"fractional usage", strings.Replace(unaryFixture, `"total":2`, `"total":0.5`, 1)},
		{"missing usage side", strings.Replace(unaryFixture, `"outputTokens"`, `"missing"`, 1)},
		{"null raw", strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":null`, 1)},
		{"array raw", strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":[]`, 1)},
		{"scalar raw", strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":1`, 1)},
		{"invalid UTF-8 raw", strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":{"nested":"`+string([]byte{0xff})+`"}`, 1)},
		{"malformed raw", strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":{"private":`, 1)},
		{"oversized raw", strings.Replace(unaryFixture, `"outputTokens":{"total":1,"text":1,"reasoning":0}`, `"outputTokens":{"total":1,"text":1,"reasoning":0},"raw":{"private":"`+strings.Repeat("x", 1<<20)+`"}`, 1)},
		{"trailing", unaryFixture + `{}`},
		{"malformed", `{"content":`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}, nil)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.Error(t, err)
			assert.Nil(t, result)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.False(t, api.IsRetryable)
			assert.NotContains(t, err.Error(), "private")
		})
	}
}

type observedResponseBody struct {
	io.ReadCloser
	closed bool
}

func (b *observedResponseBody) Close() error {
	b.closed = true
	return b.ReadCloser.Close()
}

func TestModel_PostHeaderTransportFailure(t *testing.T) {
	var calls atomic.Int32
	const privateBody = `{"private":"sensitive-value"`
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, privateBody)
		w.(http.Flusher).Flush()
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijacking response: %v", err)
			return
		}
		_ = conn.Close()
	}, nil)
	var body *observedResponseBody
	p.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err == nil {
			body = &observedResponseBody{ReadCloser: resp.Body}
			resp.Body = body
		}
		return resp, err
	})
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
	require.Error(t, err)
	assert.Nil(t, result)
	var api *provider.APICallError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, http.StatusOK, api.StatusCode)
	assert.True(t, api.IsRetryable)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.NotContains(t, err.Error(), privateBody)
	assert.NotContains(t, err.Error(), "sensitive-value")
	assert.Less(t, len(api.Message), 100)
	require.NotNil(t, body)
	assert.True(t, body.closed)
	assert.Equal(t, int32(1), calls.Load())

	t.Run("non-EOF body read failure", func(t *testing.T) {
		cause := errors.New("private socket reset")
		body := &trackedBody{Reader: &partialReadError{data: []byte(`{"partial":`), err: cause}}
		var calls atomic.Int32
		p, err := NewWithAccessToken(AccessTokenConfig{
			AccessToken: "token", BaseURL: "https://example.test",
			HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
			})},
		})
		require.NoError(t, err)
		m, err := p.LanguageModel("assistant")
		require.NoError(t, err)
		result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.Error(t, err)
		assert.Nil(t, result)
		var api *provider.APICallError
		require.ErrorAs(t, err, &api)
		assert.Equal(t, http.StatusOK, api.StatusCode)
		assert.True(t, api.IsRetryable)
		assert.ErrorIs(t, err, cause)
		assert.NotContains(t, err.Error(), "private socket reset")
		assert.Less(t, len(api.Message), 100)
		assert.True(t, body.closed)
		assert.Equal(t, int32(1), calls.Load())
	})
}

type partialReadError struct {
	data   []byte
	err    error
	onRead func()
}

func (r *partialReadError) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = nil
	if r.onRead != nil {
		r.onRead()
	}
	return n, r.err
}

func TestModel_UnaryProtocolFailures(t *testing.T) {
	for _, tc := range []struct {
		name, media, body string
		limit             int64
	}{
		{name: "wrong media", media: "text/html", body: `{"private":"sensitive-value"}`},
		{name: "malformed JSON", media: "application/json", body: `{"private":"sensitive-value"`},
		{name: "invalid schema", media: "application/json", body: `{"private":"sensitive-value"}`},
		{name: "byte limit", media: "application/json", body: `{"private":"sensitive-value"}`, limit: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			limits := DefaultLimits()
			if tc.limit != 0 {
				limits.UnaryBytes = tc.limit
			}
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tc.media)
				_, _ = io.WriteString(w, tc.body)
			}, &limits)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			require.Error(t, err)
			assert.Nil(t, result)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.False(t, api.IsRetryable)
			assert.Equal(t, http.StatusOK, api.StatusCode)
			assert.Empty(t, api.ResponseBody)
			assert.NotContains(t, err.Error(), "sensitive-value")
			assert.Less(t, len(api.Message), 100)
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_UnaryReadPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name             string
		cancelDuringRead bool
	}{
		{name: "limit before transport"},
		{name: "cancellation before limit", cancelDuringRead: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &partialReadError{data: []byte("private"), err: io.ErrUnexpectedEOF}
			if tc.cancelDuringRead {
				reader.onRead = cancel
			}
			body := &trackedBody{Reader: reader}
			limits := DefaultLimits()
			limits.UnaryBytes = 4
			var calls atomic.Int32
			p, err := NewWithAccessToken(AccessTokenConfig{
				AccessToken: "token", BaseURL: "https://example.test", Limits: &limits,
				HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
				})},
			})
			require.NoError(t, err)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{}})
			require.Error(t, err)
			assert.Nil(t, result)
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.Equal(t, http.StatusOK, api.StatusCode)
			assert.False(t, api.IsRetryable)
			if tc.cancelDuringRead {
				assert.ErrorIs(t, err, context.Canceled)
			} else {
				assert.ErrorContains(t, errors.Unwrap(api), "response byte limit exceeded")
			}
			assert.NotErrorIs(t, err, io.ErrUnexpectedEOF)
			assert.NotContains(t, err.Error(), "private")
			assert.True(t, body.closed)
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_UnaryReadCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			started := make(chan struct{})
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
			}, nil)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			var ctx context.Context
			var cancel context.CancelFunc
			expected := context.Canceled
			if deadline {
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
				expected = context.DeadlineExceeded
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				result, err := m.DoGenerate(ctx, provider.CallOptions{Prompt: []provider.Message{}})
				if result != nil {
					done <- errors.New("grafana: canceled unary call returned a result")
					return
				}
				done <- err
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("unary response headers were not sent")
			}
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, expected)
				var api *provider.APICallError
				if errors.As(err, &api) {
					assert.False(t, api.IsRetryable)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("canceled unary read did not finish")
			}
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestModel_UnaryBoundsAndTransport(t *testing.T) {
	for _, delta := range []int64{0, -1} {
		t.Run(string(rune('a'-delta)), func(t *testing.T) {
			limits := DefaultLimits()
			limits.UnaryBytes = int64(len(unaryFixture)) + delta
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, unaryFixture)
			}, &limits)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoGenerate(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
			if delta == 0 {
				require.NoError(t, err)
				assert.NotNil(t, result)
			} else {
				require.Error(t, err)
				assert.Nil(t, result)
			}
		})
	}
	t.Run("transport error", func(t *testing.T) {
		cause := errors.New("private transport detail")
		p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })}})
		require.NoError(t, err)
		m, err := p.LanguageModel("assistant")
		require.NoError(t, err)
		_, err = m.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.ErrorIs(t, err, cause)
		assert.NotContains(t, err.Error(), "private transport detail")
		var api *provider.APICallError
		require.ErrorAs(t, err, &api)
		assert.True(t, api.IsRetryable)
	})
}
