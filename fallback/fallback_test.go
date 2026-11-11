package fallback

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockModel struct {
	providerName string
	modelID      string
	doGenerate   func(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error)
	doStream     func(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error)
}

func (m *mockModel) SpecificationVersion() string               { return "v4" }
func (m *mockModel) Provider() string                           { return m.providerName }
func (m *mockModel) ModelID() string                            { return m.modelID }
func (m *mockModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }

func (m *mockModel) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	return m.doGenerate(ctx, params)
}

func (m *mockModel) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	return m.doStream(ctx, params)
}

func mustNew(t *testing.T, candidates ...provider.LanguageModel) *Model {
	t.Helper()
	m, err := New(candidates...)
	require.NoError(t, err)
	return m
}

func TestNew_ErrorOnZeroCandidates(t *testing.T) {
	_, err := New()
	require.ErrorIs(t, err, ErrNoCandidates)
}

func TestDoGenerate(t *testing.T) {
	t.Run("MetadataDelegation", func(t *testing.T) {
		primary := &mockModel{providerName: "anthropic", modelID: "claude-sonnet-4-6"}
		secondary := &mockModel{providerName: "anthropic.vertex", modelID: "claude-sonnet-4-6"}

		m := mustNew(t, primary, secondary)

		assert.Equal(t, "anthropic", m.Provider())
		assert.Equal(t, "claude-sonnet-4-6", m.ModelID())
		assert.Equal(t, "v4", m.SpecificationVersion())
	})

	t.Run("ForwardsServingCandidateProvider", func(t *testing.T) {
		// Primary fails with a retryable error; the secondary (Vertex) serves
		// the request. The fallback must forward the serving candidate's
		// response metadata verbatim, so the served provider is observable.
		primary := &mockModel{
			providerName: "anthropic",
			modelID:      "claude-sonnet-4-6",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, provider.NewAPICallError(provider.APICallErrorOptions{Message: "overloaded", StatusCode: 503})
			},
		}
		secondary := &mockModel{
			providerName: "anthropic.vertex",
			modelID:      "claude-sonnet-4-8",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					Response: &provider.GenerateResponse{
						ResponseMetadata: provider.ResponseMetadata{ModelID: "claude-sonnet-4-8", Provider: "anthropic.vertex"},
					},
				}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		result, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.Response)
		assert.Equal(t, "anthropic.vertex", result.Response.Provider, "served (fallback) provider must be forwarded")
		assert.Equal(t, "claude-sonnet-4-8", result.Response.ModelID, "served (fallback) model must be forwarded")
	})

	t.Run("PrimarySucceeds", func(t *testing.T) {
		result := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
		primary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return result, nil
			},
		}
		secondaryCalled := false
		secondary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				secondaryCalled = true
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary)
		got, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Equal(t, result, got, "expected primary result")
		assert.False(t, secondaryCalled, "secondary should not be called")
	})

	t.Run("PrimaryFails_SecondarySucceeds", func(t *testing.T) {
		result := &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
		primary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("connection refused")
			},
		}
		secondary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return result, nil
			},
		}

		m := mustNew(t, primary, secondary)
		got, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.Equal(t, result, got, "expected secondary result")
	})

	t.Run("DeciderRejectsFallback", func(t *testing.T) {
		primary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("primary error")
			},
		}
		secondaryCalled := false
		secondary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				secondaryCalled = true
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary).WithDecider(func(_ error) bool { return false })
		_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Equal(t, "primary error", err.Error())
		assert.False(t, secondaryCalled, "secondary should not be called when decider rejects")
	})

	t.Run("AllFail_PreservesEveryAttempt", func(t *testing.T) {
		primaryErr := errors.New("primary error")
		secondaryErr := errors.New("secondary error")
		primary := &mockModel{
			providerName: "bedrock",
			modelID:      "bedrock-model",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, primaryErr
			},
		}
		secondary := &mockModel{
			providerName: "vertex",
			modelID:      "vertex-model",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, secondaryErr
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.ErrorIs(t, err, primaryErr)
		assert.ErrorIs(t, err, secondaryErr)
		assert.Contains(t, err.Error(), `provider "vertex" model "vertex-model": secondary error`)
		assert.Contains(t, err.Error(), `provider "bedrock" model "bedrock-model": primary error`)
	})

	t.Run("AttemptObserver", func(t *testing.T) {
		primaryErr := errors.New("primary error")
		primary := &mockModel{
			providerName: "bedrock",
			modelID:      "bedrock-model",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, primaryErr
			},
		}
		secondary := &mockModel{
			providerName: "vertex",
			modelID:      "vertex-model",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{}, nil
			},
		}

		var attempts []Attempt
		m := mustNew(t, primary, secondary).WithAttemptObserver(func(_ context.Context, attempt Attempt) {
			attempts = append(attempts, attempt)
		})
		_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.Len(t, attempts, 2)
		assert.Equal(t, Attempt{Index: 1, Provider: "bedrock", ModelID: "bedrock-model", StartedAt: attempts[0].StartedAt, FinishedAt: attempts[0].FinishedAt, Err: primaryErr, SourceErr: primaryErr, Outcome: AttemptFailed, WillFallback: true}, attempts[0])
		assert.Equal(t, Attempt{Index: 2, Provider: "vertex", ModelID: "vertex-model", StartedAt: attempts[1].StartedAt, FinishedAt: attempts[1].FinishedAt, Outcome: AttemptSelected}, attempts[1])
		assert.False(t, attempts[0].FinishedAt.Before(attempts[0].StartedAt))
		assert.False(t, attempts[1].FinishedAt.Before(attempts[1].StartedAt))
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		primary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("primary error")
			},
		}
		secondary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("should not be called")
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		m := mustNew(t, primary, secondary)
		_, err := m.DoGenerate(ctx, provider.CallOptions{})
		require.Error(t, err)
	})
}

