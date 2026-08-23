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
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type runtimeRecorder struct {
	mu     sync.Mutex
	events []string
}

func (r *runtimeRecorder) add(event string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func (r *runtimeRecorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type recordingPolicy struct {
	recorder   *runtimeRecorder
	err        error
	mutate     func(provider.CallOptions) provider.CallOptions
	panicApply bool

	mu      sync.Mutex
	calls   int
	options provider.CallOptions
}

func (p *recordingPolicy) Apply(_ context.Context, options provider.CallOptions) (provider.CallOptions, error) {
	if p.panicApply {
		panic("private policy panic")
	}
	p.recorder.add("policy")
	p.mu.Lock()
	p.calls++
	p.options = options
	p.mu.Unlock()
	if p.mutate != nil {
		options = p.mutate(options)
	}
	return options, p.err
}

func (p *recordingPolicy) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type recordingResolver struct {
	recorder     *runtimeRecorder
	resolved     catalog.ResolvedModel
	err          error
	panicResolve bool

	mu      sync.Mutex
	calls   int
	modelID string
}

func (r *recordingResolver) ResolveModel(_ context.Context, modelID string) (catalog.ResolvedModel, error) {
	if r.panicResolve {
		panic("private resolver panic")
	}
	r.recorder.add("resolve")
	r.mu.Lock()
	r.calls++
	r.modelID = modelID
	r.mu.Unlock()
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
	return r.modelID
}

type recordingModel struct {
	recorder           *runtimeRecorder
	specification      string
	panicSpecification bool
	generate           func(context.Context, provider.CallOptions) (*provider.GenerateResult, error)

	mu      sync.Mutex
	calls   int
	options provider.CallOptions
}

func (m *recordingModel) SpecificationVersion() string {
	if m.panicSpecification {
		panic("private specification panic")
	}
	if m.specification == "" {
		return "v4"
	}
	return m.specification
}
func (m *recordingModel) Provider() string                           { return "private-provider" }
func (m *recordingModel) ModelID() string                            { return "private-backend-model" }
func (m *recordingModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *recordingModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	return nil, errors.New("unexpected stream call")
}
func (m *recordingModel) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	m.recorder.add("model")
	m.mu.Lock()
	m.calls++
	m.options = options
	m.mu.Unlock()
	if m.generate != nil {
		return m.generate(ctx, options)
	}
	return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
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

type runtimeHarness struct {
	handler  *handler
	recorder *runtimeRecorder
	policy   *recordingPolicy
	resolver *recordingResolver
	model    *recordingModel
	mapCalls int
}

func newRuntimeHarness(t *testing.T, limits Limits) *runtimeHarness {
	t.Helper()
	recorder := &runtimeRecorder{}
	model := &recordingModel{recorder: recorder}
	policy := &recordingPolicy{recorder: recorder}
	resolver := &recordingResolver{
		recorder: recorder,
		resolved: catalog.ResolvedModel{ID: "canonical/model", Model: model},
	}
	created, err := New(Config{Resolver: resolver, Policy: policy, Limits: limits})
	require.NoError(t, err)
	h := created.(*handler)
	harness := &runtimeHarness{handler: h, recorder: recorder, policy: policy, resolver: resolver, model: model}
	mapper := h.mapRequest
	h.mapRequest = func(body []byte) (provider.CallOptions, *requestFailure) {
		harness.mapCalls++
		recorder.add("map")
		return mapper(body)
	}
	return harness
}

func (h *runtimeHarness) serve(req *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	h.handler.ServeHTTP(recorder, req)
	return recorder
}

func TestRuntimeRequestBodyBoundaryStopsDownstreamStages(t *testing.T) {
	base := `{"prompt":[]}`
	limits := testLimits()
	limits.RequestBytes = int64(len(base) + 1)
	tests := []struct {
		name       string
		body       string
		status     int
		downstream int
	}{
		{name: "below", body: base, status: http.StatusOK, downstream: 1},
		{name: "at", body: base + " ", status: http.StatusOK, downstream: 1},
		{name: "above", body: base + "  ", status: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, limits)
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.downstream, harness.mapCalls)
			assert.Equal(t, tc.downstream, harness.policy.callCount())
			assert.Equal(t, tc.downstream, harness.resolver.callCount())
			assert.Equal(t, tc.downstream, harness.model.callCount())
		})
	}
}

func TestRuntimeSchemaFailureStopsAllDownstreamStages(t *testing.T) {
	bodies := []string{
		`{"prompt":[],"unknown":true}`,
		`{"prompt":[],"maxOutputTokens":null}`,
		`{"prompt":[],"maxOutputTokens":1.5}`,
		`{"prompt":[{"role":"future","content":[]}]}`,
		`{"prompt":[],"responseFormat":{"type":"text","schema":{}}}`,
		`{"prompt":[{"role":"system","content":[]}]}`,
		`{"prompt":[],"providerOptions":{"provider":1}}`,
		`{"prompt":[],"temperature":1e309}`,
	}
	for _, body := range bodies {
		t.Run(body, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(body))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Zero(t, harness.mapCalls)
			assert.Zero(t, harness.policy.callCount())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
			assert.Empty(t, harness.recorder.snapshot())
		})
	}
}

