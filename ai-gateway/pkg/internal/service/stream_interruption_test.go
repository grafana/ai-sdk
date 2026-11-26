package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/anthropic/messages"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamInterruption_AdapterBoundary(t *testing.T) {
	for _, tc := range []struct {
		name         string
		balanced     bool
		observe      bool
		denyOnFinish bool
	}{
		{name: "open block"},
		{name: "closed block", balanced: true},
		{name: "open block with observer", observe: true},
		{name: "open block with finish inspector", denyOnFinish: true},
		{name: "closed block with finish inspector", balanced: true, denyOnFinish: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := []provider.StreamPart{
				{Type: provider.PartResponseMeta, ResponseID: "first"},
				{Type: provider.PartTextStart, ID: "t"},
				{Type: provider.PartTextDelta, ID: "t", Delta: "partial-answer"},
			}
			if tc.balanced {
				parts = append(parts, provider.StreamPart{Type: provider.PartTextEnd, ID: "t"})
			}
			parts = append(parts,
				provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonInterrupted}, Usage: &provider.Usage{}},
				provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "second"},
				provider.StreamPart{Type: provider.PartTextStart, ID: "u"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "u", Delta: "later-answer"},
				provider.StreamPart{Type: provider.PartTextEnd, ID: "u"},
				provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}},
			)
			model := &observabilityTestModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				ch := make(chan provider.StreamPart, len(parts))
				for _, part := range parts {
					ch <- part
				}
				close(ch)
				return &provider.StreamResult{Stream: ch}, nil
			}}
			models, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "model"}, Model: model}})
			require.NoError(t, err)
			selector := execution.CatalogSelector(models)
			runtimeForAdapter := func(t *testing.T, acceptsOpenBlocks bool) *execution.Runtime {
				t.Helper()
				if !tc.observe && !tc.denyOnFinish {
					runtime, err := execution.New(execution.Config{Limits: execution.DefaultLimits, Selector: selector})
					require.NoError(t, err)
					return runtime
				}
				inspector := &interruptFinishInspector{denyFinish: tc.denyOnFinish, outcomes: make(chan extension.Outcome, 2)}
				observed, host := newExtensionRuntime(t, selector, inspector)
				t.Cleanup(func() {
					defer func() {
						ctx, cancel := context.WithTimeout(context.Background(), time.Second)
						defer cancel()
						host.Close(ctx)
						assert.Empty(t, inspector.outcomes, "extra observations")
					}()
					wantOutcome, wantFinishes := extension.OutcomeInvalidOutput, int32(0)
					if acceptsOpenBlocks || tc.balanced {
						wantOutcome, wantFinishes = extension.OutcomeSuccess, 2
						if tc.denyOnFinish {
							wantOutcome, wantFinishes = extension.OutcomeDenied, 1
						}
					}
					select {
					case outcome := <-inspector.outcomes:
						assert.Equal(t, wantOutcome, outcome)
					case <-time.After(time.Second):
						t.Fatal("extension observation not delivered")
					}
					assert.Equal(t, wantFinishes, inspector.finishes.Load())
				})
				return observed.runtime
			}

			t.Run("ProviderWire", func(t *testing.T) {
				handler, err := providerv4.New(runtimeForAdapter(t, true))
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[]}`))
				req.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
				req.Header.Set(providerv4.HeaderModelID, "model")
				req.Header.Set(providerv4.HeaderStreaming, "true")
				body := serveInterruption(t, handler, req)
				if tc.denyOnFinish {
					assert.NotContains(t, body, "later-answer")
					assert.NotContains(t, body, `"type":"finish"`)
					assert.True(t, strings.HasSuffix(body, "data: "+`{"type":"error","error":{"message":"forbidden","type":"forbidden","param":null,"code":"forbidden","statusCode":403,"retryable":false}}`+"\n\n"), body)
					return
				}
				beforeFinish, afterFinish, found := strings.Cut(body, `"unified":"interrupted"`)
				require.True(t, found, body)
				assert.Equal(t, tc.balanced, strings.Contains(beforeFinish, `"type":"text-end","id":"t"`))
				assert.Contains(t, afterFinish, `"delta":"later-answer"`)
				assert.Contains(t, afterFinish, `"unified":"stop"`)
				assert.NotContains(t, body, `"type":"error"`)
			})

			t.Run("Anthropic Messages", func(t *testing.T) {
				handler, err := messages.New(runtimeForAdapter(t, false))
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, messages.Path, strings.NewReader(`{"model":"model","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"stream":true}`))
				body := serveInterruption(t, handler, req)
				if tc.balanced && !tc.denyOnFinish {
					assert.Contains(t, body, "later-answer")
					assert.Contains(t, body, "event: message_stop\n")
					assert.NotContains(t, body, "event: error\n")
					return
				}
				assert.NotContains(t, body, "later-answer")
				assert.NotContains(t, body, "event: message_stop\n")
				want := `{"type":"error","error":{"type":"api_error","message":"internal error"}}`
				if tc.balanced {
					want = `{"type":"error","error":{"type":"permission_error","message":"permission denied"}}`
				} else {
					assert.NotContains(t, body, "event: content_block_stop\n")
				}
				assert.True(t, strings.HasSuffix(body, "event: error\ndata: "+want+"\n\n"), body)
			})

			t.Run("Chat Completions", func(t *testing.T) {
				handler, err := chatcompletions.New(runtimeForAdapter(t, false))
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, chatcompletions.Path, strings.NewReader(`{"model":"model","messages":[{"role":"user","content":"hi"}],"stream":true}`))
				body := serveInterruption(t, handler, req)
				assert.NotContains(t, body, "later-answer")
				assert.NotContains(t, body, "[DONE]")
				want := `{"error":{"message":"internal error","type":"server_error","param":null,"code":"internal_error"}}`
				switch {
				case tc.balanced && tc.denyOnFinish:
					want = `{"error":{"message":"permission denied","type":"permission_error","param":null,"code":"permission_denied"}}`
				case tc.balanced:
					want = `{"error":{"message":"upstream failure","type":"server_error","param":null,"code":"upstream_error"}}`
				}
				assert.True(t, strings.HasSuffix(body, "data: "+want+"\n\n"), body)
			})
		})
	}
}

type interruptFinishInspector struct {
	denyFinish bool
	finishes   atomic.Int32
	outcomes   chan extension.Outcome
}

func (i *interruptFinishInspector) Observe(_ context.Context, _ *extension.Call, record extension.Record) error {
	i.outcomes <- record.Outcome
	return nil
}

func (i *interruptFinishInspector) InspectResponse(_ context.Context, _ *extension.Call, part extension.Part) error {
	if part.Type == extension.PartFinish {
		i.finishes.Add(1)
		if i.denyFinish {
			return extension.ErrDenied
		}
	}
	return nil
}

func serveInterruption(t *testing.T, handler http.Handler, req *http.Request) string {
	t.Helper()
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req.WithContext(callerContext(t, gatewayauth.ConfiguredAccounts)))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "partial-answer")
	return response.Body.String()
}
