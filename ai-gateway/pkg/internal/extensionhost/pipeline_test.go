package extensionhost

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/provider"
)

// invoke calls model once, unary or streaming, and returns a stream's parts.
func invoke(t *testing.T, model provider.LanguageModel, stream bool) ([]provider.StreamPart, error) {
	t.Helper()
	if !stream {
		_, err := model.DoGenerate(t.Context(), provider.CallOptions{})
		return nil, err
	}
	result, err := model.DoStream(t.Context(), provider.CallOptions{})
	if err != nil {
		return nil, err
	}
	return collect(t, result.Stream), nil
}

func TestPipeline_PhaseOrder(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			log := &journal{}
			h := newHarness(t, define("all", &recorder{name: "all", journal: log}))
			h.model.generate = func(context.Context) (*provider.GenerateResult, error) {
				log.add("model")
				return textResult("hello"), nil
			}
			h.model.stream = func(context.Context) (*provider.StreamResult, error) {
				log.add("model")
				return &provider.StreamResult{Stream: streamOf(textParts("hello")...)}, nil
			}
			_, err := invoke(t, h.compose(t, extension.SourceOperatorCatalog), stream)
			require.NoError(t, err)
			h.flush()
			assert.Equal(t, []string{"all:admit", "all:transform", "all:inspect-request", "model", "all:inspect-response:text", "all:inspect-response:finish", "all:observe:success"}, log.list())
		})
	}
}

type admitObserver struct {
	admitted chan *extension.Call
	observed chan *extension.Call
}

func (a admitObserver) Admit(_ context.Context, call *extension.Call) error {
	call.State = "reservation"
	a.admitted <- call
	return nil
}

func (a admitObserver) Observe(_ context.Context, call *extension.Call, _ extension.Record) error {
	a.observed <- call
	return nil
}

func TestPipeline_PartialImplementationAndPerCallState(t *testing.T) {
	extension0 := admitObserver{admitted: make(chan *extension.Call, 64), observed: make(chan *extension.Call, 64)}
	h := newHarness(t, define("quota", extension0))
	model := h.compose(t, extension.SourceOperatorCatalog)
	var calls sync.WaitGroup
	for range 16 {
		calls.Go(func() {
			_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
			assert.NoError(t, err)
		})
	}
	calls.Wait()
	h.flush()
	close(extension0.admitted)
	close(extension0.observed)
	admitted := map[*extension.Call]bool{}
	for call := range extension0.admitted {
		admitted[call] = true
	}
	require.Len(t, admitted, 16)
	observed := 0
	for call := range extension0.observed {
		assert.True(t, admitted[call], "observe received a Call admit did not")
		assert.Equal(t, "reservation", call.State)
		observed++
	}
	assert.Equal(t, 16, observed)
}

func TestPipeline_RequestInfo(t *testing.T) {
	admitted := func(calls chan extension.Call, change func(*extension.Call)) *recorder {
		return &recorder{admit: func(_ context.Context, call *extension.Call) error {
			if change != nil {
				change(call)
			}
			calls <- *call
			return nil
		}}
	}
	base := extension.Call{Namespace: "stacks-1", Caller: extension.CallerServiceAccount, CorrelationID: "correlation-1"}

	t.Run("configured", func(t *testing.T) {
		calls := make(chan extension.Call, 2)
		h := newHarness(t,
			define("a", admitted(calls, func(call *extension.Call) { call.Candidates[0].ProviderModel = "changed" })),
			define("b", admitted(calls, nil)),
		)
		h.run(t, execution.ConfiguredSelection("canonical", h.model, configuredExpected("primary", "secondary")), "alias", false)
		want := base
		want.Keys, want.Source, want.RequestedModel, want.CanonicalModel = extension.KeysOperator, extension.SourceOperatorCatalog, "alias", "canonical"
		want.Candidates = []extension.Candidate{
			{Model: "alias", CanonicalModel: "canonical", Provider: "native", ProviderInstance: "instance-primary", ProviderModel: "primary"},
			{Model: "alias", CanonicalModel: "canonical", Provider: "native", ProviderInstance: "instance-secondary", ProviderModel: "secondary"},
		}
		changed := <-calls
		assert.Equal(t, "changed", changed.Candidates[0].ProviderModel)
		assert.Equal(t, want, <-calls, "each extension gets its own candidates")
	})

	t.Run("request BYOK", func(t *testing.T) {
		calls := make(chan extension.Call, 1)
		h := newHarness(t, define("a", admitted(calls, nil)))
		expected := []execution.ExpectedAttempt{{ModelID: "openai/gpt-5", Destination: catalog.ConfiguredCandidate{Provider: "openai", ModelID: "gpt-5"}}}
		h.run(t, execution.AccountSelection("openai/gpt-5", h.model, expected), "openai/gpt-5", false)
		want := base
		want.Keys, want.Source, want.RequestedModel = extension.KeysCustomer, extension.SourceRequestBYOK, "openai/gpt-5"
		want.Candidates = []extension.Candidate{{Model: "openai/gpt-5", Provider: "openai", ProviderModel: "gpt-5"}}
		assert.Equal(t, want, <-calls)
	})
}

