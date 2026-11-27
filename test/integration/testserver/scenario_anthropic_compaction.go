package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
)

func init() {
	registerScenario("anthropic-signed-compaction", handleAnthropicSignedCompaction)
}

type compactionTransport struct{}

func (compactionTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if err := r.Body.Close(); err != nil {
		return nil, fmt.Errorf("closing Anthropic test request: %w", err)
	}
	const sse = `event: message_start
data: {"type":"message_start","message":{"id":"msg_compaction","type":"message","role":"assistant","model":"claude-sonnet-4-6","content":[],"stop_reason":null,"usage":{"input_tokens":0,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"compaction","content":"Résumé  \n","signature":"test-signature"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"compaction"},"usage":{"output_tokens":0}}

event: message_stop
data: {"type":"message_stop"}

`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(sse)),
		Request:    r,
	}, nil
}

func handleAnthropicSignedCompaction(w http.ResponseWriter, r *http.Request) {
	model := anthropic.New("test-key", "claude-sonnet-4-6",
		anthropic.WithRequestOptions(option.WithHTTPClient(&http.Client{Transport: compactionTransport{}}), option.WithMaxRetries(0)),
	)
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("Summarize")),
		aisdk.WithProviderOptions(anthropic.AnthropicOptions{Compaction: &anthropic.CompactionConfig{Type: anthropic.CompactionSummarize}}),
	)
	if err := aisdk.PipeUIMessageStreamToResponse(w, result.ToUIMessageStream()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
