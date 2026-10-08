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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/fallback"
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
	created, err := New(Config{Selector: CatalogSelector(resolver), Limits: limits})
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

func TestRuntimeToolChoice(t *testing.T) {
	for _, streaming := range []string{"false", "true"} {
		for _, tools := range []string{"", `,"tools":[]`} {
			for _, tc := range []struct {
				name     string
				field    string
				expected *provider.ToolChoice
			}{
				{name: "omitted"},
				{name: "auto", field: `,"toolChoice":{"type":"auto"}`, expected: &provider.ToolChoice{Type: provider.ToolChoiceAuto}},
				{name: "none", field: `,"toolChoice":{"type":"none"}`, expected: &provider.ToolChoice{Type: provider.ToolChoiceNone}},
				{name: "required", field: `,"toolChoice":{"type":"required"}`, expected: &provider.ToolChoice{Type: provider.ToolChoiceRequired}},
				{name: "named", field: `,"toolChoice":{"type":"tool","toolName":"f"}`, expected: &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "f"}},
			} {
				t.Run(streaming+"/"+tools+"/"+tc.name, func(t *testing.T) {
					harness := newRuntimeHarness(t, testLimits())
					request := validRequest(`{"prompt":[]` + tools + tc.field + `}`)
					request.Header.Set(HeaderStreaming, streaming)
					response := harness.serve(request)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					assert.Equal(t, 1, harness.resolver.callCount())
					generate, stream := harness.model.invocationCounts()
					if streaming == "true" {
						assert.Equal(t, 0, generate)
						assert.Equal(t, 1, stream)
					} else {
						assert.Equal(t, 1, generate)
						assert.Equal(t, 0, stream)
					}
					opts := harness.model.receivedOptions()
					assert.Empty(t, opts.Tools)
					assert.Equal(t, tc.expected, opts.ToolChoice)
				})
			}
		}
		for _, tc := range []struct {
			name   string
			fields string
		}{
			{name: "null", fields: `"toolChoice":null`},
			{name: "unknown", fields: `"toolChoice":{"type":"future"}`},
			{name: "string", fields: `"toolChoice":"auto"`},
			{name: "extra", fields: `"toolChoice":{"type":"auto","toolName":"f"}`},
		} {
			t.Run(streaming+"/"+tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(`{"prompt":[],` + tc.fields + `}`)
				request.Header.Set(HeaderStreaming, streaming)
				response := harness.serve(request)
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
				assert.Equal(t, string(canonicalInvalidRequestError), response.Body.String())
				assert.Zero(t, harness.resolver.callCount())
				assert.Zero(t, harness.model.callCount())
			})
		}
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
	assert.Nil(t, options.Headers, "an empty header map maps to no headers")
	assert.Equal(t, provider.ProviderOptions{
		"p": provider.RawProviderOption{Key: "p", Raw: json.RawMessage(`{}`)},
	}, options.ProviderOptions, "a present namespace survives even when empty")
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

func TestRuntimeRequestRejections(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []byte
	}{
		{name: "capability/custom content", body: `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"p.x"}]}]}`, want: unsupportedCustomContentError},
		{name: "capability/tool approval", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":false}]}]}`, want: unsupportedToolApprovalsError},
		{name: "capability/denied tool output", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"execution-denied","reason":""}}]}]}`, want: unsupportedToolsError},
		{name: "capability/custom tool output", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"custom"}]}}]}]}`, want: unsupportedToolsError},
		{name: "capability/tool output options", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"text","value":"","providerOptions":{"p":{"value":true}}}}]}]}`, want: unsupportedProviderOptionsError},
		{name: "capability/nested text output options", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"text","text":"","providerOptions":{"p":{"value":true}}}]}}]}]}`, want: unsupportedProviderOptionsError},
		{name: "capability/structured output", body: `{"prompt":[],"responseFormat":{"type":"json"}}`, want: unsupportedStructuredOutputError},
		{name: "capability/raw output", body: `{"prompt":[],"includeRawChunks":true}`, want: unsupportedRawOutputError},
		{name: "policy/reserved provider namespace", body: `{"prompt":[],"providerOptions":{"grafana":{"enabled":true}}}`, want: reservedProviderOptionsError},
		{name: "policy/protected call header", body: `{"prompt":[],"headers":{"Authorization":"Bearer caller"}}`, want: protectedCallHeaderError},
		{name: "policy/protected call header case", body: `{"prompt":[],"headers":{"X-ACCESS-TOKEN":"caller"}}`, want: protectedCallHeaderError},
	}
	requestSchema := compileWireSchema(t, requestSchemaJSON)
	errorSchema := compileWireSchema(t, errorSchemaJSON)
	for _, tc := range tests {
		for _, streaming := range []string{"false", "true"} {
			for _, choice := range []string{"", `"toolChoice":{"type":"auto"},`} {
				t.Run(tc.name+"/"+streaming+"/"+choice, func(t *testing.T) {
					body := "{" + choice + tc.body[1:]
					require.NoError(t, requestSchema.Validate([]byte(body)))
					harness := newRuntimeHarness(t, testLimits())
					request := validRequest(body)
					request.Header.Set(HeaderStreaming, streaming)
					response := harness.serve(request)
					assert.Equal(t, http.StatusBadRequest, response.Code)
					assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
					assert.Equal(t, string(tc.want), response.Body.String())
					require.NoError(t, errorSchema.Validate(response.Body.Bytes()))
					assert.Zero(t, harness.resolver.callCount())
					assert.Zero(t, harness.model.callCount())
				})
			}
		}
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
	data, err := os.ReadFile(filepath.Join("../../test/providerwire-v4/goldens", name))
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
		modelCalls int
	}{
		{file: "streaming.json", status: http.StatusOK, modelCalls: 1},
		{file: "sequence.json", status: http.StatusOK, modelCalls: 1},
		{file: "sequence.json", index: 1, status: http.StatusOK, modelCalls: 1},
		{file: "scalar-presence.json", status: http.StatusOK, modelCalls: 1},
		{file: "headers.json", status: http.StatusOK, modelCalls: 1},
		{file: "headers.json", index: 1, status: http.StatusOK, modelCalls: 1},
		{file: "comprehensive-unions.json", status: http.StatusBadRequest},
		{file: "provider-tools.json", status: http.StatusOK, modelCalls: 1},
		{file: "provider-tools.json", index: 1, status: http.StatusOK, modelCalls: 1},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%d", tc.file, tc.index), func(t *testing.T) {
			records := loadGolden(t, tc.file)
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(requestFromGolden(t, records[tc.index]))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.modelCalls, harness.model.callCount())
		})
	}
}

