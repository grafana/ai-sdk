package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestSelection_CatalogIndependent(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(strconv.FormatBool(streaming), func(t *testing.T) {
			model := &recordingModel{}
			var selections atomic.Int32
			h, err := New(Config{Limits: testLimits(), Selector: func(_ context.Context, id string, options provider.CallOptions, gateway json.RawMessage) (Selection, error) {
				selections.Add(1)
				assert.Equal(t, "openai/native-model", id)
				assert.JSONEq(t, `{"byok":{"openai":[{"apiKey":"dummy-key"}]}}`, string(gateway))
				assert.NotContains(t, options.ProviderOptions, "gateway")
				assert.Contains(t, options.ProviderOptions, "openai")
				return Selection{ID: id, Model: model, Options: options}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[],"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"dummy-key"}]}},"openai":{"store":false}}}`)
			r.Header.Set(HeaderModelID, "openai/native-model")
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, int32(1), selections.Load())
			generate, stream := model.invocationCounts()
			if streaming {
				assert.Equal(t, 1, stream)
				assert.Zero(t, generate)
			} else {
				assert.Equal(t, 1, generate)
				assert.Zero(t, stream)
			}
			assert.NotContains(t, model.receivedOptions().ProviderOptions, "gateway")
			assert.NotContains(t, w.Body.String(), "dummy-key")
		})
	}
}

func TestRequestSelection_Deadline(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run("late/"+strconv.FormatBool(streaming), func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = 30 * time.Millisecond
			model := &recordingModel{}
			release, selected := make(chan struct{}), make(chan struct{})
			h, err := New(Config{Limits: limits, Selector: func(_ context.Context, id string, options provider.CallOptions, _ json.RawMessage) (Selection, error) {
				defer close(selected)
				<-release
				return Selection{ID: id, Model: model, Options: options}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[]}`)
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			w := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); h.ServeHTTP(w, r) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				close(release)
				require.FailNow(t, "selection exceeded handler latency bound")
			}
			assert.Equal(t, http.StatusGatewayTimeout, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
			assert.Zero(t, model.callCount())
			close(release)
			<-selected
			assert.Zero(t, model.callCount())
		})
		t.Run("shared/"+strconv.FormatBool(streaming), func(t *testing.T) {
			limits := testLimits()
			limits.ModelDuration = 150 * time.Millisecond
			var selectionDeadline time.Time
			model := &recordingModel{
				generate: func(ctx context.Context, _ provider.CallOptions) (*provider.GenerateResult, error) {
					deadline, ok := ctx.Deadline()
					assert.True(t, ok)
					assert.Equal(t, selectionDeadline, deadline)
					return validGenerateResult(), nil
				},
				stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
					deadline, ok := ctx.Deadline()
					assert.True(t, ok)
					assert.Equal(t, selectionDeadline, deadline)
					parts := make(chan provider.StreamPart, 1)
					parts <- finishPart()
					close(parts)
					return &provider.StreamResult{Stream: parts}, nil
				},
			}
			h, err := New(Config{Limits: limits, Selector: func(ctx context.Context, id string, options provider.CallOptions, _ json.RawMessage) (Selection, error) {
				var ok bool
				selectionDeadline, ok = ctx.Deadline()
				assert.True(t, ok)
				select {
				case <-time.After(30 * time.Millisecond):
				case <-ctx.Done():
					return Selection{}, ctx.Err()
				}
				assert.LessOrEqual(t, time.Until(selectionDeadline), limits.ModelDuration-20*time.Millisecond)
				return Selection{ID: id, Model: model, Options: options}, nil
			}})
			require.NoError(t, err)
			r := validRequest(`{"prompt":[]}`)
			r.Header.Set(HeaderStreaming, strconv.FormatBool(streaming))
			ctx, cancel := context.WithTimeout(r.Context(), 500*time.Millisecond)
			defer cancel()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r.WithContext(ctx))
			assert.Equal(t, http.StatusOK, w.Code)
			deadline, _ := ctx.Deadline()
			assert.True(t, selectionDeadline.Before(deadline))
		})
	}
}
