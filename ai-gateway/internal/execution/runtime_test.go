package execution

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTimeout = time.Second

func testLimits() Limits {
	return Limits{StreamIdleDuration: testTimeout, StreamDrainDuration: testTimeout}
}

func directSelector(model provider.LanguageModel) RequestSelector {
	return func(_ context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (Selection, error) {
		return Selection{ID: id, Model: model}, nil
	}
}

func newRuntime(t *testing.T, selector RequestSelector, limits Limits) *Runtime {
	t.Helper()
	runtime, err := New(Config{Selector: selector, Limits: limits})
	require.NoError(t, err)
	return runtime
}

func newCall(t *testing.T, ctx context.Context, model provider.LanguageModel, limits Limits) *Call {
	t.Helper()
	call, err := newRuntime(t, directSelector(model), limits).Call(ctx, "request/model", provider.CallOptions{})
	require.NoError(t, err)
	t.Cleanup(call.Close)
	return call
}

type endingContext struct {
	name string
	ctx  func() context.Context
	err  error
	wait Wait
}

func endingContexts(t *testing.T) []endingContext {
	return []endingContext{
		{name: "deadline", err: context.DeadlineExceeded, wait: WaitDeadline, ctx: func() context.Context {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			t.Cleanup(cancel)
			return ctx
		}},
		{name: "canceled", err: context.Canceled, wait: WaitCanceled, ctx: func() context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			time.AfterFunc(20*time.Millisecond, cancel)
			t.Cleanup(cancel)
			return ctx
		}},
	}
}

// readyProducer returns a stream that always has a part ready, and the number
// of parts taken from it so far.
func readyProducer(t *testing.T) (chan provider.StreamPart, *atomic.Int64) {
	parts := make(chan provider.StreamPart)
	var produced atomic.Int64
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case parts <- provider.StreamPart{Type: provider.PartRaw}:
				produced.Add(1)
			case <-stop:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(stop)
		<-stopped
	})
	return parts, &produced
}

func TestNew_Validation(t *testing.T) {
	_, err := New(Config{Limits: testLimits()})
	assert.ErrorIs(t, err, errNilSelector)
	_, err = New(Config{Selector: directSelector(&generateModel{}), Limits: Limits{StreamDrainDuration: time.Second}})
	assert.ErrorIs(t, err, errIdleDuration)
	_, err = New(Config{Selector: directSelector(&generateModel{}), Limits: DefaultLimits})
	assert.NoError(t, err)
}

func TestRuntimeCall_Selection(t *testing.T) {
	var typedNil *generateModel
	for _, tc := range []struct {
		name     string
		selector RequestSelector
		err      error
	}{
		{name: "panic", selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			panic("private")
		}, err: errInternal},
		{name: "empty id", selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			return Selection{Model: &generateModel{}}, nil
		}, err: errInternal},
		{name: "invalid id", selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			return Selection{ID: string([]byte{0xff}), Model: &generateModel{}}, nil
		}, err: errInternal},
		{name: "nil model", selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			return Selection{ID: "id"}, nil
		}, err: errInternal},
		{name: "typed nil model", selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			return Selection{ID: "id", Model: typedNil}, nil
		}, err: errInternal},
		{name: "selector error", selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			return Selection{}, ErrInvalidBYOK
		}, err: ErrInvalidBYOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newRuntime(t, tc.selector, testLimits()).Call(t.Context(), "id", provider.CallOptions{})
			assert.ErrorIs(t, err, tc.err)
		})
	}

	t.Run("passes the gateway controls", func(t *testing.T) {
		var received json.RawMessage
		runtime := newRuntime(t, func(_ context.Context, id string, _ provider.CallOptions, gateway json.RawMessage) (Selection, error) {
			received = gateway
			return Selection{ID: id, Model: &generateModel{}}, nil
		}, testLimits())
		_, err := runtime.Call(t.Context(), "id", provider.CallOptions{ProviderOptions: provider.ProviderOptions{ControlNamespace: provider.RawProviderOption{Key: ControlNamespace, Raw: json.RawMessage(`{"byok":{}}`)}}})
		require.NoError(t, err)
		assert.JSONEq(t, `{"byok":{}}`, string(received))
	})

	t.Run("ended context does not select", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var selections atomic.Int32
		runtime := newRuntime(t, func(context.Context, string, provider.CallOptions, json.RawMessage) (Selection, error) {
			selections.Add(1)
			return Selection{}, nil
		}, testLimits())
		_, err := runtime.Call(ctx, "id", provider.CallOptions{})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Zero(t, selections.Load())
	})

	for _, ending := range endingContexts(t) {
		t.Run("selection returns after "+ending.name, func(t *testing.T) {
			model := &generateModel{}
			runtime := newRuntime(t, func(ctx context.Context, id string, _ provider.CallOptions, _ json.RawMessage) (Selection, error) {
				<-ctx.Done()
				return Selection{ID: id, Model: model}, nil
			}, testLimits())
			_, err := runtime.Call(ending.ctx(), "id", provider.CallOptions{})
			assert.ErrorIs(t, err, ending.err)
			assert.Zero(t, model.callCount())
		})
	}
}