func TestPipeline_GatesStopAtFirstDenial(t *testing.T) {
	for _, phase := range []extension.Phase{extension.PhaseAdmit, extension.PhaseInspectRequest, extension.PhaseInspectResponse} {
		t.Run(string(phase), func(t *testing.T) {
			log := &journal{}
			deny := func(context.Context, *extension.Call) error { return fmt.Errorf("over quota: %w", extension.ErrDenied) }
			b := &recorder{name: "b", journal: log}
			switch phase {
			case extension.PhaseAdmit:
				b.admit = deny
			case extension.PhaseInspectRequest:
				b.inspectRequest = func(ctx context.Context, call *extension.Call, _ provider.CallOptions) error { return deny(ctx, call) }
			default:
				b.inspectResponse = func(ctx context.Context, call *extension.Call, _ extension.Part) error { return deny(ctx, call) }
			}
			h := newHarness(t, define("c", &recorder{name: "c", journal: log}), define("a", &recorder{name: "a", journal: log}), define("b", b))
			_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
			assert.Equal(t, extension.ErrDenied, err)
			h.flush()
			entries := log.list()
			observed := []string{"a:observe:denied", "b:observe:denied", "c:observe:denied"}
			requestPhases := []string{"a:admit", "b:admit", "c:admit", "a:transform", "b:transform", "c:transform", "a:inspect-request", "b:inspect-request", "c:inspect-request"}
			switch phase {
			case extension.PhaseAdmit:
				assert.Equal(t, append([]string{"a:admit", "b:admit"}, observed...), entries)
				assert.Zero(t, h.model.callCount())
			case extension.PhaseInspectRequest:
				assert.Equal(t, append(requestPhases[:8:8], observed...), entries)
				assert.Zero(t, h.model.callCount())
			default:
				assert.Equal(t, append(append(requestPhases, "a:inspect-response:text", "b:inspect-response:text"), observed...), entries)
				assert.Equal(t, 1, h.model.callCount())
			}
			assert.Equal(t, 1.0, h.events("b", string(phase), eventDenied))
		})
	}
}

func TestPipeline_TransformOrder(t *testing.T) {
	appendSystem := func(name string) func(context.Context, *extension.Call, provider.CallOptions) (provider.CallOptions, error) {
		return func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
			params.Prompt = append(params.Prompt, provider.NewSystemMessage(name))
			return params, nil
		}
	}
	h := newHarness(t,
		define("o1", &recorder{transform: appendSystem("o1")}, priority(0)),
		define("t2", &recorder{transform: appendSystem("t2")}, tenant(5)),
		define("t1", &recorder{transform: appendSystem("t1")}, tenant(5)),
		define("o0", &recorder{transform: appendSystem("o0")}, priority(-1)),
		define("omin", &recorder{transform: appendSystem("omin")}, priority(math.MinInt)),
		define("omax", &recorder{transform: appendSystem("omax")}, priority(math.MaxInt)),
	)
	_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
	require.NoError(t, err)
	var order []string
	for _, message := range h.model.received[0].Prompt {
		order = append(order, message.Content[0].Text)
	}
	assert.Equal(t, []string{"t1", "t2", "omin", "o0", "o1", "omax"}, order)
}

