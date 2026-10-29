package execution_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/internal/testadapter"
	v4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type outcome = testadapter.Outcome

// document is the ProviderWire document a category maps to.
type document struct {
	status  int
	code    string
	message string
}

var unaryDocuments = map[execution.Category]document{
	execution.CategoryInvalidRequest:   {400, "invalid_request", "invalid request"},
	execution.CategoryBYOKCredentials:  {400, "invalid_request", "providerOptions.gateway.byok requires supported provider arrays with valid accounts"},
	execution.CategoryPermission:       {403, "forbidden", "forbidden"},
	execution.CategoryRateLimit:        {429, "rate_limit_exceeded", "rate limit exceeded"},
	execution.CategoryOverload:         {503, "overloaded", "service overloaded"},
	execution.CategoryFailedDependency: {424, "failed_dependency", "failed dependency"},
	execution.CategoryUpstream:         {502, "upstream_error", "upstream failure"},
	execution.CategoryTimeout:          {504, "timeout", "request timed out"},
	execution.CategoryCancellation:     {499, "canceled", "request canceled"},
	execution.CategoryInternal:         {500, "internal_error", "internal error"},
}

var reasonMessages = map[execution.Reason]string{
	execution.ReasonReservedProviderOptions: "reserved provider option namespace",
	execution.ReasonProtectedCallHeader:     "protected call header",
}

type wireError struct {
	Type  string `json:"type"`
	Error struct {
		Message    string `json:"message"`
		Code       string `json:"code"`
		StatusCode int    `json:"statusCode"`
		Data       *struct {
			Metadata provider.ProviderMetadata `json:"providerMetadata"`
		} `json:"data"`
	} `json:"error"`
	Metadata provider.ProviderMetadata `json:"providerMetadata"`
}

func overviewFrom(t *testing.T, metadata provider.ProviderMetadata) *execution.Overview {
	t.Helper()
	if metadata == nil || metadata["gateway"] == nil {
		return nil
	}
	var namespace struct {
		Execution *execution.Overview `json:"execution"`
	}
	require.NoError(t, json.Unmarshal(metadata["gateway"], &namespace))
	return namespace.Execution
}