func TestCallGenerate(t *testing.T) {
	t.Run("result", func(t *testing.T) {
		result, err := newCall(t, t.Context(), &generateModel{}, testLimits()).Generate()
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	for _, tc := range []struct {
		name     string
		generate func(context.Context) (*provider.GenerateResult, error)
	}{
		{name: "panic", generate: func(context.Context) (*provider.GenerateResult, error) { panic("private") }},
		{name: "nil nil", generate: func(context.Context) (*provider.GenerateResult, error) { return nil, nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newCall(t, t.Context(), &generateModel{generate: tc.generate}, testLimits()).Generate()
			assert.ErrorIs(t, err, errInternal)
		})
	}

	for _, ending := range endingContexts(t) {
		t.Run("late result after "+ending.name+" is discarded", func(t *testing.T) {
			model := &generateModel{generate: func(ctx context.Context) (*provider.GenerateResult, error) {
				<-ctx.Done()
				return &provider.GenerateResult{}, nil
			}}
			call := newCall(t, ending.ctx(), model, testLimits())
			result, err := call.Generate()
			assert.Nil(t, result)
			assert.ErrorIs(t, err, ending.err)
		})
	}

	t.Run("late fallback attempts are not projected", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		first := &generateModel{modelID: "A", generate: func(context.Context) (*provider.GenerateResult, error) {
			cancel()
			return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "unowned late native failure"})
		}}
		second := &generateModel{modelID: "B"}
		model, err := fallback.New(first, second)
		require.NoError(t, err)
		resolved := catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "A"}, {Provider: "native", ModelID: "B"}}}
		runtime := newRuntime(t, CatalogSelector(staticResolver{resolved: resolved}), testLimits())
		call, err := runtime.Call(ctx, "alias", provider.CallOptions{})
		require.NoError(t, err)
		_, err = call.Generate()
		require.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, call.View(err).Overview)
		assert.Zero(t, second.callCount())
	})
}

func TestCallGenerate_CanceledAttemptIsNotPublished(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered := make(chan struct{})
	first := &generateModel{modelID: "A", generate: func(ctx context.Context) (*provider.GenerateResult, error) {
		close(entered)
		<-ctx.Done()
		return nil, provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 401, Message: "handler-rejected native failure"})
	}}
	second := &generateModel{modelID: "B"}
	model, err := fallback.New(first, second)
	require.NoError(t, err)
	observations := make(chan fallback.Attempt, 1)
	model.WithAttemptObserver(func(_ context.Context, attempt fallback.Attempt) { observations <- attempt })
	resolved := catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "A"}, {Provider: "native", ModelID: "B"}}}
	call, err := newRuntime(t, CatalogSelector(staticResolver{resolved: resolved}), testLimits()).Call(ctx, "alias", provider.CallOptions{})
	require.NoError(t, err)
	go func() { <-entered; cancel() }()
	_, err = call.Generate()
	require.ErrorIs(t, err, context.Canceled)
	attempt := <-observations
	assert.Equal(t, fallback.AttemptCanceled, attempt.Outcome)
	require.NotNil(t, attempt.SourceErr)
	assert.Contains(t, attempt.SourceErr.Error(), "handler-rejected native failure")
	assert.Nil(t, call.View(err).Overview)
	assert.Nil(t, call.View(nil).Overview)
	assert.Zero(t, second.callCount())
}

func TestDrain_DelayedClose(t *testing.T) {
	parts := make(chan provider.StreamPart)
	done := make(chan struct{})
	go func() {
		drain(parts, testTimeout)
		close(done)
	}()
	parts <- provider.StreamPart{Type: provider.PartRaw}
	close(parts)
	requireClosed(t, done, "drain did not observe delayed close")
}

