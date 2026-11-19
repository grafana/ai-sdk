// Package nativeproxy serves a synthetic provider at its native authority in tests.
package nativeproxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// Proxy terminates CONNECT for one native authority and serves a local handler.
// It never dials the original authority.
type Proxy struct {
	URL         string
	Certificate *x509.Certificate
	proxy       *httptest.Server
	server      *http.Server
}

// Start serves a synthetic native provider at authority with a fresh test CA.
func Start(authority string, handler http.Handler) (*Proxy, error) {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		return nil, err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := &x509.Certificate{
		DNSNames:              []string{host},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	tunnel := &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, NextProtos: []string{"http/1.1"}, MinVersion: tls.VersionTLS12}
	connections := &listener{accept: make(chan net.Conn), closed: make(chan struct{})}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: time.Second}
	go func() { _ = server.Serve(connections) }()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != authority {
			http.Error(w, "refused", http.StatusForbidden)
			return
		}
		connection, buffered, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection established\r\n\r\n"); err != nil || buffered.Flush() != nil {
			_ = connection.Close()
			return
		}
		select {
		case connections.accept <- tls.Server(connection, tunnel):
		case <-connections.closed:
			_ = connection.Close()
		}
	}))
	return &Proxy{URL: proxy.URL, Certificate: cert, proxy: proxy, server: server}, nil
}

// Close ends the proxy and its accepted tunnels.
func (p *Proxy) Close() {
	p.proxy.Close()
	_ = p.server.Close()
}

type listener struct {
	accept chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func (l *listener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.accept:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *listener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *listener) Addr() net.Addr { return &net.TCPAddr{} }
