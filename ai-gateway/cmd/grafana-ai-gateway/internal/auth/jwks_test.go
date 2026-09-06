package auth

import (
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/outbound"
	"github.com/grafana/authlib/authn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWKS_LazySnapshotRefreshAndMaximumAge(t *testing.T) {
	clock := newTestClock(time.Unix(100, 0))
	first := testPublicJWK(t, "first")
	rotated := testPublicJWK(t, "rotated")
	var calls atomic.Int64
	var response atomic.Value
	response.Store(marshalJWKS(t, first))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write(response.Load().([]byte))
	}))
	defer server.Close()

	retriever := newTestJWKS(t, server.Client(), server.URL, clock.Now, 4)
	assert.Zero(t, calls.Load(), "construction must not fetch")

	key, err := retriever.Get(context.Background(), "first")
	require.NoError(t, err)
	assert.Equal(t, "first", key.KeyID)
	assert.Equal(t, int64(1), calls.Load())

	_, err = retriever.Get(context.Background(), "unknown")
	require.ErrorIs(t, err, authn.ErrInvalidSigningKey)
	assert.Equal(t, int64(1), calls.Load())

	clock.Advance(time.Minute)
	response.Store(marshalJWKS(t, first, rotated))
	key, err = retriever.Get(context.Background(), "rotated")
	require.NoError(t, err)
	assert.Equal(t, "rotated", key.KeyID)
	assert.Equal(t, int64(2), calls.Load())

	clock.Advance(3 * time.Minute)
	_, err = retriever.Get(context.Background(), "first")
	require.NoError(t, err)
	assert.Equal(t, int64(3), calls.Load(), "expired snapshots refresh even for known keys")
}

func TestJWKS_RejectsInvalidResponsesWithoutReplacingValidSnapshot(t *testing.T) {
	valid := testPublicJWK(t, "valid")
	duplicate := testPublicJWK(t, "duplicate")
	private := testPrivateJWK(t, "private")
	tests := []struct {
		name   string
		status int
		body   []byte
	}{
		{name: "non 200", status: http.StatusBadGateway, body: []byte(`{"keys":[]}`)},
		{name: "malformed", status: http.StatusOK, body: []byte(`{"keys":`)},
		{name: "missing keys", status: http.StatusOK, body: []byte(`{}`)},
		{name: "duplicate keys field", status: http.StatusOK, body: []byte(`{"keys":[],"keys":[]}`)},
		{name: "trailing", status: http.StatusOK, body: append(marshalJWKS(t, valid), []byte(` {}`)...)},
		{name: "empty set", status: http.StatusOK, body: []byte(`{"keys":[]}`)},
		{name: "empty key ID", status: http.StatusOK, body: marshalJWKS(t, testPublicJWK(t, ""))},
		{name: "duplicate key ID", status: http.StatusOK, body: marshalJWKS(t, duplicate, duplicate)},
		{name: "invalid key", status: http.StatusOK, body: []byte(`{"keys":[{"kid":"invalid","kty":"EC"}]}`)},
		{name: "private key", status: http.StatusOK, body: marshalJWKS(t, private)},
		{name: "over key count", status: http.StatusOK, body: marshalJWKS(t, valid, testPublicJWK(t, "second"))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := newTestClock(time.Unix(100, 0))
			var mu sync.Mutex
			status := http.StatusOK
			body := marshalJWKS(t, valid)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.WriteHeader(status)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			retriever := newTestJWKS(t, server.Client(), server.URL, clock.Now, 1)
			_, err := retriever.Get(context.Background(), "valid")
			require.NoError(t, err)

			clock.Advance(time.Minute)
			mu.Lock()
			status = tc.status
			body = tc.body
			mu.Unlock()
			_, err = retriever.Get(context.Background(), "unknown")
			require.Error(t, err)

			key, err := retriever.Get(context.Background(), "valid")
			require.NoError(t, err)
			assert.Equal(t, "valid", key.KeyID)
			assert.Len(t, retriever.snapshot.keys, 1)
		})
	}
}

func TestJWKS_IgnoresUnknownRootMembers(t *testing.T) {
	key := testPublicJWK(t, "key")
	keyJSON, err := json.Marshal(key)
	require.NoError(t, err)
	body := []byte(fmt.Sprintf(`{"before":{"nested":[1,true,null]},"keys":[%s],"after":["extension"]}`, keyJSON))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer server.Close()
	retriever := newTestJWKS(t, server.Client(), server.URL, time.Now, 1)

	got, err := retriever.Get(context.Background(), "key")
	require.NoError(t, err)
	assert.Equal(t, "key", got.KeyID)
}