func TestPipeline_InspectRequestSeesTheModelParams(t *testing.T) {
	var inspected provider.CallOptions
	h := newHarness(t,
		define("defaults", &recorder{transform: func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
			params.Prompt = append([]provider.Message{provider.NewSystemMessage("Answer concisely.")}, params.Prompt...)
			params.Headers = map[string]string{"x-trace": "1"}
			return params, nil
		}}),
		define("guard", &recorder{inspectRequest: func(_ context.Context, _ *extension.Call, params provider.CallOptions) error {
			inspected = params
			return nil
		}}),
	)
	_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "hi"})}})
	require.NoError(t, err)
	require.Len(t, h.model.received, 1)
	assert.Equal(t, h.model.received[0], inspected)
	assert.Equal(t, provider.NewSystemMessage("Answer concisely."), inspected.Prompt[0])
}

func TestPipeline_HostPolicyAfterEachTransform(t *testing.T) {
	addHeader := func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
		params.Headers = map[string]string{"Authorization": "Bearer private"}
		return params, nil
	}
	addControls := func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
		params.ProviderOptions = provider.ProviderOptions{execution.ControlNamespace: provider.RawProviderOption{Raw: []byte(`{}`)}}
		return params, nil
	}

	t.Run("fail-closed", func(t *testing.T) {
		h := newHarness(t, define("headers", &recorder{transform: addHeader}))
		_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
		assert.Equal(t, extension.ErrUnavailable, err)
		assert.Zero(t, h.model.callCount())
		assert.Equal(t, 1.0, h.events("headers", string(extension.PhaseTransformRequest), eventFailed))
		assert.Contains(t, h.logs.String(), `"class":"policy"`)
		assert.NotContains(t, h.logs.String(), "private")
	})

	t.Run("failure logs carry no error text", func(t *testing.T) {
		h := newHarness(t, define("defaults", &recorder{transform: func(context.Context, *extension.Call, provider.CallOptions) (provider.CallOptions, error) {
			return provider.CallOptions{}, errors.New("Bearer sk-private-credential")
		}}, skip))
		_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
		require.NoError(t, err)
		logs := h.logs.String()
		assert.Contains(t, logs, `"extension":"defaults"`)
		assert.Contains(t, logs, `"phase":"transform-request"`)
		assert.Contains(t, logs, `"class":"error"`)
		assert.Contains(t, logs, `"correlation_id":"correlation-1"`)
		assert.NotContains(t, logs, "sk-private-credential")
	})

	t.Run("skip", func(t *testing.T) {
		var next provider.CallOptions
		h := newHarness(t,
			define("a-controls", &recorder{transform: addControls}, skip),
			define("b-next", &recorder{transform: func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
				next = params
				return params, nil
			}}),
		)
		_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{Headers: map[string]string{"x-trace": "1"}})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"x-trace": "1"}, next.Headers)
		assert.Nil(t, next.ProviderOptions)
		require.Len(t, h.model.received, 1)
		assert.NotContains(t, h.model.received[0].ProviderOptions, execution.ControlNamespace)
		assert.Equal(t, 1.0, h.events("a-controls", string(extension.PhaseTransformRequest), eventFailed))
		assert.Contains(t, h.logs.String(), `"msg":"gateway extension failed"`)
		assert.Contains(t, h.logs.String(), `"policy":"skip"`)
	})

	t.Run("transforms cannot mutate the caller's options", func(t *testing.T) {
		h := newHarness(t, define("mutate", &recorder{transform: func(_ context.Context, _ *extension.Call, params provider.CallOptions) (provider.CallOptions, error) {
			params.Headers["x-added"] = "1"
			return params, nil
		}}))
		headers := map[string]string{"x-trace": "1"}
		_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{Headers: headers})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"x-trace": "1"}, headers)
		assert.Equal(t, map[string]string{"x-trace": "1", "x-added": "1"}, h.model.received[0].Headers)
	})
}