// assertProviderWire checks that ProviderWire wrote the documents the
// adapter's outcome maps to, including the attempt view it published.
func assertProviderWire(t *testing.T, response *httptest.ResponseRecorder, want outcome) {
	t.Helper()
	if !want.Committed {
		expected := unaryDocuments[want.Category]
		if want.Category == 0 {
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var success struct {
				Metadata provider.ProviderMetadata `json:"providerMetadata"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &success))
			assertOverview(t, want.Overview, overviewFrom(t, success.Metadata))
			return
		}
		if message, ok := reasonMessages[want.Reason]; ok {
			expected.message = message
		}
		require.Equal(t, expected.status, response.Code, response.Body.String())
		var body wireError
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, expected.code, body.Error.Code)
		assert.Equal(t, expected.message, body.Error.Message)
		assertOverview(t, want.Overview, overviewFrom(t, body.Metadata))
		return
	}
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
	frames := strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n")
	var errors []wireError
	var finish *wireError
	var last string
	for _, frame := range frames {
		var event wireError
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &event))
		last = event.Type
		switch event.Type {
		case "error":
			errors = append(errors, event)
		case "finish":
			finish = &event
		}
	}
	codes := make([]string, 0, len(errors))
	for _, event := range errors {
		codes = append(codes, event.Error.Code)
	}
	wantCodes := make([]string, 0, len(want.PartErrors)+1)
	for _, category := range want.PartErrors {
		wantCodes = append(wantCodes, unaryDocuments[category].code)
	}
	if want.Category != 0 {
		require.Equal(t, "error", last, "a terminal failure ends with its error")
		wantCodes = append(wantCodes, unaryDocuments[want.Category].code)
		last := errors[len(errors)-1]
		var metadata provider.ProviderMetadata
		if last.Error.Data != nil {
			metadata = last.Error.Data.Metadata
		}
		assertOverview(t, want.Overview, overviewFrom(t, metadata))
	} else {
		require.NotNil(t, finish, "a successful stream ends with finish")
		assertOverview(t, want.Overview, overviewFrom(t, finish.Metadata))
	}
	assert.Equal(t, wantCodes, codes)
}

func assertOverview(t *testing.T, want, got *execution.Overview) {
	t.Helper()
	wantJSON, err := json.Marshal(want)
	require.NoError(t, err)
	gotJSON, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, string(wantJSON), string(gotJSON))
}

// probe is a fake model whose streams report when they are drained.
type probe struct {
	id       string
	generate func(context.Context) (*provider.GenerateResult, error)
	stream   func(context.Context, *probe) (*provider.StreamResult, error)
	calls    atomic.Int32
	drained  chan struct{}
	streamed atomic.Bool
	options  provider.CallOptions
}

func newProbe(id string) *probe { return &probe{id: id, drained: make(chan struct{})} }

func (p *probe) SpecificationVersion() string               { return "v4" }
func (p *probe) Provider() string                           { return "native" }
func (p *probe) ModelID() string                            { return p.id }
func (p *probe) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (p *probe) DoGenerate(ctx context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
	p.calls.Add(1)
	p.options = options
	if p.generate != nil {
		return p.generate(ctx)
	}
	return validResult(), nil
}

func (p *probe) DoStream(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
	p.calls.Add(1)
	if p.stream != nil {
		return p.stream(ctx, p)
	}
	return p.parts(ctx, finishPart())
}

// parts streams the given parts and closes after a final finish, as providers
// do. Otherwise it waits for cancellation and sends one more part, which only a
// drain receives, before closing.
func (p *probe) parts(ctx context.Context, parts ...provider.StreamPart) (*provider.StreamResult, error) {
	p.streamed.Store(true)
	stream := make(chan provider.StreamPart)
	go func() {
		defer close(stream)
		for _, part := range parts {
			select {
			case stream <- part:
			case <-ctx.Done():
			}
		}
		if len(parts) > 0 && parts[len(parts)-1].Type == provider.PartFinish {
			close(p.drained)
			return
		}
		<-ctx.Done()
		stream <- provider.StreamPart{Type: provider.PartRaw}
		close(p.drained)
	}()
	return &provider.StreamResult{Stream: stream}, nil
}

func validResult() *provider.GenerateResult {
	return &provider.GenerateResult{
		Content:      []provider.GenerateContentPart{{Type: provider.ContentText, Text: "ok"}},
		FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
	}
}

func finishPart() provider.StreamPart {
	return provider.StreamPart{Type: provider.PartFinish, Usage: &provider.Usage{}, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
}

// prompt is one conversation in both forms: the ProviderWire request JSON and
// the provider messages it maps to.
type prompt struct {
	wire     json.RawMessage
	messages []provider.Message
}

func hostedCall() provider.ContentPart {
	part := provider.ToolCallPart("hosted", "search", json.RawMessage(`{}`))
	part.ProviderExecuted = true
	return part
}

var (
	pendingPrompt = prompt{
		wire:     json.RawMessage(`[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"hosted","toolName":"search","input":{},"providerExecuted":true}]}]`),
		messages: []provider.Message{provider.NewAssistantMessage(hostedCall())},
	}
	inconsistentPrompt = prompt{
		wire:     json.RawMessage(`[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"hosted","toolName":"search","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"hosted","toolName":"other","output":{"type":"text","value":"done"}}]}]`),
		messages: []provider.Message{provider.NewAssistantMessage(hostedCall(), provider.ToolResultPart("hosted", "other", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "done"}))},
	}
)

type scenario struct {
	name       string
	streaming  bool
	prompt     *prompt
	parts      []provider.StreamPart
	headers    map[string]string
	options    map[string]string
	ctx        func(*testing.T) context.Context
	limits     execution.Limits
	models     func() []*probe
	configured bool
	accounts   bool
	selector   func([]*probe) execution.RequestSelector
	want       outcome
	attempts   []fallback.AttemptOutcome
	calls      []int32
	drained    bool
}

const scenarioTimeout = time.Second

const scenarioEnd = 30 * time.Millisecond

func expiring(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(t.Context(), scenarioEnd)
	t.Cleanup(cancel)
	return ctx
}

func canceling(t *testing.T) context.Context {
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(scenarioEnd, cancel)
	t.Cleanup(cancel)
	return ctx
}

func one(configure func(*probe)) func() []*probe {
	return func() []*probe {
		model := newProbe("A")
		if configure != nil {
			configure(model)
		}
		return []*probe{model}
	}
}

