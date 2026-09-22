package fallback

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"

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
		assert.Equal(t, Attempt{Index: 1, Provider: "bedrock", ModelID: "bedrock-model", StartedAt: attempts[0].StartedAt, FinishedAt: attempts[0].FinishedAt, Err: primaryErr, Outcome: AttemptFailed, WillFallback: true}, attempts[0])
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
			got := defaultDecider(tt.err)
			assert.Equal(t, tt.wantFall, got)
		})
	}
}