func TestRuntimeFileValidationBeforeInvocation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []byte
	}{
		{name: "missing media type", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":""}}]}]}`},
		{name: "null filename", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain","filename":null}]}]}`},
		{name: "mixed data arms", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"","url":""},"mediaType":"text/plain"}]}]}`},
		{name: "reference reserved type", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"type":"file"}},"mediaType":"application/pdf"}]}]}`},
		{name: "reference nonstring value", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"reference","reference":{"provider":1}},"mediaType":"application/pdf"}]}]}`},
		{name: "tool role ordinary file", body: `{"prompt":[{"role":"tool","content":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}]}]}`},
		{name: "reasoning file text arm", body: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"text","text":""},"mediaType":"text/plain"}]}]}`},
		{name: "reserved message option", body: `{"prompt":[{"role":"user","content":[],"providerOptions":{"gateway":{}}}]}`, want: reservedProviderOptionsError},
		{name: "reserved file option", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":""},"mediaType":"image/png","providerOptions":{"grafana":{}}}]}]}`, want: reservedProviderOptionsError},
		{name: "reserved result file option", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain","providerOptions":{"grafana-ai-sdk":{}}}]}}]}]}`, want: reservedProviderOptionsError},
		{name: "deferred tool output options", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-result","toolCallId":"call","toolName":"tool","output":{"type":"content","value":[{"type":"file","data":{"type":"text","text":""},"mediaType":"text/plain"}],"providerOptions":{"provider":{"private":true}}}}]}]}`, want: canonicalInvalidRequestError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			want := tc.want
			if want == nil {
				want = canonicalInvalidRequestError
			}
			assert.Equal(t, string(want), response.Body.String())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
		})
	}
}

func TestRuntimeFileRequestByteBoundary(t *testing.T) {
	body := `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"data","data":"AAEC"},"mediaType":"application/pdf","filename":"escaped\\u0000name"}]}]}`
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{name: "below", body: body, want: http.StatusOK},
		{name: "at", body: body + " ", want: http.StatusOK},
		{name: "above", body: body + "  ", want: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := testLimits()
			limits.RequestBytes = int64(len(body) + 1)
			harness := newRuntimeHarness(t, limits)
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, tc.want, response.Code)
			if tc.want == http.StatusBadRequest {
				assert.Zero(t, harness.resolver.callCount())
				assert.Zero(t, harness.model.callCount())
			} else {
				assert.Equal(t, 1, harness.model.callCount())
			}
		})
	}
}

func TestRuntimeFileURLIsNotFetched(t *testing.T) {
	var fetched int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { fetched++ }))
	defer server.Close()
	body := fmt.Sprintf(`{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"url","url":%q},"mediaType":"application/pdf"}]}]}`, server.URL+"/private-file")
	for _, streaming := range []string{"false", "true"} {
		t.Run(streaming, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			request := validRequest(body)
			request.Header.Set(HeaderStreaming, streaming)
			response := harness.serve(request)
			require.Equal(t, http.StatusOK, response.Code)
			assert.Zero(t, fetched)
			assert.Equal(t, server.URL+"/private-file", harness.model.receivedOptions().Prompt[0].Content[0].Data.URL)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			cancelled := newRuntimeHarness(t, testLimits())
			request = validRequest(body).WithContext(ctx)
			request.Header.Set(HeaderStreaming, streaming)
			response = cancelled.serve(request)
			assert.Equal(t, 499, response.Code)
			assert.Zero(t, cancelled.model.callCount())
			assert.Zero(t, fetched)
		})
	}
}

func TestRuntimeFileGoldenReplay(t *testing.T) {
	records := loadGolden(t, "file-inputs.json")
	require.Len(t, records, 2)
	for _, record := range records {
		t.Run(record.Headers[HeaderStreaming], func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(requestFromGolden(t, record))
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, "grafana/files", harness.resolver.requestedModelID())
			assert.Equal(t, 1, harness.resolver.callCount())
			assert.Equal(t, 1, harness.model.callCount())
			generate, stream := harness.model.invocationCounts()
			if record.Headers[HeaderStreaming] == "true" {
				assert.Equal(t, 0, generate)
				assert.Equal(t, 1, stream)
			} else {
				assert.Equal(t, 1, generate)
				assert.Equal(t, 0, stream)
			}

			prompt := harness.model.receivedOptions().Prompt
			require.Len(t, prompt, 3)
			assert.Equal(t, []provider.Role{provider.RoleUser, provider.RoleAssistant, provider.RoleTool}, []provider.Role{prompt[0].Role, prompt[1].Role, prompt[2].Role})
			assertFileOptions(t, prompt[0].ProviderOptions)
			assertFileOptions(t, prompt[0].Content[0].ProviderOptions)
			assertFileOptions(t, prompt[0].Content[4].ProviderOptions)
			assertFileOptions(t, prompt[2].Content[0].Output.Content[0].ProviderOptions)
			assert.Equal(t, provider.ProviderOptions{"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{}`)}}, prompt[1].ProviderOptions)

			userFiles := prompt[0].Content
			require.Len(t, userFiles, 5)
			for i, mediaType := range []string{"application/octet-stream", "application/pdf", "image/png", "application/pdf", "text/plain"} {
				assert.Equal(t, mediaType, userFiles[i].MediaType)
			}
			assertFileArm(t, userFiles[0].Data, `{"type":"data","data":"AAEC"}`)
			assertFileArm(t, userFiles[1].Data, `{"type":"data","data":""}`)
			assertFileArm(t, userFiles[2].Data, `{"type":"url","url":"https://example.test/file"}`)
			assertFileArm(t, userFiles[3].Data, `{"type":"reference","reference":{"provider":"file-1"}}`)
			assertFileArm(t, userFiles[4].Data, `{"type":"text","text":""}`)
			assertFilenamePresence(t, userFiles[0].Filename, "bytes.bin", true)
			assertFilenamePresence(t, userFiles[1].Filename, "", true)
			assertFilenamePresence(t, userFiles[2].Filename, "", false)
			assertFilenamePresence(t, userFiles[4].Filename, "", true)

			assistantFiles := prompt[1].Content
			require.Len(t, assistantFiles, 5)
			for i, mediaType := range []string{"application/pdf", "image/png", "application/pdf", "text/plain"} {
				assert.Equal(t, mediaType, assistantFiles[i].MediaType)
			}
			assertFileArm(t, assistantFiles[0].Data, `{"type":"data","data":"YWxyZWFkeS1iYXNlNjQ="}`)
			assertFileArm(t, assistantFiles[1].Data, `{"type":"url","url":"https://example.test/assistant"}`)
			assertFileArm(t, assistantFiles[2].Data, `{"type":"reference","reference":{"provider":"file-2"}}`)
			assertFileArm(t, assistantFiles[3].Data, `{"type":"text","text":"assistant text"}`)
			assertFilenamePresence(t, assistantFiles[0].Filename, "", false)
			assertFilenamePresence(t, assistantFiles[1].Filename, "", true)
			assert.Equal(t, provider.ContentPartTypeToolCall, assistantFiles[4].Type)

			require.Len(t, prompt[2].Content, 1)
			output := prompt[2].Content[0].Output
			require.NotNil(t, output)
			require.Len(t, output.Content, 5)
			for i, mediaType := range []string{"application/octet-stream", "application/pdf", "image/png", "application/pdf", "text/plain"} {
				assert.Equal(t, mediaType, output.Content[i].MediaType)
			}
			for i, expected := range []string{
				`{"type":"data","data":"BQY="}`,
				`{"type":"data","data":""}`,
				`{"type":"url","url":"https://example.test/result"}`,
				`{"type":"reference","reference":{"provider":"file-3"}}`,
				`{"type":"text","text":""}`,
			} {
				assertFileArm(t, output.Content[i].Data, expected)
			}
			assertFilenamePresence(t, output.Content[0].Filename, "result.bin", true)
			assertFilenamePresence(t, output.Content[1].Filename, "", true)
			assertFilenamePresence(t, output.Content[2].Filename, "", false)
			assertFilenamePresence(t, output.Content[4].Filename, "", true)
		})
	}
}

