package outbound

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		valid bool
	}{
		{name: "https", raw: "https://auth.example/jwks", valid: true},
		{name: "https with a path", raw: "https://provider.example/prefix", valid: true},
		{name: "http", raw: "http://127.0.0.1:8080/prefix", valid: true},
		{name: "relative", raw: "/jwks"},
		{name: "empty host", raw: "https:///jwks"},
		{name: "userinfo", raw: "https://user:password@auth.example/jwks"},
		{name: "malformed with userinfo", raw: "https://user:password@auth.example:port/jwks"},
		{name: "opaque", raw: "https:opaque"},
		{name: "query", raw: "https://auth.example/jwks?secret=value"},
		{name: "forced query", raw: "https://auth.example/jwks?"},
		{name: "fragment", raw: "https://auth.example/jwks#fragment"},
		{name: "unsupported scheme", raw: "ftp://auth.example/jwks"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ValidateEndpoint(tc.raw)
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, tc.raw, parsed.String())
			} else {
				require.Error(t, err)
				assert.Nil(t, parsed)
				assert.NotContains(t, err.Error(), "password")
			}
		})
	}
}

func TestNewClients_IndependentExactTransports(t *testing.T) {
	first, err := NewModelClient(5*time.Second, 1024)
	require.NoError(t, err)
	second, err := NewModelClient(10*time.Second, 2048)
	require.NoError(t, err)
	firstBounded := requireBoundedTransport(t, first)
	secondBounded := requireBoundedTransport(t, second)
	assert.NotSame(t, firstBounded, secondBounded)
	assert.NotSame(t, firstBounded.base, secondBounded.base)
	assert.Equal(t, int64(1024), firstBounded.limit)
	assert.Equal(t, int64(2048), secondBounded.limit)
	assert.Zero(t, first.Timeout)
	assert.Zero(t, second.Timeout)

	for _, tc := range []struct {
		name      string
		transport *http.Transport
		header    time.Duration
	}{
		{name: "first", transport: firstBounded.base.(*http.Transport), header: 5 * time.Second},
		{name: "second", transport: secondBounded.base.(*http.Transport), header: 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.NotNil(t, tc.transport.Proxy)
			assert.NotNil(t, tc.transport.DialContext)
			assert.True(t, tc.transport.ForceAttemptHTTP2)
			assert.False(t, tc.transport.DisableCompression)
			assert.Equal(t, outboundMaxIdleConns, tc.transport.MaxIdleConns)
			assert.Equal(t, outboundMaxIdleConnsPerHost, tc.transport.MaxIdleConnsPerHost)
			assert.Equal(t, outboundMaxConnsPerHost, tc.transport.MaxConnsPerHost)
			assert.Equal(t, outboundIdleConnTimeout, tc.transport.IdleConnTimeout)
			assert.Equal(t, outboundTLSHandshakeTimeout, tc.transport.TLSHandshakeTimeout)
			assert.Equal(t, outboundExpectContinueTimeout, tc.transport.ExpectContinueTimeout)
			assert.Equal(t, tc.header, tc.transport.ResponseHeaderTimeout)
		})
	}
}

func TestClients_RejectRedirectsBeforeCredentialForwarding(t *testing.T) {
	var targetCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalls.Add(1)
	}))
	defer target.Close()

	for _, sameOrigin := range []bool{true, false} {
		name := "cross origin"
		if sameOrigin {
			name = "same origin"
		}
		t.Run(name, func(t *testing.T) {
			var source *httptest.Server
			source = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				location := target.URL
				if sameOrigin {
					location = source.URL + "/target"
					if request.URL.Path == "/target" {
						targetCalls.Add(1)
						return
					}
				}
				http.Redirect(w, request, location, http.StatusTemporaryRedirect)
			}))
			defer source.Close()

			client, err := NewModelClient(time.Second, 1024)
			require.NoError(t, err)
			request, err := http.NewRequest(http.MethodGet, source.URL, nil)
			require.NoError(t, err)
			request.Header.Set("X-Access-Token", "secret-access")
			request.Header.Set("Authorization", "Bearer secret")
			request.Header.Set("X-Api-Key", "secret-api-key")
			response, err := client.Do(request)
			require.NoError(t, err)
			assert.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
			require.NoError(t, response.Body.Close())
		})
	}
	assert.Zero(t, targetCalls.Load())
}

