package v4

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderMetadata_Lifecycle(t *testing.T) {
	for _, mode := range []string{"cancellation", "idle", "total", "writer"} {
		t.Run(mode, func(t *testing.T) {
			limits := testLimits()
			if mode == "idle" {
				limits.StreamIdleDuration = 10 * time.Millisecond
				limits.ModelDuration = time.Second
			}
			if mode == "total" {
				limits.ModelDuration = 30 * time.Millisecond
				limits.StreamIdleDuration = time.Second
			}
			h := newRuntimeHarness(t, limits)
			stopped := make(chan struct{})
			h.model.stream = func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				stream := make(chan provider.StreamPart, 1)
				stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text", ProviderMetadata: provider.ProviderMetadata{"future": json.RawMessage(`{"opaque":true}`)}}
				go func() { defer close(stopped); defer close(stream); <-ctx.Done() }()
				return &provider.StreamResult{Stream: stream}, nil
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			w := &responseWriterProbe{}
			w.onWrite = func() {
				if w.writes == 2 {
					if mode == "cancellation" {
						cancel()
					}
					if mode == "writer" {
						w.writeErr = errors.New("test writer failure")
					}
				}
			}
			h.handler.ServeHTTP(w, streamRequest(`{"prompt":[]}`).WithContext(ctx))
			select {
			case <-stopped:
			case <-time.After(time.Second):
				require.FailNow(t, "provider was not canceled")
			}
			if mode == "writer" {
				assert.Equal(t, 2, w.writes)
				assert.NotContains(t, w.body.String(), "future")
			} else {
				assert.Contains(t, w.body.String(), `"future":{"opaque":true}`)
				assert.Equal(t, 1, strings.Count(w.body.String(), `"type":"error"`))
			}
			assert.NotContains(t, w.body.String(), `"type":"finish"`)
		})
	}
	t.Run("bounded drain after finish", func(t *testing.T) {
		limits := testLimits()
		limits.StreamParts = 1_000_000
		limits.StreamDrainDuration = 25 * time.Millisecond
		h := newRuntimeHarness(t, limits)
		stop := make(chan struct{})
		done := make(chan struct{})
		h.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart)
			go func() {
				defer close(done)
				defer close(stream)
				finish := finishPart()
				finish.ProviderMetadata = provider.ProviderMetadata{"future": json.RawMessage(`{"final":true}`)}
				select {
				case stream <- finish:
				case <-stop:
					return
				}
				for {
					select {
					case stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "late-private", ProviderMetadata: provider.ProviderMetadata{"future": json.RawMessage(`null`)}}:
					case <-stop:
						return
					}
				}
			}()
			return &provider.StreamResult{Stream: stream}, nil
		}
		t.Cleanup(func() { close(stop); <-done })
		start := time.Now()
		body := h.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		assert.Less(t, time.Since(start), time.Second)
		assert.Contains(t, body, `"future":{"final":true}`)
		assert.Equal(t, 1, strings.Count(body, `"type":"finish"`))
		assert.NotContains(t, body, "late-private")
		assert.NotContains(t, body, `"type":"error"`)
	})
	t.Run("part limit", func(t *testing.T) {
		limits := testLimits()
		limits.StreamParts = 2
		h := newRuntimeHarness(t, limits)
		metadata := provider.ProviderMetadata{"future": json.RawMessage(`{}`)}
		h.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartTextStart, ID: "text", ProviderMetadata: metadata}, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "answer", ProviderMetadata: metadata}, provider.StreamPart{Type: provider.PartTextEnd, ID: "text", ProviderMetadata: metadata}, finishPart())}, nil
		}
		body := h.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		assert.Contains(t, body, `"future":{}`)
		assert.Equal(t, 1, strings.Count(body, `"type":"error"`))
		assert.NotContains(t, body, `"type":"finish"`)
	})
}

func TestProviderMetadata_SharedModelIsolation(t *testing.T) {
	h := newRuntimeHarness(t, testLimits())
	h.model.generate = func(_ context.Context, options provider.CallOptions) (*provider.GenerateResult, error) {
		marker := options.Prompt[0].Content[0].Text
		raw, err := json.Marshal(map[string]any{"tenantMarker": marker, "routing": "ignored", "apiKey": "sk-application-data"})
		if err != nil {
			return nil, err
		}
		result := validGenerateResult()
		result.ProviderMetadata = provider.ProviderMetadata{"future": raw}
		result.Content[0].ProviderMetadata = result.ProviderMetadata
		return result, nil
	}
	var wg sync.WaitGroup
	const requests = 20
	for i := range requests {
		wg.Go(func() {
			marker := fmt.Sprintf("tenant-%d", i)
			w := httptest.NewRecorder()
			h.handler.ServeHTTP(w, validRequest(`{"prompt":[{"role":"user","content":[{"type":"text","text":"`+marker+`"}]}]}`))
			assert.Equal(t, 200, w.Code)
			var decoded struct {
				Metadata provider.ProviderMetadata `json:"providerMetadata"`
			}
			if assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &decoded)) {
				raw := string(decoded.Metadata["future"])
				assert.Contains(t, raw, `"tenantMarker":"`+marker+`"`)
				assert.Contains(t, raw, `"routing":"ignored"`)
				assert.Contains(t, raw, "sk-application-data")
			}
		})
	}
	wg.Wait()
	assert.Equal(t, requests, h.resolver.callCount())
}