func TestJWKS_KeyCountParsingBoundPreservesSnapshot(t *testing.T) {
	clock := newTestClock(time.Unix(100, 0))
	first := testPublicJWK(t, "first")
	second := testPublicJWK(t, "second")
	var mu sync.Mutex
	body := marshalJWKS(t, first, second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		_, _ = w.Write(body)
	}))
	defer server.Close()
	retriever := newTestJWKS(t, server.Client(), server.URL, clock.Now, 2)

	key, err := retriever.Get(context.Background(), "second")
	require.NoError(t, err)
	assert.Equal(t, "second", key.KeyID)
	retriever.mu.Lock()
	fetchedAt := retriever.snapshot.fetchedAt
	retriever.mu.Unlock()

	firstJSON, err := json.Marshal(first)
	require.NoError(t, err)
	secondJSON, err := json.Marshal(second)
	require.NoError(t, err)
	clock.Advance(time.Minute)
	mu.Lock()
	body = []byte(fmt.Sprintf(`{"keys":[%s,%s,{"kty":`, firstJSON, secondJSON))
	mu.Unlock()

	_, err = retriever.Get(context.Background(), "unknown")
	require.ErrorContains(t, err, "too many keys")
	key, err = retriever.Get(context.Background(), "first")
	require.NoError(t, err)
	assert.Equal(t, "first", key.KeyID)
	retriever.mu.Lock()
	assert.Equal(t, fetchedAt, retriever.snapshot.fetchedAt)
	assert.Len(t, retriever.snapshot.keys, 2)
	retriever.mu.Unlock()
}

func TestJWKS_ResponseByteBoundaries(t *testing.T) {
	body := marshalJWKS(t, testPublicJWK(t, "key"))
	for _, compressed := range []bool{false, true} {
		name := "plain"
		if compressed {
			name = "gzip"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
					writer := gzip.NewWriter(w)
					_, _ = writer.Write(body)
					_ = writer.Close()
					return
				}
				_, _ = w.Write(body)
			}))
			defer server.Close()
			for _, delta := range []int64{0, -1} {
				clients, err := outbound.NewClients(time.Second, time.Second, int64(len(body))+delta, 1024)
				require.NoError(t, err)
				retriever := newTestJWKS(t, clients.JWKS, server.URL, time.Now, 1)
				_, err = retriever.Get(context.Background(), "key")
				if delta == 0 {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
		})
	}
}

func TestJWKS_RequestTimeoutBoundsHeaderAndBodyStalls(t *testing.T) {
	for _, writeHeaders := range []bool{false, true} {
		name := "response headers"
		if writeHeaders {
			name = "response body"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if writeHeaders {
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
				}
				<-request.Context().Done()
			}))
			defer server.Close()
			retriever, err := NewJWKS(JWKSConfig{
				ServiceContext:  context.Background(),
				Client:          server.Client(),
				URL:             server.URL,
				RequestTimeout:  50 * time.Millisecond,
				MaxKeys:         1,
				RefreshInterval: time.Minute,
				MaxAge:          2 * time.Minute,
			})
			require.NoError(t, err)

			start := time.Now()
			_, err = retriever.Get(context.Background(), "key")
			require.ErrorIs(t, err, authn.ErrFetchingSigningKey)
			assert.Less(t, time.Since(start), time.Second)
		})
	}
}

func TestJWKS_ConcurrentUniqueKeyFloodUsesOneBoundedFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	body := marshalJWKS(t, testPublicJWK(t, "known"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		_, _ = w.Write(body)
	}))
	defer server.Close()
	retriever := newTestJWKS(t, server.Client(), server.URL, time.Now, 1)

	const count = 32
	results := make(chan error, count)
	for i := range count {
		go func(index int) {
			_, err := retriever.Get(context.Background(), fmt.Sprintf("unknown-%d", index))
			results <- err
		}(i)
	}
	<-started
	close(release)
	for range count {
		require.ErrorIs(t, <-results, authn.ErrInvalidSigningKey)
	}
	assert.Equal(t, int64(1), calls.Load())
	retriever.mu.Lock()
	assert.Nil(t, retriever.flight)
	assert.Len(t, retriever.snapshot.keys, 1)
	retriever.mu.Unlock()
}

