package outbound

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
)

const (
	outboundDialTimeout           = 5 * time.Second
	outboundTLSHandshakeTimeout   = 5 * time.Second
	outboundExpectContinueTimeout = time.Second
	outboundIdleConnTimeout       = 90 * time.Second
	outboundMaxIdleConns          = 32
	outboundMaxIdleConnsPerHost   = 8
	outboundMaxConnsPerHost       = 32
)

var (
	// ErrResponseTooLarge reports that an outbound decompressed body exceeded its limit.
	ErrResponseTooLarge = errors.New("gateway outbound: response exceeds byte limit")
)

// NewJWKSClient returns a client that rejects redirects and limits response bytes and total request time.
func NewJWKSClient(timeout time.Duration, responseBytes int64) (*http.Client, error) {
	client, err := NewAnthropicClient(timeout, responseBytes)
	if err != nil {
		return nil, err
	}
	client.Timeout = timeout
	return client, nil
}

// NewAnthropicClient returns a streaming client that rejects redirects and limits response bytes and response-header wait time.
func NewAnthropicClient(headerTimeout time.Duration, responseBytes int64) (*http.Client, error) {
	if headerTimeout <= 0 {
		return nil, fmt.Errorf("gateway outbound: timeout must be positive")
	}
	if responseBytes <= 0 {
		return nil, fmt.Errorf("gateway outbound: response byte limit must be positive")
	}
	return &http.Client{
		Transport:     &boundedTransport{base: newTransport(headerTimeout), limit: responseBytes},
		CheckRedirect: rejectRedirect,
	}, nil
}

// ValidateEndpoint validates a credential-free hierarchical endpoint URL.
func ValidateEndpoint(raw string, mode config.DeploymentMode) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("gateway outbound: parsing endpoint: %w", err)
	}
	if !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" {
		return nil, fmt.Errorf("gateway outbound: endpoint must be an absolute hierarchical URL with a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, fmt.Errorf("gateway outbound: endpoint must not contain userinfo, query, forced query, or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch mode {
	case config.DeploymentProduction:
		if scheme != "https" {
			return nil, fmt.Errorf("gateway outbound: production endpoint must use https")
		}
	case config.DeploymentDevelopment:
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("gateway outbound: development endpoint must use http or https")
		}
	default:
		return nil, fmt.Errorf("gateway outbound: invalid deployment mode")
	}
	return parsed, nil
}

func newTransport(responseHeaderTimeout time.Duration) *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: outboundDialTimeout, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          outboundMaxIdleConns,
		MaxIdleConnsPerHost:   outboundMaxIdleConnsPerHost,
		MaxConnsPerHost:       outboundMaxConnsPerHost,
		IdleConnTimeout:       outboundIdleConnTimeout,
		TLSHandshakeTimeout:   outboundTLSHandshakeTimeout,
		ExpectContinueTimeout: outboundExpectContinueTimeout,
		ResponseHeaderTimeout: responseHeaderTimeout,
	}
}

func rejectRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

type boundedTransport struct {
	base  http.RoundTripper
	limit int64
}

func (transport *boundedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	if response.Body != nil {
		response.Body = &boundedBody{body: response.Body, remaining: transport.limit}
	}
	return response, nil
}

type boundedBody struct {
	body      io.ReadCloser
	remaining int64
	overflow  bool
}

func (body *boundedBody) Read(buffer []byte) (int, error) {
	if body.overflow {
		return 0, ErrResponseTooLarge
	}
	if len(buffer) == 0 {
		return 0, nil
	}
	if body.remaining > 0 {
		readBuffer := buffer
		if int64(len(readBuffer)) > body.remaining {
			readBuffer = readBuffer[:body.remaining]
		}
		n, err := body.body.Read(readBuffer)
		body.remaining -= int64(n)
		return n, err
	}
	var extra [1]byte
	n, err := body.body.Read(extra[:])
	if n > 0 {
		body.overflow = true
		_ = body.body.Close()
		return 0, ErrResponseTooLarge
	}
	return 0, err
}

func (body *boundedBody) Close() error {
	return body.body.Close()
}
