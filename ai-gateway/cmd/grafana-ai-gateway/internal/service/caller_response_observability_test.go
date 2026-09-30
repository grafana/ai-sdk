package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallerResponses_MetadataOnlyObservation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, failure := range []bool{false, true} {
			t.Run(map[bool]string{false: "unary", true: "stream"}[streaming]+map[bool]string{false: "/success", true: "/error"}[failure], func(t *testing.T) {
				env := testkit.NewEnv(t, func(c *agento11y.Config) {
					c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
					c.Hooks = agento11y.HooksConfig{Enabled: false}
				})
				var logs lockedBuffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				telemetry, err := NewTelemetry(logger)
				require.NoError(t, err)
				runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
				factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, 10*time.Millisecond)
				require.NoError(t, err)
				warning := provider.Warning{Type: provider.WarnOther, Message: "caller-warning-marker"}
				source := provider.SourceInfo{SourceType: provider.SourceTypeDocument, ID: "caller-source-marker", Title: "caller-title-marker", Filename: "caller-file-marker", MediaType: "text/plain"}
				api := provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 422, Message: "protected-sdk-dump", URL: "https://protected-endpoint.invalid", Data: json.RawMessage(`{"message":"caller-error-marker","type":"invalid_request_error","code":"caller-code-marker","param":"caller-param-marker","unknown":"protected-data-marker"}`)})
				var captured provider.CallOptions
				lower := &observabilityTestModel{
					generate: func(_ context.Context, opts provider.CallOptions) (*provider.GenerateResult, error) {
						captured = opts
						if failure {
							return nil, api
						}
						return &provider.GenerateResult{Warnings: []provider.Warning{warning}, Response: &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "caller-response-marker", ModelID: "caller-model-marker", Provider: "protected-provider-marker"}, Body: json.RawMessage(`{"private":"protected-body-marker"}`)}, Content: []provider.GenerateContentPart{{Type: provider.ContentSource, SourceType: source.SourceType, ID: source.ID, Title: source.Title, Filename: source.Filename, MediaType: source.MediaType}}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
					},
					stream: func(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
						captured = opts
						parts := make(chan provider.StreamPart, 5)
						parts <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: []provider.Warning{warning}}
						parts <- provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "caller-response-marker", ModelID: "caller-model-marker"}
						parts <- provider.StreamPart{Type: provider.PartSource, Source: &source}
						if failure {
							parts <- provider.StreamPart{Type: provider.PartError, APICallError: api}
						}
						parts <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &provider.Usage{}}
						close(parts)
						return &provider.StreamResult{Stream: parts}, nil
					},
				}
				model, err := factory("grafana/assistant", lower)
				require.NoError(t, err)
				resolver, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "grafana/assistant"}, Model: model, ProviderErrors: catalog.ProviderErrorOpenAI, ProviderOptions: anthropicOptionPolicy}})
				require.NoError(t, err)
				handler, err := providerv4.New(providerv4.Config{Resolver: resolver, Limits: serviceTestLimits()})
				require.NoError(t, err)
				observed := telemetry.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					telemetry.ObserveAuthentication(r.Context(), gatewayauth.Observation{Outcome: gatewayauth.OutcomeAuthenticated, Caller: &gatewayauth.Caller{Service: "internal-team", Namespace: "authorized-fixture"}})
					handler.ServeHTTP(w, r)
				}))
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"call","toolName":"lookup","input":{},"providerOptions":{"anthropic":{"caller":{"type":"code_execution_20260120","toolId":"caller-history-marker"}}}}]}]}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderSpecificationVersion, providerv4.SpecificationVersion)
				request.Header.Set(providerv4.HeaderModelID, "grafana/assistant")
				request.Header.Set(providerv4.HeaderStreaming, map[bool]string{false: "false", true: "true"}[streaming])
				response := httptest.NewRecorder()
				observed.ServeHTTP(response, request)
				if failure && !streaming {
					require.Equal(t, 422, response.Code)
				} else {
					require.Equal(t, 200, response.Code)
				}
				markers := []string{"caller-warning-marker", "caller-response-marker", "caller-model-marker", "caller-source-marker", "caller-title-marker", "caller-file-marker"}
				if !failure || streaming {
					for _, marker := range markers {
						assert.Contains(t, response.Body.String(), marker)
					}
				}
				if failure {
					for _, marker := range []string{"caller-error-marker", "caller-code-marker", "caller-param-marker"} {
						assert.Contains(t, response.Body.String(), marker)
					}
				}
				encodedOptions, err := json.Marshal(captured)
				require.NoError(t, err)
				assert.Contains(t, string(encodedOptions), "caller-history-marker")
				require.Eventually(t, func() bool { return env.RequestCount() == 1 }, time.Second, time.Millisecond)
				env.Shutdown(t)
				generation := env.SingleGenerationJSON(t)
				encoded, err := json.Marshal(generation)
				require.NoError(t, err)
				all := string(encoded) + logs.String() + testMetrics(t, telemetry)
				for _, marker := range append(markers, "caller-history-marker", "caller-error-marker", "caller-code-marker", "caller-param-marker", "protected-sdk-dump", "protected-endpoint.invalid", "protected-data-marker", "protected-provider-marker", "protected-body-marker") {
					assert.NotContains(t, all, marker)
				}
				for _, marker := range []string{"protected-sdk-dump", "protected-endpoint.invalid", "protected-data-marker", "protected-provider-marker", "protected-body-marker"} {
					assert.NotContains(t, response.Body.String(), marker)
				}
				assert.Equal(t, "grafana/assistant", testkit.StringValue(t, generation, "model", "name"))
				assert.Equal(t, "internal-team", testkit.StringValue(t, generation, "metadata", "gateway.caller_service"))
			})
		}
	}
}
