package anthropic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_ConcurrentTransportCapture(t *testing.T) {
	for _, vertex := range []bool{false, true} {
		name := "direct"
		if vertex {
			name = "vertex"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("reading request: %v", err)
				}
				digest := sha256.Sum256(body)
				w.Header().Set("X-Call", r.Header.Get("X-Call"))
				w.Header().Set("X-Body-Digest", hex.EncodeToString(digest[:]))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"msg_test","type":"message","role":"assistant","content":[],"model":"claude-test","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
			}))
			defer server.Close()
			m := newTransportTestModel(t, server, vertex)
			type outcome struct {
				marker string
				result *provider.GenerateResult
				err    error
			}
			results := make(chan outcome, 2)
			for _, marker := range []string{"first", "second"} {
				go func(marker string) {
					maxTokens := 64
					result, err := m.DoGenerate(context.Background(), provider.CallOptions{
						Prompt: []provider.Message{provider.UserText(marker)}, MaxOutputTokens: &maxTokens,
						Headers: map[string]string{"X-Call": marker},
					})
					results <- outcome{marker: marker, result: result, err: err}
				}(marker)
			}
			for range 2 {
				got := <-results
				require.NoError(t, got.err)
				require.NotNil(t, got.result.Request)
				require.NotNil(t, got.result.Response)
				assert.Equal(t, got.marker, got.result.Response.Headers["x-call"])
				assert.Contains(t, string(got.result.Request.Body), got.marker)
				digest := sha256.Sum256(got.result.Request.Body)
				assert.Equal(t, hex.EncodeToString(digest[:]), got.result.Response.Headers["x-body-digest"])
			}
		})
	}
}
