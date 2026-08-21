package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/grafana/ai-sdk/internal/ptr"
	"github.com/grafana/ai-sdk/provider"
)

// Sentinel errors for UIMessageStreamWriter.
var (
	ErrWriterClosed  = errors.New("aisdk: write on closed stream writer")
	ErrAlreadyClosed = errors.New("aisdk: stream writer already closed")
)

const (
	// defaultWriterBuffer is the channel buffer size for UIMessageStreamWriter.
	defaultWriterBuffer = 64
)

// UIMessageStreamWriter writes UIMessageChunk events to a stream.
// It is safe for concurrent use.
type UIMessageStreamWriter struct {
	ch     chan UIMessageChunk
	closed bool
	mu     sync.Mutex
}

func newUIMessageStreamWriter(bufSize int) *UIMessageStreamWriter {
	return &UIMessageStreamWriter{
		ch: make(chan UIMessageChunk, bufSize),
	}
}

// Write sends a chunk to the stream. Returns ErrWriterClosed if the writer
// has been closed.
func (w *UIMessageStreamWriter) Write(chunk UIMessageChunk) (retErr error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return ErrWriterClosed
	}
	ch := w.ch
	w.mu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			retErr = ErrWriterClosed
		}
	}()
	ch <- chunk
	return nil
}

// Merge reads all chunks from the given channel and writes them to this stream.
func (w *UIMessageStreamWriter) Merge(stream <-chan UIMessageChunk) error {
	for chunk := range stream {
		if err := w.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the writer. No further writes are allowed.
// Returns ErrAlreadyClosed if called more than once.
func (w *UIMessageStreamWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrAlreadyClosed
	}
	w.closed = true
	close(w.ch)
	return nil
}

func (w *UIMessageStreamWriter) output() <-chan UIMessageChunk {
	return w.ch
}

// CreateUIMessageStreamParams configures CreateUIMessageStream.
type CreateUIMessageStreamParams struct {
	Execute          func(writer *UIMessageStreamWriter) error
	OnError          func(error) string
	OriginalMessages []UIMessage
	OnFinish         func(UIMessageStreamOnFinishState)
	GenerateID       func() string
}

// UIMessageStreamOnFinishState is passed to the OnFinish callback.
type UIMessageStreamOnFinishState struct {
	Messages        []UIMessage
	IsContinuation  bool
	IsAborted       bool
	ResponseMessage UIMessage
	FinishReason    provider.FinishReason
}

// CreateUIMessageStream creates a standalone UIMessageChunk stream.
// The Execute callback receives a writer; chunks written to it appear
// on the returned channel. The stream is framed with start/finish chunks.
func CreateUIMessageStream(params CreateUIMessageStreamParams) <-chan UIMessageChunk {
	out := make(chan UIMessageChunk, defaultWriterBuffer)

	go func() {
		defer close(out)

		writer := newUIMessageStreamWriter(defaultWriterBuffer)

		genID := params.GenerateID
		if genID == nil {
			genID = GenerateID
		}

		// Emit start chunk
		var messageID string
		if params.OriginalMessages != nil {
			messageID = genID()
		}
		out <- UIMessageChunk{Type: ChunkStart, MessageID: messageID}

		// Collect chunks for message assembly
		var chunks []UIMessageChunk

		// Run execute in a goroutine, forward writer output to out
		done := make(chan error, 1)
		go func() {
			defer func() { _ = writer.Close() }()
			done <- params.Execute(writer)
		}()

		for chunk := range writer.output() {
			chunks = append(chunks, chunk)
			out <- chunk
		}

		execErr := <-done
		if execErr != nil {
			errText := "An error occurred"
			if params.OnError != nil {
				errText = params.OnError(execErr)
			}
			out <- UIMessageChunk{Type: ChunkError, ErrorText: errText}
		}

		// Assemble response message for callbacks
		if params.OnFinish != nil && params.OriginalMessages != nil {
			respMsg := assembleResponseMessage(messageID, chunks)
			state := UIMessageStreamOnFinishState{
				Messages:        append(params.OriginalMessages, respMsg),
				ResponseMessage: respMsg,
			}
			params.OnFinish(state)
		}

		// Emit finish chunk
		out <- UIMessageChunk{Type: ChunkFinish}
	}()

	return out
}

func assembleResponseMessage(messageID string, chunks []UIMessageChunk) UIMessage {
	return assembleResponseMessageWithInitial(messageID, chunks, nil)
}

func assembleResponseMessageWithInitial(messageID string, chunks []UIMessageChunk, initial *UIMessage) UIMessage {
	cfg := uiMessageReaderConfig{generateID: func() string { return messageID }}
	state := newUIMessageReaderState(cfg)
	if initial != nil && initial.Role == RoleAssistant {
		state.message = cloneUIMessage(*initial)
	} else {
		state.message.ID = messageID
	}
	if state.message.ID == "" {
		state.message.ID = messageID
	}
	for _, chunk := range chunks {
		if chunk.Type == ChunkError {
			continue
		}
		_, _ = state.apply(chunk)
	}
	return state.finalMessage()
}

type uiMessageStreamConfig struct {
	originalMessages    []UIMessage
	hasOriginalMessages bool
	generateMessageID   func() string
	onFinish            func(UIMessageStreamOnFinishState)
	messageMetadata     func(part TextStreamPart) json.RawMessage
	sendReasoning       *bool
	sendSources         *bool
	sendFinish          *bool
	sendStart           *bool
	onError             func(error) string
}

// UIMessageStreamOption configures UI message stream helpers.
type UIMessageStreamOption interface {
	applyUIMessageStream(*uiMessageStreamConfig)
	uiMessageStreamOption()
}

type uiMessageStreamOptionFunc func(*uiMessageStreamConfig)

func (f uiMessageStreamOptionFunc) applyUIMessageStream(cfg *uiMessageStreamConfig) { f(cfg) }
func (uiMessageStreamOptionFunc) uiMessageStreamOption()                            {}

// WithUIMessageStreamOriginalMessages configures the original messages used
// for continuation IDs and finish callbacks. Calling this option, even with no
// messages, marks original messages as explicitly present.
func WithUIMessageStreamOriginalMessages(messages ...UIMessage) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.hasOriginalMessages = true
		cfg.originalMessages = append([]UIMessage{}, messages...)
	})
}