func TestRuntimeIntegerControlLexicalSyntax(t *testing.T) {
	fields := []struct {
		name  string
		value func(provider.CallOptions) *int
	}{
		{name: "maxOutputTokens", value: func(options provider.CallOptions) *int { return options.MaxOutputTokens }},
		{name: "topK", value: func(options provider.CallOptions) *int { return options.TopK }},
		{name: "seed", value: func(options provider.CallOptions) *int { return options.Seed }},
	}
	accepted := []struct {
		lexeme string
		want   int
	}{
		{lexeme: "1", want: 1},
		{lexeme: "0", want: 0},
		{lexeme: "-1", want: -1},
	}
	rejected := []string{"1.0", "1e0", "-0.0"}

	for _, field := range fields {
		for _, value := range accepted {
			t.Run(field.name+"/accept/"+value.lexeme, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				body := fmt.Sprintf(`{"prompt":[],%q:%s}`, field.name, value.lexeme)
				response := harness.serve(validRequest(body))
				assert.Equal(t, http.StatusOK, response.Code)
				assert.Equal(t, 1, harness.mapCalls)
				assert.Equal(t, 1, harness.policy.callCount())
				assert.Equal(t, 1, harness.resolver.callCount())
				assert.Equal(t, 1, harness.model.callCount())
				mapped := field.value(harness.model.receivedOptions())
				require.NotNil(t, mapped)
				assert.Equal(t, value.want, *mapped)
			})
		}
		for _, lexeme := range rejected {
			t.Run(field.name+"/reject/"+lexeme, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				body := fmt.Sprintf(`{"prompt":[],%q:%s}`, field.name, lexeme)
				response := harness.serve(validRequest(body))
				assert.Equal(t, http.StatusBadRequest, response.Code)
				assert.Zero(t, harness.policy.callCount())
				assert.Zero(t, harness.resolver.callCount())
				assert.Zero(t, harness.model.callCount())
			})
		}
	}
}