func TestRuntimeProviderToolDefinitions(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%t", streaming), func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			body := `{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{}},{"type":"provider","id":"anthropic.code_execution_20260120","name":"code","args":{}},{"type":"provider","id":"provider.search","name":"search","args":{"limit":0,"nested":{"value":null}}}],"providerOptions":{"anthropic":{"thinking":{"type":"enabled"}}}}`
			request := validRequest(body)
			if streaming {
				request.Header.Set(HeaderStreaming, "true")
			}
			response := harness.serve(request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Equal(t, 1, harness.model.callCount())
			opts := harness.model.receivedOptions()
			require.Len(t, opts.Tools, 3)
			assert.Equal(t, provider.ToolTypeFunction, opts.Tools[0].Type)
			assert.Equal(t, provider.ToolTypeProvider, opts.Tools[1].Type)
			assert.Equal(t, "anthropic.code_execution_20260120", opts.Tools[1].ID)
			assert.NotNil(t, opts.Tools[1].Args)
			assert.JSONEq(t, `0`, string(opts.Tools[2].Args["limit"]))
			assert.JSONEq(t, `{"value":null}`, string(opts.Tools[2].Args["nested"]))
			root, ok := opts.ProviderOptions["anthropic"].(provider.RawProviderOption)
			require.True(t, ok)
			assert.JSONEq(t, `{"thinking":{"type":"enabled"}}`, string(root.Raw))
			assert.NotContains(t, response.Body.String(), "private-token")
		})
	}
	for _, tc := range []struct{ name, body string }{
		{"missing args", `{"prompt":[],"tools":[{"type":"provider","id":"provider.search","name":"search"}]}`},
		{"null args", `{"prompt":[],"tools":[{"type":"provider","id":"provider.search","name":"search","args":null}]}`},
		{"function-only field", `{"prompt":[],"tools":[{"type":"provider","id":"provider.search","name":"search","args":{},"strict":false}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, streaming := range []bool{false, true} {
				harness := newRuntimeHarness(t, testLimits())
				request := validRequest(tc.body)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}
				response := harness.serve(request)
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Zero(t, harness.model.callCount())
				assert.NotContains(t, response.Body.String(), "secret")
			}
		})
	}
}

func assertFileArm(t *testing.T, data *provider.DataContent, expected string) {
	t.Helper()
	require.NotNil(t, data)
	encoded, err := json.Marshal(data)
	require.NoError(t, err)
	assert.JSONEq(t, expected, string(encoded))
}

func assertFilenamePresence(t *testing.T, value *string, expected string, present bool) {
	t.Helper()
	if !present {
		assert.Nil(t, value)
		return
	}
	require.NotNil(t, value)
	assert.Equal(t, expected, *value)
}

func assertFileOptions(t *testing.T, options provider.ProviderOptions) {
	t.Helper()
	require.NotNil(t, options)
	value, ok := options["provider"].(provider.RawProviderOption)
	require.True(t, ok)
	assert.JSONEq(t, `{"nested":{"nullValue":null,"falseValue":false,"zero":0,"empty":""},"array":[null,false,0,"",[],{}]}`, string(value.Raw))
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

func TestPublicError_OptionalEncoding(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, attribution := range []string{"none", "selected", "malformed"} {
			t.Run(map[bool]string{false: "HTTP", true: "SSE"}[streaming]+"/"+attribution, func(t *testing.T) {
				h := invocationHarness(t, testLimits(), &recordingModel{})
				request := newExecutionRequest("alias", selectConfiguredModel(t, h.resolver.resolved), nil, provider.CallOptions{}, h.handler.limits.UnaryResponseBytes)
				capture := &attemptCapture{}
				if attribution != "none" {
					capture.enter()
				}
				view := request.snapshot(capture, nil)
				if attribution == "malformed" {
					view.overview.Attempts[0].Error = &execution.Failure{Code: json.RawMessage("invalid")}
				}
				value := safeError{category: safeOverload}
				current := view.current(provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "current failure"}))
				metadata := view.metadata()
				encode := func(limit int64) []byte {
					if streaming {
						frame, ok := encodeStreamError(value, metadata, current, limit)
						require.True(t, ok)
						return frame
					}
					status, body := encodeHTTPError(value, metadata, limit)
					assert.Equal(t, 503, status)
					return body
				}
				original := canonicalOverloadError
				if streaming {
					original = canonicalOverloadStreamErrorFrame
				}
				full := encode(h.handler.limits.UnaryResponseBytes)
				for _, limit := range []int{len(full), max(len(original), len(full)-1), len(original)} {
					body := encode(int64(limit))
					assert.LessOrEqual(t, len(body), limit)
					decode := func(body []byte) errorResponse {
						if streaming {
							body = body[len("data: ") : len(body)-len("\n\n")]
						}
						var response errorResponse
						require.NoError(t, json.Unmarshal(body, &response))
						return response
					}
					decoded, baseline := decode(body), decode(original)
					assert.Equal(t, baseline.Error.Message, decoded.Error.Message)
					assert.Equal(t, baseline.Error.Type, decoded.Error.Type)
					assert.Equal(t, baseline.Error.Code, decoded.Error.Code)
					assert.Equal(t, baseline.Error.StatusCode, decoded.Error.StatusCode)
					assert.Equal(t, baseline.Error.Retryable, decoded.Error.Retryable)
					if limit == len(full) {
						assert.Equal(t, full, body)
						if streaming {
							require.NotNil(t, decoded.Error.Data)
							assert.Equal(t, "current failure", decoded.Error.Data.NativeError.Message)
							if attribution == "selected" {
								assert.NotEmpty(t, decoded.Error.Data.Metadata)
							}
						} else if attribution == "selected" {
							assert.NotEmpty(t, decoded.Metadata)
						} else {
							assert.Equal(t, original, body)
						}
					}
					if limit == len(original) {
						assert.Equal(t, original, body)
					}
				}
				assert.Equal(t, metadata, view.metadata())
			})
		}
	}
}

func TestPublicError_FixedBytes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		value  safeError
		status int
		body   []byte
		frame  []byte
	}{
		{"invalid request", safeError{category: safeInvalidRequest}, 400, canonicalInvalidRequestError, canonicalInternalStreamErrorFrame},
		{"model not found", safeError{category: safeModelNotFound}, 404, canonicalModelNotFoundError, canonicalInternalStreamErrorFrame},
		{"rate limit", safeError{category: safeRateLimit}, 429, canonicalRateLimitError, canonicalRateLimitStreamErrorFrame},
		{"overload", safeError{category: safeOverload}, 503, canonicalOverloadError, canonicalOverloadStreamErrorFrame},
		{"dependency", safeError{category: safeFailedDependency}, 424, canonicalDependencyError, canonicalDependencyStreamErrorFrame},
		{"upstream", safeError{category: safeUpstream}, 502, canonicalUpstreamError, canonicalUpstreamStreamErrorFrame},
		{"timeout", safeError{category: safeTimeout}, 504, canonicalTimeoutError, canonicalTimeoutStreamErrorFrame},
		{"canceled", safeError{category: safeCancellation}, 499, canonicalCancellationError, canonicalCancellationStreamErrorFrame},
		{"internal", safeError{category: safeInternal}, 500, canonicalInternalError, canonicalInternalStreamErrorFrame},
		{"authentication", safeError{category: safeAuthentication}, 401, canonicalAuthenticationError, canonicalInternalStreamErrorFrame},
		{"permission", safeError{category: safePermission}, 403, canonicalPermissionError, canonicalInternalStreamErrorFrame},
		{"zero", safeError{}, 500, canonicalInternalError, canonicalInternalStreamErrorFrame},
		{"unknown", safeError{category: 255}, 500, canonicalInternalError, canonicalInternalStreamErrorFrame},
		{"custom", safeError{category: safeInvalidRequest, reason: capabilityCustomContent}, 400, unsupportedCustomContentError, canonicalInternalStreamErrorFrame},
		{"tools", safeError{category: safeInvalidRequest, reason: capabilityTools}, 400, unsupportedToolsError, canonicalInternalStreamErrorFrame},
		{"approval", safeError{category: safeInvalidRequest, reason: capabilityToolApprovals}, 400, unsupportedToolApprovalsError, canonicalInternalStreamErrorFrame},
		{"output", safeError{category: safeInvalidRequest, reason: capabilityStructuredOutput}, 400, unsupportedStructuredOutputError, canonicalInternalStreamErrorFrame},
		{"raw", safeError{category: safeInvalidRequest, reason: capabilityRawOutput}, 400, unsupportedRawOutputError, canonicalInternalStreamErrorFrame},
		{"options", safeError{category: safeInvalidRequest, reason: capabilityProviderOptions}, 400, unsupportedProviderOptionsError, canonicalInternalStreamErrorFrame},
		{"reserved", safeError{category: safeInvalidRequest, reason: policyReservedProviderOptions}, 400, reservedProviderOptionsError, canonicalInternalStreamErrorFrame},
		{"header", safeError{category: safeInvalidRequest, reason: policyProtectedCallHeader}, 400, protectedCallHeaderError, canonicalInternalStreamErrorFrame},
		{"unknown request reason", safeError{category: safeInvalidRequest, reason: requestFailureReason("unknown")}, 400, canonicalInvalidRequestError, canonicalInternalStreamErrorFrame},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHandler(t, testLimits())
			response := httptest.NewRecorder()
			h.writeSafeError(response, tc.value, nil)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.Bytes())
			assert.Equal(t, tc.frame, streamErrorFrameForSafeError(tc.value))
		})
	}
}

func TestPublicError_InvalidOptionalFields(t *testing.T) {
	value := safeError{category: safeOverload}
	validMetadata := provider.ProviderMetadata{"gateway": json.RawMessage(`{"execution":{"requestedModelId":"alias"}}`)}
	malformedMetadata := provider.ProviderMetadata{"gateway": json.RawMessage("invalid")}
	current := &execution.Failure{Message: "current failure", StatusCode: 503}
	currentOnly, ok := encodeStreamError(value, nil, current, 1<<20)
	require.True(t, ok)
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
		current  *execution.Failure
		expected []byte
	}{
		{"malformed metadata", malformedMetadata, nil, canonicalOverloadStreamErrorFrame},
		{"current survives malformed metadata", malformedMetadata, current, currentOnly},
		{"malformed current does not publish overview", validMetadata, &execution.Failure{Code: json.RawMessage("invalid")}, canonicalOverloadStreamErrorFrame},
		{"oversized current does not publish overview", validMetadata, &execution.Failure{Message: strings.Repeat("x", 1<<20)}, canonicalOverloadStreamErrorFrame},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frame, ok := encodeStreamError(value, tc.metadata, tc.current, 1024)
			require.True(t, ok)
			assert.Equal(t, tc.expected, frame)
			requireStreamBodyMatchesSchema(t, string(frame))
		})
	}
	status, body := encodeHTTPError(value, malformedMetadata, 1024)
	assert.Equal(t, 503, status)
	assert.Equal(t, canonicalOverloadError, body)
	_, ok = encodeStreamError(value, validMetadata, current, int64(len(canonicalOverloadStreamErrorFrame)-1))
	assert.False(t, ok)
}

func TestPublicError_ClassificationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		unary  safeErrorCategory
		stream safeErrorCategory
	}{
		{"wrapped cancellation", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Cause: context.Canceled}), safeCancellation, safeFailedDependency},
		{"wrapped timeout", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Cause: context.DeadlineExceeded}), safeTimeout, safeFailedDependency},
		{"invalid status", provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 700}), safeUpstream, safeInternal},
		{"zero status", provider.NewAPICallError(provider.APICallErrorOptions{}), safeUpstream, safeUpstream},
		{"zero canceled status", provider.NewAPICallError(provider.APICallErrorOptions{Cause: context.Canceled}), safeCancellation, safeCancellation},
		{"nil API error", (*provider.APICallError)(nil), safeInternal, safeInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.unary, safeErrorFromProvider(tc.err).category)
			assert.Equal(t, tc.stream, safeErrorFromStreamProvider(tc.err).category)
		})
	}
}

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
			{value: safeError{category: safeInvalidRequest, reason: capabilityCustomContent}, status: http.StatusBadRequest},
		} {
			h := newTestHandler(t, testLimits())
			response := httptest.NewRecorder()
			h.writeSafeError(response, tc.value, nil)
			assert.Equal(t, tc.status, response.Code)
			require.NoError(t, compileWireSchema(t, errorSchemaJSON).Validate(response.Body.Bytes()))
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

func TestProviderMetadata_RuntimeFailureAndFinishAuthority(t *testing.T) {
	for _, metadata := range []provider.ProviderMetadata{{"future": json.RawMessage(`null`)}, {"future": json.RawMessage(`{"incomplete":`)}, {"future": json.RawMessage(strings.Repeat(" ", 1<<20) + `{}`)}} {
		h := newRuntimeHarness(t, testLimits())
		h.model.generate = func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			result := validGenerateResult()
			result.ProviderMetadata = metadata
			return result, nil
		}
		response := h.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "future")
		h.model.stream = func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartTextStart, ID: "text", ProviderMetadata: metadata}, finishPart())}, nil
		}
		response = h.serve(streamRequest(`{"prompt":[]}`))
		assert.NotContains(t, response.Body.String(), `"type":"text-start"`)
		assert.Equal(t, 1, strings.Count(response.Body.String(), `"type":"error"`))
		assert.NotContains(t, response.Body.String(), "future")
	}
	h := newRuntimeHarness(t, testLimits())
	finish := finishPart()
	finish.ProviderMetadata = provider.ProviderMetadata{"future": json.RawMessage(`{"final":true}`)}
	h.model.stream = func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(finish, provider.StreamPart{Type: provider.PartTextStart, ID: "late", ProviderMetadata: provider.ProviderMetadata{"future": json.RawMessage(`null`)}})}, nil
	}
	body := h.serve(streamRequest(`{"prompt":[]}`)).Body.String()
	assert.Contains(t, body, `"future":{"final":true}`)
	assert.NotContains(t, body, `"type":"error"`)
	assert.NotContains(t, body, "late")
}

func TestProviderMetadata_SharedModelIsolation(t *testing.T) {
	h := newRuntimeHarness(t, testLimits())
	h.model.generate = func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		marker := options.Prompt[0].Content[0].Text
		raw, err := json.Marshal(map[string]any{"tenantMarker": marker, "routing": "ignored", "apiKey": "sk-application-data"})
		if err != nil {
			return nil, err
		}
		result := validGenerateResult()
		result.ProviderMetadata = provider.ProviderMetadata{"future": raw}
		result.Content[0].ProviderMetadata = result.ProviderMetadata
		return result, nil
	}
	var wg sync.WaitGroup
	const requests = 20
	for i := range requests {
		wg.Go(func() {
			marker := fmt.Sprintf("tenant-%d", i)
			w := httptest.NewRecorder()
			h.handler.ServeHTTP(w, validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"`+marker+`"}]}]}`))
			assert.Equal(t, 200, w.Code)
			var decoded struct {
				Metadata provider.ProviderMetadata `json:"providerMetadata"`
			}
			if assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded)) {
				raw := string(decoded.Metadata["future"])
				assert.Contains(t, raw, `"tenantMarker":"`+marker+`"`)
				assert.Contains(t, raw, `"routing":"ignored"`)
				assert.Contains(t, raw, "sk-application-data")
			}
		})
	}
	wg.Wait()
	assert.Equal(t, requests, h.resolver.callCount())
}

