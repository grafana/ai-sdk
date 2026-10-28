package customertelemetry

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gateTestTransport func(*http.Request) (*http.Response, error)

func (f gateTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestExportGate_AuthorityAndDeadline(t *testing.T) {
	for _, tc := range []struct {
		name   string
		revoke bool
		expiry bool
	}{{name: "timeout"}, {name: "expiry", expiry: true}, {name: "revoke", revoke: true}} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				life := time.Minute
				if tc.expiry {
					life = 100 * time.Millisecond
				}
				g := newExportGate("https://ingest.example.test/v1/traces", time.Now().Add(life), time.Second)
				defer g.revoke()
				done := make(chan error, 1)
				go func() {
					done <- g.Run(context.Background(), func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
				}()
				synctest.Wait()
				if tc.revoke {
					g.revoke()
				}
				err := <-done
				if tc.revoke {
					assert.ErrorIs(t, err, context.Canceled)
				} else {
					assert.ErrorIs(t, err, context.DeadlineExceeded)
				}
				if tc.expiry || tc.revoke {
					assert.ErrorIs(t, g.Run(context.Background(), func(context.Context) error { t.Error("unexpected export"); return nil }), ErrExportDenied)
				}
			})
		})
	}
}

func TestExportGate_HTTPBoundaries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := "https://ingest.example.test/v1/traces"
		g := newExportGate(endpoint, time.Now().Add(time.Minute), time.Second)
		defer g.revoke()
		_, err := g.HTTPClient(nil)
		assert.ErrorIs(t, err, ErrExportTarget)
		var calls atomic.Int64
		var transportContext context.Context
		client, err := g.HTTPClient(gateTestTransport(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			transportContext = r.Context()
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
		}))
		require.NoError(t, err)
		for _, url := range []string{"https://other.example.test/v1/traces", endpoint + "?key=value", "https://ingest.example.test/other", "http://ingest.example.test/v1/traces"} {
			_, err := client.Get(url)
			assert.ErrorIs(t, err, ErrExportTarget)
		}
		req, err := http.NewRequest(http.MethodPost, endpoint, nil)
		require.NoError(t, err)
		req.Host = "other.example.test"
		_, err = client.Do(req)
		assert.ErrorIs(t, err, ErrExportTarget)
		assert.Zero(t, calls.Load())
		response, err := client.Get(endpoint)
		require.NoError(t, err)
		assert.NoError(t, transportContext.Err())
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, "ok", string(body))
		assert.NoError(t, response.Body.Close())
		assert.ErrorIs(t, transportContext.Err(), context.Canceled)
		redirect, err := g.HTTPClient(gateTestTransport(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: 307, Header: http.Header{"Location": []string{"https://other.example.test/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}))
		require.NoError(t, err)
		_, err = redirect.Get(endpoint)
		assert.ErrorIs(t, err, ErrExportTarget)
		assert.EqualValues(t, 2, calls.Load())
	})
}