func TestPipeline_NormalizedParts(t *testing.T) {
	inspect := func(parts *[]extension.Part) *recorder {
		return &recorder{inspectResponse: func(_ context.Context, _ *extension.Call, part extension.Part) error {
			*parts = append(*parts, part)
			return nil
		}}
	}
	text := func(parts []extension.Part) string {
		var builder strings.Builder
		for _, part := range parts {
			if part.Type == extension.PartText {
				builder.WriteString(part.Text)
			}
		}
		return builder.String()
	}

	var unary, streamed []extension.Part
	h := newHarness(t, define("guard", inspect(&unary)))
	_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
	require.NoError(t, err)
	h2 := newHarness(t, define("guard", inspect(&streamed)))
	h2.model.stream = func(context.Context) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: streamOf(textParts("hello", " ", "world")...)}, nil
	}
	_, err = invoke(t, h2.compose(t, extension.SourceOperatorCatalog), true)
	require.NoError(t, err)

	assert.Equal(t, []extension.Part{{Type: extension.PartText, Text: "hello world"}, {Type: extension.PartFinish}}, unary)
	require.Len(t, streamed, 4)
	assert.Equal(t, extension.Part{Type: extension.PartText, ID: "t", Text: "hello"}, streamed[0])
	assert.Equal(t, extension.PartFinish, streamed[3].Type)
	assert.Equal(t, text(unary), text(streamed))

	t.Run("tool call", func(t *testing.T) {
		var parts []extension.Part
		h := newHarness(t, define("guard", inspect(&parts)))
		h.model.stream = func(context.Context) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: streamOf(
				provider.StreamPart{Type: provider.PartStreamStart},
				provider.StreamPart{Type: provider.PartToolInputStart, ID: "call-1", ToolName: "search"},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-1", Delta: `{"q":`},
				provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call-1", Delta: `"x"}`},
				provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call-1"},
				provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call-1", ToolName: "search", Input: `{"q":"x"}`},
				provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}, Usage: &provider.Usage{}},
			)}, nil
		}
		_, err := invoke(t, h.compose(t, extension.SourceOperatorCatalog), true)
		require.NoError(t, err)
		assert.Equal(t, []extension.Part{{Type: extension.PartToolCall, ID: "call-1", ToolName: "search", Input: `{"q":"x"}`}, {Type: extension.PartFinish}}, parts)
	})
}

func TestPipeline_ResponseAbort(t *testing.T) {
	denyThird := func() *recorder {
		seen := 0
		return &recorder{inspectResponse: func(_ context.Context, _ *extension.Call, part extension.Part) error {
			if part.Type == extension.PartText {
				seen++
				if seen == 3 {
					return extension.ErrDenied
				}
			}
			return nil
		}}
	}

	t.Run("unary", func(t *testing.T) {
		h := newHarness(t, define("guard", &recorder{inspectResponse: func(context.Context, *extension.Call, extension.Part) error { return extension.ErrDenied }}))
		result, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
		assert.Nil(t, result)
		assert.Equal(t, extension.ErrDenied, err)
		assert.Equal(t, 1, h.model.callCount())
	})

	t.Run("stream", func(t *testing.T) {
		h := newHarness(t, define("guard", denyThird()))
		upstreamStopped := make(chan struct{})
		h.model.stream = func(ctx context.Context) (*provider.StreamResult, error) {
			parts := make(chan provider.StreamPart)
			go func() {
				defer close(parts)
				for _, part := range textParts("a", "b", "c", "d") {
					select {
					case parts <- part:
					case <-ctx.Done():
						close(upstreamStopped)
						return
					}
				}
			}()
			return &provider.StreamResult{Stream: parts}, nil
		}
		parts, err := invoke(t, h.compose(t, extension.SourceOperatorCatalog), true)
		require.NoError(t, err)
		assert.Equal(t, []provider.StreamPartType{provider.PartStreamStart, provider.PartTextStart, provider.PartTextDelta, provider.PartTextDelta, provider.PartError}, partTypes(parts))
		last := parts[len(parts)-1].APICallError
		require.NotNil(t, last)
		assert.ErrorIs(t, last, extension.ErrDenied)
		assert.Equal(t, execution.CategoryPermission, execution.ClassifyStreamPart(last))
		select {
		case <-upstreamStopped:
		case <-time.After(5 * time.Second):
			require.FailNow(t, "the model stream was not stopped")
		}
	})

	t.Run("fail-closed stream failure", func(t *testing.T) {
		h := newHarness(t, define("guard", &recorder{inspectResponse: func(context.Context, *extension.Call, extension.Part) error { panic("private") }}))
		parts, err := invoke(t, h.compose(t, extension.SourceOperatorCatalog), true)
		require.NoError(t, err)
		assert.Equal(t, []provider.StreamPartType{provider.PartStreamStart, provider.PartTextStart, provider.PartError}, partTypes(parts))
		assert.Equal(t, execution.CategoryUnavailable, execution.ClassifyStreamPart(parts[2].APICallError))
		assert.Equal(t, 1.0, h.events("guard", string(extension.PhaseInspectResponse), eventFailed))
		assert.Contains(t, h.logs.String(), `"class":"panic"`)
		assert.NotContains(t, h.logs.String(), "private")
	})

	t.Run("skipped inspector failure delivers the part", func(t *testing.T) {
		h := newHarness(t, define("guard", &recorder{inspectResponse: func(context.Context, *extension.Call, extension.Part) error { return errors.New("private") }}, skip))
		parts, err := invoke(t, h.compose(t, extension.SourceOperatorCatalog), true)
		require.NoError(t, err)
		assert.Equal(t, partTypes(textParts("hello ", "world")), partTypes(parts))
		assert.Equal(t, 3.0, h.events("guard", string(extension.PhaseInspectResponse), eventFailed))
	})
}

