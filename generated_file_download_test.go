package aisdk

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedFileDownload_DataAndValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		url  string
		want string
	}{
		{"base64", "data:text/plain;base64,SGVsbG8=", "Hello"},
		{"unpadded base64", "data:text/plain;base64,SGVsbG8", "Hello"},
		{"percent encoded", "data:text/plain,Hello%20World", "Hello World"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := downloadGeneratedFile(context.Background(), tc.url)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
		})
	}
	file, err := resolveGeneratedFile(context.Background(), &provider.StreamFileData{URL: "data:text/plain,Hello"}, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "Hello", string(file.Data))
	_, err = resolveGeneratedFile(context.Background(), &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "data:text/plain,Hello", Base64: "SGVsbG8="}, "text/plain")
	require.ErrorContains(t, err, "multiple data sources")

	for _, raw := range []string{
		"ftp://example.com/file", "http://localhost/file", "http://localhost./file",
		"https://name.local/file", "http://127.0.0.1/file", "http://169.254.169.254/file",
		"http://[::1]/file", "http://[::ffff:127.0.0.1]/file", "http://[64:ff9b::7f00:1]/file",
		(&url.URL{Scheme: "http", Host: "example.com", Path: "/file", User: url.UserPassword("user", "password")}).String(),
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := downloadGeneratedFile(context.Background(), raw)
			require.Error(t, err)
		})
	}
	for _, ip := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		assert.True(t, isPublicGeneratedFileAddress(netip.MustParseAddr(ip)))
	}
	for _, ip := range []string{"::ffff:127.0.0.1", "::ffff:0:127.0.0.1", "::7f00:1", "64:ff9b:1::7f00:1", "2001:db8::1", "198.51.100.2"} {
		assert.False(t, isPublicGeneratedFileAddress(netip.MustParseAddr(ip)), ip)
	}
}

func TestGeneratedFileDownload_ValidatedNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/file", http.StatusFound)
		case "/private-redirect":
			http.Redirect(w, r, "http://127.0.0.1/internal", http.StatusFound)
		case "/error":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = io.WriteString(w, "Hello World")
		}
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	var addresses []string
	client := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, func(ctx context.Context, network, address string) (net.Conn, error) {
		addresses = append(addresses, address)
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	})
	defer client.CloseIdleConnections()
	base := "http://example.com:" + serverURL.Port()
	data, err := downloadGeneratedFileWithClient(context.Background(), base+"/redirect", client)
	require.NoError(t, err)
	assert.Equal(t, "Hello World", string(data))
	assert.Equal(t, []string{"8.8.8.8:" + serverURL.Port()}, addresses)
	_, err = downloadGeneratedFileWithClient(context.Background(), base+"/private-redirect", client)
	require.ErrorContains(t, err, "not public")
	assert.EqualValues(t, 3, calls.Load())
	_, err = downloadGeneratedFileWithClient(context.Background(), base+"/error", client)
	require.ErrorContains(t, err, "HTTP 503")

	t.Run("tries next validated address", func(t *testing.T) {
		var attempts []string
		var lookups atomic.Int32
		fallback := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
			lookups.Add(1)
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("1.1.1.1")}}, nil
		}, func(ctx context.Context, network, address string) (net.Conn, error) {
			attempts = append(attempts, address)
			if address == "8.8.8.8:"+serverURL.Port() {
				return nil, errors.New("first address unavailable")
			}
			return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
		})
		defer fallback.CloseIdleConnections()

		data, err := downloadGeneratedFileWithClient(context.Background(), base+"/file", fallback)
		require.NoError(t, err)
		assert.Equal(t, "Hello World", string(data))
		assert.Equal(t, []string{"8.8.8.8:" + serverURL.Port(), "1.1.1.1:" + serverURL.Port()}, attempts)
		assert.EqualValues(t, 1, lookups.Load())
	})
}

