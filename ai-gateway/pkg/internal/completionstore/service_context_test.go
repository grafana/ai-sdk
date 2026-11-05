package completionstore

import (
	"context"
	"testing"
	"time"

	"github.com/grafana/authlib/types"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/grafana/grafana-app-sdk/resource"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/trace"
)

type contextAuthInfo struct{ types.AuthInfo }

func TestServiceContextRetainsLifecycleAndSpanWithoutCallerValues(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(t.Context(), deadline)
	defer cancel()
	ctx = types.WithAuthInfo(ctx, &contextAuthInfo{})
	ctx = k8s.ContextWithIDToken(ctx, "signed-caller-token")
	ctx = resource.WithRouteRequestInfo(ctx, &resource.RouteRequestInfo{Namespace: "private-caller"})
	member, err := baggage.NewMember("caller", "private")
	require.NoError(t, err)
	bag, err := baggage.New(member)
	require.NoError(t, err)
	ctx = baggage.ContextWithBaggage(ctx, bag)
	spanContext := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}, TraceFlags: trace.FlagsSampled})
	ctx = trace.ContextWithSpanContext(ctx, spanContext)
	operation := serviceContext(ctx)
	_, ok := types.AuthInfoFrom(operation)
	require.False(t, ok)
	_, ok = k8s.IDTokenFromContext(operation)
	require.False(t, ok)
	require.Empty(t, baggage.FromContext(operation).Members())
	require.Nil(t, resource.RouteRequestInfoFrom(operation))
	require.Equal(t, trace.SpanFromContext(ctx), trace.SpanFromContext(operation))
	actual, ok := operation.Deadline()
	require.True(t, ok)
	require.Equal(t, deadline, actual)
	require.Equal(t, ctx.Done(), operation.Done())
	require.NoError(t, operation.Err())
	cancel()
	require.ErrorIs(t, operation.Err(), context.Canceled)
	select {
	case <-operation.Done():
	default:
		t.Fatal("service operation lost cancellation")
	}
	expired, stop := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer stop()
	require.ErrorIs(t, serviceContext(expired).Err(), context.DeadlineExceeded)
}