func TestRuntime_ConcurrentCallsAreIsolated(t *testing.T) {
	failing := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 503, Message: "first failed"})
	resolver := func(failFirst bool) catalog.ResolvedModel {
		first := &generateModel{modelID: "A", generate: func(context.Context) (*provider.GenerateResult, error) {
			time.Sleep(10 * time.Millisecond)
			if failFirst {
				return nil, failing
			}
			return &provider.GenerateResult{}, nil
		}}
		model, err := fallback.New(first, &generateModel{modelID: "B"})
		require.NoError(t, err)
		return catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "A"}, {Provider: "native", ModelID: "B"}}}
	}
	models := map[string]catalog.ResolvedModel{"fails": resolver(true), "succeeds": resolver(false)}
	runtime := newRuntime(t, func(ctx context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (Selection, error) {
		return CatalogSelector(staticResolver{resolved: models[id]})(ctx, id, options, gateway)
	}, testLimits())
	var wg sync.WaitGroup
	overviews := make(map[string]*Overview)
	var mu sync.Mutex
	for range 8 {
		for id := range models {
			wg.Go(func() {
				call, err := runtime.Call(t.Context(), id, provider.CallOptions{})
				if !assert.NoError(t, err) {
					return
				}
				_, err = call.Generate()
				assert.NoError(t, err)
				mu.Lock()
				overviews[id] = call.View(nil).Overview
				mu.Unlock()
			})
		}
	}
	wg.Wait()
	require.Len(t, overviews["fails"].Attempts, 2)
	assert.Equal(t, fallback.AttemptFailed, overviews["fails"].Attempts[0].Outcome)
	assert.Equal(t, fallback.AttemptSelected, overviews["fails"].Attempts[1].Outcome)
	require.Len(t, overviews["succeeds"].Attempts, 1)
	assert.Equal(t, fallback.AttemptSelected, overviews["succeeds"].Attempts[0].Outcome)
}

type drainProbe struct {
	parts    chan provider.StreamPart
	drained  chan struct{}
	canceled chan struct{}
}

func newDrainProbe() *drainProbe {
	probe := &drainProbe{parts: make(chan provider.StreamPart), drained: make(chan struct{}), canceled: make(chan struct{})}
	go func() {
		probe.parts <- provider.StreamPart{Type: provider.PartRaw}
		close(probe.drained)
		close(probe.parts)
	}()
	return probe
}

func (p *drainProbe) watch(ctx context.Context) {
	go func() { <-ctx.Done(); close(p.canceled) }()
}

func requireClosed(t *testing.T, ch <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(testTimeout):
		require.FailNow(t, message)
	}
}

func TestCallStream_Setup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream func(context.Context) (*provider.StreamResult, error)
		err    error
	}{
		{name: "panic", stream: func(context.Context) (*provider.StreamResult, error) { panic("private") }, err: errInternal},
		{name: "nil nil", stream: func(context.Context) (*provider.StreamResult, error) { return nil, nil }, err: errInternal},
		{name: "nil channel", stream: func(context.Context) (*provider.StreamResult, error) { return &provider.StreamResult{}, nil }, err: errInternal},
		{name: "error", stream: func(context.Context) (*provider.StreamResult, error) { return nil, ErrInvalidBYOK }, err: ErrInvalidBYOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newCall(t, t.Context(), &generateModel{stream: tc.stream}, testLimits()).Stream()
			assert.ErrorIs(t, err, tc.err)
		})
	}

	t.Run("errored stream is canceled and drained", func(t *testing.T) {
		probe := newDrainProbe()
		call := newCall(t, t.Context(), &generateModel{stream: func(ctx context.Context) (*provider.StreamResult, error) {
			probe.watch(ctx)
			return &provider.StreamResult{Stream: probe.parts}, errors.New("private")
		}}, testLimits())
		start := time.Now()
		_, err := call.Stream()
		require.Error(t, err)
		assert.Less(t, time.Since(start), testTimeout/2)
		requireClosed(t, probe.canceled, "model context was not canceled")
		requireClosed(t, probe.drained, "errored stream was not drained")
	})

	for _, ending := range endingContexts(t) {
		t.Run("late stream after "+ending.name+" is drained", func(t *testing.T) {
			probe := newDrainProbe()
			call := newCall(t, ending.ctx(), &generateModel{stream: func(ctx context.Context) (*provider.StreamResult, error) {
				<-ctx.Done()
				return &provider.StreamResult{Stream: probe.parts}, nil
			}}, testLimits())
			_, err := call.Stream()
			assert.ErrorIs(t, err, ending.err)
			requireClosed(t, probe.drained, "late stream was not drained")
		})
	}
}

