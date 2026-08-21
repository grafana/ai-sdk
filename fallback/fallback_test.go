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

	t.Run("ExactRequestValuesForwardUnchanged", func(t *testing.T) {
		fraction, err := provider.LanguageModelNumberFromFloat64(1.5)
		require.NoError(t, err)
		explicitFalse := false
		empty := ""
		options := provider.CallOptions{
			MaxOutputTokens:  &fraction,
			IncludeRawChunks: &explicitFalse,
			ResponseFormat:   &provider.ResponseFormat{Type: provider.ResponseFormatJSON, Name: &empty},
			StopSequences:    []string{},
		}
		primary := &mockModel{doGenerate: func(_ context.Context, actual provider.CallOptions) (*provider.GenerateResult, error) {
			assert.Equal(t, options, actual)
			assert.NotNil(t, actual.StopSequences)
			return &provider.GenerateResult{}, nil
		}}
		_, err = mustNew(t, primary).DoGenerate(context.Background(), options)
		require.NoError(t, err)
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
		assert.Equal(t, Attempt{Index: 1, Provider: "bedrock", ModelID: "bedrock-model", StartedAt: attempts[0].StartedAt, FinishedAt: attempts[0].FinishedAt, Err: primaryErr}, attempts[0])
		assert.Equal(t, Attempt{Index: 2, Provider: "vertex", ModelID: "vertex-model", StartedAt: attempts[1].StartedAt, FinishedAt: attempts[1].FinishedAt}, attempts[1])
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

func TestDoStream(t *testing.T) {
	t.Run("AttemptObserver", func(t *testing.T) {
		primaryErr := provider.NewAPICallError(provider.APICallErrorOptions{Message: "primary error", StatusCode: 503})
		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: primaryErr}
		close(primaryCh)
		secondaryCh := make(chan provider.StreamPart)
		close(secondaryCh)

		primary := &mockModel{
			providerName: "bedrock",
			modelID:      "bedrock-model",
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			providerName: "vertex",
			modelID:      "vertex-model",
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: secondaryCh}, nil
			},
		}

		var attempts []Attempt
		m := mustNew(t, primary, secondary).WithAttemptObserver(func(_ context.Context, attempt Attempt) {
			attempts = append(attempts, attempt)
		})
		_, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.Len(t, attempts, 2)
		assert.Equal(t, "bedrock", attempts[0].Provider)
		assert.ErrorIs(t, attempts[0].Err, primaryErr)
		assert.Equal(t, "vertex", attempts[1].Provider)
		assert.NoError(t, attempts[1].Err)
	})

	t.Run("PrimarySyncError_SecondarySucceeds", func(t *testing.T) {
		ch := make(chan provider.StreamPart)
		close(ch)

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return nil, errors.New("connection refused")
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		got, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("ContextCancelled_BeforeDoStream", func(t *testing.T) {
		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return nil, errors.New("primary error")
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return nil, errors.New("should not be called")
			},
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		m := mustNew(t, primary, secondary)
		_, err := m.DoStream(ctx, provider.CallOptions{})
		require.Error(t, err)
	})

	t.Run("PrimaryStreamError_SecondarySucceeds", func(t *testing.T) {
		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "model not found", StatusCode: 503})}
		close(primaryCh)

		secondaryCh := make(chan provider.StreamPart, 3)
		secondaryCh <- provider.StreamPart{Type: provider.PartResponseMeta, ModelID: "fallback-model", Provider: "secondary"}
		secondaryCh <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "hello"}
		secondaryCh <- provider.StreamPart{Type: provider.PartFinish}
		close(secondaryCh)

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: secondaryCh}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		got, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range got.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 3)
		assert.Equal(t, provider.PartResponseMeta, parts[0].Type)
		assert.Equal(t, "fallback-model", parts[0].ModelID)
		assert.Equal(t, "secondary", parts[0].Provider)
		assert.Equal(t, provider.PartTextDelta, parts[1].Type)
		assert.Equal(t, "hello", parts[1].Delta)
		assert.Equal(t, provider.PartFinish, parts[2].Type)
	})

	t.Run("PrimaryStreamError_DeciderRejects", func(t *testing.T) {
		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "primary stream error"})}
		close(primaryCh)

		secondaryCalled := false
		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				secondaryCalled = true
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary).WithDecider(func(_ error) bool { return false })
		_, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "primary stream error")
		assert.False(t, secondaryCalled)
	})

	t.Run("StreamDrained_AfterPartError", func(t *testing.T) {
		drained := make(chan struct{})
		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "fail", StatusCode: 503})}

		go func() {
			primaryCh <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "leftover1"}
			primaryCh <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "leftover2"}
			close(primaryCh)
			close(drained)
		}()

		secondaryCh := make(chan provider.StreamPart)
		close(secondaryCh)

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: secondaryCh}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		<-drained
	})

	// Guards against a regression where a malformed PartError (Type=error
	// with APICallError nil) caused the fallback to return (nil, nil) on
	// the final candidate, violating the (value, error) contract. The
	// fallback must synthesize a non-retryable APICallError and surface it.
	t.Run("MalformedPartError_NilAPICallError_SynthesizesError", func(t *testing.T) {
		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError /* APICallError intentionally nil */}
		close(primaryCh)

		secondaryCh := make(chan provider.StreamPart, 1)
		secondaryCh <- provider.StreamPart{Type: provider.PartError /* APICallError intentionally nil */}
		close(secondaryCh)

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: secondaryCh}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.Error(t, err, "fallback must surface a synthesized error rather than (nil, nil)")
		var apiErr *provider.APICallError
		require.ErrorAs(t, err, &apiErr)
		assert.Contains(t, apiErr.Error(), "without APICallError details")
	})

	t.Run("AllCandidatesStreamError", func(t *testing.T) {
		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "primary error", StatusCode: 503})}
		close(primaryCh)

		secondaryCh := make(chan provider.StreamPart, 1)
		secondaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "secondary error", StatusCode: 503})}
		close(secondaryCh)

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: secondaryCh}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "secondary error")
	})

	t.Run("ValidFirstChunk_Replayed", func(t *testing.T) {
		ch := make(chan provider.StreamPart, 3)
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "first"}
		ch <- provider.StreamPart{Type: provider.PartTextDelta, Delta: "second"}
		ch <- provider.StreamPart{Type: provider.PartFinish}
		close(ch)

		secondaryCalled := false
		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				secondaryCalled = true
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary)
		got, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		assert.False(t, secondaryCalled)

		var parts []provider.StreamPart
		for p := range got.Stream {
			parts = append(parts, p)
		}
		require.Len(t, parts, 3)
		assert.Equal(t, "first", parts[0].Delta)
		assert.Equal(t, "second", parts[1].Delta)
		assert.Equal(t, provider.PartFinish, parts[2].Type)
	})

	t.Run("EmptyStream", func(t *testing.T) {
		ch := make(chan provider.StreamPart)
		close(ch)

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: ch}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary)
		got, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)

		var parts []provider.StreamPart
		for p := range got.Stream {
			parts = append(parts, p)
		}
		assert.Empty(t, parts)
	})

	t.Run("StreamError_ContextLength_DefaultDeciderRejects", func(t *testing.T) {
		ctxLenErr := provider.NewAPICallError(provider.APICallErrorOptions{
			Message:    "context length exceeded",
			StatusCode: 400,
		})

		primaryCh := make(chan provider.StreamPart, 1)
		primaryCh <- provider.StreamPart{Type: provider.PartError, APICallError: ctxLenErr}
		close(primaryCh)

		secondaryCalled := false
		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: primaryCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				secondaryCalled = true
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Equal(t, "aisdk: API call error (status 400): context length exceeded", err.Error())
		assert.False(t, secondaryCalled)
	})

	t.Run("ContextCancelled_DuringPeek", func(t *testing.T) {
		blockCh := make(chan provider.StreamPart)
		defer close(blockCh)

		ctx, cancel := context.WithCancel(context.Background())

		primary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				cancel()
				return &provider.StreamResult{Stream: blockCh}, nil
			},
		}
		secondary := &mockModel{
			doStream: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return nil, errors.New("should not be called")
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoStream(ctx, provider.CallOptions{})
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
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
