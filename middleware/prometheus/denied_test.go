package prometheus

import (
	"context"
	"errors"
	"fmt"
	"testing"

	aimiddleware "github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errPolicyDenied = errors.New("policy denied")

func isPolicyDenied(err error) bool { return errors.Is(err, errPolicyDenied) }

func TestDeniedClassification(t *testing.T) {
	deniedPart := provider.NewAPICallError(provider.APICallErrorOptions{Message: "denied", Cause: errPolicyDenied})
	for _, tc := range []struct {
		name      string
		isDenied  func(error) bool
		operation string
		model     func(*mockModel)
		want      outcome
	}{
		{
			name:      "generate error",
			isDenied:  isPolicyDenied,
			operation: operationGenerate,
			model: func(m *mockModel) {
				m.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, fmt.Errorf("wrapped: %w", errPolicyDenied)
				}
			},
			want: deniedOutcome,
		},
		{
			name:      "stream setup error",
			isDenied:  isPolicyDenied,
			operation: operationStream,
			model: func(m *mockModel) {
				m.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return nil, errPolicyDenied
				}
			},
			want: deniedOutcome,
		},
		{
			name:      "stream error part",
			isDenied:  isPolicyDenied,
			operation: operationStream,
			model:     streamingParts(provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "a"}, provider.StreamPart{Type: provider.PartError, APICallError: deniedPart}),
			want:      deniedOutcome,
		},
		{
			name:      "stream error part without an API call error",
			isDenied:  func(error) bool { return true },
			operation: operationStream,
			model:     streamingParts(provider.StreamPart{Type: provider.PartError}),
			want:      outcome{status: statusError, errorType: errorTypeProviderStreamError, statusCode: statusCodeNone, finishReason: finishReasonNone},
		},
		{
			name:      "nil classifier",
			operation: operationGenerate,
			model: func(m *mockModel) {
				m.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, errPolicyDenied
				}
			},
			want: outcome{status: statusError, errorType: errorTypeOther, statusCode: statusCodeNone, finishReason: finishReasonNone},
		},
		{
			name:      "panicking classifier",
			isDenied:  func(error) bool { panic("classifier") },
			operation: operationGenerate,
			model: func(m *mockModel) {
				m.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, errPolicyDenied
				}
			},
			want: outcome{status: statusError, errorType: errorTypeOther, statusCode: statusCodeNone, finishReason: finishReasonNone},
		},
		{
			name:      "classifier does not deny",
			isDenied:  isPolicyDenied,
			operation: operationGenerate,
			model: func(m *mockModel) {
				m.generateFunc = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					return nil, context.Canceled
				}
			},
			want: outcome{status: statusCanceled, errorType: errorTypeContextCanceled, statusCode: statusCodeNone, finishReason: finishReasonNone},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := promclient.NewRegistry()
			mw, err := Middleware(Options{Registerer: reg, IsDenied: tc.isDenied})
			require.NoError(t, err)
			model := &mockModel{providerName: "grafana", modelID: "m"}
			tc.model(model)
			wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
			if tc.operation == operationGenerate {
				_, _ = wrapped.DoGenerate(context.Background(), provider.CallOptions{})
			} else if result, err := wrapped.DoStream(context.Background(), provider.CallOptions{}); err == nil {
				drain(result.Stream)
			}

			assertCounter(t, reg, metricRequestsTotal, map[string]string{
				"operation": tc.operation, "status": tc.want.status, "error_type": tc.want.errorType,
				"status_code": tc.want.statusCode, "finish_reason": tc.want.finishReason,
			}, 1)
			assertHistogramCount(t, reg, metricRequestDurationSeconds, map[string]string{"operation": tc.operation, "status": tc.want.status})
			if tc.want.status == statusDenied {
				assertNoStatus(t, reg, statusError)
			}
		})
	}
}

func streamingParts(parts ...provider.StreamPart) func(*mockModel) {
	return func(m *mockModel) {
		m.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			stream := make(chan provider.StreamPart, len(parts))
			for _, part := range parts {
				stream <- part
			}
			close(stream)
			return &provider.StreamResult{Stream: stream}, nil
		}
	}
}

func assertNoStatus(t *testing.T, reg *promclient.Registry, status string) {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				if label.GetName() == "status" {
					assert.NotEqual(t, status, label.GetValue(), family.GetName())
				}
			}
		}
	}
}
