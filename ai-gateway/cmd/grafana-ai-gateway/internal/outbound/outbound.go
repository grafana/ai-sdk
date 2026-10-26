package outbound

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	outboundDialTimeout           = 5 * time.Second
	outboundTLSHandshakeTimeout   = 5 * time.Second
	outboundExpectContinueTimeout = time.Second
	outboundIdleConnTimeout       = 90 * time.Second
	outboundMaxIdleConns          = 32
	outboundMaxIdleConnsPerHost   = 8
	outboundMaxConnsPerHost       = 32

	eventStreamMediaType = "text/event-stream"
)

var (
	// ErrResponseTooLarge reports that an outbound decompressed body, or one
	// event of a streamed body, exceeded its limit.
	ErrResponseTooLarge = errors.New("gateway outbound: response exceeds byte limit")
)

// NewModelClient returns the streaming client shared by every provider type.
// It rejects redirects and bounds the wait for response headers. It bounds the
// decompressed body of each response independently: a text/event-stream body
// per event, so a long stream is bounded by time rather than size, and any
// other body in total.
func NewModelClient(headerTimeout time.Duration, responseBytes int64) (*http.Client, error) {
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

// ValidateEndpoint validates a credential-free hierarchical http or https
// endpoint URL.
func ValidateEndpoint(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("gateway outbound: endpoint is not a valid URL")
	}
	if !parsed.IsAbs() || parsed.Opaque != "" || parsed.Host == "" {
		return nil, fmt.Errorf("gateway outbound: endpoint must be an absolute hierarchical URL with a host")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, fmt.Errorf("gateway outbound: endpoint must not contain userinfo, query, forced query, or fragment")
	}
	if scheme := strings.ToLower(parsed.Scheme); scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("gateway outbound: endpoint must use http or https")
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
	if response.Body == nil {
		return response, nil
	}
	if isEventStream(response.Header.Get("Content-Type")) {
		response.Body = &eventBoundedBody{body: response.Body, limit: transport.limit, lineStart: true}
	} else {
		response.Body = &boundedBody{body: response.Body, remaining: transport.limit}
	}
	return response, nil
}

func isEventStream(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == eventStreamMediaType
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

// eventBoundedBody bounds each server-sent event: the bytes between blank
// lines, with LF, CRLF or CR line endings.
type eventBoundedBody struct {
	body      io.ReadCloser
	limit     int64
	size      int64
	lineStart bool
	pendingCR bool
	overflow  bool
}

func (body *eventBoundedBody) Read(buffer []byte) (int, error) {
	if body.overflow {
		return 0, ErrResponseTooLarge
	}
	n, err := body.body.Read(buffer)
	if accepted, ok := body.scan(buffer[:n]); !ok {
		body.overflow = true
		_ = body.body.Close()
		return accepted, ErrResponseTooLarge
	}
	return n, err
}

// scan advances the event state over data. When the current event exceeds the
// limit, it returns the length of the prefix within the limit and false.
func (body *eventBoundedBody) scan(data []byte) (int, bool) {
	for i := 0; i < len(data); {
		if body.pendingCR {
			body.pendingCR = false
			if data[i] == '\n' {
				if body.size > 0 {
					if body.size == body.limit {
						return i, false
					}
					body.size++
				}
				i++
				continue
			}
		}
		content := bytes.IndexAny(data[i:], "\r\n")
		if content < 0 {
			content = len(data) - i
		}
		if content > 0 {
			if body.size+int64(content) > body.limit {
				return i + int(body.limit-body.size), false
			}
			body.size += int64(content)
			body.lineStart = false
			i += content
			if i == len(data) {
				break
			}
		}
		body.pendingCR = data[i] == '\r'
		if body.lineStart {
			body.size = 0
		} else {
			if body.size == body.limit {
				return i, false
			}
			body.size++
			body.lineStart = true
		}
		i++
	}
	return len(data), true
}

func (body *eventBoundedBody) Close() error {
	return body.body.Close()
}
