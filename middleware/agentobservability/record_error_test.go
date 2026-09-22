package agentobservability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFinishRecording_ReportsValidationFailure(t *testing.T) {
	env := testkit.NewEnv(t)
	_, recorder := env.Client.StartGeneration(context.Background(), agento11y.GenerationStart{
		Model: agento11y.ModelRef{Provider: "grafana", Name: "grafana/assistant"},
	})
	recorder.SetResult(agento11y.Generation{
		Input: []agento11y.Message{{Role: agento11y.RoleUser}},
		Output: []agento11y.Message{{
			Role:  agento11y.RoleAssistant,
			Parts: []agento11y.Part{agento11y.TextPart("ok")},
		}},
	}, nil)

	calls := 0
	finishRecording(recorder, func(err error) {
		calls++
		assert.ErrorIs(t, err, agento11y.ErrValidationFailed)
	})

	assert.Equal(t, 1, calls)
}

func TestFinishRecording_SuccessAndNilHandler(t *testing.T) {
	env := testkit.NewEnv(t)
	_, recorder := env.Client.StartGeneration(context.Background(), agento11y.GenerationStart{
		Model: agento11y.ModelRef{Provider: "grafana", Name: "grafana/assistant"},
	})
	recorder.SetResult(agento11y.Generation{
		Input:  []agento11y.Message{{Role: agento11y.RoleUser, Parts: []agento11y.Part{agento11y.TextPart("hi")}}},
		Output: []agento11y.Message{{Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.TextPart("ok")}}},
	}, nil)

	calls := 0
	finishRecording(recorder, func(error) { calls++ })
	assert.Zero(t, calls)
	finishRecording(recorder, nil)
}

func TestFinishRecording_HandlerPanicIsFailOpen(t *testing.T) {
	env := testkit.NewEnv(t)
	_, recorder := env.Client.StartGeneration(context.Background(), agento11y.GenerationStart{
		Model: agento11y.ModelRef{Provider: "grafana", Name: "grafana/assistant"},
	})
	recorder.SetResult(agento11y.Generation{
		Input:  []agento11y.Message{{Role: agento11y.RoleUser}},
		Output: []agento11y.Message{{Role: agento11y.RoleAssistant, Parts: []agento11y.Part{agento11y.TextPart("ok")}}},
	}, nil)

	require.NotPanics(t, func() {
		finishRecording(recorder, func(err error) {
			require.Error(t, err)
			panic(errors.New("private callback panic"))
		})
	})
}

func TestRecordingMiddleware_RecordErrorHandlerIsFailOpen(t *testing.T) {
	for _, mode := range []string{"generate", "stream"} {
		t.Run(mode, func(t *testing.T) {
			env := testkit.NewEnv(t)
			require.NoError(t, env.Client.Shutdown(context.Background()))
			model := &mockLanguageModel{provider_: "grafana", modelID: "grafana/assistant"}
			observed := make(chan error, 1)
			completed := make(chan struct{}, 1)
			opts := RecordingOptions{
				ClientResolver:  func(context.Context) *agento11y.Client { return env.Client },
				ContextProvider: func(context.Context) ContextInfo { return ContextInfo{} },
				OnRecordError: func(err error) {
					observed <- err
					panic(errors.New("private callback panic"))
				},
				OnRecordComplete: func() {
					completed <- struct{}{}
					panic(errors.New("private completion callback panic"))
				},
			}

			if mode == "generate" {
				result, err := generateWith(t, model, opts)
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				result, err := streamWith(t, model, opts)
				require.NoError(t, err)
				require.NotNil(t, result)
				for range result.Stream {
				}
			}

			select {
			case err := <-observed:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("record error handler was not called")
			}
			select {
			case <-completed:
			case <-time.After(time.Second):
				t.Fatal("record completion handler was not called")
			}
			select {
			case <-completed:
				t.Fatal("record completion handler was called more than once")
			default:
			}
		})
	}
}

func TestRecordingMiddleware_StreamFinalizesBeforeEOFWithoutCallbackBackpressure(t *testing.T) {
	env := testkit.NewEnv(t)
	require.NoError(t, env.Client.Shutdown(context.Background()))
	upstream := make(chan provider.StreamPart)
	model := &mockLanguageModel{
		provider_: "grafana",
		modelID:   "grafana/assistant",
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: upstream}, nil
		},
	}
	completionStarted := make(chan struct{})
	releaseCompletion := make(chan struct{})
	errorStarted := make(chan struct{})
	releaseError := make(chan struct{})
	wrapped := middleware.Wrap(middleware.WrapOptions{Model: model, Middleware: []middleware.Middleware{RecordingMiddleware(RecordingOptions{
		ClientResolver: func(context.Context) *agento11y.Client { return env.Client },
		OnRecordError: func(error) {
			close(errorStarted)
			<-releaseError
		},
		OnRecordComplete: func() {
			close(completionStarted)
			<-releaseCompletion
		},
	})}})
	result, err := wrapped.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	received := make(chan struct{})
	eof := make(chan struct{})
	go func() {
		<-result.Stream
		close(received)
		for range result.Stream {
		}
		close(eof)
	}()
	upstream <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "value"}
	<-received
	close(upstream)
	select {
	case <-completionStarted:
	case <-time.After(time.Second):
		t.Fatal("record completion handler did not start")
	}
	select {
	case <-eof:
	case <-time.After(time.Second):
		t.Fatal("record completion callback delayed downstream EOF")
	}
	close(releaseCompletion)
	select {
	case <-errorStarted:
	case <-time.After(time.Second):
		t.Fatal("record error handler did not start")
	}
	close(releaseError)
}
