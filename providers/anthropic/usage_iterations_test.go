package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_IterationUsage(t *testing.T) {
	for _, tc := range []struct {
		name           string
		usage          string
		startUsage     string
		wantNoCache    int
		wantCacheRead  int
		wantCacheWrite int
		wantOutput     int
	}{
		{
			name: "on-demand compaction cache read",
			usage: `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"iterations":[
				{"type":"compaction","input_tokens":112,"output_tokens":281,"cache_read_input_tokens":79228,"cache_creation_input_tokens":0}]}`,
			wantNoCache: 112, wantCacheRead: 79228, wantOutput: 281,
		},
		{
			name: "on-demand compaction cache write",
			usage: `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"iterations":[
				{"type":"compaction","input_tokens":112,"output_tokens":281,"cache_read_input_tokens":0,"cache_creation_input_tokens":79228}]}`,
			wantNoCache: 112, wantCacheWrite: 79228, wantOutput: 281,
		},
		{
			name: "threshold compaction includes both iterations",
			usage: `{"input_tokens":30,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":5,"iterations":[
				{"type":"compaction","input_tokens":112,"output_tokens":281,"cache_read_input_tokens":79228,"cache_creation_input_tokens":71},
				{"type":"message","input_tokens":30,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":5}]}`,
			wantNoCache: 142, wantCacheRead: 79231, wantCacheWrite: 76, wantOutput: 285,
		},
		{
			name:       "threshold delta zero does not lose compaction cache",
			startUsage: `{"input_tokens":112,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":79299}`,
			usage: `{"input_tokens":30,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"iterations":[
				{"type":"compaction","input_tokens":112,"output_tokens":281,"cache_read_input_tokens":0,"cache_creation_input_tokens":79299},
				{"type":"message","input_tokens":30,"output_tokens":4,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}]}`,
			wantNoCache: 142, wantCacheWrite: 79299, wantOutput: 285,
		},
		{
			name: "server tool loop cache is counted once",
			usage: `{"input_tokens":30,"output_tokens":7,"cache_read_input_tokens":80,"cache_creation_input_tokens":60,"iterations":[
				{"type":"message","input_tokens":10,"output_tokens":3,"cache_read_input_tokens":30,"cache_creation_input_tokens":20},
				{"type":"message","input_tokens":20,"output_tokens":4,"cache_read_input_tokens":50,"cache_creation_input_tokens":40}]}`,
			wantNoCache: 30, wantCacheRead: 80, wantCacheWrite: 60, wantOutput: 7,
		},
		{
			name: "advisor tokens excluded from executor totals",
			usage: `{"input_tokens":30,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":5,"iterations":[
				{"type":"compaction","input_tokens":100,"output_tokens":10,"cache_read_input_tokens":7,"cache_creation_input_tokens":11},
				{"type":"advisor_message","model":"claude-haiku-4-5","input_tokens":200,"output_tokens":20,"cache_read_input_tokens":1000,"cache_creation_input_tokens":2000},
				{"type":"message","input_tokens":30,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":5}]}`,
			wantNoCache: 130, wantCacheRead: 10, wantCacheWrite: 16, wantOutput: 14,
		},
		{
			name: "fallback preserves top-level usage",
			usage: `{"input_tokens":40,"output_tokens":7,"cache_read_input_tokens":1,"cache_creation_input_tokens":2,"iterations":[
				{"type":"message","input_tokens":50,"output_tokens":0,"cache_read_input_tokens":300,"cache_creation_input_tokens":400},
				{"type":"fallback_message","model":"claude-sonnet-4-6","input_tokens":40,"output_tokens":7,"cache_read_input_tokens":10,"cache_creation_input_tokens":20},
				{"type":"compaction","input_tokens":100,"output_tokens":10,"cache_read_input_tokens":30,"cache_creation_input_tokens":40}]}`,
			wantNoCache: 40, wantCacheRead: 1, wantCacheWrite: 2, wantOutput: 7,
		},
		{
			name: "advisor only preserves top-level usage",
			usage: `{"input_tokens":40,"output_tokens":7,"cache_read_input_tokens":1,"cache_creation_input_tokens":2,"iterations":[
				{"type":"advisor_message","model":"claude-haiku-4-5","input_tokens":200,"output_tokens":20,"cache_read_input_tokens":1000,"cache_creation_input_tokens":2000}]}`,
			wantNoCache: 40, wantCacheRead: 1, wantCacheWrite: 2, wantOutput: 7,
		},
		{
			name:        "absent iterations preserve top-level usage",
			usage:       `{"input_tokens":40,"output_tokens":7,"cache_read_input_tokens":1,"cache_creation_input_tokens":2}`,
			wantNoCache: 40, wantCacheRead: 1, wantCacheWrite: 2, wantOutput: 7,
		},
		{
			name:        "empty iterations preserve top-level usage",
			usage:       `{"input_tokens":40,"output_tokens":7,"cache_read_input_tokens":1,"cache_creation_input_tokens":2,"iterations":[]}`,
			wantNoCache: 40, wantCacheRead: 1, wantCacheWrite: 2, wantOutput: 7,
		},
		{
			name:        "null iterations preserve top-level usage",
			usage:       `{"input_tokens":40,"output_tokens":7,"cache_read_input_tokens":1,"cache_creation_input_tokens":2,"iterations":null}`,
			wantNoCache: 40, wantCacheRead: 1, wantCacheWrite: 2, wantOutput: 7,
		},
		{
			name: "missing and null iteration cache count as zero",
			usage: `{"input_tokens":30,"output_tokens":4,"cache_read_input_tokens":3,"cache_creation_input_tokens":5,"iterations":[
				{"type":"compaction","input_tokens":100,"output_tokens":10,"cache_read_input_tokens":null,"cache_creation_input_tokens":null},
				{"type":"message","input_tokens":30,"output_tokens":4}]}`,
			wantNoCache: 130, wantOutput: 14,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage bytes.Buffer
			require.NoError(t, json.Compact(&usage, []byte(tc.usage)))
			for _, mode := range []string{"generate", "stream"} {
				t.Run(mode, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if mode == "generate" {
							w.Header().Set("Content-Type", "application/json")
							_, _ = fmt.Fprint(w, `{"id":"msg_usage","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":"end_turn","usage":`+tc.usage+`}`)
							return
						}
						start := tc.startUsage
						if start == "" {
							start = `{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprint(w, "event: message_start\ndata: "+`{"type":"message_start","message":{"id":"msg_usage","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"usage":`+start+`}}`+"\n\n")
						_, _ = fmt.Fprint(w, "event: message_delta\ndata: "+`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":`+usage.String()+`}`+"\n\n")
						_, _ = fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
					}))
					defer server.Close()
					model := New("test-key", "claude-sonnet-4-6", WithRequestOptions(option.WithBaseURL(server.URL), option.WithHTTPClient(server.Client()), option.WithMaxRetries(0)))
					opts := provider.CallOptions{Prompt: []provider.Message{provider.UserText("hello")}, MaxOutputTokens: ptrInt(1024)}
					var got *provider.Usage
					if mode == "generate" {
						result, err := model.DoGenerate(t.Context(), opts)
						require.NoError(t, err)
						got = &result.Usage
					} else {
						result, err := model.DoStream(t.Context(), opts)
						require.NoError(t, err)
						finishes := 0
						for part := range result.Stream {
							require.NotEqual(t, provider.PartError, part.Type, "%+v", part)
							if part.Type == provider.PartFinish {
								finishes++
								got = part.Usage
							}
						}
						require.Equal(t, 1, finishes)
					}
					require.NotNil(t, got)
					assert.Equal(t, provider.InputTokenUsage{
						Total:      ptrInt(tc.wantNoCache + tc.wantCacheRead + tc.wantCacheWrite),
						NoCache:    ptrInt(tc.wantNoCache),
						CacheRead:  ptrInt(tc.wantCacheRead),
						CacheWrite: ptrInt(tc.wantCacheWrite),
					}, got.InputTokens)
					assert.Equal(t, ptrInt(tc.wantOutput), got.OutputTokens.Total)
					assert.JSONEq(t, tc.usage, string(got.Raw))
				})
			}
		})
	}
}