func TestPipeline_FailurePolicies(t *testing.T) {
	failing := func(err error, panics bool) func(context.Context, *extension.Call) error {
		return func(context.Context, *extension.Call) error {
			if panics {
				panic("private")
			}
			return err
		}
	}
	for _, tc := range []struct {
		name   string
		admit  func(context.Context, *extension.Call) error
		policy func(*extension.Definition)
		err    error
		calls  int
	}{
		{name: "fail-closed error", admit: failing(errors.New("connection refused"), false), err: extension.ErrUnavailable},
		{name: "fail-closed unavailable", admit: failing(extension.ErrUnavailable, false), err: extension.ErrUnavailable},
		{name: "fail-closed panic", admit: failing(nil, true), err: extension.ErrUnavailable},
		{name: "skip error", admit: failing(errors.New("connection refused"), false), policy: skip, calls: 1},
		{name: "skip panic", admit: failing(nil, true), policy: skip, calls: 1},
		{name: "skip still denies", admit: failing(extension.ErrDenied, false), policy: skip, err: extension.ErrDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var change []func(*extension.Definition)
			if tc.policy != nil {
				change = append(change, tc.policy)
			}
			records := make(chan extension.Record, 1)
			h := newHarness(t, define("gate", &recorder{admit: tc.admit, observe: func(_ context.Context, _ *extension.Call, record extension.Record) error {
				records <- record
				return nil
			}}, change...))
			_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
			assert.Equal(t, tc.err, err)
			assert.Equal(t, tc.calls, h.model.callCount())
			h.flush()
			record := <-records
			switch {
			case tc.err == nil:
				assert.Equal(t, extension.OutcomeSuccess, record.Outcome)
			case errors.Is(tc.err, extension.ErrDenied):
				assert.Equal(t, extension.OutcomeDenied, record.Outcome)
			default:
				assert.Equal(t, extension.OutcomeError, record.Outcome)
			}
		})
	}

	t.Run("a transform cannot deny", func(t *testing.T) {
		h := newHarness(t, define("defaults", &recorder{transform: func(context.Context, *extension.Call, provider.CallOptions) (provider.CallOptions, error) {
			return provider.CallOptions{}, extension.ErrDenied
		}}))
		_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
		assert.Equal(t, extension.ErrUnavailable, err)
		assert.Equal(t, 1.0, h.events("defaults", string(extension.PhaseTransformRequest), eventFailed))
	})
}

func TestPipeline_PanicContainment(t *testing.T) {
	panics := func() error { panic("private") }
	for _, tc := range []struct {
		name  string
		value *recorder
		phase extension.Phase
	}{
		{name: "admit", value: &recorder{admit: func(context.Context, *extension.Call) error { return panics() }}, phase: extension.PhaseAdmit},
		{name: "transform", value: &recorder{transform: func(context.Context, *extension.Call, provider.CallOptions) (provider.CallOptions, error) {
			return provider.CallOptions{}, panics()
		}}, phase: extension.PhaseTransformRequest},
		{name: "inspect request", value: &recorder{inspectRequest: func(context.Context, *extension.Call, provider.CallOptions) error { return panics() }}, phase: extension.PhaseInspectRequest},
		{name: "inspect response", value: &recorder{inspectResponse: func(context.Context, *extension.Call, extension.Part) error { return panics() }}, phase: extension.PhaseInspectResponse},
		{name: "observe", value: &recorder{observe: func(context.Context, *extension.Call, extension.Record) error { return panics() }}, phase: extension.PhaseObserve},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, define("broken", tc.value, skip))
			model := h.compose(t, extension.SourceOperatorCatalog)
			var calls sync.WaitGroup
			for range 4 {
				calls.Go(func() {
					result, err := model.DoGenerate(context.Background(), provider.CallOptions{})
					assert.NoError(t, err)
					assert.NotNil(t, result)
				})
			}
			calls.Wait()
			h.flush()
			assert.Positive(t, h.events("broken", string(tc.phase), eventFailed))
		})
	}
}

