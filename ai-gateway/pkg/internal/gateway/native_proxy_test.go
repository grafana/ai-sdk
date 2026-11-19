//go:debug x509usefallbackroots=1

package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/test/nativeproxy"
)

var nativeTarget struct {
	mu     sync.Mutex
	target *url.URL
}

func useNativeAnthropic(t *testing.T, fake *fakeAnthropic) {
	t.Helper()
	useNativeAnthropicURL(t, fake.server.URL)
}

func useNativeAnthropicURL(t *testing.T, rawURL string) {
	t.Helper()
	target, err := url.Parse(rawURL)
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

var endpointCertificate tls.Certificate

func TestMain(m *testing.M) {
	forward := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		nativeTarget.mu.Lock()
		target := nativeTarget.target
		nativeTarget.mu.Unlock()
		if target != nil {
			request.SetURL(target)
		}
	}, FlushInterval: -1}
	proxy, err := nativeproxy.Start("api.anthropic.com:443", forward, "localhost", "127.0.0.1")
	if err != nil {
		fmt.Fprintln(os.Stderr, "native proxy:", err)
		os.Exit(1)
	}
	roots := x509.NewCertPool()
	endpointCertificate = proxy.Certificate
	roots.AddCert(proxy.Certificate.Leaf)
	x509.SetFallbackRoots(roots)
	if err := os.Setenv("HTTPS_PROXY", proxy.URL); err != nil {
		fmt.Fprintln(os.Stderr, "native proxy:", err)
		os.Exit(1)
	}
	code := m.Run()
	proxy.Close()
	os.Exit(code)
}
