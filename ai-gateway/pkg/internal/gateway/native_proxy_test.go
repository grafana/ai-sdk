//go:debug x509usefallbackroots=1

package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"
)

const nativeAnthropicAuthority = "api.anthropic.com:443"

// nativeTarget is the fake provider that native Anthropic requests reach
// through the test proxy, set by each test that needs one.
var nativeTarget struct {
	mu     sync.Mutex
	target *url.URL
}

// useNativeAnthropic routes native Anthropic requests of the current test to
// fake. Stack catalogs have no endpoint setting, so their calls go to the
// native endpoint; the package's proxy terminates their TLS with a test CA
// the process trusts.
func useNativeAnthropic(t *testing.T, fake *fakeAnthropic) {
	t.Helper()
	target, err := url.Parse(fake.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	nativeTarget.mu.Lock()
	nativeTarget.target = target
	nativeTarget.mu.Unlock()
	t.Cleanup(func() {
		nativeTarget.mu.Lock()
		nativeTarget.target = nil
		nativeTarget.mu.Unlock()
	})
}

func TestMain(m *testing.M) {
	proxy, err := startNativeProxy()
	if err != nil {
		fmt.Fprintln(os.Stderr, "native proxy:", err)
		os.Exit(1)
	}
	if err := os.Setenv("HTTPS_PROXY", proxy.URL); err != nil {
		fmt.Fprintln(os.Stderr, "native proxy:", err)
		os.Exit(1)
	}
	code := m.Run()
	proxy.Close()
	os.Exit(code)
}

// startNativeProxy trusts a fresh test CA for the process and serves CONNECT
// tunnels to the native Anthropic authority, terminating their TLS and
// forwarding each request to the current native target. Every other
// authority is refused.
func startNativeProxy() (*httptest.Server, error) {
	certificate, roots, err := nativeCertificate()
	if err != nil {
		return nil, err
	}
	x509.SetFallbackRoots(roots)
	forward := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		nativeTarget.mu.Lock()
		target := nativeTarget.target
		nativeTarget.mu.Unlock()
		if target != nil {
			request.SetURL(target)
		}
	}, FlushInterval: -1}
	tunnel := &tls.Config{Certificates: []tls.Certificate{certificate}, NextProtos: []string{"http/1.1"}}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != nativeAnthropicAuthority {
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		connection, buffered, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		_, _ = buffered.WriteString("HTTP/1.1 200 Connection established\r\n\r\n")
		_ = buffered.Flush()
		server := &http.Server{Handler: forward, ReadHeaderTimeout: time.Second}
		go func() { _ = server.Serve(&singleListener{connection: tls.Server(connection, tunnel)}) }()
	})), nil
}

func nativeCertificate() (tls.Certificate, *x509.CertPool, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "gateway test native provider"},
		DNSNames:              []string{"api.anthropic.com", "api.openai.com"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	roots := x509.NewCertPool()
	roots.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, roots, nil
}

// singleListener serves one tunneled connection.
type singleListener struct {
	once       sync.Once
	connection net.Conn
	done       chan struct{}
}

func (l *singleListener) Accept() (net.Conn, error) {
	var connection net.Conn
	l.once.Do(func() {
		connection, l.done = l.connection, make(chan struct{})
	})
	if connection != nil {
		return &closeNotifyConn{Conn: connection, closed: l.done}, nil
	}
	<-l.done
	return nil, net.ErrClosed
}

func (l *singleListener) Close() error   { return nil }
func (l *singleListener) Addr() net.Addr { return l.connection.LocalAddr() }

// closeNotifyConn ends its listener when the tunnel closes.
type closeNotifyConn struct {
	net.Conn
	closed chan struct{}
	once   sync.Once
}

func (c *closeNotifyConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.Conn.Close()
}

