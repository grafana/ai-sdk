//go:debug x509usefallbackroots=1

package gateway

import (
	"crypto/x509"
	"fmt"
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

func TestMain(m *testing.M) {
	forward := &httputil.ReverseProxy{Rewrite: func(request *httputil.ProxyRequest) {
		nativeTarget.mu.Lock()
		target := nativeTarget.target
		nativeTarget.mu.Unlock()
		if target != nil {
			request.SetURL(target)
		}
	}, FlushInterval: -1}
	proxy, err := nativeproxy.Start("api.anthropic.com:443", forward)
	if err != nil {
		fmt.Fprintln(os.Stderr, "native proxy:", err)
		os.Exit(1)
	}
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate)
	x509.SetFallbackRoots(roots)
	if err := os.Setenv("HTTPS_PROXY", proxy.URL); err != nil {
		fmt.Fprintln(os.Stderr, "native proxy:", err)
		os.Exit(1)
	}
	code := m.Run()
	proxy.Close()
	os.Exit(code)
}