func TestDefaultDecider(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantFall bool
	}{
		{
			"retryable APICallError triggers fallback",
			provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "rate limit exceeded",
				StatusCode: 429,
			}),
			true,
		},
		{
			"non-retryable APICallError stops fallback",
			provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "bad request",
				StatusCode: 400,
			}),
			false,
		},
		{
			"non-APICallError triggers fallback",
			errors.New("connection refused"),
			true,
		},
		{
			"400 context-length class stops fallback",
			provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "context length exceeded",
				StatusCode: 400,
			}),
			false,
		},
		{
			"400 context-window signal in Data stops fallback",
			func() error {
				e := provider.NewAPICallError(provider.APICallErrorOptions{
					Message:    "invalid request",
					StatusCode: 400,
				})
				e.Data = json.RawMessage(`{"error":{"type":"invalid_request_error","message":"prompt is too long: maximum context length is 200000 tokens"}}`)
				return e
			}(),
			false,
		},
		{
			"400 generic bad request without context signal stops fallback (non-retryable)",
			provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "missing field",
				StatusCode: 400,
			}),
			false,
		},
		{
			"stream buffer limit stops fallback",
			fmt.Errorf("candidate: %w", ErrStreamBufferLimit),
			false,
		},
		{
			"unknown error triggers fallback",
			errors.New("unknown failure"),
			true,
		},
		{
			"500 server error triggers fallback",
			provider.NewAPICallError(provider.APICallErrorOptions{
				Message:    "internal server error",
				StatusCode: 500,
			}),
			true,
		},
		{
			"wire-reconstructed retryable APICallError triggers fallback",
			func() error {
				orig := provider.NewAPICallError(provider.APICallErrorOptions{
					Message:    "rate limited",
					StatusCode: 429,
				})
				data, err := json.Marshal(orig)
				require.NoError(t, err)
				var rebuilt provider.APICallError
				require.NoError(t, json.Unmarshal(data, &rebuilt))
				return &rebuilt
			}(),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDecider(tt.err)
			assert.Equal(t, tt.wantFall, got)
		})
	}
}

func rejectionFor(name string) error {
	return fmt.Errorf("%s: unsupported tool content part type: custom: %w", name, provider.ErrUnsupportedFunctionality)
}

type rejectingCandidate struct {
	*mockModel
	generateCalls *int
	streamCalls   *int
}

