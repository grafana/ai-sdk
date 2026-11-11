package outbound

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

const httpsScheme = "https"

var (
	// ErrEgressConnection reports an egress connection, TLS handshake or
	// response read that failed after the policy permitted the address. It
	// names no host, address or certificate.
	ErrEgressConnection = errors.New("gateway outbound: provider connection failed")
	// ErrEgressTimeout is ErrEgressConnection for a timeout. It wraps
	// [context.DeadlineExceeded], so it classifies as a timeout.
	ErrEgressTimeout = fmt.Errorf("gateway outbound: provider connection timed out: %w", context.DeadlineExceeded)

	errAddressRefused = errors.New("gateway outbound: address refused")
)

// NewEgressClient returns the client for customer-chosen provider endpoints.
// It has the bounds and redirect refusal of [NewModelClient], its own
// connections, and no proxy. It sends only https requests and checks every
// address it connects to against policy after DNS resolution, so a refused
// address receives no packet. Refusals and resolution failures fail with
// [execution.ErrEgressRefused], which names no host or address.
func NewEgressClient(headerTimeout time.Duration, responseBytes int64, policy EgressPolicy) (*http.Client, error) {
	return newEgressClient(headerTimeout, responseBytes, policy, nil, nil)
}

// newEgressClient is NewEgressClient with a resolver and TLS roots, which
// tests replace; nil selects the system ones.
func newEgressClient(headerTimeout time.Duration, responseBytes int64, policy EgressPolicy, resolver *net.Resolver, roots *x509.CertPool) (*http.Client, error) {
	if err := validateLimits(headerTimeout, responseBytes); err != nil {
		return nil, err
	}
	dialer := egressDialer{policy: policy, resolver: resolver}
	transport := newTransport(headerTimeout, nil, dialer.DialContext)
	if roots != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	return boundedClient(httpsOnly{base: transport}, responseBytes), nil
}

// httpsOnly refuses every request whose URL is not https before it reaches
// the transport. It replaces every other connection, TLS and read error, of
// the request or its response body, with [ErrEgressConnection] or
// [ErrEgressTimeout],
// because those errors name the Gateway's own address, the addresses an
// endpoint resolved to and the names in its certificate.
type httpsOnly struct {
	base http.RoundTripper
}

func (transport httpsOnly) RoundTrip(request *http.Request) (*http.Response, error) {
	if !strings.EqualFold(request.URL.Scheme, httpsScheme) {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, execution.ErrEgressRefused
	}
	ctx := request.Context()
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return response, sanitize(ctx, err)
	}
	if response.Body != nil {
		response.Body = sanitizedBody{ReadCloser: response.Body, ctx: ctx}
	}
	return response, nil
}

// sanitize returns a refusal unchanged, the request context's error once it
// is done, and otherwise [ErrEgressTimeout] or [ErrEgressConnection].
func sanitize(ctx context.Context, err error) error {
	if errors.Is(err, execution.ErrEgressRefused) {
		return err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	var netError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &netError) && netError.Timeout() {
		return ErrEgressTimeout
	}
	return ErrEgressConnection
}

type sanitizedBody struct {
	io.ReadCloser
	ctx context.Context
}

func (body sanitizedBody) Read(buffer []byte) (int, error) {
	n, err := body.ReadCloser.Read(buffer)
	if err != nil && err != io.EOF {
		err = sanitize(body.ctx, err)
	}
	return n, err
}

type egressDialer struct {
	policy   EgressPolicy
	resolver *net.Resolver
}

// DialContext connects to address, a host and port, when the policy permits
// the host and the address each attempt connects to.
func (dialer egressDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, execution.ErrEgressRefused
	}
	hostAllowed := false
	if _, err := netip.ParseAddr(host); err != nil {
		if !ValidHostname(normalizeHost(host)) || dialer.policy.Deny.matchesHost(host) {
			return nil, execution.ErrEgressRefused
		}
		hostAllowed = dialer.policy.Allow.matchesHost(host)
	}
	connector := &net.Dialer{
		Timeout:   outboundDialTimeout,
		KeepAlive: outboundKeepAlive,
		Resolver:  dialer.resolver,
		Control: func(_, address string, _ syscall.RawConn) error {
			target, err := netip.ParseAddrPort(address)
			if err != nil || !dialer.policy.permits(target.Addr(), hostAllowed) {
				return errAddressRefused
			}
			return nil
		},
	}
	connection, err := connector.DialContext(ctx, network, address)
	if err == nil {
		return connection, nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	var dnsError *net.DNSError
	if errors.Is(err, errAddressRefused) || errors.As(err, &dnsError) {
		return nil, execution.ErrEgressRefused
	}
	return nil, err
}