func establish(t *testing.T, ctx context.Context, parts <-chan provider.StreamPart, limits Limits) (*Stream, func() context.Context) {
	t.Helper()
	var modelContext atomic.Value
	call := newCall(t, ctx, &generateModel{stream: func(ctx context.Context) (*provider.StreamResult, error) {
		modelContext.Store(ctx)
		return &provider.StreamResult{Stream: parts}, nil
	}}, limits)
	stream, err := call.Stream()
	require.NoError(t, err)
	t.Cleanup(stream.Close)
	return stream, func() context.Context { return modelContext.Load().(context.Context) }
}

func TestStreamNext_Outcomes(t *testing.T) {
	t.Run("part then closed", func(t *testing.T) {
		parts := make(chan provider.StreamPart, 1)
		parts <- provider.StreamPart{Type: provider.PartTextDelta}
		close(parts)
		stream, model := establish(t, t.Context(), parts, testLimits())
		part, wait := stream.Next()
		assert.Equal(t, WaitPart, wait)
		assert.Equal(t, provider.PartTextDelta, part.Type)
		assert.NoError(t, model().Err())
		_, wait = stream.Next()
		assert.Equal(t, WaitClosed, wait)
		assert.Error(t, model().Err())
	})

	for _, ending := range endingContexts(t) {
		t.Run(ending.name, func(t *testing.T) {
			stream, model := establish(t, ending.ctx(), make(chan provider.StreamPart), testLimits())
			_, wait := stream.Next()
			assert.Equal(t, ending.wait, wait)
			assert.Error(t, model().Err())
			assert.ErrorIs(t, wait.Err(), ending.err)
		})
	}

	t.Run("idle timeout", func(t *testing.T) {
		limits := testLimits()
		limits.StreamIdleDuration = 20 * time.Millisecond
		stream, model := establish(t, t.Context(), make(chan provider.StreamPart), limits)
		_, wait := stream.Next()
		assert.Equal(t, WaitIdleTimeout, wait)
		assert.Error(t, model().Err())
		assert.ErrorIs(t, wait.Err(), context.DeadlineExceeded)
	})

	t.Run("idle includes time before the first wait", func(t *testing.T) {
		limits := testLimits()
		limits.StreamIdleDuration = 20 * time.Millisecond
		parts := make(chan provider.StreamPart, 1)
		stream, _ := establish(t, t.Context(), parts, limits)
		time.Sleep(50 * time.Millisecond)
		_, wait := stream.Next()
		assert.Equal(t, WaitIdleTimeout, wait)
	})

	t.Run("activity resets idle time", func(t *testing.T) {
		limits := testLimits()
		limits.StreamIdleDuration = 100 * time.Millisecond
		parts := make(chan provider.StreamPart)
		go func() {
			defer close(parts)
			for range 6 {
				time.Sleep(50 * time.Millisecond)
				parts <- provider.StreamPart{Type: provider.PartTextDelta}
			}
		}()
		stream, _ := establish(t, t.Context(), parts, limits)
		for range 6 {
			_, wait := stream.Next()
			require.Equal(t, WaitPart, wait)
		}
		_, wait := stream.Next()
		assert.Equal(t, WaitClosed, wait)
	})
}

func TestStreamClose_Drain(t *testing.T) {
	t.Run("provider closes within the drain", func(t *testing.T) {
		parts := make(chan provider.StreamPart, 10)
		for range 10 {
			parts <- provider.StreamPart{Type: provider.PartRaw}
		}
		close(parts)
		stream, model := establish(t, t.Context(), parts, testLimits())
		stream.Close()
		assert.Error(t, model().Err())
		assert.Eventually(t, func() bool { return len(parts) == 0 }, testTimeout, time.Millisecond)
	})

	t.Run("permanently ready stream stops at the drain duration", func(t *testing.T) {
		limits := testLimits()
		limits.StreamDrainDuration = 25 * time.Millisecond
		parts, produced := readyProducer(t)
		stream, _ := establish(t, t.Context(), parts, limits)
		stream.Close()
		require.Eventually(t, func() bool { return produced.Load() > 0 }, testTimeout, time.Millisecond)
		time.Sleep(4 * limits.StreamDrainDuration)
		settled := produced.Load()
		time.Sleep(4 * limits.StreamDrainDuration)
		assert.Equal(t, settled, produced.Load())
	})

	t.Run("repeated close starts no second drain", func(t *testing.T) {
		limits := testLimits()
		limits.StreamDrainDuration = 25 * time.Millisecond
		parts, produced := readyProducer(t)
		stream, _ := establish(t, t.Context(), parts, limits)
		stream.Close()
		time.Sleep(4 * limits.StreamDrainDuration)
		settled := produced.Load()
		stream.Close()
		time.Sleep(4 * limits.StreamDrainDuration)
		assert.Equal(t, settled, produced.Load())
	})
}
