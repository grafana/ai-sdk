package providerwirev4

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type handlerTestModel struct {
	generate      func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	generateCalls atomic.Int32
	streamCalls   atomic.Int32
}

func (m *handlerTestModel) SpecificationVersion() string               { return "v4" }
func (m *handlerTestModel) Provider() string                           { return "test" }
func (m *handlerTestModel) ModelID() string                            { return "backend-secret" }
func (m *handlerTestModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *handlerTestModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.streamCalls.Add(1)
	return nil, errors.New("unexpected stream call")
}
func (m *handlerTestModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	m.generateCalls.Add(1)
	if m.generate != nil {
		return m.generate(ctx, options)
	}
	return emptyGenerateResult(), nil
}

type handlerTestResolver struct {
	model   provider.LanguageModel
	err     error
	calls   atomic.Int32
	resolve func(*http.Request, string) (provider.LanguageModel, error)
}

func (r *handlerTestResolver) ResolveLanguageModel(request *http.Request, modelID string) (provider.LanguageModel, error) {
	r.calls.Add(1)
	if r.resolve != nil {
		return r.resolve(request, modelID)
	}
	return r.model, r.err
}

func emptyGenerateResult() *provider.GenerateResult {
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther},
		Warnings:     []provider.Warning{},
	}
}

func validV4Request(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, PathLanguageModel, strings.NewReader(body))
	request.Header.Set("Content-Type", MIMEJSON)
	request.Header.Set("Accept", "*/*")
	request.Header.Set(HeaderModelID, "public-model")
	request.Header.Set(HeaderSpecVersion, SpecVersionV4)
	request.Header.Set(HeaderStreaming, "false")
	return request
}

func serveV4(t *testing.T, handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeSafeFailure(t *testing.T, recorder *httptest.ResponseRecorder) safeErrorEnvelope {
	t.Helper()
	var envelope safeErrorEnvelope
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &envelope))
	return envelope
}

func TestNewHandler_Configuration(t *testing.T) {
	model := &handlerTestModel{}
	resolver := &handlerTestResolver{model: model}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	assert.Equal(t, DefaultTotalTimeout, handler.totalTimeout)
	assert.Equal(t, DefaultMaxRequestBodyBytes, handler.maxRequestBodyBytes)
	assert.Equal(t, DefaultMaxInlineFileBytes, handler.maxInlineFileBytes)
	assert.Equal(t, DefaultMaxResponseBodyBytes, handler.maxResponseBodyBytes)
	assert.Equal(t, DefaultMaxErrorBodyBytes, handler.maxErrorBodyBytes)

	var typedNil *handlerTestResolver
	var nilFunc ModelResolverFunc
	for _, tc := range []struct {
		name     string
		resolver ModelResolver
		options  []Option
	}{
		{name: "nil resolver"},
		{name: "typed nil resolver", resolver: typedNil},
		{name: "nil function resolver", resolver: nilFunc},
		{name: "nil option", resolver: resolver, options: []Option{nil}},
		{name: "zero timeout", resolver: resolver, options: []Option{WithTotalTimeout(0)}},
		{name: "negative timeout", resolver: resolver, options: []Option{WithTotalTimeout(-1)}},
		{name: "zero request limit", resolver: resolver, options: []Option{WithMaxRequestBodyBytes(0)}},
		{name: "negative request limit", resolver: resolver, options: []Option{WithMaxRequestBodyBytes(-1)}},
		{name: "zero file limit", resolver: resolver, options: []Option{WithMaxInlineFileBytes(0)}},
		{name: "negative file limit", resolver: resolver, options: []Option{WithMaxInlineFileBytes(-1)}},
		{name: "zero result limit", resolver: resolver, options: []Option{WithMaxResponseBodyBytes(0)}},
		{name: "negative result limit", resolver: resolver, options: []Option{WithMaxResponseBodyBytes(-1)}},
		{name: "zero error limit", resolver: resolver, options: []Option{WithMaxErrorBodyBytes(0)}},
		{name: "negative error limit", resolver: resolver, options: []Option{WithMaxErrorBodyBytes(-1)}},

		{name: "error limit below fallback", resolver: resolver, options: []Option{WithMaxErrorBodyBytes(int64(len(fallbackErrorJSON) - 1))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewHandler(tc.resolver, tc.options...)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func TestHandler_RequestGatesBypassResolver(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		wantMessage string
		mutate      func(*http.Request)
		option      Option
	}{
		{name: "method", status: 405, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Method = http.MethodGet }},
		{name: "path", status: 404, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.URL.Path = "/prefix/language-model" }},
		{name: "model id", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Set(HeaderModelID, " padded ") }},
		{name: "duplicate model id", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Add(HeaderModelID, "other-model") }},
		{name: "duplicate equal model id", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Add(HeaderModelID, "public-model") }},
		{name: "version", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Set(HeaderSpecVersion, "v4") }},
		{name: "duplicate version", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Add(HeaderSpecVersion, "3") }},
		{name: "duplicate equal version", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Add(HeaderSpecVersion, SpecVersionV4) }},
		{name: "stream", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Set(HeaderStreaming, "true") }},
		{name: "duplicate stream", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Add(HeaderStreaming, "true") }},
		{name: "duplicate equal stream", status: 400, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Add(HeaderStreaming, "false") }},
		{name: "missing content type", status: 415, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Del("Content-Type") }},
		{name: "accept", status: 406, body: `{"prompt":[]}`, mutate: func(r *http.Request) { r.Header.Set("Accept", "text/plain") }},
		{name: "body limit plus one", status: 413, body: `{"prompt":[]}`, option: WithMaxRequestBodyBytes(12)},
		{name: "syntax before schema", status: 400, body: `{"prompt":[],"prompt":null,"unknown":true}`, wantMessage: "invalid JSON syntax"},
		{name: "schema before policy", status: 400, body: `{"prompt":[],"unknown":true,"providerOptions":{"gateway":{"order":[]}}}`, wantMessage: "does not match the ProviderWire V4 schema"},
		{name: "policy before adaptation", status: 400, body: `{"prompt":[],"providerOptions":{"gateway":{"order":[]}},"maxOutputTokens":1.5}`, wantMessage: "violates this service's policy"},
		{name: "nested gateway", status: 400, body: `{"prompt":[],"providerOptions":{"bedrock":{"nested":{"gateway":{}}}}}`},
		{name: "body header", status: 400, body: `{"prompt":[],"headers":{"authorization":"secret"}}`},
		{name: "raw chunks", status: 400, body: `{"prompt":[],"includeRawChunks":true}`},
		{name: "fractional integer", status: 400, body: `{"prompt":[],"maxOutputTokens":1.5}`},
		{name: "integer overflow", status: 400, body: `{"prompt":[],"seed":9223372036854775808}`},
		{name: "float overflow", status: 400, body: `{"prompt":[],"temperature":1e400}`},
		{name: "malformed base64", status: 400, body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"%%%"},"mediaType":"x"}]}]}`, wantMessage: "cannot be represented by this service"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &handlerTestResolver{model: &handlerTestModel{}}
			options := []Option{}
			if tc.option != nil {
				options = append(options, tc.option)
			}
			handler, err := NewHandler(resolver, options...)
			require.NoError(t, err)
			request := validV4Request(tc.body)
			if tc.mutate != nil {
				tc.mutate(request)
			}
			recorder := serveV4(t, handler, request)
			assert.Equal(t, tc.status, recorder.Code)
			assert.Zero(t, resolver.calls.Load())
			envelope := decodeSafeFailure(t, recorder)
			assert.Equal(t, tc.status, envelope.Error.StatusCode)
			assert.False(t, envelope.Error.IsRetryable)
			if tc.wantMessage != "" {
				assert.Contains(t, envelope.Error.Message, tc.wantMessage)
			}
		})
	}
}

func TestHandler_ExactRequestBodyLimit(t *testing.T) {
	body := `{"prompt":[]}`
	resolver := &handlerTestResolver{model: &handlerTestModel{}}
	handler, err := NewHandler(resolver, WithMaxRequestBodyBytes(int64(len(body))))
	require.NoError(t, err)
	recorder := serveV4(t, handler, validV4Request(body))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, int32(1), resolver.calls.Load())
}

func TestHandler_RequestBodyReadFailureAndClose(t *testing.T) {
	resolver := &handlerTestResolver{model: &handlerTestModel{}}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)

	t.Run("read failure", func(t *testing.T) {
		body := &failingReadCloser{}
		request := validV4Request(`{"prompt":[]}`)
		request.Body = body
		recorder := serveV4(t, handler, request)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.True(t, body.closed)
	})

	t.Run("canceled read", func(t *testing.T) {
		body := &failingReadCloser{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		request := validV4Request(`{"prompt":[]}`).WithContext(ctx)
		request.Body = body
		recorder := serveV4(t, handler, request)
		assert.Equal(t, 499, recorder.Code)
		assert.False(t, decodeSafeFailure(t, recorder).Error.IsRetryable)
		assert.True(t, body.closed)
	})
	assert.Zero(t, resolver.calls.Load())
}

func TestHandler_PreCanceledRequestBypassesResolver(t *testing.T) {
	resolver := &handlerTestResolver{model: &handlerTestModel{}}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`).WithContext(ctx))

	assert.Equal(t, 499, recorder.Code)
	assert.Zero(t, resolver.calls.Load())
}

