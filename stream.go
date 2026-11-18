package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/grafana/ai-sdk/provider"
)

// Sentinel errors for UIMessageStreamWriter.
var (
	ErrWriterClosed  = errors.New("aisdk: write on closed stream writer")
	ErrAlreadyClosed = errors.New("aisdk: stream writer already closed")
	// ErrExecuteRequired reaches OnError when CreateUIMessageStreamParams.Execute is nil.
	ErrExecuteRequired = errors.New("aisdk: CreateUIMessageStreamParams.Execute is required")
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

// runExecute runs Execute, turning a missing callback or a panic into an error
// so it reaches OnError as an error chunk instead of killing the process. A
// panicked error keeps its chain so errors.As still works in OnError.
func runExecute(execute func(*UIMessageStreamWriter) error, writer *UIMessageStreamWriter) (err error) {
	defer func() {
		switch r := recover().(type) {
		case nil:
		case error:
			err = fmt.Errorf("aisdk: panic in Execute: %w", r)
		default:
			err = fmt.Errorf("aisdk: panic in Execute: %v", r)
		}
	}()
	if execute == nil {
		return ErrExecuteRequired
	}
	return execute(writer)
}

// CreateUIMessageStreamParams configures CreateUIMessageStream.
type CreateUIMessageStreamParams struct {
	// Execute writes the whole stream through the supplied writer. It is required.
	Execute func(writer *UIMessageStreamWriter) error
	// OnError maps a stream failure to the text of the emitted error chunk. It
	// receives the error Execute returns, ErrExecuteRequired when Execute is nil,
	// and a panic recovered from Execute. That text reaches the client, so return
	// err.Error() only when every one of those sources is safe to disclose. The
	// default masks them all as "An error occurred." and discards the detail, so
	// log it here when an operator needs it.
	OnError func(error) string
	// OriginalMessages is the prior conversation. It supplies the continuation
	// message ID and history for callbacks; it is not a persistence mode switch.
	OriginalMessages []UIMessage
	// OnStepEnd runs after each finish-step chunk is applied and before it is
	// forwarded, including for chunks from a merged stream.
	OnStepEnd func(UIMessageStreamOnStepEndState)
	// OnFinish runs once the stream ends, whether or not OriginalMessages is set.
	OnFinish func(UIMessageStreamOnFinishState)
	// GenerateID mints the response message ID. It defaults to the package-level
	// GenerateID and is called at most once per stream: a continued assistant
	// message supplies its own ID instead, and GenerateID is not called.
	GenerateID func() string
}

// UIMessageStreamOnStepEndState contains the cumulative response at a step
// boundary. Messages replaces the last original assistant message when
// IsContinuation is true and appends the response otherwise. The message
// snapshots are isolated from the stream and from each other.
type UIMessageStreamOnStepEndState struct {
	Messages        []UIMessage
	IsContinuation  bool
	ResponseMessage UIMessage
	// AssemblyError reports assembly errors up to and including this step.
	// A non-nil value means the snapshot may be missing content the client saw.
	AssemblyError error
}

// UIMessageStreamOnFinishState is passed to the OnFinish callback.
type UIMessageStreamOnFinishState struct {
	Messages        []UIMessage
	IsContinuation  bool
	IsAborted       bool
	ResponseMessage UIMessage
	FinishReason    provider.FinishReason
	// AssemblyError reports chunks that could not be applied while building
	// ResponseMessage, which is otherwise complete. Check it before persisting:
	// a non-nil value means the stored message is missing content the client saw.
	AssemblyError error
}

// CreateUIMessageStream creates a standalone UIMessageChunk stream.
// The Execute callback owns the whole stream: nothing is added to it, so
// Execute writes its own StartChunk and FinishChunk, or merges a stream that
// already carries them, such as StreamTextResult.ToUIMessageStream.
//
// A start chunk written without a message ID receives the response message ID:
// the last OriginalMessages entry's ID when that entry is an assistant message,
// which continues it, and otherwise a generated ID. A start chunk that carries
// its own ID keeps it, which lets a caller supply a request-scoped ID. Without
// a start chunk, GenerateID and OriginalMessages affect only callback state.
// When Execute merges a stream that stamps its own start chunk, that ID wins.
//
// A nil Execute, or a panic inside it, ends the stream with an error chunk
// rather than a crash. A chunk that cannot be applied while assembling a
// callback snapshot leaves the wire untouched and is reported through the
// callback state's AssemblyError.
func CreateUIMessageStream(params CreateUIMessageStreamParams) <-chan UIMessageChunk {
	out := make(chan UIMessageChunk, defaultWriterBuffer)

	go func() {
		defer close(out)

		writer := newUIMessageStreamWriter(defaultWriterBuffer)

		// Like upstream createUIMessageStream, emit no start or finish chunk of
		// our own. Start chunks without a message ID get the response ID, which
		// continues the last original message when it is an assistant message.
		cfg := uiMessageStreamConfig{
			originalMessages:    params.OriginalMessages,
			hasOriginalMessages: true,
			generateMessageID:   params.GenerateID,
			onStepEnd:           params.OnStepEnd,
			onFinish:            params.OnFinish,
			onError:             params.OnError,
		}
		messageID := responseUIMessageID(cfg)
		assembly := cfg.newCallbackAssembly(messageID)

		// Run execute in a goroutine, forward writer output to out
		done := make(chan error, 1)
		go func() {
			defer func() { _ = writer.Close() }()
			done <- runExecute(params.Execute, writer)
		}()

		for chunk := range writer.output() {
			if chunk.Type == ChunkStart && chunk.MessageID == "" {
				chunk.MessageID = messageID
			}
			assembly.beforeForward(chunk, cfg)
			out <- chunk
		}

		if execErr := <-done; execErr != nil {
			out <- UIMessageChunk{Type: ChunkError, ErrorText: errorText(execErr, cfg)}
		}

		assembly.finish(cfg)
	}()

	return out
}

type uiMessageStreamAssembly struct {
	reader       *uiMessageReaderState
	initErr      error
	applyErrs    []error
	finishReason provider.FinishReason
	isAborted    bool
}

func newUIMessageStreamAssembly(messageID string, initial *UIMessage) *uiMessageStreamAssembly {
	cfg := uiMessageReaderConfig{generateID: func() string { return messageID }}
	if initial != nil && initial.Role == RoleAssistant {
		cfg.initialMessage = initial
	}
	reader, err := newUIMessageReaderState(cfg)
	if err == nil && reader.message.ID == "" {
		reader.message.ID = messageID
	}
	return &uiMessageStreamAssembly{reader: reader, initErr: err}
}

func (cfg uiMessageStreamConfig) newCallbackAssembly(messageID string) *uiMessageStreamAssembly {
	if cfg.onStepEnd == nil && cfg.onFinish == nil {
		return nil
	}
	return newUIMessageStreamAssembly(messageID, lastOriginalAssistantMessage(cfg))
}

func (a *uiMessageStreamAssembly) apply(chunk UIMessageChunk) {
	switch chunk.Type {
	case ChunkError:
		return
	case ChunkFinish:
		if chunk.FinishReason != "" {
			a.finishReason = provider.FinishReason{Unified: provider.UnifiedFinishReason(chunk.FinishReason)}
		}
	case ChunkAbort:
		a.isAborted = true
	}
	if a.reader != nil {
		if _, err := a.reader.apply(chunk); err != nil {
			a.applyErrs = append(a.applyErrs, err)
		}
	}
}

func (a *uiMessageStreamAssembly) snapshot() (UIMessage, error) {
	if a.initErr != nil {
		return UIMessage{}, a.initErr
	}
	return a.reader.finalMessage(), errors.Join(a.applyErrs...)
}

func (a *uiMessageStreamAssembly) beforeForward(chunk UIMessageChunk, cfg uiMessageStreamConfig) {
	if a == nil {
		return
	}
	a.apply(chunk)
	if chunk.Type == ChunkFinishStep && cfg.onStepEnd != nil {
		response, err := a.snapshot()
		messages, isContinuation := cfg.historyWithResponse(response)
		for i := range messages {
			messages[i] = cloneUIMessage(messages[i])
		}
		cfg.onStepEnd(UIMessageStreamOnStepEndState{
			Messages:        messages,
			IsContinuation:  isContinuation,
			ResponseMessage: response,
			AssemblyError:   err,
		})
	}
}

func (a *uiMessageStreamAssembly) finish(cfg uiMessageStreamConfig) {
	if cfg.onFinish == nil {
		return
	}
	response, err := a.snapshot()
	messages, isContinuation := cfg.historyWithResponse(response)
	cfg.onFinish(UIMessageStreamOnFinishState{
		Messages:        messages,
		IsContinuation:  isContinuation,
		IsAborted:       a.isAborted,
		ResponseMessage: response,
		FinishReason:    a.finishReason,
		AssemblyError:   err,
	})
}

func (cfg uiMessageStreamConfig) historyWithResponse(response UIMessage) ([]UIMessage, bool) {
	last := lastOriginalAssistantMessage(cfg)
	isContinuation := last != nil && response.ID == last.ID
	original := cfg.originalMessages
	if isContinuation {
		original = original[:len(original)-1]
	}
	messages := make([]UIMessage, len(original)+1)
	copy(messages, original)
	messages[len(original)] = response
	return messages, isContinuation
}

type uiMessageStreamConfig struct {
	toolTypes           map[string]UserToolType
	originalMessages    []UIMessage
	hasOriginalMessages bool
	generateMessageID   func() string
	onStepEnd           func(UIMessageStreamOnStepEndState)
	onFinish            func(UIMessageStreamOnFinishState)
	messageMetadata     func(part TextStreamPart) json.RawMessage
	sendReasoning       *bool
	sendSources         *bool
	sendFinish          *bool
	sendStart           *bool
	onError             func(error) string
	redactToolErrors    *bool
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
// for continuation IDs and message snapshots. Calling this option, even with no
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

// OnUIMessageStreamStepEnd configures a synchronous callback for each emitted
// finish-step. The callback receives the assembled snapshot before the
// boundary is forwarded to the consumer. It is separate from OnStepEnd, which
// observes model generation rather than UI messages.
func OnUIMessageStreamStepEnd(fn func(UIMessageStreamOnStepEndState)) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.onStepEnd = fn
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
// Citations attached to text-end chunks and custom chunks are sent regardless.
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

// WithUIMessageStreamRedactToolErrors configures whether the error text of
// failed tool calls is replaced with the OnUIMessageStreamError result. By
// default tool errors are redacted only when OnUIMessageStreamError is set;
// otherwise the UI receives the error text the model received, so UI messages
// converted back to model messages reproduce the original history. Stream
// errors are masked by default either way, and provider-executed tool
// execution errors always keep their original text.
func WithUIMessageStreamRedactToolErrors(redact bool) UIMessageStreamOption {
	return uiMessageStreamOptionFunc(func(cfg *uiMessageStreamConfig) {
		cfg.redactToolErrors = &redact
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

func sendOption(ptr *bool, defaultVal bool) bool {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

func shouldSendChunk(chunk UIMessageChunk, cfg uiMessageStreamConfig) bool {
	sendReasoning := sendOption(cfg.sendReasoning, true)
	sendSources := sendOption(cfg.sendSources, false)
	sendFinish := sendOption(cfg.sendFinish, true)
	sendStart := sendOption(cfg.sendStart, true)

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
