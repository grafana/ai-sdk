package service

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/agento11y/go/agento11y/testkit"
	gatewayauth "github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBYOKObservability_LogicalExportAndCredentialPrivacy(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		for _, status := range []int{503, 401, -1} {
			t.Run(strconv.FormatBool(streaming)+"/"+strconv.Itoa(status), func(t *testing.T) {
				env := testkit.NewEnv(t, func(c *agento11y.Config) {
					c.ContentCapture = agento11y.ContentCaptureModeMetadataOnly
					c.Hooks = agento11y.HooksConfig{Enabled: false}
				})
				var logs lockedBuffer
				logger := slog.New(slog.NewJSONHandler(&logs, nil))
				telemetry, err := NewTelemetry(logger)
				require.NoError(t, err)
				runtime := &AgentObservabilityRuntime{client: env.Client, telemetry: telemetry, flushTimeout: time.Second, shutdownTimeout: time.Second}
				_, observe, err := NewModelObservabilityFactories(telemetry, logger, runtime, 10*time.Millisecond)
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var keys []string
				client := &http.Client{Transport: selectionTransport(func(r *http.Request) (*http.Response, error) {
					keys = append(keys, r.Header.Get("X-Api-Key"))
					if status == -1 {
						cancel()
						<-r.Context().Done()
						return nil, r.Context().Err()
					}
					raw, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Contains(t, string(raw), "application-content")
					assert.NotContains(t, string(raw), "gateway")
					assert.NotContains(t, string(raw), "dummy-")
					code := http.StatusOK
					contentType := "application/json"
					body := `{"id":"msg_1","type":"message","role":"assistant","model":"backend-private","content":[{"type":"text","text":"application-output"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`
					if streaming {
						contentType = "text/event-stream"
						body = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"backend-private\",\"content\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
					}
					if len(keys) == 1 {
						code = status
						contentType = "application/json"
						body = `{"type":"error","error":{"type":"api_error","message":"dummy-first dummy-second"}}`
					}
					return &http.Response{StatusCode: code, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
				})}
				wire, err := providerv4.New(providerv4.Config{Selector: NewBYOKSelector(client, observe), Limits: serviceTestLimits()})
				require.NoError(t, err)
				writer := providerv4.NewHostErrorWriter()
				authenticator, headers := serviceModeAuthentication(gatewayauth.SourceCloudGateway)
				handler := gatewayauth.Middleware(authenticator, func(w http.ResponseWriter) { writer.Write(w, providerv4.HostErrorAuthentication) }, func(context.Context, gatewayauth.Observation) {}, wire)
				request := httptest.NewRequest(http.MethodPost, providerv4.LanguageModelPath, strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"application-content"}]}],"maxOutputTokens":32,"providerOptions":{"gateway":{"byok":{"anthropic":[{"apiKey":"dummy-first"},{"apiKey":"dummy-second"}]}}}}`))
				requestID := "anthropic/native-" + strings.Repeat("x", 140)
				request = request.WithContext(ctx)
				request.Header = headers
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set(providerv4.HeaderModelID, requestID)
				request.Header.Set(providerv4.HeaderSpecificationVersion, "4")
				request.Header.Set(providerv4.HeaderStreaming, strconv.FormatBool(streaming))
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				switch status {
				case -1:
					assert.Equal(t, []string{"dummy-first"}, keys)
				case 401:
					assert.Equal(t, http.StatusFailedDependency, response.Code)
					assert.Equal(t, []string{"dummy-first"}, keys)
				default:
					assert.Equal(t, http.StatusOK, response.Code)
					assert.Equal(t, []string{"dummy-first", "dummy-second"}, keys)
				}
				runtime.Close()
				generation := env.SingleGenerationJSON(t)
				assert.Equal(t, "anthropic", testkit.StringValue(t, generation, "model", "provider"))
				assert.Equal(t, requestID, testkit.StringValue(t, generation, "model", "name"))
				exported, err := json.Marshal(generation)
				require.NoError(t, err)
				metrics := testMetrics(t, telemetry)
				assert.Contains(t, metrics, `model="byok"`)
				assert.NotContains(t, metrics, requestID)
				assert.Contains(t, logs.String(), requestID)
				for _, secret := range []string{"dummy-first", "dummy-second"} {
					assert.NotContains(t, response.Body.String()+logs.String()+metrics+string(exported), secret)
				}
				for _, private := range []string{"backend-private", "application-content", "application-output"} {
					assert.NotContains(t, logs.String()+metrics+string(exported), private)
				}
			})
		}
	}
}