// WithUIMessageStreamGenerateID configures generated UI message IDs.
func WithUIMessageStreamGenerateID(fn func() string) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.generateMessageID = fn
	})
}

// OnUIMessageStreamFinish configures a callback invoked after stream finish.
func OnUIMessageStreamFinish(fn func(UIMessageStreamOnFinishState)) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.onFinish = fn
	})
}

// WithUIMessageStreamMessageMetadata configures message metadata extraction.
func WithUIMessageStreamMessageMetadata(fn func(part TextStreamPart) json.RawMessage) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.messageMetadata = fn
	})
}

// WithUIMessageStreamReasoning configures whether reasoning chunks are sent.
func WithUIMessageStreamReasoning(send bool) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.sendReasoning = &send
	})
}

// WithUIMessageStreamSources configures whether source chunks are sent.
func WithUIMessageStreamSources(send bool) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.sendSources = &send
	})
}

// WithUIMessageStreamFinish configures whether finish chunks are sent.
func WithUIMessageStreamFinish(send bool) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.sendFinish = &send
	})
}

// WithUIMessageStreamStart configures whether start chunks are sent.
func WithUIMessageStreamStart(send bool) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.sendStart = &send
	})
}

// OnUIMessageStreamError configures error-to-text conversion for UI streams.
func OnUIMessageStreamError(fn func(error) string) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.onError = fn
	})
}

func buildUIMessageStreamConfig(opts []UIMessageStreamOption) uiMessageStreamConfig {
	var cfg uiMessageStreamConfig
	for _, opt := range opts {
		if opt != nil {
			opt.applyUIMessageStream(&cfg)
		}
	}
	return cfg
}

func shouldSendChunk(chunk UIMessageChunk, cfg uiMessageStreamConfig) bool {
	sendReasoning := ptr.Deref(cfg.sendReasoning, true)
	sendSources := ptr.Deref(cfg.sendSources, false)
	sendFinish := ptr.Deref(cfg.sendFinish, true)
	sendStart := ptr.Deref(cfg.sendStart, true)

	switch chunk.Type {
	case ChunkStart:
		return sendStart
	case ChunkFinish:
		return sendFinish
	case ChunkReasoningStart, ChunkReasoningDelta, ChunkReasoningEnd, ChunkReasoningFile:
		return sendReasoning
	case ChunkSourceURL, ChunkSourceDocument:
		return sendSources
	default:
		return true
	}
}

func filterChunks(chunks <-chan UIMessageChunk, cfg uiMessageStreamConfig) <-chan UIMessageChunk {
	out := make(chan UIMessageChunk, defaultWriterBuffer)
	go func() {
		defer close(out)
		for chunk := range chunks {
			if shouldSendChunk(chunk, cfg) {
				out <- chunk
			}
		}
	}()
	return out
}

// FormatSSEEvent serializes a UIMessageChunk as an SSE data line.
func FormatSSEEvent(chunk UIMessageChunk) ([]byte, error) {
	b, err := json.Marshal(chunk)
	if err != nil {
		return nil, fmt.Errorf("marshaling chunk: %w", err)
	}
	return append(append([]byte("data: "), b...), '\n', '\n'), nil
}

// SSEDone is the sentinel SSE event that terminates a stream.
var SSEDone = []byte("data: [DONE]\n\n")