func apiError(status int) error {
	return provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: status, Message: "native failure"})
}

func (sc scenario) runtime(t *testing.T, models []*probe) *execution.Runtime {
	t.Helper()
	limits := sc.limits
	if limits == (execution.Limits{}) {
		limits = execution.Limits{StreamIdleDuration: scenarioTimeout, StreamDrainDuration: scenarioTimeout}
	}
	var selector execution.RequestSelector
	switch {
	case sc.selector != nil:
		selector = sc.selector(models)
	case sc.configured:
		languageModels := make([]provider.LanguageModel, len(models))
		candidates := make([]catalog.ConfiguredCandidate, len(models))
		for i, model := range models {
			languageModels[i] = model
			candidates[i] = catalog.ConfiguredCandidate{Provider: "native", ProviderInstance: "configured", ModelID: model.id}
		}
		var model provider.LanguageModel = models[0]
		if len(models) > 1 {
			chain, err := fallback.New(languageModels...)
			require.NoError(t, err)
			model = chain
		}
		selector = execution.CatalogSelector(resolver{resolved: catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: candidates}})
	case sc.accounts:
		languageModels := make([]provider.LanguageModel, len(models))
		for i, model := range models {
			languageModels[i] = model
		}
		chain, err := fallback.New(languageModels...)
		require.NoError(t, err)
		selector = func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
			return execution.AccountSelection(id, chain, execution.RequestAccounts{Provider: "native", ModelID: models[0].id, Count: len(models)}), nil
		}
	default:
		selector = func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
			return execution.Selection{ID: id, Model: models[0]}, nil
		}
	}
	runtime, err := execution.New(execution.Config{Selector: selector, Limits: limits})
	require.NoError(t, err)
	return runtime
}

func (sc scenario) callOptions() provider.CallOptions {
	options := provider.CallOptions{Headers: sc.headers}
	if sc.prompt != nil {
		options.Prompt = sc.prompt.messages
	}
	if len(sc.options) > 0 {
		options.ProviderOptions = make(provider.ProviderOptions, len(sc.options))
		for namespace, raw := range sc.options {
			options.ProviderOptions[namespace] = provider.RawProviderOption{Key: namespace, Raw: json.RawMessage(raw)}
		}
	}
	return options
}

func (sc scenario) request(t *testing.T, ctx context.Context) *http.Request {
	t.Helper()
	body := map[string]any{"prompt": []any{}}
	if sc.prompt != nil {
		body["prompt"] = sc.prompt.wire
	}
	if sc.headers != nil {
		body["headers"] = sc.headers
	}
	if sc.options != nil {
		options := make(map[string]json.RawMessage, len(sc.options))
		for namespace, raw := range sc.options {
			options[namespace] = json.RawMessage(raw)
		}
		body["providerOptions"] = options
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, v4.LanguageModelPath, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(v4.HeaderSpecificationVersion, v4.SpecificationVersion)
	request.Header.Set(v4.HeaderModelID, "alias")
	request.Header.Set(v4.HeaderStreaming, strconv.FormatBool(sc.streaming))
	return request
}

func (sc scenario) context(t *testing.T) context.Context {
	if sc.ctx != nil {
		return sc.ctx(t)
	}
	return t.Context()
}

func (sc scenario) assertModels(t *testing.T, models []*probe) {
	t.Helper()
	for i, model := range models {
		assert.Equal(t, sc.calls[i], model.calls.Load(), "model %d calls", i)
		if sc.drained && model.streamed.Load() {
			select {
			case <-model.drained:
			case <-time.After(scenarioTimeout):
				assert.Fail(t, "stream was not drained", "model %d", i)
			}
		}
	}
}

// streamed is a configured stream of parts over one candidate.
func streamed(name string, want outcome, parts ...provider.StreamPart) scenario {
	return scenario{name: name, streaming: true, configured: true, parts: parts, calls: []int32{1}, drained: true, want: want,
		attempts: []fallback.AttemptOutcome{fallback.AttemptSelected},
		models: one(func(p *probe) {
			p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) { return p.parts(ctx, parts...) }
		})}
}

// invalidStream is a stream the runtime refuses after accepted parts.
func invalidStream(name string, accepted int, parts ...provider.StreamPart) scenario {
	return streamed(name, outcome{Category: execution.CategoryInternal, Committed: true, InvalidOutput: true, Parts: accepted}, parts...)
}