func TestRuntimeSupportedMapping(t *testing.T) {
	t.Run("ordered text and scalar presence", func(t *testing.T) {
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
			"seed":0,
			"stopSequences":["stop",""],
			"responseFormat":{"type":"text"},
			"reasoning":"high"
		}`
		response := harness.serve(validRequest(body))
		assert.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, 1, harness.model.callCount())
		options := harness.model.receivedOptions()
		require.Len(t, options.Prompt, 4)
		assert.Equal(t, provider.RoleSystem, options.Prompt[0].Role)
		require.Len(t, options.Prompt[0].Content, 1)
		assert.Equal(t, "", options.Prompt[0].Content[0].Text)
		assert.Equal(t, "system-2", options.Prompt[1].Content[0].Text)
		require.Len(t, options.Prompt[2].Content, 2)
		assert.Equal(t, "", options.Prompt[2].Content[0].Text)
		assert.Equal(t, "user-2", options.Prompt[2].Content[1].Text)
		assert.Equal(t, "assistant", options.Prompt[3].Content[0].Text)
		require.NotNil(t, options.MaxOutputTokens)
		assert.Zero(t, *options.MaxOutputTokens)
		require.NotNil(t, options.Temperature)
		assert.Zero(t, *options.Temperature)
		require.NotNil(t, options.TopP)
		assert.Zero(t, *options.TopP)
		require.NotNil(t, options.TopK)
		assert.Zero(t, *options.TopK)
		require.NotNil(t, options.PresencePenalty)
		assert.Zero(t, *options.PresencePenalty)
		require.NotNil(t, options.FrequencyPenalty)
		assert.Zero(t, *options.FrequencyPenalty)
		require.NotNil(t, options.Seed)
		assert.Zero(t, *options.Seed)
		assert.Equal(t, []string{"stop", ""}, options.StopSequences)
		assert.Equal(t, provider.ReasoningHigh, options.Reasoning)
		assert.Nil(t, options.ResponseFormat)
		assert.Nil(t, options.Headers)
		assert.Nil(t, options.ProviderOptions)
		assert.Equal(t, []string{"map", "policy", "resolve", "model"}, harness.recorder.snapshot())
	})

	t.Run("wire reasoning values map to typed provider values", func(t *testing.T) {
		tests := []struct {
			wire string
			want provider.ReasoningEffort
		}{
			{wire: "provider-default", want: provider.ReasoningProviderDefault},
			{wire: "none", want: provider.ReasoningNone},
			{wire: "minimal", want: provider.ReasoningMinimal},
			{wire: "low", want: provider.ReasoningLow},
			{wire: "medium", want: provider.ReasoningMedium},
			{wire: "high", want: provider.ReasoningHigh},
			{wire: "xhigh", want: provider.ReasoningXHigh},
		}
		for _, tc := range tests {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(fmt.Sprintf(`{"prompt":[],"reasoning":%q}`, tc.wire)))
			assert.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, 1, harness.model.callCount())
			assert.Equal(t, tc.want, harness.model.receivedOptions().Reasoning)
		}
	})

	t.Run("absent and empty stop sequences map to none", func(t *testing.T) {
		for _, body := range []string{`{"prompt":[]}`, `{"prompt":[],"stopSequences":[]}`} {
			harness := newRuntimeHarness(t, testLimits())
			harness.serve(validRequest(body))
			assert.Nil(t, harness.model.receivedOptions().StopSequences)
		}
	})

	t.Run("empty deferred values are no-op", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		body := `{"prompt":[{"role":"system","content":"","providerOptions":{"one":{}}},{"role":"user","content":[{"type":"text","text":"","providerOptions":{"two":{}}}]}],"tools":[],"headers":{},"includeRawChunks":false,"responseFormat":{"type":"text"},"providerOptions":{"one":{},"two":{}}}`
		harness.serve(validRequest(body))
		assert.Equal(t, 1, harness.model.callCount())
	})

	t.Run("integer outside Go range fails before policy", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		response := harness.serve(validRequest(`{"prompt":[],"maxOutputTokens":9223372036854775808}`))
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Equal(t, 1, harness.mapCalls)
		assert.Zero(t, harness.policy.callCount())
		assert.Zero(t, harness.resolver.callCount())
		assert.Zero(t, harness.model.callCount())
	})

	t.Run("non-finite float is rejected by checked mapper", func(t *testing.T) {
		_, err := parseWireFloat(json.RawMessage(`1e309`))
		assert.Error(t, err)
	})
}

func TestRuntimeUnsupportedCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		capability unsupportedCapability
	}{
		{name: "files", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain"}]}]}`, capability: capabilityFiles},
		{name: "reasoning files", body: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning-file","data":{"type":"url","url":"https://example.test/r"},"mediaType":"text/plain"}]}]}`, capability: capabilityFiles},
		{name: "reasoning content", body: `{"prompt":[{"role":"assistant","content":[{"type":"reasoning","text":"x"}]}]}`, capability: capabilityReasoningContent},
		{name: "custom content", body: `{"prompt":[{"role":"assistant","content":[{"type":"custom","kind":"p.x"}]}]}`, capability: capabilityCustomContent},
		{name: "function tool", body: `{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{}}]}`, capability: capabilityTools},
		{name: "provider tool", body: `{"prompt":[],"tools":[{"type":"provider","id":"p.search","name":"search","args":{}}]}`, capability: capabilityTools},
		{name: "tool choice auto", body: `{"prompt":[],"toolChoice":{"type":"auto"}}`, capability: capabilityTools},
		{name: "tool choice none", body: `{"prompt":[],"toolChoice":{"type":"none"}}`, capability: capabilityTools},
		{name: "tool choice required", body: `{"prompt":[],"toolChoice":{"type":"required"}}`, capability: capabilityTools},
		{name: "tool choice named", body: `{"prompt":[],"toolChoice":{"type":"tool","toolName":"f"}}`, capability: capabilityTools},
		{name: "tool call content", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"c","toolName":"f","input":{}}]}]}`, capability: capabilityTools},
		{name: "tool result text", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"text","value":""}}]}]}`, capability: capabilityTools},
		{name: "tool result json", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"json","value":null}}]}]}`, capability: capabilityTools},
		{name: "tool result denied", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"execution-denied","reason":""}}]}]}`, capability: capabilityTools},
		{name: "tool result error text", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"error-text","value":""}}]}]}`, capability: capabilityTools},
		{name: "tool result error json", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"error-json","value":null}}]}]}`, capability: capabilityTools},
		{name: "tool result content union", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"content","value":[{"type":"text","text":""},{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain"},{"type":"custom"}]}}]}]}`, capability: capabilityTools},
		{name: "tool approvals", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":false}]}]}`, capability: capabilityToolApprovals},
		{name: "structured output", body: `{"prompt":[],"responseFormat":{"type":"json"}}`, capability: capabilityStructuredOutput},
		{name: "root provider options", body: `{"prompt":[],"providerOptions":{"p":{"enabled":true}}}`, capability: capabilityProviderOptions},
		{name: "message provider options", body: `{"prompt":[{"role":"system","content":"","providerOptions":{"p":{"enabled":true}}}]}`, capability: capabilityProviderOptions},
		{name: "direct part provider options", body: `{"prompt":[{"role":"user","content":[{"type":"text","text":"","providerOptions":{"p":{"enabled":true}}}]}]}`, capability: capabilityProviderOptions},
		{name: "tool result part provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"text","value":""},"providerOptions":{"p":{"enabled":true}}}]}]}`, capability: capabilityProviderOptions},
		{name: "approval part provider options", body: `{"prompt":[{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":false,"providerOptions":{"p":{"enabled":true}}}]}]}`, capability: capabilityProviderOptions},
		{name: "function tool provider options", body: `{"prompt":[],"tools":[{"type":"function","name":"f","inputSchema":{},"providerOptions":{"p":{"enabled":true}}}]}`, capability: capabilityProviderOptions},
		{name: "text output provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"text","value":"","providerOptions":{"p":{"enabled":true}}}}]}]}`, capability: capabilityProviderOptions},
		{name: "json output provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"json","value":null,"providerOptions":{"p":{"enabled":true}}}}]}]}`, capability: capabilityProviderOptions},
		{name: "denied output provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"execution-denied","providerOptions":{"p":{"enabled":true}}}}]}]}`, capability: capabilityProviderOptions},
		{name: "error text output provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"error-text","value":"","providerOptions":{"p":{"enabled":true}}}}]}]}`, capability: capabilityProviderOptions},
		{name: "error json output provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"error-json","value":null,"providerOptions":{"p":{"enabled":true}}}}]}]}`, capability: capabilityProviderOptions},
		{name: "nested text provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"content","value":[{"type":"text","text":"","providerOptions":{"p":{"enabled":true}}}]}}]}]}`, capability: capabilityProviderOptions},
		{name: "nested file provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"content","value":[{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain","providerOptions":{"p":{"enabled":true}}}]}}]}]}`, capability: capabilityProviderOptions},
		{name: "nested custom provider options", body: `{"prompt":[{"role":"assistant","content":[{"type":"tool-result","toolCallId":"c","toolName":"f","output":{"type":"content","value":[{"type":"custom","providerOptions":{"p":{"enabled":true}}}]}}]}]}`, capability: capabilityProviderOptions},
		{name: "body headers with empty value", body: `{"prompt":[],"headers":{"x-example":""}}`, capability: capabilityBodyHeaders},
		{name: "raw output", body: `{"prompt":[],"includeRawChunks":true}`, capability: capabilityRawOutput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			response := harness.serve(validRequest(tc.body))
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Equal(t, fmt.Sprintf(`{"error":{"message":"unsupported capability: %s","type":"invalid_request_error","param":null,"code":"invalid_request"}}`, tc.capability), response.Body.String())
			assert.Equal(t, 1, harness.mapCalls)
			assert.Zero(t, harness.policy.callCount())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
		})
	}

	t.Run("unknown mapper discriminators fail closed", func(t *testing.T) {
		_, failure := inspectWireTool(json.RawMessage(`{"type":"future"}`))
		require.NotNil(t, failure)
		assert.Equal(t, stageMapping, failure.stage)
		require.NotNil(t, inspectWireToolChoice(json.RawMessage(`{"type":"future"}`)))
		require.NotNil(t, inspectWireToolResultOutput(json.RawMessage(`{"type":"future"}`)))
		require.NotNil(t, inspectWireToolResultOutput(json.RawMessage(`{"type":"content","value":[{"type":"future"}]}`)))
	})

	t.Run("fixed multi-capability order", func(t *testing.T) {
		tests := []struct {
			name string
			body string
			want unsupportedCapability
		}{
			{name: "message provider options before file and headers", body: `{"prompt":[{"role":"user","providerOptions":{"p":{"x":1}},"content":[{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain"}]}],"headers":{"x":""}}`, want: capabilityProviderOptions},
			{name: "part provider options before file", body: `{"prompt":[{"role":"user","content":[{"type":"file","data":{"type":"text","text":"x"},"mediaType":"text/plain","providerOptions":{"p":{"x":1}}}]}]}`, want: capabilityProviderOptions},
			{name: "headers before tools and root provider options", body: `{"prompt":[],"headers":{"x":""},"tools":[{"type":"function","name":"f","inputSchema":{}}],"providerOptions":{"p":{"x":1}}}`, want: capabilityBodyHeaders},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				response := harness.serve(validRequest(tc.body))
				assert.Contains(t, response.Body.String(), "unsupported capability: "+string(tc.want))
			})
		}
	})
}

type goldenRecord struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func requestFromGolden(t *testing.T, record goldenRecord) *http.Request {
	t.Helper()
	req := httptest.NewRequest(record.Method, record.Path, bytes.NewReader(record.Body))
	req.Header = make(http.Header, len(record.Headers))
	for name, value := range record.Headers {
		req.Header.Set(name, value)
	}
	return req
}

func loadGolden(t *testing.T, name string) []goldenRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../../test/providerwire-v4/goldens", name))
	require.NoError(t, err)
	var records []goldenRecord
	require.NoError(t, json.Unmarshal(data, &records))
	return records
}

func TestRuntimeGoldenReplay(t *testing.T) {
	tests := []struct {
		file         string
		index        int
		status       int
		stage        requestStage
		capability   unsupportedCapability
		mapCalls     int
		policyCalls  int
		resolveCalls int
		modelCalls   int
	}{
		{file: "streaming.json", index: 0, status: http.StatusBadRequest, stage: stageEnvelope},
		{file: "sequence.json", index: 0, status: http.StatusOK, mapCalls: 1, policyCalls: 1, resolveCalls: 1, modelCalls: 1},
		{file: "sequence.json", index: 1, status: http.StatusBadRequest, stage: stageEnvelope},
		{file: "scalar-presence.json", index: 0, status: http.StatusBadRequest, capability: capabilityBodyHeaders, mapCalls: 1},
		{file: "headers.json", index: 0, status: http.StatusBadRequest, capability: capabilityBodyHeaders, mapCalls: 1},
		{file: "headers.json", index: 1, status: http.StatusBadRequest, capability: capabilityBodyHeaders, mapCalls: 1},
		{file: "comprehensive-unions.json", index: 0, status: http.StatusBadRequest, capability: capabilityProviderOptions, mapCalls: 1},
	}
	for _, tc := range tests {
		name := fmt.Sprintf("%s/%d", tc.file, tc.index)
		t.Run(name, func(t *testing.T) {
			records := loadGolden(t, tc.file)
			require.Greater(t, len(records), tc.index)
			harness := newRuntimeHarness(t, testLimits())
			if tc.stage != "" {
				_, failure := harness.handler.validateRequest(requestFromGolden(t, records[tc.index]))
				require.NotNil(t, failure)
				assert.Equal(t, tc.stage, failure.stage)
			}
			response := harness.serve(requestFromGolden(t, records[tc.index]))
			assert.Equal(t, tc.status, response.Code)
			if tc.capability != "" {
				assert.Contains(t, response.Body.String(), "unsupported capability: "+string(tc.capability))
			}
			assert.Equal(t, tc.mapCalls, harness.mapCalls)
			assert.Equal(t, tc.policyCalls, harness.policy.callCount())
			assert.Equal(t, tc.resolveCalls, harness.resolver.callCount())
			assert.Equal(t, tc.modelCalls, harness.model.callCount())
		})
	}
}

func TestRuntimePolicyResolutionAndOrder(t *testing.T) {
	t.Run("policy mutation reaches model in exact order", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.policy.mutate = func(options provider.CallOptions) provider.CallOptions {
			value := 42
			options.Seed = &value
			return options
		}
		harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, []string{"map", "policy", "resolve", "model"}, harness.recorder.snapshot())
		require.NotNil(t, harness.model.receivedOptions().Seed)
		assert.Equal(t, 42, *harness.model.receivedOptions().Seed)
	})

	t.Run("categorized policy failures stop resolution", func(t *testing.T) {
		var typedNil *provider.APICallError
		tests := []struct {
			err    error
			status int
			wrap   bool
		}{
			{err: ErrPolicyAuthentication, status: http.StatusUnauthorized, wrap: true},
			{err: ErrPolicyPermission, status: http.StatusForbidden, wrap: true},
			{err: ErrPolicyRateLimit, status: http.StatusTooManyRequests, wrap: true},
			{err: ErrPolicyOverload, status: http.StatusServiceUnavailable, wrap: true},
			{err: errors.New("private policy secret"), status: http.StatusInternalServerError, wrap: true},
			{err: typedNil, status: http.StatusInternalServerError},
		}
		for _, tc := range tests {
			harness := newRuntimeHarness(t, testLimits())
			harness.policy.err = tc.err
			if tc.wrap {
				harness.policy.err = fmt.Errorf("wrapped: %w", tc.err)
			}
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, 1, harness.policy.callCount())
			assert.Zero(t, harness.resolver.callCount())
			assert.Zero(t, harness.model.callCount())
			assert.NotContains(t, response.Body.String(), "private")
		}
	})

	t.Run("policy resolver and specification panics are contained", func(t *testing.T) {
		tests := []struct {
			name   string
			mutate func(*runtimeHarness)
		}{
			{name: "policy", mutate: func(h *runtimeHarness) { h.policy.panicApply = true }},
			{name: "resolver", mutate: func(h *runtimeHarness) { h.resolver.panicResolve = true }},
			{name: "specification", mutate: func(h *runtimeHarness) { h.model.panicSpecification = true }},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				tc.mutate(harness)
				response := harness.serve(validRequest(`{"prompt":[]}`))
				assert.Equal(t, http.StatusInternalServerError, response.Code)
				assert.Equal(t, string(canonicalInternalError), response.Body.String())
				assert.NotContains(t, response.Body.String(), "private")
			})
		}
	})

	t.Run("resolution failures are normalized", func(t *testing.T) {
		var typedNil *provider.APICallError
		tests := []struct {
			err    error
			status int
		}{
			{err: &catalog.UnknownModelError{ModelID: "secret-alias"}, status: http.StatusNotFound},
			{err: errors.New("private resolver secret"), status: http.StatusInternalServerError},
			{err: typedNil, status: http.StatusInternalServerError},
		}
		for _, tc := range tests {
			harness := newRuntimeHarness(t, testLimits())
			harness.resolver.err = tc.err
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.NotContains(t, response.Body.String(), "secret")
			assert.Zero(t, harness.model.callCount())
		}
	})

	t.Run("exact alias and invalid catalog results", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		req := validRequest(`{"prompt":[]}`)
		req.Header.Set(HeaderModelID, " alias with spaces ")
		harness.serve(req)
		assert.Equal(t, " alias with spaces ", harness.resolver.requestedModelID())

		var nilModel *recordingModel
		emptyIDModel := &recordingModel{recorder: &runtimeRecorder{}}
		v3Model := &recordingModel{recorder: &runtimeRecorder{}, specification: "v3"}
		tests := []struct {
			resolved    catalog.ResolvedModel
			actualModel *recordingModel
		}{
			{resolved: catalog.ResolvedModel{Model: emptyIDModel}, actualModel: emptyIDModel},
			{resolved: catalog.ResolvedModel{ID: "canonical"}},
			{resolved: catalog.ResolvedModel{ID: "canonical", Model: nilModel}},
			{resolved: catalog.ResolvedModel{ID: "canonical", Model: v3Model}, actualModel: v3Model},
		}
		for _, tc := range tests {
			candidate := newRuntimeHarness(t, testLimits())
			candidate.resolver.resolved = tc.resolved
			response := candidate.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Zero(t, candidate.model.callCount())
			if tc.actualModel != nil {
				assert.Zero(t, tc.actualModel.callCount())
			}
		}
	})
}

