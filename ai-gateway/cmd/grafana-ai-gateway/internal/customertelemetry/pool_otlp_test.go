package customertelemetry

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

type otlpTestRequest struct {
	user, password, path string
	spans                []string
}
type otlpTestReceiver struct {
	mu             sync.Mutex
	requests       []otlpTestRequest
	beforeResponse func(*http.Request, int)
}

func (r *otlpTestReceiver) serve(t *testing.T, w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if !assert.NoError(t, err) {
		w.WriteHeader(400)
		return
	}
	var message collectortrace.ExportTraceServiceRequest
	if !assert.NoError(t, proto.Unmarshal(body, &message)) {
		w.WriteHeader(400)
		return
	}
	user, password, ok := req.BasicAuth()
	assert.True(t, ok)
	entry := otlpTestRequest{user: user, password: password, path: req.URL.Path}
	for _, resource := range message.ResourceSpans {
		for _, scope := range resource.ScopeSpans {
			for _, span := range scope.Spans {
				entry.spans = append(entry.spans, span.Name)
			}
		}
	}
	r.mu.Lock()
	r.requests = append(r.requests, entry)
	count := len(r.requests)
	r.mu.Unlock()
	if r.beforeResponse != nil {
		r.beforeResponse(req, count)
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(200)
}

func (r *otlpTestReceiver) snapshot() []otlpTestRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]otlpTestRequest(nil), r.requests...)
}

func otlpTestFactory(t *testing.T, server *httptest.Server, queue, batch int, retry bool) ResourceFactory[*sdktrace.TracerProvider] {
	t.Helper()
	return func(ctx context.Context, target ExportTarget, gate *ExportGate) (Resource[*sdktrace.TracerProvider], error) {
		roots := x509.NewCertPool()
		roots.AddCert(server.Certificate())
		transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}
		client, err := gate.HTTPClient(transport)
		if err != nil {
			return Resource[*sdktrace.TracerProvider]{}, err
		}
		auth := base64.StdEncoding.EncodeToString([]byte(target.Destination.IngestionIdentity + ":dummy-" + target.Destination.Credential.Version))
		exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(target.Destination.Endpoint), otlptracehttp.WithHTTPClient(client), otlptracehttp.WithHeaders(map[string]string{"Authorization": "Basic " + auth}), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: retry, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond, MaxElapsedTime: 20 * time.Millisecond}))
		if err != nil {
			transport.CloseIdleConnections()
			return Resource[*sdktrace.TracerProvider]{}, err
		}
		provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, sdktrace.WithMaxQueueSize(queue), sdktrace.WithMaxExportBatchSize(batch), sdktrace.WithBatchTimeout(time.Hour), sdktrace.WithExportTimeout(time.Second)))
		return Resource[*sdktrace.TracerProvider]{Value: provider, Shutdown: func(ctx context.Context) error { defer transport.CloseIdleConnections(); return provider.Shutdown(ctx) }}, nil
	}
}

func otlpTestBinding(t *testing.T, p *Pool[*sdktrace.TracerProvider], server *httptest.Server, s Stack, version string) *Binding[*sdktrace.TracerProvider] {
	t.Helper()
	d, policy := poolTestSnapshots(t, s, version)
	input := DestinationInput{Namespace: s.Namespace(), Revision: d.revision, ValidUntil: d.validUntil, Signals: map[Signal]Destination{SignalTraces: {Endpoint: server.URL + "/v1/traces", IngestionIdentity: s.Namespace() + "-trace-tenant", Credential: CredentialRef{Name: "test-reference", Version: version}}}}
	d, err := NewDestinations(s, input, time.Now())
	require.NoError(t, err)
	b, err := p.Update(s, SignalTraces, d, policy)
	require.NoError(t, err)
	return b
}

func otlpTestLease(t *testing.T, b *Binding[*sdktrace.TracerProvider]) *Lease[*sdktrace.TracerProvider] {
	t.Helper()
	l, err := b.Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(l.Release)
	return l
}
func otlpTestSpan(provider *sdktrace.TracerProvider, name string) {
	_, span := provider.Tracer("lifecycle-test").Start(context.Background(), name)
	span.End()
}
func otlpTestFlush(t *testing.T, provider *sdktrace.TracerProvider) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, provider.ForceFlush(ctx))
}

