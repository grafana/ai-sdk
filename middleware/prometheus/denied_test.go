package prometheus

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

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
			name:      "denial after a finish",
			isDenied:  isPolicyDenied,
			operation: operationStream,
			model:     streamingParts(provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}, provider.StreamPart{Type: provider.PartError, APICallError: deniedPart}),
			want:      deniedOutcome,
		},
		{
			name:      "denial before a later error part",
			isDenied:  isPolicyDenied,
			operation: operationStream,
			model:     streamingParts(provider.StreamPart{Type: provider.PartError, APICallError: deniedPart}, provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500})}),
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

func TestStreamOutcome_CancellationAndInBandOutcome(t *testing.T) {
	errorPart := provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500})}
	deniedPart := provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "denied", Cause: errPolicyDenied})}
	textPart := provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "a"}
	for _, tc := range []struct {
		name string
		part provider.StreamPart
		// cancelFirst cancels before the part is sent. Otherwise the
		// consumer reads the part and then cancels, or waits for the
		// deadline.
		cancelFirst   bool
		deadline      bool
		wantStatus    string
		wantErrorType string
	}{
		{name: "error part then cancellation", part: errorPart, wantStatus: statusError, wantErrorType: errorTypeAPICallError},
		{name: "denial then cancellation", part: deniedPart, wantStatus: statusDenied, wantErrorType: errorTypeNone},
		{name: "error part then deadline", part: errorPart, deadline: true, wantStatus: statusError, wantErrorType: errorTypeAPICallError},
		{name: "plain cancellation", part: textPart, wantStatus: statusCanceled, wantErrorType: errorTypeContextCanceled},
		{name: "cancellation before an error part", part: errorPart, cancelFirst: true, wantStatus: statusCanceled, wantErrorType: errorTypeContextCanceled},
		{name: "cancellation before a denial", part: deniedPart, cancelFirst: true, wantStatus: statusCanceled, wantErrorType: errorTypeContextCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reg := promclient.NewRegistry()
			mw, err := Middleware(Options{Registerer: reg, IsDenied: isPolicyDenied})
			require.NoError(t, err)
			upstream := make(chan provider.StreamPart, 1)
			model := &mockModel{providerName: "openai", modelID: "gpt"}
			model.streamFunc = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: upstream}, nil
			}
			wrapped := aimiddleware.Wrap(aimiddleware.WrapOptions{Model: model, Middleware: []aimiddleware.Middleware{mw}})
			ctx, cancel := context.WithCancel(context.Background())
			if tc.deadline {
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
			}
			defer cancel()

			if tc.cancelFirst {
				cancel()
				upstream <- tc.part
			}
			result, err := wrapped.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			if !tc.cancelFirst {
				upstream <- tc.part
				<-result.Stream
				if tc.deadline {
					<-ctx.Done()
				} else {
					cancel()
				}
			}
			if tc.cancelFirst {
				close(upstream)
			} else {
				defer close(upstream)
			}
			drain(result.Stream)

			assertCounter(t, reg, metricRequestsTotal, map[string]string{"operation": operationStream, "status": tc.wantStatus, "error_type": tc.wantErrorType}, 1)
		})
	}
}

func TestStreamObservation_IgnoresErrorPartsAfterCancellation(t *testing.T) {
	errorPart := provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{StatusCode: 500})}
	deniedPart := provider.StreamPart{Type: provider.PartError, APICallError: provider.NewAPICallError(provider.APICallErrorOptions{Message: "denied", Cause: errPolicyDenied})}
	for _, tc := range []struct {
		name     string
		part     provider.StreamPart
		canceled bool
		want     *outcome
	}{
		{name: "error before cancellation", part: errorPart, want: &outcome{status: statusError, errorType: errorTypeAPICallError, statusCode: "500", finishReason: finishReasonNone}},
		{name: "denial before cancellation", part: deniedPart, want: &deniedOutcome},
		{name: "error after cancellation", part: errorPart, canceled: true},
		{name: "denial after cancellation", part: deniedPart, canceled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obs := newStreamObservation(config{isDenied: isPolicyDenied})
			now := time.Now()
			obs.observe(tc.part, now, now, tc.canceled)
			assert.Equal(t, tc.want, obs.streamError)
			assert.Equal(t, 1, obs.chunkCounts[streamChunkTypeLabel(provider.PartError)])
		})
	}
}