func TestRuntimeModelExecutionBoundaries(t *testing.T) {
	t.Run("already canceled request does not start model call", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response := harness.serve(validRequest(`{"prompt":[]}`).WithContext(ctx))
		assert.Equal(t, 499, response.Code)
		assert.Zero(t, harness.model.callCount())
	})

	t.Run("panic nil result and typed nil error are internal", func(t *testing.T) {
		var typedNil *provider.APICallError
		tests := []func(context.Context, provider.CallOptions) (*provider.GenerateResult, error){
			func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { panic("private panic") },
			func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, nil },
			func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, typedNil },
		}
		for _, generate := range tests {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = generate
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
		}
	})

	t.Run("caller cancellation wins", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		started := make(chan struct{})
		release := make(chan struct{})
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			close(started)
			<-release
			return &provider.GenerateResult{}, nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		req := validRequest(`{"prompt":[]}`).WithContext(ctx)
		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(response, req)
			close(done)
		}()
		<-started
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("handler did not return after cancellation")
		}
		assert.Equal(t, 499, response.Code)
		close(release)
	})

	t.Run("observable caller cancellation overrides a returned result", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return validGenerateResult(), nil
		}
		ctx, cancel := context.WithCancel(context.Background())
		req := validRequest(`{"prompt":[]}`).WithContext(ctx)
		response := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			harness.handler.ServeHTTP(response, req)
			close(done)
		}()
		require.Eventually(t, func() bool { return harness.model.callCount() == 1 }, time.Second, time.Millisecond)
		cancel()
		require.Eventually(t, func() bool {
			select {
			case <-done:
				return true
			default:
				return false
			}
		}, time.Second, time.Millisecond)
		assert.Equal(t, 499, response.Code)
	})

	t.Run("observable model deadline overrides a returned provider error", func(t *testing.T) {
		limits := testLimits()
		limits.ModelDuration = 20 * time.Millisecond
		harness := newRuntimeHarness(t, limits)
		harness.model.generate = func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
			<-ctx.Done()
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: http.StatusTooManyRequests})
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusGatewayTimeout, response.Code)
		assert.Equal(t, `{"error":{"message":"request timed out","type":"internal_server_error","param":null,"code":"timeout"}}`, response.Body.String())
	})

	t.Run("timeout bounds ignored cancellation and late return does not block", func(t *testing.T) {
		limits := testLimits()
		limits.ModelDuration = 20 * time.Millisecond
		harness := newRuntimeHarness(t, limits)
		started := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan struct{})
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			close(started)
			<-release
			close(returned)
			return &provider.GenerateResult{}, nil
		}
		start := time.Now()
		response := harness.serve(validRequest(`{"prompt":[]}`))
		elapsed := time.Since(start)
		<-started
		assert.Equal(t, http.StatusGatewayTimeout, response.Code)
		assert.Less(t, elapsed, time.Second)
		close(release)
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Fatal("late model return blocked")
		}
	})
}