func TestPipeline_Observe(t *testing.T) {
	observed := func(records chan extension.Record) *recorder {
		return &recorder{observe: func(_ context.Context, _ *extension.Call, record extension.Record) error {
			records <- record
			return nil
		}}
	}

	t.Run("outcomes", func(t *testing.T) {
		providerErr := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500, Message: "private"})
		for _, tc := range []struct {
			name    string
			stream  bool
			model   func(*fakeModel)
			outcome extension.Outcome
			usage   bool
		}{
			{name: "unary success", outcome: extension.OutcomeSuccess, usage: true},
			{name: "stream success", stream: true, outcome: extension.OutcomeSuccess, usage: true},
			{name: "unary provider error", model: func(m *fakeModel) {
				m.generate = func(context.Context) (*provider.GenerateResult, error) { return nil, providerErr }
			}, outcome: extension.OutcomeError},
			{name: "stream setup error", stream: true, model: func(m *fakeModel) {
				m.stream = func(context.Context) (*provider.StreamResult, error) { return nil, providerErr }
			}, outcome: extension.OutcomeError},
			{name: "stream provider error part", stream: true, model: func(m *fakeModel) {
				m.stream = func(context.Context) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: streamOf(provider.StreamPart{Type: provider.PartStreamStart}, provider.StreamPart{Type: provider.PartError, APICallError: providerErr})}, nil
				}
			}, outcome: extension.OutcomeError},
			{name: "unary invalid output", model: func(m *fakeModel) {
				m.generate = func(context.Context) (*provider.GenerateResult, error) {
					return &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentToolResult, ToolCallID: "unknown", ToolName: "search"}}}, nil
				}
			}, outcome: extension.OutcomeInvalidOutput},
			{name: "stream invalid output", stream: true, model: func(m *fakeModel) {
				m.stream = func(context.Context) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: streamOf(provider.StreamPart{Type: provider.PartStreamStart}, provider.StreamPart{Type: provider.PartTextDelta, ID: "never-started", Delta: "x"})}, nil
				}
			}, outcome: extension.OutcomeInvalidOutput},
		} {
			t.Run(tc.name, func(t *testing.T) {
				records := make(chan extension.Record, 2)
				h := newHarness(t, define("audit", observed(records)))
				if tc.model != nil {
					tc.model(h.model)
				}
				before := time.Now()
				_, _ = invoke(t, h.compose(t, extension.SourceOperatorCatalog), tc.stream)
				h.flush()
				require.Len(t, records, 1)
				record := <-records
				assert.Equal(t, tc.outcome, record.Outcome)
				assert.Nil(t, record.DeniedBy)
				assert.False(t, record.Started.Before(before))
				assert.Positive(t, record.Duration)
				if tc.usage {
					require.NotNil(t, record.Usage.OutputTokens.Total)
					assert.Equal(t, 5, *record.Usage.OutputTokens.Total)
				}
			})
		}
	})

	t.Run("denial names the extension and phase", func(t *testing.T) {
		records := make(chan extension.Record, 1)
		h := newHarness(t, define("audit", observed(records)), define("quota", &recorder{admit: func(context.Context, *extension.Call) error { return extension.ErrDenied }}))
		_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
		assert.Equal(t, extension.ErrDenied, err)
		h.flush()
		record := <-records
		assert.Equal(t, extension.OutcomeDenied, record.Outcome)
		assert.Equal(t, &extension.Denial{Extension: "quota", Phase: extension.PhaseAdmit}, record.DeniedBy)
		assert.Zero(t, record.Usage)
	})

	t.Run("stream cancelled by the caller", func(t *testing.T) {
		records := make(chan extension.Record, 1)
		h := newHarness(t, define("audit", observed(records)))
		h.model.stream = func(ctx context.Context) (*provider.StreamResult, error) {
			parts := make(chan provider.StreamPart)
			go func() {
				defer close(parts)
				parts <- provider.StreamPart{Type: provider.PartStreamStart}
				<-ctx.Done()
			}()
			return &provider.StreamResult{Stream: parts}, nil
		}
		ctx, cancel := context.WithCancel(t.Context())
		result, err := h.compose(t, extension.SourceOperatorCatalog).DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)
		<-result.Stream
		cancel()
		collect(t, result.Stream)
		h.flush()
		assert.Equal(t, extension.OutcomeCancelled, (<-records).Outcome)
	})

	t.Run("failing observers leave the call unchanged", func(t *testing.T) {
		records := make(chan extension.Record, 1)
		h := newHarness(t,
			define("a-error", &recorder{observe: func(context.Context, *extension.Call, extension.Record) error { return errors.New("private") }}),
			define("b-panic", &recorder{observe: func(context.Context, *extension.Call, extension.Record) error { panic("private") }}),
			define("c-audit", observed(records)),
		)
		result, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Equal(t, textResult("hello world"), result)
		h.flush()
		assert.Equal(t, extension.OutcomeSuccess, (<-records).Outcome)
		assert.Equal(t, 1.0, h.events("a-error", string(extension.PhaseObserve), eventFailed))
		assert.Equal(t, 1.0, h.events("b-panic", string(extension.PhaseObserve), eventFailed))
		assert.NotContains(t, h.logs.String(), "private")
	})
}