func streamPart(kind provider.StreamPartType, id string) provider.StreamPart {
	return provider.StreamPart{Type: kind, ID: id}
}

func toolCall(id string, providerExecuted bool) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: "search", Input: `{}`, ProviderExecuted: providerExecuted}
}

func toolResult(id string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolResult, ToolCallID: id, ToolName: "search", Result: json.RawMessage(`{}`)}
}

func approval(callID, approvalID string) provider.StreamPart {
	return provider.StreamPart{Type: provider.PartToolApprovalRequest, ToolCallID: callID, ApprovalID: approvalID}
}

// providerWireFrames is the number of frames ProviderWire writes for the
// first accepted parts of a stream refused as invalid output: its leading
// stream-start, which a real first stream-start part also produces, one frame
// per other part except raw parts, which it drops unless requested, and the
// terminal error.
func providerWireFrames(parts []provider.StreamPart, accepted int) int {
	frames := 2
	for i, part := range parts[:accepted] {
		if part.Type == provider.PartRaw || (i == 0 && part.Type == provider.PartStreamStart) {
			continue
		}
		frames++
	}
	return frames
}

type resolver struct{ resolved catalog.ResolvedModel }

func (r resolver) ResolveModel(context.Context, string) (catalog.ResolvedModel, error) {
	return r.resolved, nil
}