func newRejecting(name string) rejectingCandidate {
	generateCalls, streamCalls := new(int), new(int)
	err := rejectionFor(name)
	return rejectingCandidate{
		generateCalls: generateCalls,
		streamCalls:   streamCalls,
		mockModel: &mockModel{
			providerName: name,
			modelID:      name + "-model",
			doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				*generateCalls++
				return nil, err
			},
			doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				*streamCalls++
				return nil, err
			},
		},
	}
}

func succeedingCandidate() *mockModel {
	return &mockModel{
		providerName: "ok",
		modelID:      "ok-model",
		doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		},
		doStream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			parts := make(chan provider.StreamPart, 1)
			parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
			close(parts)
			return &provider.StreamResult{Stream: parts}, nil
		},
	}
}

func TestUnsupportedFunctionality_Fallback(t *testing.T) {
	modes := []struct {
		name  string
		run   func(*Model) error
		calls func(rejectingCandidate) int
		setup func(*testing.T, *Model) *Model
	}{
		{
			name: "unary",
			run: func(m *Model) error {
				_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
				return err
			},
			calls: func(c rejectingCandidate) int { return *c.generateCalls },
		},
		{
			name: "stream setup",
			run: func(m *Model) error {
				result, err := m.DoStream(context.Background(), provider.CallOptions{})
				if err == nil {
					for range result.Stream {
					}
				}
				return err
			},
			calls: func(c rejectingCandidate) int { return *c.streamCalls },
		},
		{
			name: "buffered stream setup",
			run: func(m *Model) error {
				result, err := m.DoStream(context.Background(), provider.CallOptions{})
				if err == nil {
					for range result.Stream {
					}
				}
				return err
			},
			calls: func(c rejectingCandidate) int { return *c.streamCalls },
			setup: func(t *testing.T, m *Model) *Model {
				buffered, err := m.WithBufferedStream(1<<20, time.Second)
				require.NoError(t, err)
				return buffered
			},
		},
	}

	for _, mode := range modes {
		setup := func(t *testing.T, m *Model) *Model {
			if mode.setup == nil {
				return m
			}
			return mode.setup(t, m)
		}

		t.Run(mode.name+"/default decider advances to a succeeding candidate", func(t *testing.T) {
			rejecting := newRejecting("bedrock")
			model := setup(t, mustNew(t, rejecting, succeedingCandidate()))

			require.NoError(t, mode.run(model))
			assert.Equal(t, 1, mode.calls(rejecting))
		})

		t.Run(mode.name+"/all candidates rejecting keeps every identity", func(t *testing.T) {
			first, second := newRejecting("first"), newRejecting("second")
			model := setup(t, mustNew(t, first, second))

			err := mode.run(model)

			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
			assert.Equal(t, 1, mode.calls(first))
			assert.Equal(t, 1, mode.calls(second))
			assert.Contains(t, err.Error(), "first: unsupported tool content part type: custom")
			assert.Contains(t, err.Error(), "second: unsupported tool content part type: custom")
		})

		t.Run(mode.name+"/custom decider returning false stops", func(t *testing.T) {
			rejecting := newRejecting("bedrock")
			next := newRejecting("next")
			model := setup(t, mustNew(t, rejecting, next).WithDecider(func(error) bool { return false }))

			err := mode.run(model)

			require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
			assert.Equal(t, 1, mode.calls(rejecting))
			assert.Zero(t, mode.calls(next))
		})
	}
}

func TestWithAttemptObserver_RequestIsolationAndIndependentPanics(t *testing.T) {
	model := mustNew(t, &mockModel{providerName: "native", modelID: "model", doGenerate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{}, nil
	}})
	model.WithAttemptObserver(func(context.Context, Attempt) { panic("operator observer failure") })
	for range 2 {
		var attempts []Attempt
		ctx := WithAttemptObserver(context.Background(), func(_ context.Context, attempt Attempt) { attempts = append(attempts, attempt) })
		_, err := model.DoGenerate(ctx, provider.CallOptions{})
		require.NoError(t, err)
		require.Len(t, attempts, 1)
		assert.Equal(t, AttemptSelected, attempts[0].Outcome)
	}
	calls := 0
	model.WithAttemptObserver(func(context.Context, Attempt) { calls++ })
	ctx := WithAttemptObserver(context.Background(), func(context.Context, Attempt) { panic("consumer observer failure") })
	_, err := model.DoGenerate(ctx, provider.CallOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}
