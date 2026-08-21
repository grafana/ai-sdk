package v4

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/failure"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateStrictJSON(t *testing.T) {
	tests := []struct {
		name  string
		body  []byte
		valid bool
	}{
		{"object", []byte(`{"a":{"value":1},"b":{"value":2}}`), true},
		{"array", []byte(`[1,true,null,"x"]`), true},
		{"invalid UTF-8", []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}, false},
		{"duplicate", []byte(`{"a":1,"a":2}`), false},
		{"nested duplicate", []byte(`{"a":{"x":1,"x":2}}`), false},
		{"comment", []byte(`{"a":1/* no */}`), false},
		{"trailing comma", []byte(`{"a":1,}`), false},
		{"invalid number", []byte(`{"a":01}`), false},
		{"trailing value", []byte(`{} {}`), false},
		{"trailing text", []byte(`{} nope`), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateStrictJSON(tc.body)
			if tc.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestMapRequest_TextAndSettings(t *testing.T) {
	body := []byte(`{
		"prompt":[
			{"role":"system","content":""},
			{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":""}]},
			{"role":"assistant","content":[{"type":"text","text":"reply"}]}
		],
		"maxOutputTokens":0,"temperature":0,"topP":0.5,"topK":7,
		"presencePenalty":0,"frequencyPenalty":-0.5,"stopSequences":[],"seed":42,
		"reasoning":"high"
	}`)
	options, err := mapRequest(body)
	require.NoError(t, err)
	require.Len(t, options.Prompt, 3)
	assert.Equal(t, provider.RoleSystem, options.Prompt[0].Role)
	assert.Equal(t, "", options.Prompt[0].Content[0].Text)
	assert.Equal(t, []provider.ContentPart{provider.TextPart("hello"), provider.TextPart("")}, options.Prompt[1].Content)
	require.NotNil(t, options.MaxOutputTokens)
	integer, ok := options.MaxOutputTokens.Int64()
	assert.True(t, ok)
	assert.Equal(t, int64(0), integer)
	require.NotNil(t, options.Temperature)
	assert.Equal(t, float64(0), *options.Temperature)
	require.NotNil(t, options.TopK)
	integer, ok = options.TopK.Int64()
	assert.True(t, ok)
	assert.Equal(t, int64(7), integer)
	assert.NotNil(t, options.StopSequences)
	assert.Empty(t, options.StopSequences)
	require.NotNil(t, options.Reasoning)
	assert.Equal(t, provider.ReasoningHigh, *options.Reasoning)

	absent, err := mapRequest([]byte(`{"prompt":[]}`))
	require.NoError(t, err)
	assert.Nil(t, absent.MaxOutputTokens)
	assert.Nil(t, absent.Temperature)
	assert.Nil(t, absent.StopSequences)
	assert.Nil(t, absent.Reasoning)
}

func TestMapRequest_MapsExactLanguageModelNumbers(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		integer   int64
		wantError bool
	}{
		{"decimal integer beyond JS safe range", "9007199254740993.0", 9007199254740993, false},
		{"exponent integer", "9.007199254740993e15", 9007199254740993, false},
		{"int64 maximum exponent", "9.223372036854775807e18", 9223372036854775807, false},
		{"plain integer outside int64", "9223372036854775808", 0, true},
		{"integral exponent outside int64", "1e20", 0, true},
		{"non-integral value that rounds to integer", "9007199254740992.5", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			options, err := mapRequest([]byte(`{"prompt":[],"seed":` + tc.token + `}`))
			if tc.wantError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, options.Seed)
			integer, ok := options.Seed.Int64()
			assert.True(t, ok)
			assert.Equal(t, tc.integer, integer)
		})
	}

	options, err := mapRequest([]byte(`{"prompt":[],"topK":0.125}`))
	require.NoError(t, err)
	value, ok := options.TopK.Float64()
	assert.True(t, ok)
	assert.Equal(t, 0.125, value)
}