func invocationHarness(t *testing.T, limits Limits, models ...*recordingModel) *runtimeHarness {
	t.Helper()
	h := newRuntimeHarness(t, limits)
	candidates := make([]provider.LanguageModel, len(models))
	for i, model := range models {
		candidates[i] = model
		h.resolver.resolved.Candidates = append(h.resolver.resolved.Candidates, catalog.ConfiguredCandidate{Provider: "native", ProviderInstance: "configured", ModelID: string(rune('A' + i))})
	}
	h.resolver.resolved.Model = candidates[0]
	if len(candidates) > 1 {
		ordered, err := fallback.New(candidates...)
		require.NoError(t, err)
		ordered.WithAttemptObserver(func(context.Context, fallback.Attempt) { panic("operator panic") })
		h.resolver.resolved.Model = ordered
	}
	return h
}

func TestInvocation_Unary(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		primary, secondary       int
		wantStatus, wantAttempts int
	}{
		{name: "direct", wantStatus: 200, wantAttempts: 1},
		{name: "secondary", primary: 503, wantStatus: 200, wantAttempts: 2},
		{name: "noneligible", primary: 401, wantStatus: 424, wantAttempts: 1},
		{name: "exhausted", primary: 503, secondary: 400, wantStatus: 424, wantAttempts: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				if tc.primary != 0 {
					return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "first failure", StatusCode: tc.primary})
				}
				return validGenerateResult(), nil
			}}
			models := []*recordingModel{first}
			if tc.name != "direct" {
				models = append(models, &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if tc.secondary != 0 {
						return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "second failure", StatusCode: tc.secondary})
					}
					return validGenerateResult(), nil
				}})
			}
			h := invocationHarness(t, testLimits(), models...)
			response := h.serve(validRequest(`{"prompt":[]}`))
			require.Equal(t, tc.wantStatus, response.Code)
			overview := readOverview(t, response.Body.Bytes())
			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, tc.wantAttempts)
			assert.Equal(t, "A", overview.Attempts[0].ModelID)
			if tc.primary != 0 {
				require.NotNil(t, overview.Attempts[0].Error)
				assert.Equal(t, "first failure", overview.Attempts[0].Error.Message)
			}
			assert.Equal(t, 1, first.callCount())
			if len(models) == 2 {
				assert.Equal(t, tc.wantAttempts-1, models[1].callCount())
			}
		})
	}
}

