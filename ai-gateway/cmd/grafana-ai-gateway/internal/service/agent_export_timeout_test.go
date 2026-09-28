package service

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	agento11yv1 "github.com/grafana/agento11y/go/proto/agento11y/v1"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type stalledAgentIngest struct {
	agento11yv1.UnimplementedGenerationIngestServiceServer
	canceled chan struct{}
}

func (server *stalledAgentIngest) ExportGenerations(ctx context.Context, _ *agento11yv1.ExportGenerationsRequest) (*agento11yv1.ExportGenerationsResponse, error) {
	<-ctx.Done()
	server.canceled <- struct{}{}
	return nil, status.Error(codes.DeadlineExceeded, "export deadline")
}

func TestAgentObservabilityRuntime_ExportAttemptTimeout(t *testing.T) {
	for _, protocol := range []config.AgentObservabilityProtocol{config.AgentObservabilityHTTP, config.AgentObservabilityGRPC} {
		t.Run(string(protocol), func(t *testing.T) {
			canceled := make(chan struct{}, 4)
			release := make(chan struct{})
			defer close(release)
			var endpoint string
			switch protocol {
			case config.AgentObservabilityHTTP:
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, err := io.Copy(io.Discard, r.Body)
					assert.NoError(t, err)
					select {
					case <-r.Context().Done():
						canceled <- struct{}{}
					case <-release:
						http.Error(w, "test stopped", http.StatusServiceUnavailable)
					}
				}))
				t.Cleanup(server.Close)
				endpoint = server.URL
			case config.AgentObservabilityGRPC:
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				server := grpc.NewServer()
				agento11yv1.RegisterGenerationIngestServiceServer(server, &stalledAgentIngest{canceled: canceled})
				go func() { assert.NoError(t, server.Serve(listener)) }()
				t.Cleanup(server.Stop)
				endpoint = listener.Addr().String()
			}
			var logs lockedBuffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			telemetry, err := NewTelemetry(logger)
			require.NoError(t, err)
			runtime, err := NewAgentObservabilityRuntime(config.AgentObservabilitySettings{
				Enabled: true, Protocol: protocol, Endpoint: endpoint, AuthSecretEnv: "AO_SECRET",
				QueueSize: 4, BatchSize: 1, PayloadMaxBytes: 1 << 20, MaxRetries: 1,
				InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
				ExportTimeout: 100 * time.Millisecond, FlushInterval: time.Hour,
				FlushTimeout: time.Second, ShutdownTimeout: time.Second,
			}, func(name string) (string, bool) { return "private-secret", name == "AO_SECRET" }, telemetry)
			require.NoError(t, err)
			t.Cleanup(runtime.Close)
			t.Setenv("AGENTO11Y_ENABLE_EXPERIMENTAL_FEATURES", "true")
			require.ErrorIs(t, runtime.client.RequireExperimental("test"), agento11y.ErrExperimentalFeatureDisabled)
			factory, err := NewModelObservabilityFactory(telemetry, logger, runtime, time.Millisecond)
			require.NoError(t, err)
			want := &provider.GenerateResult{Content: []provider.GenerateContentPart{{Type: provider.ContentText, Text: "private-output"}}}
			model, err := factory("grafana/assistant", &observabilityTestModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return want, nil }})
			require.NoError(t, err)
			result, err := model.DoGenerate(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			assert.Same(t, want, result)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			for range 2 {
				select {
				case <-canceled:
				case <-ctx.Done():
					t.Fatal("export attempt did not honor configured timeout")
				}
			}
			require.Eventually(t, func() bool {
				return strings.Contains(testMetrics(t, telemetry), `grafana_ai_gateway_agento11y_export_failures_total{class="transport"} 1`)
			}, time.Second, time.Millisecond)
			for _, private := range []string{endpoint, "private-secret", "private-output"} {
				assert.NotContains(t, logs.String(), private)
			}
		})
	}
}