type failingReadCloser struct{ closed bool }

func (r *failingReadCloser) Read([]byte) (int, error) { return 0, errors.New("secret read error") }
func (r *failingReadCloser) Close() error             { r.closed = true; return nil }

func TestHandler_AdaptsAcceptedRequest(t *testing.T) {
	body := `{
		"prompt":[
			{"role":"system","content":""},
			{"role":"user","content":[
				{"type":"text","text":"hello","providerOptions":{"part-provider":{"enabled":true}}},
				{"type":"file","data":{"type":"data","data":"AQID"},"mediaType":"application/octet-stream"},
				{"type":"file","data":{"type":"url","url":""},"mediaType":"text/plain"}
			]},
			{"role":"assistant","content":[
				{"type":"tool-call","toolCallId":"call","toolName":"echo","input":null,"providerExecuted":false},
				{"type":"tool-result","toolCallId":"call","toolName":"echo","output":{"type":"json","value":null}}
			]},
			{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"approval","approved":false,"reason":""}]}
		],
		"maxOutputTokens":0,"temperature":0,"topP":0,"topK":0,"presencePenalty":0,"frequencyPenalty":0,"seed":0,
		"tools":[],"stopSequences":[],"headers":{"user-agent":"ai/7.0.65"},
		"providerOptions":{"gateway":{},"bedrock":{}},"includeRawChunks":false
	}`
	var captured provider.CallOptions
	model := &handlerTestModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		captured = options
		return emptyGenerateResult(), nil
	}}
	resolver := &handlerTestResolver{model: model}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	recorder := serveV4(t, handler, validV4Request(body))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Equal(t, int32(1), resolver.calls.Load())
	assert.Equal(t, int32(1), model.generateCalls.Load())
	assert.Zero(t, model.streamCalls.Load())
	require.Len(t, captured.Prompt, 4)
	assert.NotNil(t, captured.Tools)
	assert.Empty(t, captured.Tools)
	assert.NotNil(t, captured.StopSequences)
	assert.Empty(t, captured.StopSequences)
	assert.Nil(t, captured.Headers)
	require.Contains(t, captured.ProviderOptions, "bedrock")
	assert.NotContains(t, captured.ProviderOptions, "gateway")
	assert.Equal(t, 0, *captured.MaxOutputTokens)
	assert.Equal(t, 0, *captured.TopK)
	assert.Equal(t, 0, *captured.Seed)
	require.Equal(t, []byte{1, 2, 3}, captured.Prompt[1].Content[1].Data.Bytes)
	encodedURL, err := json.Marshal(captured.Prompt[1].Content[2].Data)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"url","url":""}`, string(encodedURL))
	assert.Equal(t, json.RawMessage("null"), captured.Prompt[2].Content[0].Input)
	assert.Equal(t, json.RawMessage("null"), captured.Prompt[2].Content[1].Output.JSON)
}

func TestRequestAdapter_CompleteUnionInventory(t *testing.T) {
	fixture := findCorpusCase(t, readPositiveCorpus(t), "request complete union inventory")
	request, err := decodeWireRequest(fixture.Document)
	require.NoError(t, err)
	request.Headers = nil
	delete(request.ProviderOptions, "gateway")
	adapter := requestAdapter{maxInlineBytes: 1024}
	options, err := adapter.adapt(request)
	require.NoError(t, err)
	require.Len(t, options.Prompt, 4)
	require.Len(t, options.Tools, 2)

	functionTool := options.Tools[0]
	assert.Equal(t, provider.ToolTypeFunction, functionTool.Type)
	assert.Equal(t, "echo", functionTool.Name)
	assert.Equal(t, "echo input", functionTool.Description)
	assert.JSONEq(t, `{"type":"object","properties":{"text":{"type":"string"}}}`, string(functionTool.InputSchema))
	require.Len(t, functionTool.InputExamples, 2)
	assert.JSONEq(t, `{}`, string(functionTool.InputExamples[0].Input))
	assert.JSONEq(t, `{"text":"example"}`, string(functionTool.InputExamples[1].Input))
	require.NotNil(t, functionTool.Strict)
	assert.False(t, *functionTool.Strict)
	assertRawProviderOption(t, functionTool.ProviderOptions, "function-provider", `{}`)

	providerTool := options.Tools[1]
	assert.Equal(t, provider.ToolTypeProvider, providerTool.Type)
	assert.Equal(t, "provider.search", providerTool.ID)
	assert.Equal(t, "search", providerTool.Name)
	require.NotNil(t, providerTool.Args)
	assert.JSONEq(t, `0`, string(providerTool.Args["limit"]))
	assert.JSONEq(t, `{}`, string(providerTool.Args["filters"]))

	require.NotNil(t, options.ResponseFormat)
	assert.Equal(t, provider.ResponseFormatJSON, options.ResponseFormat.Type)
	assert.JSONEq(t, `{"type":"object"}`, string(options.ResponseFormat.Schema))
	assert.Equal(t, "result", options.ResponseFormat.Name)
	assert.Equal(t, "structured", options.ResponseFormat.Description)
	require.NotNil(t, options.ToolChoice)
	assert.Equal(t, provider.ToolChoiceTool, options.ToolChoice.Type)
	assert.Equal(t, "echo", options.ToolChoice.ToolName)
	require.NotNil(t, options.Reasoning)
	assert.Equal(t, provider.ReasoningProviderDefault, *options.Reasoning)

	require.Len(t, options.Prompt[1].Content, 5)
	assertRawProviderOption(t, options.Prompt[1].Content[0].ProviderOptions, "text-provider", `{"enabled":true}`)
	assertDataContentJSON(t, options.Prompt[1].Content[1].Data, `{"type":"data","data":"AQIDBA=="}`)
	assertDataContentJSON(t, options.Prompt[1].Content[2].Data, `{"type":"url","url":"https://example.test/file.png"}`)
	assertDataContentJSON(t, options.Prompt[1].Content[3].Data, `{"type":"reference","reference":{}}`)
	assertDataContentJSON(t, options.Prompt[1].Content[4].Data, `{"type":"text","text":"inline"}`)

	require.Len(t, options.Prompt[2].Content, 7)
	assertRawProviderOption(t, options.Prompt[2].Content[2].ProviderOptions, "custom-provider", `{}`)
	assertDataContentJSON(t, options.Prompt[2].Content[3].Data, `{"type":"reference","reference":{"provider":"file-1"}}`)
	assertDataContentJSON(t, options.Prompt[2].Content[4].Data, `{"type":"data","data":"BQY="}`)
	assert.JSONEq(t, `null`, string(options.Prompt[2].Content[5].Input))
	assert.JSONEq(t, `null`, string(options.Prompt[2].Content[6].Output.JSON))

	require.Len(t, options.Prompt[3].Content, 6)
	assert.Equal(t, provider.ToolOutputText, options.Prompt[3].Content[0].Output.Type)
	assert.Equal(t, provider.ToolOutputErrorText, options.Prompt[3].Content[1].Output.Type)
	assert.Equal(t, provider.ToolOutputErrorJSON, options.Prompt[3].Content[2].Output.Type)
	assert.Equal(t, provider.ToolOutputExecutionDenied, options.Prompt[3].Content[3].Output.Type)
	assertRawProviderOption(t, options.Prompt[3].Content[3].Output.ProviderOptions, "", ``)
	contentOutput := options.Prompt[3].Content[4].Output
	assert.Equal(t, provider.ToolOutputContent, contentOutput.Type)
	require.Len(t, contentOutput.Content, 6)
	assertDataContentJSON(t, contentOutput.Content[1].Data, `{"type":"data","data":"AQIDBA=="}`)
	assertDataContentJSON(t, contentOutput.Content[2].Data, `{"type":"url","url":"https://example.test/nested"}`)
	assertDataContentJSON(t, contentOutput.Content[3].Data, `{"type":"reference","reference":{"provider":"nested-1"}}`)
	assertDataContentJSON(t, contentOutput.Content[4].Data, `{"type":"text","text":"nested inline"}`)
	assertRawProviderOption(t, contentOutput.Content[5].ProviderOptions, "custom-provider", `{"value":null}`)
	assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, options.Prompt[3].Content[5].Type)
	assert.Equal(t, int64(10), adapter.inlineBytes)

	for _, tc := range []struct {
		name               string
		responseFormatType provider.ResponseFormatType
		toolChoiceType     provider.ToolChoiceType
	}{
		{name: "request text response auto choice", responseFormatType: provider.ResponseFormatText, toolChoiceType: provider.ToolChoiceAuto},
		{name: "request none choice explicit empties", toolChoiceType: provider.ToolChoiceNone},
		{name: "request required choice", toolChoiceType: provider.ToolChoiceRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := findCorpusCase(t, readPositiveCorpus(t), tc.name)
			request, err := decodeWireRequest(fixture.Document)
			require.NoError(t, err)
			adapted, err := (&requestAdapter{maxInlineBytes: 1024}).adapt(request)
			require.NoError(t, err)
			if tc.responseFormatType == "" {
				assert.Nil(t, adapted.ResponseFormat)
			} else {
				require.NotNil(t, adapted.ResponseFormat)
				assert.Equal(t, tc.responseFormatType, adapted.ResponseFormat.Type)
			}
			require.NotNil(t, adapted.ToolChoice)
			assert.Equal(t, tc.toolChoiceType, adapted.ToolChoice.Type)
		})
	}
}

func assertRawProviderOption(t *testing.T, options provider.ProviderOptions, key, expected string) {
	t.Helper()
	if key == "" {
		assert.Empty(t, options)
		return
	}
	require.Contains(t, options, key)
	raw, ok := options[key].(provider.RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, expected, string(raw.Raw))
}

func assertDataContentJSON(t *testing.T, data *provider.DataContent, expected string) {
	t.Helper()
	require.NotNil(t, data)
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	assert.JSONEq(t, expected, string(encoded))
}

func TestAdaptInteger_IntegralJSONForms(t *testing.T) {
	for _, value := range []string{"1", "1.0", "1e3", "-0"} {
		t.Run(value, func(t *testing.T) {
			number := json.Number(value)
			adapted, err := adaptInteger(&number, "seed")
			require.NoError(t, err)
			require.NotNil(t, adapted)
		})
	}
	for _, value := range []string{"1e1000000", strings.Repeat("1", maxNumericLexemeBytes+1)} {
		t.Run("reject "+value[:min(len(value), 16)], func(t *testing.T) {
			number := json.Number(value)
			_, err := adaptInteger(&number, "seed")
			require.Error(t, err)
		})
	}
}

func TestAdaptFloat_CanonicalDecimalRoundTrip(t *testing.T) {
	for _, value := range []string{"0", "-0", "0.1", "1e3", "1.25"} {
		t.Run("accept "+value, func(t *testing.T) {
			number := json.Number(value)
			adapted, err := adaptFloat(&number, "temperature")
			require.NoError(t, err)
			require.NotNil(t, adapted)
		})
	}
	for _, value := range []string{"1e400", "1e-400", "9007199254740993", "1e1000000"} {
		t.Run("reject "+value, func(t *testing.T) {
			number := json.Number(value)
			_, err := adaptFloat(&number, "temperature")
			require.Error(t, err)
		})
	}
}

func TestHandler_InlineFileLimit(t *testing.T) {
	body := `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"AQID"},"mediaType":"x"},{"type":"file","data":{"type":"data","data":"BAUG"},"mediaType":"x"}]}]}`

	t.Run("aggregate limit plus one", func(t *testing.T) {
		resolver := &handlerTestResolver{model: &handlerTestModel{}}
		handler, err := NewHandler(resolver, WithMaxInlineFileBytes(5))
		require.NoError(t, err)
		recorder := serveV4(t, handler, validV4Request(body))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Zero(t, resolver.calls.Load())
	})

	t.Run("exact aggregate limit", func(t *testing.T) {
		resolver := &handlerTestResolver{model: &handlerTestModel{}}
		handler, err := NewHandler(resolver, WithMaxInlineFileBytes(6))
		require.NoError(t, err)
		recorder := serveV4(t, handler, validV4Request(body))
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, int32(1), resolver.calls.Load())
	})
}

func TestDecodeInlineFileData_BoundsBeforeDecodedAllocation(t *testing.T) {
	decoded, err := decodeInlineFileData("AQ==", 1)
	require.NoError(t, err)
	assert.Equal(t, []byte{1}, decoded)

	_, err = decodeInlineFileData(base64.StdEncoding.EncodeToString(make([]byte, 1<<20)), 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds configured limit")

	_, err = decodeInlineFileData("%%%", 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decoding inline file data")
}

func TestRequestPolicy_RejectsNestedGatewayBeforeResourceDecoding(t *testing.T) {
	body := `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"AQID"},"mediaType":"x","providerOptions":{"bedrock":{"nested":{"gateway":{}}}}}]}]}`
	request, err := decodeWireRequest([]byte(body))
	require.NoError(t, err)
	adapter := requestAdapter{maxInlineBytes: 1024}
	_, err = adapter.adapt(request)
	require.Error(t, err)
	assert.Zero(t, adapter.inlineBytes)
}

func TestRequestPolicy_RejectsReservedGatewayAtEveryProviderOptionLocation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "message", body: `{"prompt":[{"role":"system","content":"system","providerOptions":{"bedrock":{"gateway":{}}}}]}`},
		{name: "part", body: `{"prompt":[{"role":"user","content":[{"type":"text","text":"text","providerOptions":{"bedrock":{"gateway":{}}}}]}]}`},
		{name: "tool", body: `{"prompt":[],"tools":[{"type":"function","name":"echo","inputSchema":{},"providerOptions":{"bedrock":{"gateway":{}}}}]}`},
		{name: "output", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"echo","output":{"type":"text","value":"ok","providerOptions":{"bedrock":{"gateway":{}}}}}]}]}`},
		{name: "output content", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"echo","output":{"type":"content","value":[{"type":"custom","providerOptions":{"bedrock":{"gateway":{}}}}]}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := decodeWireRequest([]byte(tc.body))
			require.NoError(t, err)
			adapter := requestAdapter{maxInlineBytes: 1024}
			_, err = adapter.adapt(request)
			require.Error(t, err)
		})
	}
}