func TestBoundedTransport_DecompressedResponseBoundaries(t *testing.T) {
	payloads := []struct {
		name    string
		payload string
		status  int
	}{
		{name: "plain success", payload: "0123456789", status: http.StatusOK},
		{name: "plain error", payload: "0123456789", status: http.StatusBadGateway},
		{name: "single SSE line", payload: "data: 0123456789\n\n", status: http.StatusOK},
		{name: "multiline SSE event", payload: "data: 01234\ndata: 56789\n\n", status: http.StatusOK},
	}
	for _, payload := range payloads {
		for _, compressed := range []bool{false, true} {
			compressionName := "plain"
			if compressed {
				compressionName = "gzip"
			}
			t.Run(payload.name+"/"+compressionName, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if compressed {
						w.Header().Set("Content-Encoding", "gzip")
					}
					w.WriteHeader(payload.status)
					if compressed {
						writer := gzip.NewWriter(w)
						_, _ = io.WriteString(writer, payload.payload)
						_ = writer.Close()
						return
					}
					_, _ = io.WriteString(w, payload.payload)
				}))
				defer server.Close()

				for _, delta := range []int64{1, 0, -1} {
					limit := int64(len(payload.payload)) + delta
					client, err := NewModelClient(time.Second, limit)
					require.NoError(t, err)
					response, err := client.Get(server.URL)
					require.NoError(t, err)
					body, readErr := io.ReadAll(response.Body)
					_ = response.Body.Close()
					if delta >= 0 {
						require.NoError(t, readErr)
						assert.Equal(t, payload.payload, string(body))
					} else {
						require.ErrorIs(t, readErr, ErrResponseTooLarge)
						assert.Len(t, body, int(limit))
					}
				}
			})
		}
	}
}

func TestBoundedTransport_CountsDecompressedBytes(t *testing.T) {
	const limit = 64 << 10
	payload := strings.Repeat("a", limit+1)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := io.WriteString(writer, payload)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.Less(t, compressed.Len(), limit)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()

	client, err := NewModelClient(time.Second, limit)
	require.NoError(t, err)
	response, err := client.Get(server.URL)
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	require.ErrorIs(t, err, ErrResponseTooLarge)
	assert.Len(t, body, limit)
}

func TestBoundedTransport_BoundsEachResponseIndependently(t *testing.T) {
	const limit = 64 << 10
	payload := strings.Repeat("a", limit)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	client, err := NewModelClient(time.Second, limit)
	require.NoError(t, err)
	for range 2 {
		response, err := client.Get(server.URL)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.NoError(t, err)
		assert.Len(t, body, limit)
	}
}

func TestEventBoundedBody_BoundsEachEvent(t *testing.T) {
	const limit = 64
	event := func(terminator string, size int) string {
		return "data: " + strings.Repeat("x", size-len("data: ")-len(terminator)) + terminator
	}
	type eventCase struct {
		name     string
		payload  string
		accepted int
	}
	var tests []eventCase
	for _, terminator := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}, {"CR", "\r"}} {
		atLimit := event(terminator.value, limit) + terminator.value
		tests = append(tests,
			eventCase{name: terminator.name + "/many events at the limit", payload: strings.Repeat(atLimit, 8), accepted: -1},
			eventCase{name: terminator.name + "/event above the limit", payload: atLimit + event(terminator.value, limit+1) + terminator.value, accepted: len(atLimit) + limit},
			eventCase{name: terminator.name + "/multi-line event above the limit", payload: event(terminator.value, limit/2) + event(terminator.value, limit/2+1) + terminator.value, accepted: limit},
		)
	}
	for _, tc := range tests {
		for _, split := range []bool{false, true} {
			name := tc.name
			if split {
				name += "/one byte per read"
			}
			t.Run(name, func(t *testing.T) {
				var reader io.Reader = strings.NewReader(tc.payload)
				if split {
					reader = iotest.OneByteReader(reader)
				}
				body := &eventBoundedBody{body: io.NopCloser(reader), limit: limit, lineStart: true}
				read, err := io.ReadAll(body)
				if tc.accepted < 0 {
					require.NoError(t, err)
					assert.Equal(t, tc.payload, string(read))
					return
				}
				require.ErrorIs(t, err, ErrResponseTooLarge)
				assert.Equal(t, tc.payload[:tc.accepted], string(read))
				_, err = body.Read(make([]byte, 1))
				require.ErrorIs(t, err, ErrResponseTooLarge)
			})
		}
	}
}

