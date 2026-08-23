package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingResolver struct {
	mu           sync.Mutex
	calls        int
	requestedID  string
	resolved     catalog.ResolvedModel
	err          error
	panicResolve bool
}

func (r *recordingResolver) ResolveModel(_ context.Context, modelID string) (catalog.ResolvedModel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.requestedID = modelID
	if r.panicResolve {
		panic("private resolver panic")
	}
	return r.resolved, r.err
}

func (r *recordingResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *recordingResolver) requestedModelID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requestedID
}

type recordingModel struct {
	mu                 sync.Mutex
	calls              int
	generateCalls      int
	streamCalls        int
	options            provider.CallOptions
	generate           func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)
	stream             func(context.Context, provider.CallOptions) (*provider.StreamResult, error)
	specification      string
	panicSpecification bool
}

func (m *recordingModel) SpecificationVersion() string {
	if m.panicSpecification {
		panic("private specification panic")
	}
	if m.specification != "" {
		return m.specification
	}
	return "v4"
}

func (*recordingModel) Provider() string                           { return "private-provider" }
func (*recordingModel) ModelID() string                            { return "private-backend-model" }
func (*recordingModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *recordingModel) DoStream(ctx context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.mu.Lock()
	m.calls++
	m.streamCalls++
	m.options = options
	stream := m.stream
	m.mu.Unlock()
	if stream != nil {
		return stream(ctx, options)
	}
	parts := make(chan provider.StreamPart, 1)
	parts <- finishPart()
	close(parts)
	return &provider.StreamResult{Stream: parts}, nil
}
func (m *recordingModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	m.mu.Lock()
	m.calls++
	m.generateCalls++
	m.options = options
	generate := m.generate
	m.mu.Unlock()
	if generate != nil {
		return generate(ctx, options)
	}
	return validGenerateResult(), nil
}
func (m *recordingModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}
func (m *recordingModel) receivedOptions() provider.CallOptions {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.options
}
func (m *recordingModel) invocationCounts() (generate, stream int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.generateCalls, m.streamCalls
}

type runtimeHarness struct {
	handler  *handler
	resolver *recordingResolver
	model    *recordingModel
}

func newRuntimeHarness(t *testing.T, limits Limits) *runtimeHarness {
	t.Helper()
	model := &recordingModel{}
	resolver := &recordingResolver{resolved: catalog.ResolvedModel{ID: "canonical/model", Model: model}}
	created, err := New(Config{Resolver: resolver, Limits: limits})
	require.NoError(t, err)
	return &runtimeHarness{handler: created.(*handler), resolver: resolver, model: model}
}

func (h *runtimeHarness) serve(req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, req)
	return recorder
}

func TestRuntimeModeDispatch(t *testing.T) {
	for _, tc := range []struct {
		name          string
		streaming     string
		generateCalls int
		streamCalls   int
	}{
		{name: "unary", streaming: "false", generateCalls: 1},
		{name: "streaming", streaming: "true", streamCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			request := validRequest(`{"prompt":[]}`)
			request.Header.Set(HeaderStreaming, tc.streaming)
			response := harness.serve(request)
			assert.Equal(t, http.StatusOK, response.Code)
			generateCalls, streamCalls := harness.model.invocationCounts()
			assert.Equal(t, tc.generateCalls, generateCalls)
			assert.Equal(t, tc.streamCalls, streamCalls)
			assert.Equal(t, 1, harness.resolver.callCount())
		})
	}
}

func TestRuntimeRequestValidationStopsDownstream(t *testing.T) {
	bodies := [][]byte{
		append([]byte(`{"prompt":[],"providerOptions":{"example":"`), append([]byte{0xff}, []byte(`"}}`)...)...),
		[]byte(`{"prompt":[}`),
		[]byte(`{"prompt":[]} {}`),
		[]byte(`{"prompt":[],"unknown":true}`),
		[]byte(`{"prompt":[],"maxOutputTokens":null}`),
		[]byte(`{"prompt":[],"maxOutputTokens":1.5}`),
		[]byte(`{"prompt":[],"temperature":1e309}`),
	}
	for _, body := range bodies {
		harness := newRuntimeHarness(t, testLimits())
		req := validRequest("")
		req.Body = ioNopCloserBytes(body)
		response := harness.serve(req)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Zero(t, harness.resolver.callCount())
		assert.Zero(t, harness.model.callCount())
	}
}

func ioNopCloserBytes(body []byte) *byteReadCloser {
	return &byteReadCloser{Reader: bytes.NewReader(body)}
}