func TestJWKS_ConcurrentRotatedKeyWaitersJoinAcrossCompletion(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	var observations atomic.Int64
	body := marshalJWKS(t, testPublicJWK(t, "rotated"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		_, _ = w.Write(body)
	}))
	defer server.Close()
	retriever := newTestJWKS(t, server.Client(), server.URL, func() time.Time {
		observations.Add(1)
		return time.Unix(100, 0)
	}, 1)

	const count = 24
	results := make(chan error, count+1)
	go func() {
		_, err := retriever.Get(context.Background(), "rotated")
		results <- err
	}()
	<-started
	for range count - 1 {
		go func() {
			_, err := retriever.Get(context.Background(), "rotated")
			results <- err
		}()
	}
	require.Eventually(t, func() bool { return observations.Load() >= count }, time.Second, time.Millisecond)
	close(release)
	go func() {
		_, err := retriever.Get(context.Background(), "rotated")
		results <- err
	}()
	for range count + 1 {
		require.NoError(t, <-results)
	}
	assert.Equal(t, int64(1), calls.Load())
}

func TestJWKS_CanceledStarterDoesNotCancelSharedRefresh(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	var observations atomic.Int64
	body := marshalJWKS(t, testPublicJWK(t, "key"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		close(started)
		<-release
		_, _ = w.Write(body)
	}))
	defer server.Close()
	retriever := newTestJWKS(t, server.Client(), server.URL, func() time.Time {
		observations.Add(1)
		return time.Unix(100, 0)
	}, 1)

	starterContext, cancelStarter := context.WithCancel(context.Background())
	starter := make(chan error, 1)
	go func() {
		_, err := retriever.Get(starterContext, "key")
		starter <- err
	}()
	<-started
	waiter := make(chan error, 1)
	go func() {
		_, err := retriever.Get(context.Background(), "key")
		waiter <- err
	}()
	require.Eventually(t, func() bool { return observations.Load() >= 2 }, time.Second, time.Millisecond)
	cancelStarter()
	select {
	case err := <-starter:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("canceled starter remained blocked on shared refresh")
	}

	select {
	case err := <-waiter:
		t.Fatalf("shared refresh completed before release: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-waiter)
	assert.Equal(t, int64(1), calls.Load())
}

func TestJWKS_CanceledWaiterDoesNotCancelSharedRefresh(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	body := marshalJWKS(t, testPublicJWK(t, "key"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write(body)
	}))
	defer server.Close()
	retriever := newTestJWKS(t, server.Client(), server.URL, time.Now, 1)

	first := make(chan error, 1)
	go func() {
		_, err := retriever.Get(context.Background(), "key")
		first <- err
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	second := make(chan error, 1)
	go func() {
		_, err := retriever.Get(ctx, "key")
		second <- err
	}()
	cancel()
	require.ErrorIs(t, <-second, context.Canceled)
	close(release)
	require.NoError(t, <-first)
}

func newTestJWKS(t *testing.T, client *http.Client, url string, now func() time.Time, maxKeys int) *JWKS {
	t.Helper()
	retriever, err := NewJWKS(JWKSConfig{
		ServiceContext:  context.Background(),
		Client:          client,
		URL:             url,
		RequestTimeout:  time.Second,
		MaxKeys:         maxKeys,
		RefreshInterval: time.Minute,
		MaxAge:          2 * time.Minute,
		Now:             now,
	})
	require.NoError(t, err)
	return retriever
}

func testPublicJWK(t *testing.T, keyID string) jose.JSONWebKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return jose.JSONWebKey{Key: &private.PublicKey, KeyID: keyID, Algorithm: string(jose.ES256), Use: "sig"}
}

func testPrivateJWK(t *testing.T, keyID string) jose.JSONWebKey {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return jose.JSONWebKey{Key: private, KeyID: keyID, Algorithm: string(jose.ES256), Use: "sig"}
}

func marshalJWKS(t *testing.T, keys ...jose.JSONWebKey) []byte {
	t.Helper()
	body, err := json.Marshal(jose.JSONWebKeySet{Keys: keys})
	require.NoError(t, err)
	return body
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(now time.Time) *testClock {
	return &testClock{now: now}
}

func (clock *testClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *testClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.now = clock.now.Add(duration)
}