func TestBoundedTransport_SelectsBoundByMediaType(t *testing.T) {
	const limit = 64 << 10
	events := strings.Repeat("data: "+strings.Repeat("e", limit/2)+"\n\n", 4)
	oversizedEvent := "data: " + strings.Repeat("e", limit) + "\n\n"
	tests := []struct {
		name        string
		contentType string
		payload     string
		compressed  bool
		tooLarge    bool
	}{
		{name: "stream above the total within the event bound", contentType: "text/event-stream", payload: events},
		{name: "stream with parameters", contentType: "text/event-stream; charset=utf-8", payload: events},
		{name: "stream with mixed case", contentType: "Text/Event-Stream", payload: events},
		{name: "compressed stream", contentType: "text/event-stream", payload: events, compressed: true},
		{name: "single event above the bound", contentType: "text/event-stream", payload: oversizedEvent, tooLarge: true},
		{name: "compressed event above the bound", contentType: "text/event-stream", payload: oversizedEvent, compressed: true, tooLarge: true},
		{name: "JSON above the total", contentType: "application/json", payload: events, tooLarge: true},
		{name: "unparsable media type", contentType: "text/event-stream; =", payload: events, tooLarge: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				if !tc.compressed {
					_, _ = io.WriteString(w, tc.payload)
					return
				}
				w.Header().Set("Content-Encoding", "gzip")
				writer := gzip.NewWriter(w)
				_, _ = io.WriteString(writer, tc.payload)
				_ = writer.Close()
			}))
			defer server.Close()

			client, err := NewModelClient(time.Second, limit)
			require.NoError(t, err)
			response, err := client.Get(server.URL)
			require.NoError(t, err)
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if tc.tooLarge {
				require.ErrorIs(t, err, ErrResponseTooLarge)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.payload, string(body))
		})
	}
}

func TestClients_HostileServersTerminateWithinBounds(t *testing.T) {
	t.Run("dial cancellation", func(t *testing.T) {
		transport := newTransport(time.Second)
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		client := &http.Client{Transport: &boundedTransport{base: transport, limit: 1024}, CheckRedirect: rejectRedirect}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://dial.invalid", nil)
		require.NoError(t, err)
		_, err = client.Do(request)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("response-header timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		}))
		defer server.Close()
		client, err := NewModelClient(50*time.Millisecond, 1024)
		require.NoError(t, err)
		_, err = client.Get(server.URL)
		require.Error(t, err)
	})

	t.Run("stalled body request cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			_, _ = fmt.Fprint(w, "initial")
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-request.Context().Done()
		}))
		defer server.Close()
		client, err := NewModelClient(time.Second, 1024)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		response, err := client.Do(request)
		require.NoError(t, err)
		_, err = io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
	})
}

func TestNewModelClient_InvalidLimits(t *testing.T) {
	for _, limits := range []struct {
		timeout time.Duration
		bytes   int64
	}{
		{timeout: 0, bytes: 1},
		{timeout: -time.Second, bytes: 1},
		{timeout: time.Second, bytes: 0},
		{timeout: time.Second, bytes: -1},
	} {
		client, err := NewModelClient(limits.timeout, limits.bytes)
		require.Error(t, err)
		assert.Nil(t, client)
	}
}

func requireBoundedTransport(t *testing.T, client *http.Client) *boundedTransport {
	t.Helper()
	transport, ok := client.Transport.(*boundedTransport)
	require.True(t, ok)
	return transport
}