func TestHandler_ResolverAndTimeoutLifecycle(t *testing.T) {
	t.Run("same timed context reaches resolver and model", func(t *testing.T) {
		type contextKey struct{}
		key := contextKey{}
		var resolverContext context.Context
		var modelContext context.Context
		model := &handlerTestModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			modelContext = ctx
			return emptyGenerateResult(), nil
		}}
		resolver := &handlerTestResolver{resolve: func(request *http.Request, modelID string) (provider.LanguageModel, error) {
			resolverContext = request.Context()
			assert.Equal(t, "public-model", modelID)
			assert.Equal(t, "value", request.Context().Value(key))
			_, hasDeadline := request.Context().Deadline()
			assert.True(t, hasDeadline)
			return model, nil
		}}
		handler, err := NewHandler(resolver, WithTotalTimeout(time.Second))
		require.NoError(t, err)
		request := validV4Request(`{"prompt":[]}`).WithContext(context.WithValue(context.Background(), key, "value"))
		recorder := serveV4(t, handler, request)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, resolverContext, modelContext)
	})

	t.Run("total timeout", func(t *testing.T) {
		cause := make(chan error, 1)
		model := &handlerTestModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			cause <- context.Cause(ctx)
			return nil, ctx.Err()
		}}
		handler, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(5*time.Millisecond))
		require.NoError(t, err)
		recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
		assert.Equal(t, http.StatusGatewayTimeout, recorder.Code)
		assert.ErrorIs(t, <-cause, ErrTotalTimeout)
		assert.True(t, decodeSafeFailure(t, recorder).Error.IsRetryable)
	})

	t.Run("blocking resolver cannot exceed total timeout", func(t *testing.T) {
		release := make(chan struct{})
		done := make(chan struct{})
		model := &handlerTestModel{}
		resolver := &handlerTestResolver{resolve: func(*http.Request, string) (provider.LanguageModel, error) {
			defer close(done)
			<-release
			return model, nil
		}}
		handler, err := NewHandler(resolver, WithTotalTimeout(5*time.Millisecond))
		require.NoError(t, err)
		started := time.Now()
		recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
		assert.Equal(t, http.StatusGatewayTimeout, recorder.Code)
		assert.Less(t, time.Since(started), 100*time.Millisecond)
		assert.Zero(t, model.generateCalls.Load())
		close(release)
		<-done
	})

	t.Run("blocking model cannot exceed total timeout", func(t *testing.T) {
		release := make(chan struct{})
		done := make(chan struct{})
		model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			defer close(done)
			<-release
			return emptyGenerateResult(), nil
		}}
		handler, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(5*time.Millisecond))
		require.NoError(t, err)
		started := time.Now()
		recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
		assert.Equal(t, http.StatusGatewayTimeout, recorder.Code)
		assert.Less(t, time.Since(started), 100*time.Millisecond)
		assert.Equal(t, int32(1), model.generateCalls.Load())
		close(release)
		<-done
	})

	t.Run("success returned after deadline", func(t *testing.T) {
		model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			time.Sleep(10 * time.Millisecond)
			return emptyGenerateResult(), nil
		}}
		handler, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(time.Millisecond))
		require.NoError(t, err)
		recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
		assert.Equal(t, http.StatusGatewayTimeout, recorder.Code)
	})

	t.Run("parent deadline is consumer cancellation", func(t *testing.T) {
		release := make(chan struct{})
		done := make(chan struct{})
		model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			defer close(done)
			<-release
			return nil, context.DeadlineExceeded
		}}
		handler, err := NewHandler(&handlerTestResolver{model: model}, WithTotalTimeout(time.Second))
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`).WithContext(ctx))
		assert.Equal(t, 499, recorder.Code)
		assert.False(t, decodeSafeFailure(t, recorder).Error.IsRetryable)
		close(release)
		<-done
	})

	t.Run("active resolver observes request cancellation", func(t *testing.T) {
		started := make(chan struct{})
		observed := make(chan struct{})
		resolver := &handlerTestResolver{resolve: func(request *http.Request, _ string) (provider.LanguageModel, error) {
			close(started)
			<-request.Context().Done()
			close(observed)
			return nil, request.Context().Err()
		}}
		handler, err := NewHandler(resolver)
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		response := make(chan *httptest.ResponseRecorder, 1)
		go func() { response <- serveV4(t, handler, validV4Request(`{"prompt":[]}`).WithContext(ctx)) }()
		<-started
		cancel()
		recorder := <-response
		<-observed
		assert.Equal(t, 499, recorder.Code)
		assert.False(t, decodeSafeFailure(t, recorder).Error.IsRetryable)
	})

	t.Run("active model observes request cancellation", func(t *testing.T) {
		started := make(chan struct{})
		observed := make(chan struct{})
		model := &handlerTestModel{generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			close(started)
			<-ctx.Done()
			close(observed)
			return nil, ctx.Err()
		}}
		handler, err := NewHandler(&handlerTestResolver{model: model})
		require.NoError(t, err)
		ctx, cancel := context.WithCancel(context.Background())
		response := make(chan *httptest.ResponseRecorder, 1)
		go func() { response <- serveV4(t, handler, validV4Request(`{"prompt":[]}`).WithContext(ctx)) }()
		<-started
		cancel()
		recorder := <-response
		<-observed
		assert.Equal(t, 499, recorder.Code)
		assert.False(t, decodeSafeFailure(t, recorder).Error.IsRetryable)
	})
}

func TestHandler_PreCommitContextRecheck(t *testing.T) {
	handler, err := NewHandler(&handlerTestResolver{model: &handlerTestModel{}})
	require.NoError(t, err)
	encoded, err := handler.prepareGenerateResponse(context.Background(), emptyGenerateResult())
	require.NoError(t, err)

	parentCanceled, cancelParent := context.WithCancel(context.Background())
	cancelParent()
	parentDeadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	totalTimeout, cancelTotal := context.WithCancelCause(context.Background())
	cancelTotal(ErrTotalTimeout)

	for _, tc := range []struct {
		name      string
		ctx       context.Context
		status    int
		retryable bool
	}{
		{name: "parent cancellation after preparation", ctx: parentCanceled, status: 499},
		{name: "parent deadline after preparation", ctx: parentDeadline, status: 499},
		{name: "total timeout after preparation", ctx: totalTimeout, status: http.StatusGatewayTimeout, retryable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer := &countingResponseWriter{header: make(http.Header)}
			handler.commitGenerateResponse(writer, tc.ctx, "public-model", encoded)
			assert.Equal(t, []int{tc.status}, writer.statuses)
			assert.Equal(t, 1, writer.writeCalls)
			assert.NotContains(t, writer.body.String(), `"content"`)
			var envelope safeErrorEnvelope
			require.NoError(t, json.Unmarshal(writer.body.Bytes(), &envelope))
			assert.Equal(t, tc.retryable, envelope.Error.IsRetryable)
		})
	}
}

func TestHandler_ResolverAndProviderFailures(t *testing.T) {
	unsafe := provider.NewAPICallError(provider.APICallErrorOptions{
		Message: "secret diagnostic", URL: "https://backend-secret", ResponseBody: "credential-secret", Data: json.RawMessage(`{"secret":true}`), StatusCode: 429,
	})
	cases := []struct {
		name      string
		resolver  *handlerTestResolver
		status    int
		typeName  safeErrorType
		retryable bool
	}{
		{name: "nil model", resolver: &handlerTestResolver{}, status: 500, typeName: errorInternal, retryable: true},
		{name: "resolver arbitrary", resolver: &handlerTestResolver{err: errors.New("secret resolver")}, status: 500, typeName: errorInternal, retryable: true},
		{name: "resolver authentication", resolver: &handlerTestResolver{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "secret"})}, status: 401, typeName: errorAuthentication},
		{name: "resolver forbidden", resolver: &handlerTestResolver{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 403, Message: "secret"})}, status: 403, typeName: errorForbidden},
		{name: "resolver not found", resolver: &handlerTestResolver{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 404})}, status: 404, typeName: errorModelNotFound},
		{name: "provider invalid request", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400, Message: "secret"})
		}}}, status: 400, typeName: errorInvalidRequest},
		{name: "provider rate limit", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, unsafe }}}, status: 429, typeName: errorRateLimit, retryable: true},
		{name: "provider 503 explicit nonretry", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			value := false
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, IsRetryable: &value, Message: "secret"})
		}}}, status: 503, typeName: errorDependency},
		{name: "provider arbitrary", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, errors.New("secret")
		}}}, status: 424, typeName: errorDependency, retryable: true},
		{name: "provider invalid status", resolver: &handlerTestResolver{model: &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 200, Message: "secret"})
		}}}, status: 500, typeName: errorInternal, retryable: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(tc.resolver)
			require.NoError(t, err)
			recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
			assert.Equal(t, tc.status, recorder.Code)
			envelope := decodeSafeFailure(t, recorder)
			assert.Equal(t, tc.typeName, envelope.Error.Type)
			assert.Equal(t, tc.retryable, envelope.Error.IsRetryable)
			assert.NotContains(t, recorder.Body.String(), "secret")
			assert.NotContains(t, recorder.Body.String(), "backend")
		})
	}
}

func TestHandler_ResolverPanicIsContained(t *testing.T) {
	model := &handlerTestModel{}
	resolver := &handlerTestResolver{resolve: func(*http.Request, string) (provider.LanguageModel, error) {
		panic("secret resolver panic")
	}}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	writer := &countingResponseWriter{header: make(http.Header)}

	handler.ServeHTTP(writer, validV4Request(`{"prompt":[]}`))

	assert.Equal(t, []int{http.StatusInternalServerError}, writer.statuses)
	assert.Equal(t, 1, writer.writeCalls)
	assert.Equal(t, int32(1), resolver.calls.Load())
	assert.Zero(t, model.generateCalls.Load())
	assert.NotContains(t, writer.body.String(), "secret")
	assert.NotContains(t, writer.body.String(), "panic")
	var envelope safeErrorEnvelope
	require.NoError(t, json.Unmarshal(writer.body.Bytes(), &envelope))
	assert.Equal(t, errorInternal, envelope.Error.Type)
}

func TestHandler_ProviderPanicIsContained(t *testing.T) {
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		panic("secret provider panic")
	}}
	resolver := &handlerTestResolver{model: model}
	handler, err := NewHandler(resolver)
	require.NoError(t, err)
	writer := &countingResponseWriter{header: make(http.Header)}

	handler.ServeHTTP(writer, validV4Request(`{"prompt":[]}`))

	assert.Equal(t, []int{http.StatusInternalServerError}, writer.statuses)
	assert.Equal(t, 1, writer.writeCalls)
	assert.Equal(t, int32(1), resolver.calls.Load())
	assert.Equal(t, int32(1), model.generateCalls.Load())
	assert.Zero(t, model.streamCalls.Load())
	assert.NotContains(t, writer.body.String(), "secret")
	assert.NotContains(t, writer.body.String(), "panic")
	var envelope safeErrorEnvelope
	require.NoError(t, json.Unmarshal(writer.body.Bytes(), &envelope))
	assert.Equal(t, errorInternal, envelope.Error.Type)
}

func TestHandler_ResultProjectionPrivacyAndArms(t *testing.T) {
	zero := 0
	falseValue := false
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "text", ProviderMetadata: provider.ProviderMetadata{"secret": json.RawMessage(`{"value":true}`)}},
			{Type: provider.ContentReasoning, Text: "reason"},
			{Type: provider.ContentCustom, Kind: "provider.custom"},
			{Type: provider.ContentFile, MediaType: "application/octet-stream", Data: &provider.DataContent{Bytes: []byte{}}},
			{Type: provider.ContentReasoningFile, MediaType: "text/plain", Data: &provider.DataContent{URL: "https://example.test/file"}},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "source", URL: "https://example.test"},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "doc", MediaType: "application/pdf", Title: "Document"},
			{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{"x":1}`), Dynamic: &falseValue},
			{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Preliminary: &falseValue, Dynamic: &falseValue},
			{Type: provider.ContentToolApprovalRequest, ApprovalID: "approval", ToolCallID: "call"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end"},
		Usage: provider.Usage{
			InputTokens: provider.InputTokenUsage{Total: &zero}, OutputTokens: provider.OutputTokenUsage{Total: &zero}, Raw: json.RawMessage(`{"secret":1}`),
		},
		ProviderMetadata: provider.ProviderMetadata{"secret": json.RawMessage(`{"value":true}`)},
		Request:          &provider.RequestMetadata{Body: json.RawMessage(`{"prompt":"secret"}`)},
		Response:         &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "response", Provider: "secret-provider", ModelID: "secret-model", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, Headers: map[string]string{"authorization": "secret"}, Body: json.RawMessage(`{"secret":true}`)},
		Warnings: []provider.Warning{
			{Type: provider.WarnOther, Message: "safe warning"},
			{Type: provider.WarnCompatibility, Feature: "compatibility", Details: "safe details"},
		},
	}
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}
	handler, err := NewHandler(&handlerTestResolver{model: model})
	require.NoError(t, err)
	recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, handler.registry.validate("generate-result", recorder.Body.Bytes()))
	for _, forbidden := range []string{"providerMetadata", `"raw":{"`, `"request"`, `"headers"`, `"body"`, "secret-provider", "secret-model", "authorization"} {
		assert.NotContains(t, recorder.Body.String(), forbidden)
	}
	assert.Contains(t, recorder.Body.String(), `"content"`)
	assert.Contains(t, recorder.Body.String(), `"warnings"`)
	assert.Contains(t, recorder.Body.String(), `"feature":"compatibility"`)
	assert.Contains(t, recorder.Body.String(), `"id":"response"`)
	assert.Contains(t, recorder.Body.String(), `"timestamp":"2026-01-02T03:04:05.000Z"`)
}

