package customertelemetry

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"
)

var (
	// ErrExportDenied indicates revoked or expired export authority.
	ErrExportDenied = errors.New("customertelemetry: export denied")
	// ErrExportTarget indicates an attempted export outside the bound endpoint.
	ErrExportTarget = errors.New("customertelemetry: export target mismatch")
)

// ExportGate bounds export operations independently of request cancellation.
// Factories must install this gate at the actual export seam. Revocation prevents
// new sends and cancels in-flight sends; it cannot retract delivered bytes.
type ExportGate struct {
	mu       sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	until    time.Time
	timeout  time.Duration
	endpoint string
}

func newExportGate(endpoint string, until time.Time, timeout time.Duration) *ExportGate {
	ctx, cancel := context.WithCancel(context.Background())
	return &ExportGate{ctx: ctx, cancel: cancel, until: until, timeout: timeout, endpoint: endpoint}
}

func (g *ExportGate) valid() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.ctx.Err() == nil && time.Now().Before(g.until)
}

func (g *ExportGate) refresh(until time.Time) {
	g.mu.Lock()
	g.until = until
	g.mu.Unlock()
}

func (g *ExportGate) revoke() { g.cancel() }

func (g *ExportGate) operation(parent context.Context) (context.Context, func(), error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ctx.Err() != nil || !time.Now().Before(g.until) {
		return nil, nil, ErrExportDenied
	}
	deadline := time.Now().Add(g.timeout)
	if g.until.Before(deadline) {
		deadline = g.until
	}
	ctx, cancel := context.WithDeadline(parent, deadline)
	stop := context.AfterFunc(g.ctx, cancel)
	return ctx, func() { stop(); cancel() }, nil
}

// Run applies authority and time limits to a supported synchronous export seam.
// The callback must honor its context and must not start detached export work.
func (g *ExportGate) Run(ctx context.Context, export func(context.Context) error) error {
	op, cancel, err := g.operation(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	return export(op)
}

// HTTPClient wraps an exclusively owned, already configured transport. It permits
// only the exact destination URL and refuses redirects. The factory owns transport
// credentials, TLS/egress policy, and closing idle connections during shutdown.
// A nil transport is rejected rather than falling back to ambient defaults.
func (g *ExportGate) HTTPClient(transport http.RoundTripper) (*http.Client, error) {
	if transport == nil {
		return nil, ErrExportTarget
	}
	return &http.Client{
		Transport:     &gatedTransport{gate: g, base: transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return ErrExportTarget },
	}, nil
}

type gatedTransport struct {
	gate *ExportGate
	base http.RoundTripper
}

func (t *gatedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.URL == nil || request.URL.String() != t.gate.endpoint || request.Host != "" && request.Host != request.URL.Host {
		return nil, ErrExportTarget
	}
	ctx, cancel, err := t.gate.operation(request.Context())
	if err != nil {
		return nil, err
	}
	response, err := t.base.RoundTrip(request.Clone(ctx))
	if err != nil {
		cancel()
		return nil, err
	}
	if response.Body == nil {
		cancel()
	} else {
		response.Body = &gatedBody{ReadCloser: response.Body, cancel: cancel}
	}
	return response, nil
}

type gatedBody struct {
	io.ReadCloser
	once   sync.Once
	cancel func()
}

func (b *gatedBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(b.cancel)
	}
	return n, err
}

func (b *gatedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}