func TestPool_OTLPTenantIsolationAndRotation(t *testing.T) {
	r := &otlpTestReceiver{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.serve(t, w, req) }))
	defer server.Close()
	p, err := NewPool(otlpTestFactory(t, server, 8, 8, false), poolTestOptions())
	require.NoError(t, err)
	defer func() { assert.NoError(t, p.Close(context.Background())) }()
	a, b := testStack(t, "A"), testStack(t, "B")
	old := otlpTestLease(t, otlpTestBinding(t, p, server, a, "v1"))
	other := otlpTestLease(t, otlpTestBinding(t, p, server, b, "v1"))
	newer := otlpTestLease(t, otlpTestBinding(t, p, server, a, "v2"))
	for _, tc := range []struct {
		lease *Lease[*sdktrace.TracerProvider]
		name  string
	}{{old, "old-stream"}, {other, "other-stack"}, {newer, "new-stream"}} {
		otlpTestSpan(tc.lease.Value(), tc.name)
		otlpTestFlush(t, tc.lease.Value())
	}
	requests := r.snapshot()
	require.Len(t, requests, 3)
	assert.Equal(t, otlpTestRequest{user: "A-trace-tenant", password: "dummy-v1", path: "/v1/traces", spans: []string{"old-stream"}}, requests[0])
	assert.Equal(t, "B-trace-tenant", requests[1].user)
	assert.Equal(t, []string{"other-stack"}, requests[1].spans)
	assert.Equal(t, "A-trace-tenant", requests[2].user)
	assert.Equal(t, "dummy-v2", requests[2].password)

	d, policy := poolTestSnapshots(t, a, "v2")
	target := newer.Value()
	input := DestinationInput{Namespace: a.Namespace(), Revision: "moved", ValidUntil: d.validUntil, Signals: map[Signal]Destination{SignalTraces: {Endpoint: server.URL + "/v1/moved-traces", IngestionIdentity: "A-trace-tenant", Credential: CredentialRef{Name: "test-reference", Version: "v2"}}}}
	d, err = NewDestinations(a, input, time.Now())
	require.NoError(t, err)
	moved, err := p.Update(a, SignalTraces, d, policy)
	require.NoError(t, err)
	movedLease := otlpTestLease(t, moved)
	otlpTestSpan(movedLease.Value(), "moved-endpoint")
	otlpTestFlush(t, movedLease.Value())
	otlpTestSpan(target, "existing-stream-after-move")
	otlpTestFlush(t, target)
	requests = r.snapshot()
	require.Len(t, requests, 5)
	assert.Equal(t, "/v1/moved-traces", requests[3].path)
	assert.Equal(t, "/v1/traces", requests[4].path)
	movedLease.Release()
	old.Release()
	other.Release()
	newer.Release()
}

func TestPool_OTLPQueuedRevocation(t *testing.T) {
	r := &otlpTestReceiver{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.serve(t, w, req) }))
	defer server.Close()
	p, err := NewPool(otlpTestFactory(t, server, 8, 8, false), poolTestOptions())
	require.NoError(t, err)
	s := testStack(t, "A")
	lease := otlpTestLease(t, otlpTestBinding(t, p, server, s, "v1"))
	otlpTestSpan(lease.Value(), "queued-before-disable")
	_, err = p.Update(s, SignalTraces, Destinations{}, Policy{})
	assert.ErrorIs(t, err, ErrBinding)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	assert.Error(t, lease.Value().ForceFlush(ctx))
	lease.Release()
	assert.NoError(t, p.Close(ctx))
	assert.Empty(t, r.snapshot())
}

func TestPool_OTLPBoundedQueueAndRetry(t *testing.T) {
	t.Run("bounded queue", func(t *testing.T) {
		started, release := make(chan struct{}), make(chan struct{})
		r := &otlpTestReceiver{beforeResponse: func(req *http.Request, n int) {
			if n == 1 {
				close(started)
				select {
				case <-release:
				case <-req.Context().Done():
				}
			}
		}}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { r.serve(t, w, req) }))
		defer server.Close()
		p, err := NewPool(otlpTestFactory(t, server, 4, 4, false), poolTestOptions())
		require.NoError(t, err)
		lease := otlpTestLease(t, otlpTestBinding(t, p, server, testStack(t, "A"), "v1"))
		for range 4 {
			otlpTestSpan(lease.Value(), "first-batch")
		}
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			require.FailNow(t, "export did not start")
		}
		for range 64 {
			otlpTestSpan(lease.Value(), "bounded-queue")
		}
		close(release)
		otlpTestFlush(t, lease.Value())
		lease.Release()
		assert.NoError(t, p.Close(context.Background()))
		count := 0
		for _, request := range r.snapshot() {
			count += len(request.spans)
		}
		assert.GreaterOrEqual(t, count, 4)
		assert.LessOrEqual(t, count, 8)
	})
	t.Run("sdk retry", func(t *testing.T) {
		var calls atomic.Int64
		r := &otlpTestReceiver{}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(503)
				return
			}
			r.serve(t, w, req)
		}))
		defer server.Close()
		p, err := NewPool(otlpTestFactory(t, server, 8, 8, true), poolTestOptions())
		require.NoError(t, err)
		lease := otlpTestLease(t, otlpTestBinding(t, p, server, testStack(t, "A"), "v1"))
		otlpTestSpan(lease.Value(), "retried")
		otlpTestFlush(t, lease.Value())
		lease.Release()
		assert.NoError(t, p.Close(context.Background()))
		assert.EqualValues(t, 2, calls.Load())
		require.Len(t, r.snapshot(), 1)
	})
}
