package openai

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
)

// consumeStream drives the Responses SSE stream and emits provider.StreamParts.
// Errors are emitted as PartError carrying an APICallError; the stream is never
// fatal across the API boundary.
func consumeStream(stream *ssestream.Stream[responses.ResponseStreamEventUnion], ch chan<- provider.StreamPart, warnings []provider.Warning, br buildResult, generateID func() string, providerName string) {
	adapter := newStreamAdapter(warnings, br, generateID, providerName)

	// Ensure stream-start is emitted even when no events arrive.
	if !adapter.startEmitted {
		adapter.startEmitted = true
		ch <- provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}
	}

	for stream.Next() {
		adapter.handleEvent(stream.Current(), ch)
	}

	if err := stream.Err(); err != nil {
		ch <- provider.StreamPart{
			Type:         provider.PartError,
			APICallError: wrapAsAPICallError(err),
		}
	}
}