type byteReadCloser struct{ *bytes.Reader }

func (r *byteReadCloser) Close() error { return nil }

func TestRuntimeIntegerControls(t *testing.T) {
	fields := []struct {
		name  string
		value func(provider.CallOptions) *int
	}{
		{name: "maxOutputTokens", value: func(options provider.CallOptions) *int { return options.MaxOutputTokens }},
		{name: "topK", value: func(options provider.CallOptions) *int { return options.TopK }},
		{name: "seed", value: func(options provider.CallOptions) *int { return options.Seed }},
	}
	for _, field := range fields {
		for _, value := range []int{-1, 0, 1} {
			t.Run(fmt.Sprintf("%s/%d", field.name, value), func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				body := fmt.Sprintf(`{"prompt":[],%q:%d}`, field.name, value)
				response := harness.serve(validRequest(body))
				assert.Equal(t, http.StatusOK, response.Code)
				mapped := field.value(harness.model.receivedOptions())
				require.NotNil(t, mapped)
				assert.Equal(t, value, *mapped)
			})
		}
		for _, lexeme := range []string{"1.0", "1e0", "-0.0"} {
			t.Run(field.name+"/reject/"+lexeme, func(t *testing.T) {
				body := fmt.Sprintf(`{"prompt":[],%q:%s}`, field.name, lexeme)
				_, failure := mapWireRequest([]byte(body))
				require.NotNil(t, failure)
				harness := newRuntimeHarness(t, testLimits())
				response := harness.serve(validRequest(body))
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Zero(t, harness.resolver.callCount())
			})
		}
	}
}

func TestRuntimeSupportedMapping(t *testing.T) {
	harness := newRuntimeHarness(t, testLimits())
	body := `{
		"prompt":[
			{"role":"system","content":""},
			{"role":"system","content":"system-2"},
			{"role":"user","content":[{"type":"text","text":""},{"type":"text","text":"user-2"}]},
			{"role":"assistant","content":[{"type":"text","text":"assistant"}]}
		],
		"maxOutputTokens":0,
		"temperature":0,
		"topP":0,
		"topK":0,
		"presencePenalty":0,
		"frequencyPenalty":0,
		"stopSequences":["first","second"],
		"seed":0,
		"reasoning":"high",
		"responseFormat":{"type":"text"},
		"tools":[],
		"headers":{},
		"providerOptions":{"p":{}},
		"includeRawChunks":false
	}`
	response := harness.serve(validRequest(body))
	require.Equal(t, http.StatusOK, response.Code)
	options := harness.model.receivedOptions()
	assert.Equal(t, []provider.Message{
		provider.NewSystemMessage(""),
		provider.NewSystemMessage("system-2"),
		provider.NewUserMessage(provider.TextPart(""), provider.TextPart("user-2")),
		provider.NewAssistantMessage(provider.TextPart("assistant")),
	}, options.Prompt)
	for _, value := range []*int{options.MaxOutputTokens, options.TopK, options.Seed} {
		require.NotNil(t, value)
		assert.Zero(t, *value)
	}
	for _, value := range []*float64{options.Temperature, options.TopP, options.PresencePenalty, options.FrequencyPenalty} {
		require.NotNil(t, value)
		assert.Zero(t, *value)
	}
	assert.Equal(t, []string{"first", "second"}, options.StopSequences)
	assert.Equal(t, provider.ReasoningHigh, options.Reasoning)
	assert.Nil(t, options.Headers)
	assert.Nil(t, options.ProviderOptions)
}

func TestRuntimeStandardJSONNormalization(t *testing.T) {
	t.Run("duplicate members use the last value", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		response := harness.serve(validRequest(`{"prompt":[{"role":"system","content":"first"}],"prompt":[{"role":"system","content":"last"}]}`))
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, []provider.Message{provider.NewSystemMessage("last")}, harness.model.receivedOptions().Prompt)
	})

	t.Run("escaped lone surrogate normalizes to replacement character", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		response := harness.serve(validRequest(`{"prompt":[{"role":"system","content":"before\ud800after"}]}`))
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, []provider.Message{provider.NewSystemMessage("before\ufffdafter")}, harness.model.receivedOptions().Prompt)
	})
}

func TestRuntimeUnsupportedCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		capability unsupportedCapability
	}{
		{name: "files", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain"}]}]}`, capability: capabilityFiles},
		{name: "reasoning", body: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":"x"}]}]}`, capability: capabilityReasoningContent},
		{name: "custom", body: `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"p.x"}]}]}`, capability: capabilityCustomContent},
		{name: "tools", body: `{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{}}]}`, capability: capabilityTools},
		{name: "tool approvals", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":false}]}]}`, capability: capabilityToolApprovals},
		{name: "structured output", body: `{"prompt":[],"responseFormat":{"type":"json"}}`, capability: capabilityStructuredOutput},
		{name: "provider options", body: `{"prompt":[],"providerOptions":{"p":{"enabled":true}}}`, capability: capabilityProviderOptions},
		{name: "body headers", body: `{"prompt":[],"headers":{"x-example":""}}`, capability: capabilityBodyHeaders},
		{name: "raw output", body: `{"prompt":[],"includeRawChunks":true}`, capability: capabilityRawOutput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, string(unsupportedCapabilityDocument(tc.capability)), response.Body.String())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
		})
	}
}

type goldenRecord struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func loadGolden(t *testing.T, name string) []goldenRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../../test/providerwire-v4/goldens", name))
	require.NoError(t, err)
	var records []goldenRecord
	require.NoError(t, json.Unmarshal(data, &records))
	return records
}

func requestFromGolden(t *testing.T, record goldenRecord) *http.Request {
	t.Helper()
	req := httptest.NewRequest(record.Method, record.Path, bytes.NewReader(record.Body))
	for name, value := range record.Headers {
		req.Header.Set(name, value)
	}
	return req
}

func TestRuntimeGoldenReplay(t *testing.T) {
	tests := []struct {
		file       string
		index      int
		status     int
		capability unsupportedCapability
		modelCalls int
	}{
		{file: "streaming.json", status: http.StatusOK, modelCalls: 1},
		{file: "sequence.json", status: http.StatusOK, modelCalls: 1},
		{file: "sequence.json", index: 1, status: http.StatusOK, modelCalls: 1},
		{file: "scalar-presence.json", status: http.StatusBadRequest, capability: capabilityBodyHeaders},
		{file: "headers.json", status: http.StatusBadRequest, capability: capabilityBodyHeaders},
		{file: "headers.json", index: 1, status: http.StatusBadRequest, capability: capabilityBodyHeaders},
		{file: "comprehensive-unions.json", status: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%d", tc.file, tc.index), func(t *testing.T) {
			records := loadGolden(t, tc.file)
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(requestFromGolden(t, records[tc.index]))
			assert.Equal(t, tc.status, response.Code)
			if tc.capability != "" {
				assert.Equal(t, string(unsupportedCapabilityDocument(tc.capability)), response.Body.String())
			}
			assert.Equal(t, tc.modelCalls, harness.model.callCount())
		})
	}
}

func TestRuntimeResolution(t *testing.T) {
	t.Run("exact alias resolves once", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		req := validRequest(`{"prompt":[]}`)
		req.Header.Set(HeaderModelID, " alias with spaces ")
		response := harness.serve(req)
		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, " alias with spaces ", harness.resolver.requestedModelID())
		assert.Equal(t, 1, harness.resolver.callCount())
		assert.Equal(t, 1, harness.model.callCount())
	})

	t.Run("resolution failures remain private", func(t *testing.T) {
		var typedNil *provider.APICallError
		for _, tc := range []struct {
			err    error
			status int
		}{
			{err: &catalog.UnknownModelError{ModelID: "private-alias"}, status: http.StatusNotFound},
			{err: errors.New("private resolver failure"), status: http.StatusInternalServerError},
			{err: typedNil, status: http.StatusInternalServerError},
		} {
			harness := newRuntimeHarness(t, testLimits())
			harness.resolver.err = tc.err
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.NotContains(t, response.Body.String(), "private")
			assert.Zero(t, harness.model.callCount())
		}
	})

	t.Run("invalid and panicking resolved models fail safely", func(t *testing.T) {
		var typedNil *recordingModel
		for _, resolved := range []catalog.ResolvedModel{
			{Model: &recordingModel{}},
			{ID: "canonical"},
			{ID: "canonical", Model: typedNil},
			{ID: "canonical", Model: &recordingModel{specification: "v3"}},
			{ID: "canonical", Model: &recordingModel{panicSpecification: true}},
		} {
			harness := newRuntimeHarness(t, testLimits())
			harness.resolver.resolved = resolved
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
		}
	})

	t.Run("invalid canonical id fails before streaming invocation and commitment", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.resolver.resolved.ID = string([]byte{0xff})
		response := harness.serve(streamRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		assert.Equal(t, string(canonicalInternalError), response.Body.String())
		assert.Zero(t, harness.model.callCount())
	})
}