func TestInvocation_PrivateSourcesAndErrorLimit(t *testing.T) {
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "native rejected actual-source", StatusCode: 401})
	}}
	h := invocationHarness(t, testLimits(), first)
	h.resolver.resolved.ProtectedSources = []string{"actual-source"}
	response := h.serve(validRequest(`{"prompt":[]}`))
	assert.Equal(t, 424, response.Code)
	assert.NotContains(t, response.Body.String(), "actual-source")
	overview := readOverview(t, response.Body.Bytes())
	require.NotNil(t, overview)
	assert.Equal(t, 401, overview.Attempts[0].Error.StatusCode)
	first.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: strings.Repeat("x", maxErrorResponseBytes), StatusCode: 401})
	}
	response = h.serve(validRequest(`{"prompt":[]}`))
	assert.Equal(t, 424, response.Code)
	assert.Equal(t, canonicalDependencyError, response.Body.Bytes())
}

func TestInvocation_RequestCredentialEcho(t *testing.T) {
	for _, tc := range []struct {
		name, request string
		secrets       []string
	}{
		{"anthropic MCP", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"authorizationToken":"remote-credential"}]}}}`, []string{"remote-credential"}},
		{"OpenAI MCP authorization", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"authorization":"remote-credential"}}]}`, []string{"remote-credential"}},
		{"OpenAI MCP headers", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"headers":{"authorization":"Bearer remote-credential"}}}]}`, []string{"remote-credential"}},
		{"body header", `{"prompt":[],"headers":{"x-goog-api-key":"remote-credential"}}`, []string{"remote-credential"}},
		{"Anthropic MCP query", `{"prompt":[],"providerOptions":{"anthropic":{"mcpServers":[{"type":"url","name":"echo","url":"https://mcp.example/tools?api_key=remote-credential"}]}}}`, []string{"remote-credential"}},
		{"OpenAI MCP query", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"serverUrl":"https://mcp.example/tools?api_key=remote-credential"}}]}`, []string{"remote-credential"}},
		{"OpenAI MCP userinfo", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"serverUrl":"https://user:remote-credential@mcp.example/tools"}}]}`, []string{"remote-credential"}},
		{"OpenAI API key header", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"headers":{"openai-api-key":"remote-credential"}}}]}`, []string{"remote-credential"}},
		{"Anthropic API key header", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"headers":{"anthropic-api-key":"remote-credential"}}}]}`, []string{"remote-credential"}},
		{"case distinct headers", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"headers":{"Authorization":"Bearer first-secret","authorization":"Bearer second-secret"}}}]}`, []string{"first-secret", "second-secret"}},
		{"nonstring sibling header", `{"prompt":[],"tools":[{"type":"provider","id":"openai.mcp","name":"mcp","args":{"headers":{"authorization":"Bearer remote-credential","x-note":7}}}]}`, []string{"remote-credential"}},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(tc.name+"/"+map[bool]string{false: "unary", true: "committed"}[streaming], func(t *testing.T) {
				native := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Data: json.RawMessage(`{"message":"provider echoed remote-credential first-secret","type":"second-secret"}`)})
				first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, native }, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartError, APICallError: native}, finishPart())}, nil
				}}
				request := validRequest(tc.request)
				status := 424
				if streaming {
					request = streamRequest(tc.request)
					status = 200
				}
				response := invocationHarness(t, testLimits(), first).serve(request)
				assert.Equal(t, status, response.Code)
				for _, secret := range tc.secrets {
					assert.NotContains(t, response.Body.String(), secret)
				}
				if streaming {
					assert.Contains(t, response.Body.String(), `"nativeError"`)
					assert.Contains(t, response.Body.String(), `"statusCode":401`)
					assert.Contains(t, response.Body.String(), `"type":"finish"`)
				} else {
					overview := readOverview(t, response.Body.Bytes())
					require.NotNil(t, overview)
					assert.Equal(t, 401, overview.Attempts[0].Error.StatusCode)
				}
			})
		}
	}
}

func TestInvocation_SharedModelIsolation(t *testing.T) {
	first := &recordingModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "failed-" + options.Headers["request-id"]})
	}}
	second := &recordingModel{generate: func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		result := validGenerateResult()
		result.ProviderMetadata = provider.ProviderMetadata{"native": json.RawMessage(`{"id":"` + options.Headers["request-id"] + `"}`)}
		return result, nil
	}}
	h := invocationHarness(t, testLimits(), first, second)
	var group sync.WaitGroup
	for i := range 24 {
		group.Go(func() {
			id := string(rune('A' + i))
			request := validRequest(`{"prompt":[],"headers":{"request-id":"` + id + `"}}`)
			response := h.serve(request)
			require.Equal(t, 200, response.Code)
			overview := readOverview(t, response.Body.Bytes())
			require.NotNil(t, overview)
			require.Len(t, overview.Attempts, 2)
			assert.Equal(t, "failed-"+id, overview.Attempts[0].Error.Message)
			assert.Equal(t, fallback.AttemptFailed, overview.Attempts[0].Outcome)
			assert.Equal(t, fallback.AttemptSelected, overview.Attempts[1].Outcome)
			assert.Contains(t, response.Body.String(), `"native":{"id":"`+id+`"}`)
		})
	}
	group.Wait()
	assert.Equal(t, 24, first.callCount())
	assert.Equal(t, 24, second.callCount())
}