func TestTestAdapter_MatchesProviderWire(t *testing.T) {
	for _, sc := range []scenario{
		{name: "configured unary success", configured: true, models: one(nil), calls: []int32{1},
			attempts: []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{name: "configured stream success", streaming: true, configured: true, models: one(nil), calls: []int32{1}, drained: true,
			want: outcome{Committed: true}, attempts: []fallback.AttemptOutcome{fallback.AttemptSelected}},
		{name: "fallback after an overload", configured: true, calls: []int32{1, 1},
			models: func() []*probe {
				first, second := newProbe("A"), newProbe("B")
				first.generate = func(context.Context) (*provider.GenerateResult, error) { return nil, apiError(503) }
				return []*probe{first, second}
			},
			attempts: []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{name: "provider rate limit", calls: []int32{1}, want: outcome{Category: execution.CategoryRateLimit},
			models: one(func(p *probe) {
				p.generate = func(context.Context) (*provider.GenerateResult, error) { return nil, apiError(429) }
			})},
		{name: "selector panic", calls: []int32{0}, want: outcome{Category: execution.CategoryInternal}, models: one(nil),
			selector: func([]*probe) execution.RequestSelector {
				return func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
					panic("private")
				}
			}},
		{name: "invalid selection", calls: []int32{0}, want: outcome{Category: execution.CategoryInternal}, models: one(nil),
			selector: func([]*probe) execution.RequestSelector {
				return func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (execution.Selection, error) {
					return execution.Selection{ID: id}, nil
				}
			}},
		{name: "BYOK credentials", calls: []int32{0}, want: outcome{Category: execution.CategoryBYOKCredentials}, models: one(nil),
			selector: func([]*probe) execution.RequestSelector {
				return func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
					return execution.Selection{}, execution.ErrInvalidBYOK
				}
			}},
		{name: "model panic", calls: []int32{1}, want: outcome{Category: execution.CategoryInternal},
			models: one(func(p *probe) {
				p.generate = func(context.Context) (*provider.GenerateResult, error) { panic("private") }
			})},
		{name: "nil result", calls: []int32{1}, want: outcome{Category: execution.CategoryInternal},
			models: one(func(p *probe) {
				p.generate = func(context.Context) (*provider.GenerateResult, error) { return nil, nil }
			})},
		{name: "late result after the deadline", ctx: expiring, calls: []int32{1}, want: outcome{Category: execution.CategoryTimeout},
			models: one(func(p *probe) {
				p.generate = func(ctx context.Context) (*provider.GenerateResult, error) { <-ctx.Done(); return validResult(), nil }
			})},
		{name: "late result after cancellation", ctx: canceling, calls: []int32{1}, want: outcome{Category: execution.CategoryCancellation},
			models: one(func(p *probe) {
				p.generate = func(ctx context.Context) (*provider.GenerateResult, error) { <-ctx.Done(); return validResult(), nil }
			})},
		{name: "stream setup error with a stream", streaming: true, calls: []int32{1}, drained: true, want: outcome{Category: execution.CategoryOverload},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) {
					result, _ := p.parts(ctx)
					return result, apiError(503)
				}
			})},
		{name: "late stream after cancellation", streaming: true, ctx: canceling, calls: []int32{1}, drained: true, want: outcome{Category: execution.CategoryCancellation},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) {
					<-ctx.Done()
					return p.parts(ctx)
				}
			})},
		{name: "idle timeout", streaming: true, calls: []int32{1}, drained: true, want: outcome{Category: execution.CategoryTimeout, Committed: true},
			limits: execution.Limits{StreamIdleDuration: scenarioEnd, StreamDrainDuration: scenarioTimeout},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) { return p.parts(ctx) }
			})},
		{name: "deadline during the stream", streaming: true, ctx: expiring, calls: []int32{1}, drained: true, want: outcome{Category: execution.CategoryTimeout, Committed: true},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) {
					return p.parts(ctx, provider.StreamPart{Type: provider.PartStreamStart})
				}
			})},
		{name: "cancellation during the stream", streaming: true, ctx: canceling, calls: []int32{1}, drained: true, want: outcome{Category: execution.CategoryCancellation, Committed: true},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) {
					return p.parts(ctx, provider.StreamPart{Type: provider.PartStreamStart})
				}
			})},
		{name: "in-band stream error", streaming: true, calls: []int32{1}, drained: true,
			want: outcome{Committed: true, PartErrors: []execution.Category{execution.CategoryRateLimit}},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) {
					failure := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Message: "native failure"})
					return p.parts(ctx, provider.StreamPart{Type: provider.PartError, APICallError: failure}, finishPart())
				}
			})},
		{name: "configured deadline during the stream", streaming: true, configured: true, ctx: expiring, calls: []int32{1}, drained: true,
			want: outcome{Category: execution.CategoryTimeout, Committed: true}, attempts: []fallback.AttemptOutcome{fallback.AttemptCanceled},
			models: one(func(p *probe) {
				p.stream = func(ctx context.Context, p *probe) (*provider.StreamResult, error) { return p.parts(ctx) }
			})},
		{name: "configured setup failure", streaming: true, configured: true, calls: []int32{1},
			want: outcome{Category: execution.CategoryFailedDependency}, attempts: []fallback.AttemptOutcome{fallback.AttemptFailed},
			models: one(func(p *probe) {
				p.stream = func(context.Context, *probe) (*provider.StreamResult, error) { return nil, apiError(400) }
			})},
		{name: "BYOK failed then selected", accounts: true, calls: []int32{1, 1},
			models: func() []*probe {
				first, second := newProbe("model"), newProbe("model")
				first.generate = func(context.Context) (*provider.GenerateResult, error) { return nil, apiError(503) }
				return []*probe{first, second}
			},
			attempts: []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptSelected}},
		{name: "BYOK exhausted stream setup", streaming: true, accounts: true, calls: []int32{1, 1},
			want: outcome{Category: execution.CategoryOverload},
			models: func() []*probe {
				first, second := newProbe("model"), newProbe("model")
				for _, p := range []*probe{first, second} {
					p.stream = func(context.Context, *probe) (*provider.StreamResult, error) { return nil, apiError(503) }
				}
				return []*probe{first, second}
			},
			attempts: []fallback.AttemptOutcome{fallback.AttemptFailed, fallback.AttemptFailed}},
		{name: "protected call header", headers: map[string]string{"Authorization": "Bearer caller"}, configured: true, models: one(nil), calls: []int32{0},
			want: outcome{Category: execution.CategoryInvalidRequest, Reason: execution.ReasonProtectedCallHeader}},
		{name: "configured gateway controls", options: map[string]string{"gateway": `{"byok":{}}`}, configured: true, models: one(nil), calls: []int32{0},
			want: outcome{Category: execution.CategoryInvalidRequest, Reason: execution.ReasonReservedProviderOptions}},
		invalidStream("repeated stream-start", 1, provider.StreamPart{Type: provider.PartStreamStart}, provider.StreamPart{Type: provider.PartStreamStart}),
		invalidStream("stream-start after raw", 1, provider.StreamPart{Type: provider.PartRaw}, provider.StreamPart{Type: provider.PartStreamStart}),
		invalidStream("metadata after content", 1, streamPart(provider.PartTextStart, "t"), provider.StreamPart{Type: provider.PartResponseMeta}),
		invalidStream("metadata with open reasoning", 2, provider.StreamPart{Type: provider.PartStreamStart}, streamPart(provider.PartReasoningStart, "r"), provider.StreamPart{Type: provider.PartResponseMeta}),
		invalidStream("tool ID reused in a later segment", 2, toolCall("segment", false), provider.StreamPart{Type: provider.PartResponseMeta}, toolCall("segment", false)),
		invalidStream("content after finish", 3, streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextEnd, "t"), finishPart(), streamPart(provider.PartTextStart, "late")),
		invalidStream("second finish", 2, finishPart(), provider.StreamPart{Type: provider.PartRaw}, finishPart()),
		invalidStream("text delta first", 0, streamPart(provider.PartTextDelta, "t")),
		invalidStream("overlapping text", 1, streamPart(provider.PartTextStart, "t"), streamPart(provider.PartTextStart, "u")),
		invalidStream("reasoning delta first", 0, streamPart(provider.PartReasoningDelta, "r")),
		invalidStream("reused reasoning ID", 2, streamPart(provider.PartReasoningStart, "r"), streamPart(provider.PartReasoningEnd, "r"), streamPart(provider.PartReasoningStart, "r")),
		invalidStream("tool result first", 0, toolResult("c")),
		invalidStream("repeated tool call", 1, toolCall("c", false), toolCall("c", false)),
		invalidStream("approval first", 0, approval("c", "a")),
		invalidStream("repeated approval", 2, toolCall("c", true), approval("c", "a"), approval("c", "b")),
		invalidStream("finish first without usage", 0, provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}),
		invalidStream("finish with active text", 1, streamPart(provider.PartTextStart, "t"), finishPart()),
		invalidStream("unknown part first", 0, provider.StreamPart{Type: "future"}),
		func() scenario {
			sc := invalidStream("unknown part after an error", 2, provider.StreamPart{Type: provider.PartStreamStart}, provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 429, Message: "native failure"})}, provider.StreamPart{Type: "future"})
			sc.want.PartErrors = []execution.Category{execution.CategoryRateLimit}
			return sc
		}(),
		streamed("two segments", outcome{Committed: true},
			provider.StreamPart{Type: provider.PartResponseMeta}, streamPart(provider.PartTextStart, "0"), streamPart(provider.PartTextEnd, "0"), finishPart(),
			provider.StreamPart{Type: provider.PartRaw},
			provider.StreamPart{Type: provider.PartResponseMeta}, streamPart(provider.PartTextStart, "0"), streamPart(provider.PartTextEnd, "0"), finishPart()),
		{name: "close after a later metadata", streaming: true, configured: true, calls: []int32{1},
			want: outcome{Category: execution.CategoryInternal, Committed: true}, attempts: []fallback.AttemptOutcome{fallback.AttemptSelected},
			models: one(func(p *probe) {
				p.stream = func(context.Context, *probe) (*provider.StreamResult, error) {
					parts := make(chan provider.StreamPart, 2)
					parts <- finishPart()
					parts <- provider.StreamPart{Type: provider.PartResponseMeta}
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				}
			})},
		func() scenario {
			sc := streamed("stall after finish until cancellation", outcome{Category: execution.CategoryCancellation, Committed: true}, finishPart(), provider.StreamPart{Type: provider.PartRaw})
			sc.ctx = canceling
			return sc
		}(),
		func() scenario {
			sc := streamed("history-seeded stream result", outcome{Committed: true}, toolResult("hosted"), finishPart())
			sc.prompt = &pendingPrompt
			return sc
		}(),
		{name: "history-seeded unary result", configured: true, prompt: &pendingPrompt, calls: []int32{1}, attempts: []fallback.AttemptOutcome{fallback.AttemptSelected},
			models: one(func(p *probe) {
				p.generate = func(context.Context) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{
						Content:      []provider.GenerateContentPart{{Type: provider.ContentToolResult, ToolCallID: "hosted", ToolName: "search", Result: json.RawMessage(`{}`)}},
						FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					}, nil
				}
			})},
		{name: "inconsistent history", configured: true, prompt: &inconsistentPrompt, models: one(nil), calls: []int32{0},
			want: outcome{Category: execution.CategoryInvalidRequest}},
		{name: "inconsistent history while streaming", streaming: true, configured: true, prompt: &inconsistentPrompt, models: one(nil), calls: []int32{0},
			want: outcome{Category: execution.CategoryInvalidRequest}},
		{name: "unary invalid output", configured: true, calls: []int32{1}, attempts: []fallback.AttemptOutcome{fallback.AttemptSelected},
			want: outcome{Category: execution.CategoryInternal, InvalidOutput: true},
			models: one(func(p *probe) {
				p.generate = func(context.Context) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: "future"}}, nil
				}
			})},
	} {
		t.Run(sc.name, func(t *testing.T) {
			models := sc.models()
			got := testadapter.New(sc.runtime(t, models), nil).Run(sc.context(t), "alias", sc.callOptions(), sc.streaming)
			sc.assertModels(t, models)
			assert.Equal(t, sc.want.Category, got.Category)
			assert.Equal(t, sc.want.Reason, got.Reason)
			assert.Equal(t, sc.want.Committed, got.Committed)
			assert.Equal(t, sc.want.PartErrors, got.PartErrors)
			assert.Equal(t, sc.want.InvalidOutput, got.InvalidOutput)
			if sc.want.InvalidOutput && sc.streaming {
				assert.Equal(t, sc.want.Parts, got.Parts, "accepted parts")
			}
			var attempts []fallback.AttemptOutcome
			if got.Overview != nil {
				for _, attempt := range got.Overview.Attempts {
					attempts = append(attempts, attempt.Outcome)
				}
			}
			assert.Equal(t, sc.attempts, attempts)
			if sc.accounts && got.Overview != nil {
				assert.Empty(t, got.Overview.CanonicalModelID)
				for _, attempt := range got.Overview.Attempts {
					assert.Empty(t, attempt.ProviderInstance)
				}
			}

			models = sc.models()
			handler, err := v4.New(sc.runtime(t, models))
			require.NoError(t, err)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, sc.request(t, sc.context(t)))
			sc.assertModels(t, models)
			assertProviderWire(t, response, got)
			if got.InvalidOutput && got.Committed {
				frames := strings.Split(strings.TrimSuffix(response.Body.String(), "\n\n"), "\n\n")
				assert.Len(t, frames, providerWireFrames(sc.parts, got.Parts), "ProviderWire stops at the same part")
			}
		})
	}
}