type testNetError struct {
	timeout bool
}

func (e testNetError) Error() string   { return "private transport details" }
func (e testNetError) Timeout() bool   { return e.timeout }
func (e testNetError) Temporary() bool { return false }

func modelErrorRequest(status int) func(*runtimeHarness) *http.Request {
	return func(h *runtimeHarness) *http.Request {
		h.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: status, Message: "private"})
		}
		return validRequest(`{"prompt":[]}`)
	}
}

func TestSafeErrorReductionAndWire(t *testing.T) {
	t.Run("exact category documents", func(t *testing.T) {
		tests := []struct {
			value     safeError
			status    int
			body      string
			retryable bool
		}{
			{value: safeError{category: safeInvalidRequest}, status: 400, body: `{"error":{"message":"invalid request","type":"invalid_request_error","param":null,"code":"invalid_request"}}`},
			{value: safeError{category: safeAuthentication}, status: 401, body: `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"authentication_error"}}`},
			{value: safeError{category: safePermission}, status: 403, body: `{"error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden"}}`},
			{value: safeError{category: safeModelNotFound}, status: 404, body: `{"error":{"message":"model not found","type":"model_not_found","param":null,"code":"model_not_found"}}`},
			{value: safeError{category: safeRateLimit}, status: 429, body: `{"error":{"message":"rate limit exceeded","type":"rate_limit_exceeded","param":null,"code":"rate_limit_exceeded"}}`, retryable: true},
			{value: safeError{category: safeOverload}, status: 503, body: `{"error":{"message":"service overloaded","type":"internal_server_error","param":null,"code":"overloaded"}}`, retryable: true},
			{value: safeError{category: safeFailedDependency}, status: 424, body: `{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`},
			{value: safeError{category: safeUpstream}, status: 502, body: `{"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error"}}`, retryable: true},
			{value: safeError{category: safeTimeout}, status: 504, body: `{"error":{"message":"request timed out","type":"internal_server_error","param":null,"code":"timeout"}}`, retryable: true},
			{value: safeError{category: safeCancellation}, status: 499, body: `{"error":{"message":"request canceled","type":"internal_server_error","param":null,"code":"canceled"}}`},
			{value: safeError{category: safeInternal}, status: 500, body: string(canonicalInternalError), retryable: true},
		}
		for _, tc := range tests {
			h := newTestHandler(t, testLimits())
			response := httptest.NewRecorder()
			h.writeSafeError(response, tc.value)
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.String())
			assert.NotContains(t, response.Body.String(), "retryable")
			derivedRetryable := tc.status == 408 || tc.status == 409 || tc.status == 429 || tc.status >= 500
			assert.Equal(t, tc.retryable, derivedRetryable)
		}
	})

	t.Run("production origins emit exact category documents", func(t *testing.T) {
		tests := []struct {
			name  string
			want  safeError
			setup func(*runtimeHarness) *http.Request
		}{
			{name: "invalid request", want: safeError{category: safeInvalidRequest}, setup: func(*runtimeHarness) *http.Request {
				return validRequest(`{"prompt":[],"unknown":true}`)
			}},
			{name: "authentication", want: safeError{category: safeAuthentication}, setup: func(h *runtimeHarness) *http.Request {
				h.policy.err = ErrPolicyAuthentication
				return validRequest(`{"prompt":[]}`)
			}},
			{name: "permission", want: safeError{category: safePermission}, setup: func(h *runtimeHarness) *http.Request {
				h.policy.err = ErrPolicyPermission
				return validRequest(`{"prompt":[]}`)
			}},
			{name: "model not found", want: safeError{category: safeModelNotFound}, setup: func(h *runtimeHarness) *http.Request {
				h.resolver.err = catalog.ErrUnknownModel
				return validRequest(`{"prompt":[]}`)
			}},
			{name: "rate limit", want: safeError{category: safeRateLimit}, setup: modelErrorRequest(http.StatusTooManyRequests)},
			{name: "overload", want: safeError{category: safeOverload}, setup: modelErrorRequest(http.StatusServiceUnavailable)},
			{name: "failed dependency", want: safeError{category: safeFailedDependency}, setup: modelErrorRequest(http.StatusBadRequest)},
			{name: "upstream", want: safeError{category: safeUpstream}, setup: modelErrorRequest(http.StatusInternalServerError)},
			{name: "timeout", want: safeError{category: safeTimeout}, setup: modelErrorRequest(http.StatusRequestTimeout)},
			{name: "cancellation", want: safeError{category: safeCancellation}, setup: func(*runtimeHarness) *http.Request {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return validRequest(`{"prompt":[]}`).WithContext(ctx)
			}},
			{name: "internal", want: safeError{category: safeInternal}, setup: func(h *runtimeHarness) *http.Request {
				h.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, errors.New("private")
				}
				return validRequest(`{"prompt":[]}`)
			}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				response := harness.serve(tc.setup(harness))
				expectedBody, expectedStatus, ok := encodeSafeError(tc.want, harness.handler.limits.ErrorResponseBytes)
				require.True(t, ok)
				assert.Equal(t, expectedStatus, response.Code)
				assert.Equal(t, string(expectedBody), response.Body.String())
			})
		}
	})

	t.Run("provider status reduction", func(t *testing.T) {
		tests := []struct {
			status   int
			category safeErrorCategory
		}{
			{status: 408, category: safeTimeout},
			{status: 504, category: safeTimeout},
			{status: 429, category: safeRateLimit},
			{status: 503, category: safeOverload},
			{status: 529, category: safeOverload},
			{status: 400, category: safeFailedDependency},
			{status: 401, category: safeFailedDependency},
			{status: 403, category: safeFailedDependency},
			{status: 404, category: safeFailedDependency},
			{status: 499, category: safeFailedDependency},
			{status: 0, category: safeUpstream},
			{status: 500, category: safeUpstream},
			{status: 302, category: safeUpstream},
		}
		for _, tc := range tests {
			err := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: tc.status, Message: "private"})
			assert.Equal(t, tc.category, safeErrorFromProvider(err).category)
		}
	})

	t.Run("context and transport precedence", func(t *testing.T) {
		var typedNil *provider.APICallError
		assert.Equal(t, safeInternal, safeErrorFromPolicy(typedNil).category)
		assert.Equal(t, safeInternal, safeErrorFromResolution(typedNil).category)
		assert.Equal(t, safeInternal, safeErrorFromProvider(typedNil).category)
		wrappedTypedNil := fmt.Errorf("wrapped: %w", typedNil)
		assert.Equal(t, safeInternal, safeErrorFromPolicy(wrappedTypedNil).category)
		assert.Equal(t, safeInternal, safeErrorFromResolution(wrappedTypedNil).category)
		assert.Equal(t, safeInternal, safeErrorFromProvider(wrappedTypedNil).category)
		assert.Equal(t, safeCancellation, safeErrorFromProvider(fmt.Errorf("wrapped: %w", context.Canceled)).category)
		assert.Equal(t, safeTimeout, safeErrorFromProvider(fmt.Errorf("wrapped: %w", context.DeadlineExceeded)).category)
		assert.Equal(t, safeCancellation, safeErrorFromProvider(provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Cause: context.Canceled})).category)
		assert.Equal(t, safeTimeout, safeErrorFromProvider(provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Cause: context.DeadlineExceeded})).category)
		assert.Equal(t, safeFailedDependency, safeErrorFromProvider(provider.NewAPICallError(provider.APICallErrorOptions{
			StatusCode: http.StatusBadRequest,
			Cause:      &url.Error{Op: "Post", URL: "https://private", Err: testNetError{timeout: true}},
		})).category)
		assert.Equal(t, safeTimeout, safeErrorFromProvider(testNetError{timeout: true}).category)
		assert.Equal(t, safeUpstream, safeErrorFromProvider(testNetError{}).category)
		assert.Equal(t, safeTimeout, safeErrorFromProvider(&url.Error{Op: "Post", URL: "https://private", Err: testNetError{timeout: true}}).category)
		assert.Equal(t, safeUpstream, safeErrorFromProvider(&url.Error{Op: "Post", URL: "https://private", Err: testNetError{}}).category)
		assert.Equal(t, safeUpstream, safeErrorFromProvider(&net.DNSError{Name: "private.internal", Err: "no such host"}).category)
		assert.Equal(t, safeUpstream, safeErrorFromProvider(&net.OpError{Op: "dial", Net: "tcp", Addr: testAddr("private:443"), Err: errors.New("connection refused")}).category)
		assert.Equal(t, safeInternal, safeErrorFromProvider(errors.New("arbitrary internal")).category)
	})

	t.Run("transport failures map through raw handler", func(t *testing.T) {
		tests := []struct {
			err    error
			status int
			body   string
		}{
			{err: &net.OpError{Op: "dial", Net: "tcp", Addr: testAddr("private:443"), Err: errors.New("connection refused")}, status: http.StatusBadGateway, body: `{"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error"}}`},
			{err: &net.DNSError{Name: "private.internal", Err: "no such host"}, status: http.StatusBadGateway, body: `{"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error"}}`},
			{err: &url.Error{Op: "Post", URL: "https://private", Err: testNetError{timeout: true}}, status: http.StatusGatewayTimeout, body: `{"error":{"message":"request timed out","type":"internal_server_error","param":null,"code":"timeout"}}`},
		}
		for _, tc := range tests {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, tc.err
			}
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.String())
			assert.NotContains(t, response.Body.String(), "private")
		}
	})

	t.Run("hostile provider error is private", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{
				StatusCode:        500,
				Message:           "credential=secret provider=private backend=model-private",
				URL:               "https://provider.invalid/private",
				RequestBodyValues: json.RawMessage(`{"authorization":"secret"}`),
				ResponseHeaders:   map[string][]string{"Authorization": {"secret"}},
				ResponseBody:      `{"secret":true}`,
				Data:              json.RawMessage(`{"metadata":"private"}`),
				Cause:             errors.New("cause credential=private-cause"),
			})
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusBadGateway, response.Code)
		assert.Equal(t, `{"error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error"}}`, response.Body.String())
		for _, secret := range []string{"credential", "secret", "provider.invalid", "model-private", "private-cause", "authorization", "metadata"} {
			assert.NotContains(t, response.Body.String(), secret)
		}
	})

	t.Run("hostile arbitrary model error is private", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return nil, errors.New("credential=secret url=https://private.invalid backend=private-model")
		}
		response := harness.serve(validRequest(`{"prompt":[]}`))
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, string(canonicalInternalError), response.Body.String())
		for _, private := range []string{"credential", "secret", "private.invalid", "private-model"} {
			assert.NotContains(t, response.Body.String(), private)
		}
	})

	t.Run("schema-invalid safe error uses canonical fallback", func(t *testing.T) {
		h := newTestHandler(t, testLimits())
		rejectingSchema, err := schema.CompileSchema(json.RawMessage(`false`))
		require.NoError(t, err)
		h.errorSchema = rejectingSchema
		response := httptest.NewRecorder()
		h.writeSafeError(response, safeError{category: safeRateLimit})
		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Equal(t, string(canonicalInternalError), response.Body.String())
	})

	t.Run("invalid safe value and oversized ordinary error use canonical fallback", func(t *testing.T) {
		limits := testLimits()
		limits.ErrorResponseBytes = int64(len(canonicalInternalError))
		h := newTestHandler(t, limits)
		for _, value := range []safeError{{category: 255}, {category: safeInvalidRequest}} {
			response := httptest.NewRecorder()
			h.writeSafeError(response, value)
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, string(canonicalInternalError), response.Body.String())
			assert.LessOrEqual(t, response.Body.Len(), int(limits.ErrorResponseBytes))
		}
	})
}

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }
