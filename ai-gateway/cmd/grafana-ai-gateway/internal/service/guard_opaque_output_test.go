package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardRuntime_OpaqueOutputBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, namespace, fields string
		opaque                  bool
	}{
		{"anthropic redacted", "anthropic", `{"redactedData":"hidden-output-canary"}`, true},
		{"bedrock redacted data", "bedrock", `{"redactedData":"hidden-output-canary"}`, true},
		{"bedrock redacted content", "bedrock", `{"redactedContent":"hidden-output-canary"}`, true},
		{"amazonBedrock redacted data", "amazonBedrock", `{"redactedData":"hidden-output-canary"}`, true},
		{"amazonBedrock redacted content", "amazonBedrock", `{"redactedContent":"hidden-output-canary"}`, true},
		{"openai encrypted", "openai", `{"itemId":"rs_1","reasoningEncryptedContent":"hidden-output-canary"}`, true},
		{"anthropic empty redacted", "anthropic", `{"redactedData":""}`, true},
		{"openai empty encrypted", "openai", `{"reasoningEncryptedContent":""}`, true},
		{"anthropic signed", "anthropic", `{"signature":"signed-control"}`, false},
		{"bedrock signed", "bedrock", `{"signature":"signed-control"}`, false},
		{"amazonBedrock signed", "amazonBedrock", `{"signature":"signed-control"}`, false},
		{"openai item id", "openai", `{"itemId":"rs_1"}`, false},
		{"openai null encryption", "openai", `{"itemId":"rs_1","reasoningEncryptedContent":null}`, false},
	} {
		for _, text := range []string{"", "represented thought"} {
			for _, location := range []string{"unary", "start", "delta", "end"} {
				for _, policy := range []string{"closed", "open", "disabled"} {
					t.Run(fmt.Sprintf("%s/%s/%s/text=%q", tc.name, policy, location, text), func(t *testing.T) {
						var pre, post, calls, unused atomic.Int64
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							var request guardHookRequest
							require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
							if request.Phase == guardPreflight {
								pre.Add(1)
							} else {
								post.Add(1)
								require.Len(t, request.Input.Output, 1)
								require.Len(t, request.Input.Output[0].Parts, 2)
								assert.Equal(t, text, request.Input.Output[0].Parts[1].Thinking)
							}
							_, _ = io.WriteString(w, `{"action":"allow"}`)
						}))
						defer server.Close()
						settings := guardRuntimeSettings(server.URL)
						settings.FailOpen = policy == "open"
						var guard providerv4.Guard
						if policy != "disabled" {
							guard, _ = newTestGuardRuntime(t, settings)
						}
						metadata := provider.ProviderMetadata{tc.namespace: json.RawMessage(tc.fields)}
						finish := provider.FinishReason{Unified: provider.FinishReasonStop}
						model := &observabilityTestModel{
							generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
								calls.Add(1)
								return &provider.GenerateResult{Content: []provider.GenerateContentPart{
									{Type: provider.ContentText, Text: "withheld-output-canary"},
									{Type: provider.ContentReasoning, Text: text, ProviderMetadata: metadata},
								}, FinishReason: finish}, nil
							},
							stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
								calls.Add(1)
								parts := []provider.StreamPart{
									{Type: provider.PartTextStart, ID: "text"},
									{Type: provider.PartTextDelta, ID: "text", Delta: "withheld-output-canary"},
									{Type: provider.PartTextEnd, ID: "text"},
									{Type: provider.PartReasoningStart, ID: "reasoning"},
									{Type: provider.PartReasoningDelta, ID: "reasoning", Delta: text},
									{Type: provider.PartReasoningEnd, ID: "reasoning"},
									{Type: provider.PartFinish, FinishReason: &finish, Usage: &provider.Usage{}},
								}
								index := map[string]int{"start": 3, "delta": 4, "end": 5}[location]
								parts[index].ProviderMetadata = metadata
								if location != "end" {
									parts[5].ProviderMetadata = provider.ProviderMetadata{tc.namespace: json.RawMessage(`{}`)}
								}
								ch := make(chan provider.StreamPart, len(parts))
								for _, part := range parts {
									ch <- part
								}
								close(ch)
								return &provider.StreamResult{Stream: ch}, nil
							},
						}
						other := &observabilityTestModel{
							generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
								unused.Add(1)
								return nil, nil
							},
							stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
								unused.Add(1)
								return nil, nil
							},
						}
						chain, err := fallback.New(model, other)
						require.NoError(t, err)
						handler, err := providerv4.New(providerv4.Config{Resolver: guardRecordingResolver{chain}, Guard: guard, GuardRetainedBytes: settings.RetainedBytes / 2, Limits: providerv4.Limits{RequestBytes: 1 << 20, UnaryResponseBytes: 1 << 20, StreamFrameBytes: 1 << 20, StreamParts: 100, ModelDuration: time.Second, StreamIdleDuration: time.Second, StreamDrainDuration: 10 * time.Millisecond}})
						require.NoError(t, err)
						request := guardRecordingRequest()
						request.Header.Set(providerv4.HeaderStreaming, fmt.Sprint(location != "unary"))
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, request)
						assert.Equal(t, int64(1), calls.Load())
						assert.Zero(t, unused.Load())
						if policy == "disabled" {
							assert.Zero(t, pre.Load()+post.Load())
						} else {
							assert.Equal(t, int64(1), pre.Load())
						}
						if tc.opaque && policy != "disabled" {
							assert.Equal(t, http.StatusFailedDependency, response.Code, response.Body.String())
							assert.Equal(t, `{"error":{"message":"failed dependency","type":"failed_dependency","param":null,"code":"failed_dependency"}}`, response.Body.String())
							assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
							assert.False(t, response.Flushed)
							assert.Zero(t, post.Load())
							assert.NotContains(t, response.Body.String(), "data: ")
							assert.NotContains(t, response.Body.String(), "output-canary")
						} else {
							assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
							assert.Contains(t, response.Body.String(), tc.fields)
							assert.Contains(t, response.Body.String(), "withheld-output-canary")
							if policy != "disabled" {
								assert.Equal(t, int64(1), post.Load())
							}
							assert.Equal(t, location != "unary", strings.HasPrefix(response.Body.String(), "data: "))
						}
					})
				}
			}
		}
	}
}