func TestTestAdapter_TranslatesEachAttempt(t *testing.T) {
	first, second := newProbe("A"), newProbe("B")
	first.generate = func(context.Context) (*provider.GenerateResult, error) { return nil, apiError(503) }
	chain, err := fallback.New(
		execution.Translate(first, execution.Target{Type: execution.TargetAnthropic, ModelID: "A"}),
		execution.Translate(second, execution.Target{Type: execution.TargetOpenAI, ModelID: "B"}),
	)
	require.NoError(t, err)
	chain.WithDecider(execution.FallbackDecider)
	candidates := []catalog.ConfiguredCandidate{{Provider: "anthropic", ProviderInstance: "primary", ModelID: "A"}, {Provider: "openai", ProviderInstance: "backup", ModelID: "B"}}
	runtime, err := execution.New(execution.Config{
		Selector: execution.CatalogSelector(resolver{resolved: catalog.ResolvedModel{ID: "canonical", Model: chain, Candidates: candidates}}),
		Limits:   execution.Limits{StreamIdleDuration: scenarioTimeout, StreamDrainDuration: scenarioTimeout},
	})
	require.NoError(t, err)
	translator := func(target execution.Target, options provider.CallOptions) (provider.CallOptions, error) {
		options.Headers = map[string]string{"X-Target": string(target.Type) + "/" + target.ModelID}
		return options, nil
	}
	got := testadapter.New(runtime, translator).Run(t.Context(), "alias", provider.CallOptions{}, false)
	assert.Zero(t, got.Category)
	require.NotNil(t, got.Overview)
	require.Len(t, got.Overview.Attempts, 2)
	assert.Equal(t, map[string]string{"X-Target": "anthropic/A"}, first.options.Headers)
	assert.Equal(t, map[string]string{"X-Target": "openai/B"}, second.options.Headers)
}