func TestRuntimeModelContainment(t *testing.T) {
	t.Run("already canceled request does not invoke model", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := harness.serve(validRequest(`{"prompt":[]}`).WithContext(ctx))
		assert.Equal(t, 499, response.Code)
		assert.Zero(t, harness.model.callCount())
	})

	t.Run("panic nil result and typed nil error are internal", func(t *testing.T) {
		var typedNil *provider.APICallError
		for _, generate := range []func(context.Context, provider.CallOptions) (*provider.GenerateResult, error){
			func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { panic("private panic") },
			func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, nil },
			func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, typedNil },
		} {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = generate
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
		}
	})

	t.Run("timeout bounds a model that ignores context", func(t *testing.T) {
		limits := testLimits()
		limits.ModelDuration = 20 * time.Millisecond
		harness := newRuntimeHarness(t, limits)
		release := make(chan struct{})
		returned := make(chan struct{})
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			<-release
			close(returned)
			return validGenerateResult(), nil
		}
		start := time.Now()
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusGatewayTimeout, response.Code)
		assert.Less(t, time.Since(start), time.Second)
		close(release)
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("late model return blocked")
		}
	})

	t.Run("caller cancellation returns without waiting", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		started := make(chan struct{})
		release := make(chan struct{})
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			close(started)
			<-release
			return validGenerateResult(), nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(response, validRequest(`{"prompt":[]}`).WithContext(ctx))
			close(done)
		}()
		<-started
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("handler did not return")
		}
		assert.Equal(t, 499, response.Code)
		close(release)
	})
}

type testNetError struct{ timeout bool }

func (e testNetError) Error() string   { return "private transport details" }
func (e testNetError) Timeout() bool   { return e.timeout }
func (e testNetError) Temporary() bool { return false }

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

func TestSafeErrorReduction(t *testing.T) {
	t.Run("fixed documents", func(t *testing.T) {
		for _, tc := range []struct {
			value  safeError
			status int
		}{
			{value: safeError{category: safeInvalidRequest}, status: http.StatusBadRequest},
			{value: safeError{category: safeModelNotFound}, status: http.StatusNotFound},
			{value: safeError{category: safeRateLimit}, status: http.StatusTooManyRequests},
			{value: safeError{category: safeOverload}, status: http.StatusServiceUnavailable},
			{value: safeError{category: safeFailedDependency}, status: http.StatusFailedDependency},
			{value: safeError{category: safeUpstream}, status: http.StatusBadGateway},
			{value: safeError{category: safeTimeout}, status: http.StatusGatewayTimeout},
			{value: safeError{category: safeCancellation}, status: 499},
			{value: safeError{category: safeInternal}, status: http.StatusInternalServerError},
			{value: safeError{category: safeInvalidRequest, capability: capabilityFiles}, status: http.StatusBadRequest},
		} {
			h := newTestHandler(t, testLimits())
			response := httptest.NewRecorder()
			h.writeSafeError(response, tc.value)
			document := documentForSafeError(tc.value)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, string(document.body), response.Body.String())
		}
	})

	t.Run("provider status and transport reduction", func(t *testing.T) {
		for _, tc := range []struct {
			err      error
			category safeErrorCategory
		}{
			{err: context.Canceled, category: safeCancellation},
			{err: context.DeadlineExceeded, category: safeTimeout},
			{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 408}), category: safeTimeout},
			{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429}), category: safeRateLimit},
			{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503}), category: safeOverload},
			{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 400}), category: safeFailedDependency},
			{err: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500}), category: safeUpstream},
			{err: testNetError{timeout: true}, category: safeTimeout},
			{err: &url.Error{Op: "Post", URL: "https://private", Err: testNetError{}}, category: safeUpstream},
			{err: &net.OpError{Op: "dial", Net: "tcp", Addr: testAddr("private:443"), Err: errors.New("refused")}, category: safeUpstream},
			{err: errors.New("private internal"), category: safeInternal},
		} {
			assert.Equal(t, tc.category, safeErrorFromProvider(tc.err).category)
		}
	})

	t.Run("hostile provider errors remain private", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{
				StatusCode:        500,
				Message:           "credential=secret provider=private",
				URL:               "https://provider.invalid/private",
				RequestBodyValues: json.RawMessage(`{"authorization":"secret"}`),
				ResponseHeaders:   map[string][]string{"Authorization": {"secret"}},
				ResponseBody:      `{"secret":true}`,
			})
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusBadGateway, response.Code)
		assert.Equal(t, string(canonicalUpstreamError), response.Body.String())
		assert.NotContains(t, response.Body.String(), "secret")
	})
}
