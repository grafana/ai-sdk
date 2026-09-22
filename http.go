package aisdk

import (
	"fmt"
	"net/http"
)

// PipeUIMessageStreamToResponse writes a UIMessageChunk stream to an HTTP
// response as Server-Sent Events. It sets the appropriate SSE headers and
// terminates with a [DONE] sentinel.
//
// When a write fails, it returns at once and drains the rest of the stream in
// the background, so the producer never blocks on a full buffer. Bind that
// producer to the request context so a client disconnect ends it.
func PipeUIMessageStreamToResponse(w http.ResponseWriter, stream <-chan UIMessageChunk) error {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("x-vercel-ai-ui-message-stream", "v1")
	w.Header().Set("x-accel-buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	// An early return leaves chunks in the stream, so drain it in the background:
	// the producing goroutine never blocks on a full buffer and the handler still
	// returns at once. The loop drains it on the way out.
	drained := false
	defer func() {
		if drained {
			return
		}
		go func() {
			for range stream { //nolint:revive // discard remaining chunks
			}
		}()
	}()

	for chunk := range stream {
		event, err := FormatSSEEvent(chunk)
		if err != nil {
			return fmt.Errorf("formatting SSE event: %w", err)
		}
		if _, err := w.Write(event); err != nil {
			return fmt.Errorf("writing SSE event: %w", err)
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	drained = true
	if _, err := w.Write(SSEDone); err != nil {
		return fmt.Errorf("writing DONE: %w", err)
	}
	if flusher != nil {
		flusher.Flush()
	}

	return nil
}

// WriteUIMessageStream is a shortcut that converts a StreamTextResult to a
// UIMessageChunk stream and pipes it to the HTTP response as SSE.
func WriteUIMessageStream(w http.ResponseWriter, result *StreamTextResult, opts ...UIMessageStreamOption) error {
	stream := result.ToUIMessageStream(opts...)
	return PipeUIMessageStreamToResponse(w, stream)
}

// WriteTextStream writes the text deltas from a StreamTextResult to an HTTP
// response as a plain text stream (for useCompletion).
func WriteTextStream(w http.ResponseWriter, result *StreamTextResult) error {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("x-accel-buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)

	for part := range result.FullStream() {
		if p, ok := part.(StreamTextDelta); ok {
			if _, err := w.Write([]byte(p.Text)); err != nil {
				return fmt.Errorf("writing text stream: %w", err)
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
	if err := result.Err(); err != nil {
		return fmt.Errorf("streaming text response: %w", err)
	}
	return nil
}

// StreamUIMessage reads a UIMessageChunk stream and emits progressive
// UIMessage snapshots at upstream-compatible write points.
func StreamUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) <-chan UIMessage {
	out := make(chan UIMessage, defaultWriterBuffer)
	cfg := buildUIMessageReaderConfig(opts)
	go func() {
		defer close(out)
		state := newUIMessageReaderState(cfg)
		for chunk := range stream {
			if chunk.Type == ChunkError {
				continue
			}
			write, err := state.apply(chunk)
			if err != nil {
				return
			}
			if write {
				out <- state.snapshot()
			}
		}
	}()
	return out
}

// AssembleUIMessage reads a UIMessageChunk stream to completion and returns the
// final assembled assistant message.
func AssembleUIMessage(stream <-chan UIMessageChunk, opts ...UIMessageReaderOption) (UIMessage, error) {
	cfg := buildUIMessageReaderConfig(opts)
	state := newUIMessageReaderState(cfg)
	var streamErr error
	for chunk := range stream {
		if chunk.Type == ChunkError {
			if streamErr == nil {
				streamErr = fmt.Errorf("aisdk: ui message stream error: %s", chunk.ErrorText)
			}
			continue
		}
		if _, err := state.apply(chunk); err != nil {
			return UIMessage{}, err
		}
	}
	msg := state.finalMessage()
	if streamErr != nil {
		return msg, streamErr
	}
	return msg, nil
}