func TestPipeline_ObservationIsOffTheResponsePath(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 2)
	blocking := &recorder{observe: func(ctx context.Context, _ *extension.Call, _ extension.Record) error {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil
	}}
	h := newHarness(t, define("billing", blocking))
	model := h.compose(t, extension.SourceOperatorCatalog)

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
		assert.NoError(t, err)
		result, err := model.DoStream(context.Background(), provider.CallOptions{})
		assert.NoError(t, err)
		for range result.Stream {
		}
	}()
	select {
	case <-returned:
	case <-time.After(observeTimeout / 2):
		require.FailNow(t, "a blocked observer delayed the unary result or the stream close")
	}
	<-started
	<-started

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		h.host.Close(ctx)
	}()
	select {
	case <-closed:
		require.FailNow(t, "close returned before in-flight observations finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	<-closed

	_, err := model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1.0, h.dropped())
}

func TestHost_CloseRespectsTheDeadline(t *testing.T) {
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })
	h := newHarness(t,
		define("stuck", &recorder{close: func(context.Context) error { <-never; return nil }}),
		define("failing", &recorder{close: func(context.Context) error { return errors.New("private") }}),
		define("panicking", &recorder{close: func(context.Context) error { panic("private") }}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	h.host.Close(ctx)
	assert.Less(t, time.Since(start), time.Second)
	assert.Equal(t, 1.0, h.events("stuck", string(phaseClose), eventFailed))
	assert.Contains(t, h.logs.String(), `"class":"timeout"`)
	assert.Equal(t, 1.0, h.events("failing", string(phaseClose), eventFailed))
	assert.Equal(t, 1.0, h.events("panicking", string(phaseClose), eventFailed))
	assert.NotContains(t, h.logs.String(), "private")
}

func TestPipeline_ModelPanicIsObserved(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			records := make(chan extension.Record, 1)
			h := newHarness(t, define("audit", &recorder{observe: func(_ context.Context, _ *extension.Call, record extension.Record) error {
				records <- record
				return nil
			}}))
			h.model.generate = func(context.Context) (*provider.GenerateResult, error) { panic("private") }
			h.model.stream = func(context.Context) (*provider.StreamResult, error) { panic("private") }
			model := h.compose(t, extension.SourceOperatorCatalog)
			assert.Panics(t, func() { _, _ = invoke(t, model, stream) })
			h.flush()
			require.Len(t, records, 1)
			assert.Equal(t, extension.OutcomeError, (<-records).Outcome)
		})
	}
}

func TestHost_CloseCountsAbandonedObservations(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	started := make(chan struct{}, 1)
	h := newHarness(t, define("billing", &recorder{observe: func(context.Context, *extension.Call, extension.Record) error {
		started <- struct{}{}
		<-release
		return nil
	}}))
	_, err := h.compose(t, extension.SourceOperatorCatalog).DoGenerate(t.Context(), provider.CallOptions{})
	require.NoError(t, err)
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	h.host.Close(ctx)
	assert.Equal(t, 1.0, h.dropped())
}
