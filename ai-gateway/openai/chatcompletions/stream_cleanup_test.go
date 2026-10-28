package chatcompletions

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaimedStreamCleanupDoesNotDelayFailure(t *testing.T) {
	for _, setupFailure := range []bool{true, false} {
		name := "committed failure"
		if setupFailure {
			name = "setup failure"
		}
		t.Run(name, func(t *testing.T) {
			ch := make(chan provider.StreamPart, 1)
			defer close(ch)
			cancelled := make(chan struct{})
			f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				go func() { <-ctx.Done(); close(cancelled) }()
				if setupFailure {
					return &provider.StreamResult{Stream: ch}, errors.New("private setup failure")
				}
				ch <- provider.StreamPart{Type: provider.PartSource}
				return &provider.StreamResult{Stream: ch}, nil
			}}
			h := testHandler(t, f)
			server := httptest.NewServer(h)
			defer server.Close()
			start := time.Now()
			res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
			require.NoError(t, err)
			defer func() { _ = res.Body.Close() }()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Less(t, time.Since(start), 100*time.Millisecond, "response must not wait for the runtime cleanup budget")
			assert.Contains(t, string(body), `"error"`)
			assert.NotContains(t, string(body), "[DONE]")
			assert.NotContains(t, string(body), "private")
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("provider context not cancelled")
			}
		})
	}
}