func TestHandler_EnvelopeAndBodyFailuresBypassDispatch(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*http.Request)
		body    io.ReadCloser
		options []Option
		status  int
	}{
		{"method", func(r *http.Request) { r.Method = http.MethodGet }, nil, nil, http.StatusBadRequest},
		{"missing model", func(r *http.Request) { delete(r.Header, HeaderModelID) }, nil, nil, http.StatusBadRequest},
		{"duplicate model", func(r *http.Request) { r.Header[HeaderModelID] = []string{"one", "two"} }, nil, nil, http.StatusBadRequest},
		{"invalid version", func(r *http.Request) { r.Header[HeaderSpecVersion] = []string{"3"} }, nil, nil, http.StatusBadRequest},
		{"invalid streaming", func(r *http.Request) { r.Header[HeaderStreaming] = []string{"yes"} }, nil, nil, http.StatusBadRequest},
		{"missing content type", func(r *http.Request) { delete(r.Header, "Content-Type") }, nil, nil, http.StatusBadRequest},
		{"duplicate content type", func(r *http.Request) { r.Header["Content-Type"] = []string{MIMEJSON, MIMEJSON} }, nil, nil, http.StatusBadRequest},
		{"parameterized content type", func(r *http.Request) { r.Header["Content-Type"] = []string{"application/json; charset=utf-8"} }, nil, nil, http.StatusBadRequest},
		{"body read", nil, errorReadCloser{}, nil, http.StatusBadRequest},
		{"oversized", nil, nil, []Option{WithMaxRequestBodyBytes(2)}, http.StatusBadRequest},
		{"invalid UTF-8", nil, io.NopCloser(bytes.NewReader([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})), nil, http.StatusBadRequest},
		{"schema invalid", nil, io.NopCloser(strings.NewReader(`{"prompt":[],"unknown":true}`)), nil, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model := &testModel{}
			resolver := resolverFor(model)
			policyCalls := 0
			options := append([]Option{}, tc.options...)
			options = append(options, WithPolicy(PolicyFunc(func(context.Context, PolicyRequest) *failure.Failure {
				policyCalls++
				return nil
			})))
			handler, err := NewHandler(resolver, options...)
			require.NoError(t, err)
			request := validRequest(`{"prompt":[]}`, false)
			if tc.body != nil {
				request.Body = tc.body
			}
			trackedBody := &trackingReadCloser{ReadCloser: request.Body}
			request.Body = trackedBody
			if tc.mutate != nil {
				tc.mutate(request)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"))
			require.NoError(t, handler.schemas.error.Validate(response.Body.Bytes()))
			assert.Zero(t, policyCalls)
			assert.Zero(t, resolver.calls)
			assert.Zero(t, model.calls)
			if tc.name == "body read" || tc.name == "oversized" || tc.name == "invalid UTF-8" || tc.name == "schema invalid" {
				assert.True(t, trackedBody.closed)
			}
		})
	}
}

func TestHandler_CancellationDuringBodyTakesPrecedence(t *testing.T) {
	canceledContext, cancelCanceled := context.WithCancel(context.Background())
	cancelCanceled()
	deadlineContext, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	tests := []struct {
		name   string
		ctx    context.Context
		cancel context.CancelFunc
		status int
	}{
		{"cancellation", canceledContext, cancelCanceled, 499},
		{"deadline", deadlineContext, cancelDeadline, http.StatusGatewayTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer tc.cancel()
			model := &testModel{}
			resolver := resolverFor(model)
			handler, err := NewHandler(resolver)
			require.NoError(t, err)
			request := validRequest(`{"prompt":[]}`, false).WithContext(tc.ctx)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, tc.status, response.Code)
			assert.Zero(t, resolver.calls)
			assert.Zero(t, model.calls)
		})
	}
}

func TestHandler_AdditionalHeadersAreAccepted(t *testing.T) {
	model := &testModel{}
	handler := newTestHandler(t, model)
	request := validRequest(`{"prompt":[]}`, false)
	trackedBody := &trackingReadCloser{ReadCloser: request.Body}
	request.Body = trackedBody
	request.Header.Set("Authorization", "private")
	request.Header.Set("X-Custom", "value")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, trackedBody.closed)
	assert.Nil(t, model.options.Headers)
}

func TestHandler_DeferredFeaturesFailClosed(t *testing.T) {
	bodies := []string{
		`{"prompt":[],"tools":[]}`,
		`{"prompt":[],"tools":[{"type":"function","name":"tool","inputSchema":{"type":"object"}}]}`,
		`{"prompt":[],"toolChoice":{"type":"auto"}}`,
		`{"prompt":[],"headers":{}}`,
		`{"prompt":[],"providerOptions":{}}`,
		`{"prompt":[],"includeRawChunks":false}`,
		`{"prompt":[],"responseFormat":{"type":"text"}}`,
		`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain"}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":"x"}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"data","data":""},"mediaType":"image/png"}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"example.value"}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"tool","input":{}}]}]}`,
		`{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"text","value":"x"}}]}]}`,
		`{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"approval","approved":true}]}]}`,
		`{"prompt":[{"role":"system","content":"x","providerOptions":{}}]}`,
		`{"prompt":[{"role":"user","content":[{"type":"text","text":"x","providerOptions":{}}]}]}`,
	}
	for _, body := range bodies {
		model := &testModel{}
		resolver := resolverFor(model)
		policyCalls := 0
		handler, err := NewHandler(resolver, WithPolicy(PolicyFunc(func(context.Context, PolicyRequest) *failure.Failure {
			policyCalls++
			return nil
		})))
		require.NoError(t, err)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, validRequest(body, false))
		assert.Equal(t, http.StatusBadRequest, response.Code, body)
		assert.Equal(t, MIMEJSON, response.Header().Get("Content-Type"), body)
		assert.Zero(t, policyCalls, body)
		assert.Zero(t, resolver.calls, body)
		assert.Zero(t, model.calls, body)
	}
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) { return 0, errors.New("private read failure") }
func (errorReadCloser) Close() error             { return nil }

type trackingReadCloser struct {
	io.ReadCloser
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return r.ReadCloser.Close()
}
