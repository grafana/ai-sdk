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
			modelID:      "claude-sonnet-4-6",
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return &provider.GenerateResult{
					FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop},
					Response: &provider.GenerateResponse{
						ResponseMetadata: provider.ResponseMetadata{ModelID: "claude-sonnet-4-6", Provider: "anthropic.vertex"},
					},
				}, nil
			},
		}

		m := mustNew(t, primary, secondary)
		result, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		require.NotNil(t, result.Response)
		assert.Equal(t, "anthropic.vertex", result.Response.Provider, "served (fallback) provider must be forwarded")
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

	t.Run("AllFail_ReturnsLastError", func(t *testing.T) {
		primary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("primary error")
			},
		}
		secondary := &mockModel{
			doGenerate: func(_ context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
				return nil, errors.New("secondary error")
			},
		}

		m := mustNew(t, primary, secondary)
		_, err := m.DoGenerate(context.Background(), provider.CallOptions{})
		require.Error(t, err)
		assert.Equal(t, "secondary error", err.Error())
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

		secondaryCh := make(chan provider.StreamPart, 2)
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
		require.Len(t, parts, 2)
		assert.Equal(t, provider.PartTextDelta, parts[0].Type)
		assert.Equal(t, "hello", parts[0].Delta)
		assert.Equal(t, provider.PartFinish, parts[1].Type)
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