func TestInvocation_CanceledLateFallbackObservation(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	observed := make(chan struct{})
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		close(entered)
		<-release
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "unowned late native failure", StatusCode: 401})
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	switch model := h.resolver.resolved.Model.(type) {
	case *fallback.Model:
		model.WithAttemptObserver(func(context.Context, fallback.Attempt) { close(observed) })
	default:
		require.FailNow(t, "expected configured fallback")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	responses := make(chan string, 1)
	go func() { responses <- h.serve(validRequest(`{"prompt":[]}`).WithContext(ctx)).Body.String() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "candidate did not enter")
	}
	cancel()
	select {
	case body := <-responses:
		assert.NotContains(t, body, "unowned late native failure")
	case <-time.After(time.Second):
		require.FailNow(t, "cancellation did not return")
	}
	close(release)
	select {
	case <-observed:
	case <-time.After(time.Second):
		require.FailNow(t, "fallback observation did not arrive")
	}
	assert.Zero(t, second.callCount())
}

func TestInvocation_RejectedUnaryOutcomeBeforePublication(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	observations := make(chan fallback.Attempt, 1)
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		close(entered)
		<-release
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "handler-rejected native failure"})
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	switch model := h.resolver.resolved.Model.(type) {
	case *fallback.Model:
		model.WithAttemptObserver(func(_ context.Context, attempt fallback.Attempt) { observations <- attempt })
	default:
		require.FailNow(t, "expected configured fallback")
	}
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := newExecutionRequest("alias", selectConfiguredModel(t, h.resolver.resolved), nil, provider.CallOptions{}, h.handler.limits.UnaryResponseBytes)
	capture := &attemptCapture{}
	ctx := fallback.WithAttemptObserver(parent, capture.observe)
	results := make(chan error, 1)
	go func() {
		_, err := h.handler.invokeModel(ctx, h.resolver.resolved.Model, provider.CallOptions{}, capture)
		results <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "candidate did not enter")
	}
	cancel()
	var err error
	select {
	case err = <-results:
	case <-time.After(time.Second):
		require.FailNow(t, "cancellation did not return")
	}
	require.ErrorIs(t, err, context.Canceled)
	close(release)
	var attempt fallback.Attempt
	select {
	case attempt = <-observations:
	case <-time.After(time.Second):
		require.FailNow(t, "fallback observation did not arrive")
	}
	assert.Equal(t, fallback.AttemptCanceled, attempt.Outcome)
	require.NotNil(t, attempt.SourceErr)
	assert.Contains(t, attempt.SourceErr.Error(), "handler-rejected native failure")
	view := request.snapshot(capture, err)
	assert.Nil(t, view.overview)
	capture.mu.Lock()
	defer capture.mu.Unlock()
	assert.True(t, capture.sealed)
	assert.Empty(t, capture.attempts)
	assert.Zero(t, second.callCount())
}

func TestInvocation_RejectRecordedFallbackOutcome(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := &recordingModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		cancel()
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "handler-rejected native failure"})
	}}
	second := &recordingModel{}
	h := invocationHarness(t, testLimits(), first, second)
	request := newExecutionRequest("alias", selectConfiguredModel(t, h.resolver.resolved), nil, provider.CallOptions{}, h.handler.limits.UnaryResponseBytes)
	capture := &attemptCapture{}
	ctx := fallback.WithAttemptObserver(parent, capture.observe)
	_, err := h.resolver.resolved.Model.DoGenerate(ctx, provider.CallOptions{})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, capture.attempts, 1)
	require.NotNil(t, capture.attempts[0].SourceErr)
	capture.discard()
	view := request.snapshot(capture, ctx.Err())
	assert.Nil(t, view.overview)
	assert.Empty(t, capture.attempts)
	assert.True(t, capture.sealed)
	assert.Zero(t, second.callCount())
}

func selectConfiguredModel(t *testing.T, resolved catalog.ResolvedModel) Selection {
	t.Helper()
	selected, err := CatalogSelector(&recordingResolver{resolved: resolved})(t.Context(), "alias", provider.CallOptions{}, nil)
	require.NoError(t, err)
	return selected
}

func TestRequestSelection_ConfiguredExecution(t *testing.T) {
	resolved := catalog.ResolvedModel{
		ID:               "canonical",
		Model:            &recordingModel{},
		Candidates:       []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "backend", ProviderInstance: "configured"}},
		ProtectedSources: []string{"configured-secret"},
	}
	selected := selectConfiguredModel(t, resolved)
	resolved.Candidates[0].ModelID = "mutated"
	resolved.ProtectedSources[0] = "mutated"
	request := newExecutionRequest("alias", selected, nil, provider.CallOptions{}, 4096)
	selected.configured.candidates[0].ModelID = "mutated-again"
	selected.configured.sources[0] = "mutated-again"
	assert.Equal(t, "canonical", request.canonical)
	assert.Equal(t, "backend", request.candidates[0].ModelID)
	assert.Equal(t, []string{"configured-secret"}, request.sources)
	encoded, err := json.Marshal(selected)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "configured")
	assert.NotContains(t, string(encoded), "secret")
	assert.NotContains(t, fmt.Sprintf("%+v", selected), "configured-secret")
	capture := &attemptCapture{}
	capture.enter()
	view := request.snapshot(capture, nil)
	require.NotNil(t, view.overview)
	assert.Equal(t, "alias", view.overview.RequestedModelID)
	assert.Equal(t, "canonical", view.overview.CanonicalModelID)
	assert.Equal(t, "configured", view.overview.Attempts[0].ProviderInstance)
	assert.Nil(t, view.current(provider.NewAPICallError(provider.APICallErrorOptions{Message: "configured-secret"})))
}