func TestHandler_ResultRequiredEmptyValuesRemainPresent(t *testing.T) {
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentFile, MediaType: "", Data: &provider.DataContent{Bytes: []byte{}}},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "", MediaType: "", Title: ""},
			{Type: provider.ContentToolCall, ToolCallID: "", ToolName: "", Input: json.RawMessage(`{}`)},
			{Type: provider.ContentToolResult, ToolCallID: "", ToolName: "", Result: json.RawMessage(`false`)},
			{Type: provider.ContentToolApprovalRequest, ApprovalID: "", ToolCallID: ""},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther},
		Warnings: []provider.Warning{
			{Type: provider.WarnUnsupported, Feature: ""},
			{Type: provider.WarnDeprecated, Setting: "", Message: ""},
			{Type: provider.WarnOther, Message: ""},
		},
	}
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }}
	handler, err := NewHandler(&handlerTestResolver{model: model})
	require.NoError(t, err)
	recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.NoError(t, handler.registry.validate("generate-result", recorder.Body.Bytes()))
	assert.Contains(t, recorder.Body.String(), `"mediaType":""`)
	assert.Contains(t, recorder.Body.String(), `"toolCallId":""`)
	assert.Contains(t, recorder.Body.String(), `"feature":""`)
}

func TestGenerateResultSizeEstimate_AccountsForEscapedPayload(t *testing.T) {
	falseValue := false
	htmlSensitiveResult := json.RawMessage("\"<>&\u2028\u2029\"")
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{
			{Type: provider.ContentText, Text: "text\n\u0000"},
			{Type: provider.ContentReasoning, Text: "reason"},
			{Type: provider.ContentCustom, Kind: "provider.custom"},
			{Type: provider.ContentFile, MediaType: "application/octet-stream", Data: &provider.DataContent{Bytes: []byte{1, 2, 3}}},
			{Type: provider.ContentReasoningFile, MediaType: "text/plain", Data: &provider.DataContent{URL: "https://example.test/file"}},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: "source", URL: "https://example.test", Title: "title"},
			{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: "doc", MediaType: "application/pdf", Title: "Document", Filename: "doc.pdf"},
			{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{"x":"value"}`), ProviderExecuted: true, Dynamic: &falseValue},
			{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: htmlSensitiveResult, IsError: true, Preliminary: &falseValue},
			{Type: provider.ContentToolApprovalRequest, ApprovalID: "approval", ToolCallID: "call"},
		},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "end"},
		Warnings: []provider.Warning{
			{Type: provider.WarnUnsupported, Feature: "feature", Details: "details"},
			{Type: provider.WarnDeprecated, Setting: "old", Message: "new"},
			{Type: provider.WarnOther, Message: "message"},
		},
		Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "response", Timestamp: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}},
	}
	estimate, err := estimateGenerateResultPayload(context.Background(), result, math.MaxInt64)
	require.NoError(t, err)
	projected, err := projectGenerateResult(result)
	require.NoError(t, err)
	encoded, err := json.Marshal(projected)
	require.NoError(t, err)
	assert.LessOrEqual(t, estimate, int64(len(encoded)))
	assert.Contains(t, string(encoded), `\u003c\u003e\u0026\u2028\u2029`)
	handler, err := NewHandler(&handlerTestResolver{model: &handlerTestModel{}})
	require.NoError(t, err)
	streamed, err := handler.prepareGenerateResponse(context.Background(), result)
	require.NoError(t, err)
	assert.Equal(t, encoded, streamed)

	rawEstimate := responseSizeEstimate{ctx: context.Background(), limit: math.MaxInt64}
	require.NoError(t, rawEstimate.addRawJSON(htmlSensitiveResult))
	assert.Equal(t, int64(len(htmlSensitiveResult)+5*3+3*2), rawEstimate.total)
}

func TestHandler_HTMLEscapedToolResultRejectedByPreflightLimit(t *testing.T) {
	result := &provider.GenerateResult{
		Content: []provider.GenerateContentPart{{
			Type:       provider.ContentToolResult,
			ToolCallID: "call",
			ToolName:   "echo",
			Result:     json.RawMessage("\"<>&\u2028\u2029\""),
		}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther},
		Warnings:     []provider.Warning{},
	}
	estimate, err := estimateGenerateResultPayload(context.Background(), result, math.MaxInt64)
	require.NoError(t, err)
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return result, nil
	}}
	handler, err := NewHandler(&handlerTestResolver{model: model}, WithMaxResponseBodyBytes(estimate-1))
	require.NoError(t, err)

	recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "<>&")
}

func TestProjectGeneratedFileData_Validation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    provider.DataContent
		want    string
		wantErr bool
	}{
		{name: "bytes", data: provider.DataContent{Bytes: []byte{1, 2, 3}}, want: `{"type":"data","data":"AQID"}`},
		{name: "empty bytes", data: provider.DataContent{Bytes: []byte{}}, want: `{"type":"data","data":""}`},
		{name: "base64", data: provider.DataContent{Base64: "AQID"}, want: `{"type":"data","data":"AQID"}`},
		{name: "URL", data: provider.DataContent{URL: "https://example.test/file"}, want: `{"type":"url","url":"https://example.test/file"}`},
		{name: "empty", data: provider.DataContent{}, wantErr: true},
		{name: "multiple", data: provider.DataContent{Bytes: []byte{1}, URL: "https://example.test"}, wantErr: true},
		{name: "malformed base64", data: provider.DataContent{Base64: "%%%"}, wantErr: true},
		{name: "reference", data: provider.DataContent{Reference: json.RawMessage(`{"provider":"file"}`)}, wantErr: true},
		{name: "text", data: provider.DataContent{Text: "inline"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projected, err := projectGeneratedFileData(tc.data)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			encoded, err := json.Marshal(projected)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestHandler_ExactResponseLimit(t *testing.T) {
	result := emptyGenerateResult()
	projected, err := projectGenerateResult(result)
	require.NoError(t, err)
	encoded, err := json.Marshal(projected)
	require.NoError(t, err)
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return result, nil
	}}
	for _, tc := range []struct {
		name   string
		limit  int64
		status int
	}{
		{name: "exact", limit: int64(len(encoded)), status: http.StatusOK},
		{name: "one byte over", limit: int64(len(encoded) - 1), status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewHandler(&handlerTestResolver{model: model}, WithMaxResponseBodyBytes(tc.limit))
			require.NoError(t, err)
			recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
			assert.Equal(t, tc.status, recorder.Code)
		})
	}
}

func TestHandler_LargeTextResponseBelowLimitSucceeds(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "ASCII", text: strings.Repeat("x", 1_400_000)},
		{name: "invalid UTF-8", text: strings.Repeat("\xff", 1_400_000)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &provider.GenerateResult{
				Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: tc.text}},
				FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
				Warnings:     []provider.Warning{},
			}
			model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return result, nil
			}}
			handler, err := NewHandler(&handlerTestResolver{model: model})
			require.NoError(t, err)

			recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))

			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Less(t, int64(recorder.Body.Len()), DefaultMaxResponseBodyBytes)
			require.NoError(t, handler.registry.validate("generate-result", recorder.Body.Bytes()))
		})
	}
}

func TestHandler_StructurallyOversizedResponseFailsBeforeSuccess(t *testing.T) {
	content := make([]provider.GenerateContentPart, 10_000)
	for i := range content {
		content[i] = provider.GenerateContentPart{Type: provider.ContentText}
	}
	result := &provider.GenerateResult{
		Content:      content,
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
		Warnings:     []provider.Warning{},
	}
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return result, nil
	}}
	handler, err := NewHandler(&handlerTestResolver{model: model}, WithMaxResponseBodyBytes(100_000))
	require.NoError(t, err)

	recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, fallbackErrorJSON, recorder.Body.String())
}

func TestHandler_InvalidOrOversizedResultFailsBeforeSuccess(t *testing.T) {
	cases := []struct {
		name   string
		result *provider.GenerateResult
		option Option
	}{
		{name: "nil", result: nil},
		{name: "unknown arm", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: "future"}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}}},
		{name: "invalid raw", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolCall, ToolCallID: "c", ToolName: "t", Input: json.RawMessage(`{`)}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}}},
		{name: "null tool result", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolResult, ToolCallID: "c", ToolName: "t", Result: json.RawMessage(`null`)}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}}},
		{name: "unencodable timestamp", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}, Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}}},
		{name: "oversized", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: strings.Repeat("x", 1000)}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}}, option: WithMaxResponseBodyBytes(100)},
		{name: "huge file rejected by preflight", result: &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentFile, MediaType: "application/octet-stream", Data: &provider.DataContent{Bytes: make([]byte, 1<<20)}}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonOther}}, option: WithMaxResponseBodyBytes(100)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return tc.result, nil }}
			options := []Option{}
			if tc.option != nil {
				options = append(options, tc.option)
			}
			handler, err := NewHandler(&handlerTestResolver{model: model}, options...)
			require.NoError(t, err)
			recorder := serveV4(t, handler, validV4Request(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), strings.Repeat("x", 10))
		})
	}
}

func TestHandler_ErrorLimitFallbackAndWriteFailure(t *testing.T) {
	model := &handlerTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, errors.New("provider failed")
	}}
	handler, err := NewHandler(&handlerTestResolver{model: model}, WithMaxErrorBodyBytes(int64(len(fallbackErrorJSON))))
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	handler.writeFailure(recorder, safeFailure{status: http.StatusBadRequest, typeName: errorInvalidRequest, message: strings.Repeat("safe", 100)})
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, fallbackErrorJSON, recorder.Body.String())

	writer := &failingResponseWriter{header: make(http.Header)}
	handler.ServeHTTP(writer, validV4Request(`{"prompt":[]}`))
	assert.Equal(t, 1, writer.writeHeaderCalls)
	assert.Equal(t, 1, writer.writeCalls)
}

type countingResponseWriter struct {
	header     http.Header
	statuses   []int
	writeCalls int
	body       bytes.Buffer
}

func (w *countingResponseWriter) Header() http.Header { return w.header }
func (w *countingResponseWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
}
func (w *countingResponseWriter) Write(body []byte) (int, error) {
	w.writeCalls++
	return w.body.Write(body)
}

type failingResponseWriter struct {
	header           http.Header
	writeHeaderCalls int
	writeCalls       int
}

func (w *failingResponseWriter) Header() http.Header { return w.header }
func (w *failingResponseWriter) WriteHeader(int)     { w.writeHeaderCalls++ }
func (w *failingResponseWriter) Write([]byte) (int, error) {
	w.writeCalls++
	return 0, io.ErrClosedPipe
}

func TestEmbeddedRegistry_SharedConcurrentAndUnknown(t *testing.T) {
	t.Run("compiles outside repository working directory", func(t *testing.T) {
		t.Chdir(t.TempDir())
		registry, err := compileEmbeddedContractRegistry()
		require.NoError(t, err)
		require.NoError(t, registry.validate("request", []byte(`{"prompt":[]}`)))
	})

	resolver := &handlerTestResolver{model: &handlerTestModel{}}
	handlers := make([]*Handler, 8)
	for i := range handlers {
		handler, err := NewHandler(resolver)
		require.NoError(t, err)
		handlers[i] = handler
		assert.Same(t, handlers[0].registry, handler.registry)
	}
	assert.Error(t, handlers[0].registry.validate("unknown", []byte(`{}`)))

	statuses := make(chan int, 32)
	var wait sync.WaitGroup
	for i := range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			body := `{"prompt":[]}`
			want := http.StatusOK
			if i%2 != 0 {
				body = `{"prompt":[],"unknown":true}`
				want = http.StatusBadRequest
			}
			recorder := httptest.NewRecorder()
			handlers[i%len(handlers)].ServeHTTP(recorder, validV4Request(body))
			if recorder.Code != want {
				statuses <- recorder.Code
			}
		}()
	}
	wait.Wait()
	close(statuses)
	assert.Empty(t, statuses)
}

func TestSafeValidationPath_NormalizesJSONPointer(t *testing.T) {
	registry, err := loadEmbeddedContractRegistry()
	require.NoError(t, err)
	err = registry.validate("request", []byte(`{"prompt":[],"a/b~c":true}`))
	require.Error(t, err)
	path := safeValidationPath(err)
	assert.True(t, path == "" || strings.HasPrefix(path, "/"))
}

func TestAcceptRepresentation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		header     string
		compatible bool
		valid      bool
	}{
		{name: "positive wildcard wins", header: "application/json;q=0, */*;q=0.5", compatible: true, valid: true},
		{name: "quoted comma", header: `application/json;q=1;foo="a,b"`, compatible: true, valid: true},
		{name: "quoted q text", header: `application/json;foo="q=invalid"`, compatible: true, valid: true},
		{name: "quoted q value", header: `application/json;q="0.5"`, compatible: false, valid: false},
		{name: "unterminated quote", header: `application/json;foo="a,b`, compatible: false, valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compatible, valid := acceptsRepresentation(tc.header, MIMEJSON)
			assert.Equal(t, tc.compatible, compatible)
			assert.Equal(t, tc.valid, valid)
		})
	}
}

func TestWritePreparedResponse_DoesNotRetry(t *testing.T) {
	writer := &failingResponseWriter{header: make(http.Header)}
	err := writePreparedResponse(writer, http.StatusOK, bytes.Repeat([]byte("x"), 4))
	require.Error(t, err)
	assert.Equal(t, 1, writer.writeHeaderCalls)
	assert.Equal(t, 1, writer.writeCalls)
}