func TestGeneratedFileDownload_DNSAndBounds(t *testing.T) {
	var dialed bool
	client := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, nil
	})
	_, err := downloadGeneratedFileWithClient(context.Background(), "http://example.com/file", client)
	require.ErrorContains(t, err, "disallowed address")
	assert.False(t, dialed)

	mixed := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, nil
	})
	_, err = downloadGeneratedFileWithClient(context.Background(), "http://example.com/file", mixed)
	require.ErrorContains(t, err, "disallowed address")
	assert.False(t, dialed)

	translated := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("::ffff:0:127.0.0.1")}}, nil
	}, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, nil
	})
	_, err = downloadGeneratedFileWithClient(context.Background(), "http://example.com/file", translated)
	require.ErrorContains(t, err, "disallowed address")
	assert.False(t, dialed)

	tooLarge := &http.Response{StatusCode: http.StatusOK, ContentLength: maxGeneratedFileBytes + 1, Body: io.NopCloser(strings.NewReader(""))}
	_, err = readGeneratedFileResponse(tooLarge, maxGeneratedFileBytes)
	require.ErrorContains(t, err, "maximum size")

	for _, tc := range []struct {
		name   string
		length int64
		body   string
	}{
		{"content length", 4, ""},
		{"stream limit", -1, "abcd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: http.StatusOK, ContentLength: tc.length, Body: io.NopCloser(strings.NewReader(tc.body))}
			_, err := readGeneratedFileResponse(resp, 3)
			require.ErrorContains(t, err, "maximum size of 3 bytes")
		})
	}
}

func TestGeneratedFileDownload_Cancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	}, func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, serverURL.Host)
	})
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		_, err := downloadGeneratedFileWithClient(ctx, "http://example.com:"+serverURL.Port()+"/file", client)
		finished <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-finished, context.Canceled)

	t.Run("does not try another address after cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var attempts atomic.Int32
		client := newGeneratedFileClient(func(context.Context, string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("1.1.1.1")}}, nil
		}, func(context.Context, string, string) (net.Conn, error) {
			attempts.Add(1)
			cancel()
			return nil, context.Canceled
		})
		defer client.CloseIdleConnections()

		_, err := downloadGeneratedFileWithClient(ctx, "http://example.com/file", client)
		require.ErrorIs(t, err, context.Canceled)
		assert.EqualValues(t, 1, attempts.Load())
	})
}

func TestGeneratedFileDownload_FailurePropagation(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 1)
		stream <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "http://127.0.0.1/file"}, MediaType: "text/plain"}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	for range result.FullStream() {
	}
	require.ErrorContains(t, result.Err(), "not public")
	assert.Empty(t, result.Files())
	generated, err := GenerateText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	require.ErrorContains(t, err, "not public")
	assert.Nil(t, generated)
}

func TestGeneratedFileDownload_StreamAndGenerate(t *testing.T) {
	model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
		stream := make(chan provider.StreamPart, 3)
		stream <- provider.StreamPart{Type: provider.PartFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "data:text/plain;base64,SGVsbG8="}, MediaType: "text/plain"}
		stream <- provider.StreamPart{Type: provider.PartReasoningFile, Data: &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: "data:text/plain,Reason"}, MediaType: "text/plain"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	result := StreamText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	var files []StreamFile
	var reasoning []StreamReasoningFile
	for part := range result.FullStream() {
		switch part := part.(type) {
		case StreamFile:
			files = append(files, part)
		case StreamReasoningFile:
			reasoning = append(reasoning, part)
		}
	}
	require.Len(t, files, 1)
	require.Len(t, reasoning, 1)
	assert.Equal(t, "Hello", string(files[0].File.Data))
	assert.Equal(t, "Reason", string(reasoning[0].File.Data))
	assert.Equal(t, "data:text/plain;base64,SGVsbG8=", files[0].File.DataURL())
	assert.Equal(t, "data:text/plain;base64,UmVhc29u", reasoning[0].File.DataURL())

	generated, err := GenerateText(context.Background(), model, WithModelMessages(provider.UserText("test")))
	require.NoError(t, err)
	require.Len(t, generated.Files, 1)
	assert.Equal(t, "Hello", string(generated.Files[0].Data))
}
