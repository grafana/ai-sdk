package outbound

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		mode  config.DeploymentMode
		valid bool
	}{
		{name: "production https", raw: "https://auth.example/jwks", mode: config.DeploymentProduction, valid: true},
		{name: "development https", raw: "https://provider.example/prefix", mode: config.DeploymentDevelopment, valid: true},
		{name: "development http", raw: "http://127.0.0.1:8080/prefix", mode: config.DeploymentDevelopment, valid: true},
		{name: "production http", raw: "http://auth.example/jwks", mode: config.DeploymentProduction},
		{name: "relative", raw: "/jwks", mode: config.DeploymentDevelopment},
		{name: "empty host", raw: "https:///jwks", mode: config.DeploymentDevelopment},
		{name: "userinfo", raw: "https://user:password@auth.example/jwks", mode: config.DeploymentDevelopment},
		{name: "opaque", raw: "https:opaque", mode: config.DeploymentDevelopment},
		{name: "query", raw: "https://auth.example/jwks?secret=value", mode: config.DeploymentDevelopment},
		{name: "forced query", raw: "https://auth.example/jwks?", mode: config.DeploymentDevelopment},
		{name: "fragment", raw: "https://auth.example/jwks#fragment", mode: config.DeploymentDevelopment},
		{name: "unsupported scheme", raw: "ftp://auth.example/jwks", mode: config.DeploymentDevelopment},
		{name: "invalid mode", raw: "https://auth.example/jwks", mode: config.DeploymentMode("other")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ValidateEndpoint(tc.raw, tc.mode)
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, tc.raw, parsed.String())
			} else {
				require.Error(t, err)
				assert.Nil(t, parsed)
			}
		})
	}
}

func TestNewClients_IndependentExactTransports(t *testing.T) {
	clients, err := NewClients(5*time.Second, 10*time.Second, 1024, 2048)
	require.NoError(t, err)
	jwksBounded := requireBoundedTransport(t, clients.JWKS)
	anthropicBounded := requireBoundedTransport(t, clients.Anthropic)
	assert.NotSame(t, jwksBounded, anthropicBounded)
	assert.NotSame(t, jwksBounded.base, anthropicBounded.base)
	assert.Equal(t, int64(1024), jwksBounded.limit)
	assert.Equal(t, int64(2048), anthropicBounded.limit)
	assert.Equal(t, 5*time.Second, clients.JWKS.Timeout)
	assert.Zero(t, clients.Anthropic.Timeout)

	for _, tc := range []struct {
		name      string
		transport *http.Transport
		header    time.Duration
	}{
		{name: "jwks", transport: jwksBounded.base.(*http.Transport), header: 5 * time.Second},
		{name: "anthropic", transport: anthropicBounded.base.(*http.Transport), header: 10 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
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

			clients, err := NewClients(time.Second, time.Second, 1024, 1024)
			require.NoError(t, err)
			for _, client := range []*http.Client{clients.JWKS, clients.Anthropic} {
				request, err := http.NewRequest(http.MethodGet, source.URL, nil)
				require.NoError(t, err)
				request.Header.Set("X-Access-Token", "secret-access")
				request.Header.Set("Authorization", "Bearer secret")
				request.Header.Set("X-Api-Key", "secret-api-key")
				response, err := client.Do(request)
				require.NoError(t, err)
				assert.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
				require.NoError(t, response.Body.Close())
			}
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
					clients, err := NewClients(time.Second, time.Second, limit, limit)
					require.NoError(t, err)
					response, err := clients.Anthropic.Get(server.URL)
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
		clients, err := NewClients(time.Second, 50*time.Millisecond, 1024, 1024)
		require.NoError(t, err)
		_, err = clients.Anthropic.Get(server.URL)
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
		clients, err := NewClients(time.Second, time.Second, 1024, 1024)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		require.NoError(t, err)
		response, err := clients.Anthropic.Do(request)
		require.NoError(t, err)
		_, err = io.ReadAll(response.Body)
		_ = response.Body.Close()
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled))
	})
}

func requireBoundedTransport(t *testing.T, client *http.Client) *boundedTransport {
	t.Helper()
	transport, ok := client.Transport.(*boundedTransport)
	require.True(t, ok)
	return transport
}