func TestRequestSelection_UnconfiguredExecutionOmitted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		streaming bool
		failSetup bool
	}{
		{name: "unary success"},
		{name: "unary failure", failSetup: true},
		{name: "committed error", streaming: true},
		{name: "stream setup failure", streaming: true, failSetup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			native := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "unconfigured-native-detail"})
			metadata := provider.ProviderMetadata{"gateway": json.RawMessage(`{"opaque":"native-value"}`)}
			model := &recordingModel{
				generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					if tc.failSetup {
						return nil, native
					}
					result := validGenerateResult()
					result.ProviderMetadata = metadata
					return result, nil
				},
				stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					if tc.failSetup {
						return nil, native
					}
					finish := finishPart()
					finish.ProviderMetadata = metadata
					parts := make(chan provider.StreamPart, 2)
					parts <- provider.StreamPart{Type: provider.PartError, APICallError: native}
					parts <- finish
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			ordered, err := fallback.New(model)
			require.NoError(t, err)
			h, err := New(Config{Limits: testLimits(), Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
				return Selection{ID: "request/model", Model: ordered}, nil
			}})
			require.NoError(t, err)
			var observed int
			ctx := fallback.WithAttemptObserver(t.Context(), func(context.Context, fallback.Attempt) { observed++ })
			request := validRequest(`{"prompt":[]}`).WithContext(ctx)
			request.Header.Set(HeaderStreaming, fmt.Sprint(tc.streaming))
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			assert.Equal(t, 1, observed)
			assert.Equal(t, 1, model.callCount())
			assert.NotContains(t, response.Body.String(), "unconfigured-native-detail")
			assert.NotContains(t, response.Body.String(), "execution")
			assert.NotContains(t, response.Body.String(), "nativeError")
			assert.NotContains(t, response.Body.String(), "nativeMetadata")
			if tc.failSetup {
				assert.Equal(t, http.StatusServiceUnavailable, response.Code)
			} else {
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Contains(t, response.Body.String(), `"gateway":{"opaque":"native-value"}`)
				if tc.streaming {
					assert.Contains(t, response.Body.String(), `"type":"error"`)
					assert.Contains(t, response.Body.String(), `"type":"finish"`)
				}
			}
		})
	}
}

func TestExecutionRequest_Snapshot(t *testing.T) {
	failure := errors.New("candidate failure")
	for _, tc := range []struct {
		name       string
		candidates int
		entered    bool
		attempts   []fallback.Attempt
		err        error
		discard    bool
		outcomes   []fallback.AttemptOutcome
	}{
		{name: "not entered", candidates: 1},
		{name: "direct selected", candidates: 1, entered: true, outcomes: []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{name: "direct failed", candidates: 1, entered: true, err: failure, outcomes: []fallback.AttemptOutcome{fallback.AttemptFailed}},
		{name: "rejected direct canceled", candidates: 1, entered: true, err: context.Canceled, discard: true, outcomes: []fallback.AttemptOutcome{fallback.AttemptCanceled}},
		{name: "rejected before entry", candidates: 1, discard: true},
		{name: "fallback selected", candidates: 2, entered: true, attempts: []fallback.Attempt{
			{Index: 1, Outcome: fallback.AttemptFailed, SourceErr: failure, WillFallback: true},
			{Index: 2, Outcome: fallback.AttemptSelected},
		}, outcomes: []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{name: "rejected fallback", candidates: 2, entered: true, attempts: []fallback.Attempt{
			{Index: 1, Outcome: fallback.AttemptCanceled, SourceErr: failure},
		}, err: context.Canceled, discard: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resolved := catalog.ResolvedModel{ID: "canonical", ProtectedSources: []string{"credential"}}
			for i := range tc.candidates {
				resolved.Candidates = append(resolved.Candidates, catalog.ConfiguredCandidate{Provider: "native", ModelID: fmt.Sprint(i)})
			}
			request := newExecutionRequest("alias", selectConfiguredModel(t, resolved), nil, provider.CallOptions{}, 4096)
			resolved.Candidates[0].ModelID = "mutated"
			resolved.ProtectedSources[0] = "changed"
			capture := &attemptCapture{}
			if tc.entered {
				capture.enter()
			}
			for _, attempt := range tc.attempts {
				capture.observe(t.Context(), attempt)
			}
			if tc.discard {
				capture.discard()
			}
			view := request.snapshot(capture, tc.err)
			if len(tc.outcomes) == 0 {
				assert.Nil(t, view.overview)
			} else {
				require.NotNil(t, view.overview)
				require.Len(t, view.overview.Attempts, len(tc.outcomes))
				for i, outcome := range tc.outcomes {
					assert.Equal(t, outcome, view.overview.Attempts[i].Outcome)
					assert.Equal(t, fmt.Sprint(i), view.overview.Attempts[i].ModelID)
				}
			}
			assert.Nil(t, view.current(provider.NewAPICallError(provider.APICallErrorOptions{Message: "credential"})))
			before, err := json.Marshal(view.overview)
			require.NoError(t, err)
			var late sync.WaitGroup
			for range 8 {
				late.Go(func() {
					capture.enter()
					capture.observe(t.Context(), fallback.Attempt{Index: 1, Outcome: fallback.AttemptFailed, SourceErr: errors.New("late failure")})
				})
			}
			late.Wait()
			after, err := json.Marshal(view.overview)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			entered, attempts := capture.seal()
			assert.Equal(t, tc.entered, entered)
			assert.Empty(t, attempts)
			assert.True(t, capture.sealed)
		})
	}
}

type projectionError struct {
	inspect func()
}

func (err projectionError) Error() string {
	err.inspect()
	return "candidate failure"
}

func TestExecutionRequest_ProjectionAfterSeal(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(fmt.Sprint(panics), func(t *testing.T) {
			request := newExecutionRequest("alias", selectConfiguredModel(t, catalog.ResolvedModel{ID: "canonical", Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "model"}}}), nil, provider.CallOptions{}, 4096)
			capture := &attemptCapture{}
			capture.enter()
			inspected := make(chan struct{})
			failure := projectionError{inspect: func() {
				capture.observe(t.Context(), fallback.Attempt{Index: 2, Outcome: fallback.AttemptSelected})
				close(inspected)
				if panics {
					panic("projection failed")
				}
			}}
			views := make(chan executionView, 1)
			go func() { views <- request.snapshot(capture, failure) }()
			var view executionView
			select {
			case view = <-views:
			case <-time.After(time.Second):
				require.FailNow(t, "projection held the capture lock")
			}
			select {
			case <-inspected:
			default:
				require.FailNow(t, "native error was not inspected")
			}
			if panics {
				assert.Nil(t, view.overview)
			} else {
				require.NotNil(t, view.overview)
				require.Len(t, view.overview.Attempts, 1)
				assert.Equal(t, "candidate failure", view.overview.Attempts[0].Error.Message)
			}
			_, attempts := capture.seal()
			assert.Empty(t, attempts)
		})
	}
}
