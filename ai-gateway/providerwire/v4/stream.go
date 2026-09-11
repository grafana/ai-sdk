package v4

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/provider"
)

const (
	minimumMappedStreamWarningBytes   = int64(len(`{"type":"other","message":"the model reported a warning"}`))
	streamWarningUnsupportedFeature   = "model capability"
	streamWarningUnsupportedDetails   = "a requested model capability is unsupported"
	streamWarningCompatibilityFeature = "model compatibility"
	streamWarningCompatibilityDetails = "a requested setting was adjusted for model compatibility"
	streamWarningDeprecatedSetting    = "model setting"
	streamWarningDeprecatedMessage    = "a requested model setting is deprecated"
	streamWarningOtherMessage         = "the model reported a warning"
)

var (
	canonicalEmptyStartFrame              = []byte("data: {\"type\":\"stream-start\",\"warnings\":[]}\n\n")
	canonicalRateLimitStreamErrorFrame    = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"rate limit exceeded\",\"type\":\"rate_limit_exceeded\",\"param\":null,\"code\":\"rate_limit_exceeded\",\"statusCode\":429,\"retryable\":true}}\n\n")
	canonicalOverloadStreamErrorFrame     = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"service overloaded\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"overloaded\",\"statusCode\":503,\"retryable\":true}}\n\n")
	canonicalDependencyStreamErrorFrame   = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"failed dependency\",\"type\":\"failed_dependency\",\"param\":null,\"code\":\"failed_dependency\",\"statusCode\":424,\"retryable\":false}}\n\n")
	canonicalUpstreamStreamErrorFrame     = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"upstream failure\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"upstream_error\",\"statusCode\":502,\"retryable\":true}}\n\n")
	canonicalTimeoutStreamErrorFrame      = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"request timed out\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"timeout\",\"statusCode\":504,\"retryable\":true}}\n\n")
	canonicalCancellationStreamErrorFrame = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"request canceled\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"canceled\",\"statusCode\":499,\"retryable\":false}}\n\n")
	canonicalInternalStreamErrorFrame     = []byte("data: {\"type\":\"error\",\"error\":{\"message\":\"internal error\",\"type\":\"internal_server_error\",\"param\":null,\"code\":\"internal_error\",\"statusCode\":500,\"retryable\":true}}\n\n")
)

type streamWarning struct {
	Type    provider.WarningType `json:"type"`
	Feature string               `json:"feature,omitempty"`
	Setting string               `json:"setting,omitempty"`
	Message string               `json:"message,omitempty"`
	Details string               `json:"details,omitempty"`
}

type streamEvent struct {
	typeName     provider.StreamPartType
	warnings     []streamWarning
	id           string
	modelID      string
	delta        string
	timestamp    time.Time
	finishReason provider.FinishReason
	inputUsage   unaryInputTokenUsage
	outputUsage  unaryOutputTokenUsage
}

type streamStartEvent struct {
	Type     provider.StreamPartType `json:"type"`
	Warnings []streamWarning         `json:"warnings"`
}

type streamMetadataEvent struct {
	Type      provider.StreamPartType `json:"type"`
	ID        string                  `json:"id,omitempty"`
	ModelID   string                  `json:"modelId"`
	Timestamp string                  `json:"timestamp,omitempty"`
}

type streamTextEvent struct {
	Type  provider.StreamPartType `json:"type"`
	ID    string                  `json:"id"`
	Delta *string                 `json:"delta,omitempty"`
}

type streamFinishEvent struct {
	Type         provider.StreamPartType `json:"type"`
	Usage        unaryUsage              `json:"usage"`
	FinishReason unaryFinishReason       `json:"finishReason"`
}

func encodeStreamFrame(value streamEvent, limit int64) ([]byte, bool) {
	if !streamEventPreflight(value, limit) {
		return nil, false
	}

	var payload []byte
	var err error
	switch value.typeName {
	case provider.PartStreamStart:
		warnings := value.warnings
		if warnings == nil {
			warnings = []streamWarning{}
		}
		payload, err = json.Marshal(streamStartEvent{Type: value.typeName, Warnings: warnings})
	case provider.PartResponseMeta:
		var timestamp string
		if !value.timestamp.IsZero() {
			timestamp = value.timestamp.UTC().Format(time.RFC3339Nano)
		}
		payload, err = json.Marshal(streamMetadataEvent{Type: value.typeName, ID: value.id, ModelID: value.modelID, Timestamp: timestamp})
	case provider.PartTextStart, provider.PartTextEnd:
		payload, err = json.Marshal(streamTextEvent{Type: value.typeName, ID: value.id})
	case provider.PartTextDelta:
		payload, err = json.Marshal(streamTextEvent{Type: value.typeName, ID: value.id, Delta: &value.delta})
	case provider.PartFinish:
		payload, err = json.Marshal(streamFinishEvent{
			Type:         value.typeName,
			Usage:        unaryUsage{InputTokens: value.inputUsage, OutputTokens: value.outputUsage},
			FinishReason: unaryFinishReason{Unified: value.finishReason.Unified, Raw: value.finishReason.Raw},
		})
	default:
		return nil, false
	}
	if err != nil || int64(len(payload)) > limit-int64(len("data: ")+len("\n\n")) {
		return nil, false
	}

	frame := make([]byte, 0, len("data: ")+len(payload)+len("\n\n"))
	frame = append(frame, "data: "...)
	frame = append(frame, payload...)
	frame = append(frame, '\n', '\n')
	return frame, true
}

func streamEventPreflight(value streamEvent, limit int64) bool {
	if limit < int64(len("data: ")+len("{}\n\n")) {
		return false
	}
	remaining := limit
	check := func(values ...string) bool {
		for _, value := range values {
			if int64(len(value)) > remaining || !utf8.ValidString(value) {
				return false
			}
			remaining -= int64(len(value))
		}
		return true
	}

	if !check(string(value.typeName)) {
		return false
	}
	switch value.typeName {
	case provider.PartStreamStart:
		if !streamWarningCountFits(len(value.warnings), limit) {
			return false
		}
		for _, warning := range value.warnings {
			if !check(string(warning.Type), warning.Feature, warning.Setting, warning.Message, warning.Details) {
				return false
			}
		}
		return true
	case provider.PartResponseMeta:
		return check(value.id, value.modelID)
	case provider.PartTextStart, provider.PartTextEnd:
		return check(value.id)
	case provider.PartTextDelta:
		return check(value.id, value.delta)
	case provider.PartFinish:
		return check(string(value.finishReason.Unified), value.finishReason.Raw)
	default:
		return false
	}
}

func streamWarningCountFits(count int, limit int64) bool {
	if count == 0 {
		return int64(len(canonicalEmptyStartFrame)) <= limit
	}
	available := limit - int64(len(canonicalEmptyStartFrame))
	if available < minimumMappedStreamWarningBytes {
		return false
	}
	return int64(count) <= (available+1)/(minimumMappedStreamWarningBytes+1)
}

func mapStreamWarnings(warnings []provider.Warning, limit int64) ([]streamWarning, error) {
	if !streamWarningCountFits(len(warnings), limit) {
		return nil, errInvalidStreamWarning
	}
	mapped := make([]streamWarning, 0, len(warnings))
	for _, warning := range warnings {
		switch warning.Type {
		case provider.WarnUnsupported:
			mapped = append(mapped, streamWarning{
				Type:    warning.Type,
				Feature: streamWarningUnsupportedFeature,
				Details: streamWarningUnsupportedDetails,
			})
		case provider.WarnCompatibility:
			mapped = append(mapped, streamWarning{
				Type:    warning.Type,
				Feature: streamWarningCompatibilityFeature,
				Details: streamWarningCompatibilityDetails,
			})
		case provider.WarnDeprecated:
			mapped = append(mapped, streamWarning{
				Type:    warning.Type,
				Setting: streamWarningDeprecatedSetting,
				Message: streamWarningDeprecatedMessage,
			})
		case provider.WarnOther:
			mapped = append(mapped, streamWarning{
				Type:    warning.Type,
				Message: streamWarningOtherMessage,
			})
		default:
			return nil, errInvalidStreamWarning
		}
	}
	return mapped, nil
}

var errInvalidStreamWarning = errors.New("providerwire v4: invalid stream warning")

type streamOutcome struct {
	result *provider.StreamResult
	err    error
}

type streamPartCounter struct {
	count atomic.Int64
	limit int64
}

func newStreamPartCounter(limit int) *streamPartCounter {
	return &streamPartCounter{limit: int64(limit)}
}

func (c *streamPartCounter) take() bool     { return c.count.Add(1) <= c.limit }
func (c *streamPartCounter) exceeded() bool { return c.count.Load() > c.limit }

func (h *handler) serveStream(w http.ResponseWriter, requestContext context.Context, model provider.LanguageModel, options provider.CallOptions, modelID string) {
	if err := requestContext.Err(); err != nil {
		h.writeSafeError(w, safeErrorFromProvider(err))
		return
	}

	modelContext, cancel := context.WithTimeout(requestContext, h.limits.ModelDuration)
	counter := newStreamPartCounter(h.limits.StreamParts)
	outcomes := make(chan streamOutcome)
	go func() {
		outcome := callStream(modelContext, model, options)
		select {
		case outcomes <- outcome:
		case <-modelContext.Done():
			if outcome.result != nil {
				h.startStreamDrain(outcome.result.Stream, counter)
			}
		}
	}()

	var outcome streamOutcome
	select {
	case outcome = <-outcomes:
	case <-modelContext.Done():
		cancel()
		h.writeSafeError(w, safeError{category: streamContextErrorCategory(requestContext)})
		return
	}

	if !isNilInterface(outcome.err) || outcome.result == nil || outcome.result.Stream == nil {
		cancel()
		if outcome.result != nil {
			h.startStreamDrain(outcome.result.Stream, counter)
		}
		if !isNilInterface(outcome.err) {
			h.writeSafeError(w, safeErrorFromProvider(outcome.err))
		} else {
			h.writeSafeError(w, safeError{category: safeInternal})
		}
		return
	}

	defer func() {
		cancel()
		h.startStreamDrain(outcome.result.Stream, counter)
	}()
	idleTimer := time.NewTimer(h.limits.StreamIdleDuration)
	defer idleTimer.Stop()
	if !commitStreamResponse(w) {
		return
	}
	h.runStream(w, requestContext, modelContext, cancel, outcome.result.Stream, counter, idleTimer, modelID)
}

func callStream(ctx context.Context, model provider.LanguageModel, options provider.CallOptions) (outcome streamOutcome) {
	defer func() {
		if recover() != nil {
			outcome = streamOutcome{err: errModelInternal}
		}
		if outcome.result == nil && isNilInterface(outcome.err) {
			outcome.err = errModelInternal
		}
	}()
	outcome.result, outcome.err = model.DoStream(ctx, options)
	return outcome
}

func streamContextErrorCategory(requestContext context.Context) safeErrorCategory {
	if requestContext.Err() != nil {
		return safeCancellation
	}
	return safeTimeout
}

func commitStreamResponse(w http.ResponseWriter) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.WriteHeader(http.StatusOK)
	return flushStreamResponse(w)
}

func flushStreamResponse(w http.ResponseWriter) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	err := http.NewResponseController(w).Flush()
	return err == nil || errors.Is(err, http.ErrNotSupported)
}

func writeCompleteStreamFrame(w http.ResponseWriter, frame []byte) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	n, err := w.Write(frame)
	if err != nil || n != len(frame) {
		return false
	}
	return flushStreamResponse(w)
}

type streamWaitResult uint8

const (
	streamWaitPart streamWaitResult = iota + 1
	streamWaitClosed
	streamWaitCanceled
	streamWaitTotalTimeout
	streamWaitIdleTimeout
	streamWaitPartLimit
)

func waitStreamPart(requestContext, modelContext context.Context, stream <-chan provider.StreamPart, counter *streamPartCounter, idle <-chan time.Time) (provider.StreamPart, streamWaitResult) {
	select {
	case part, ok := <-stream:
		if !ok {
			return provider.StreamPart{}, streamWaitClosed
		}
		if !counter.take() {
			return provider.StreamPart{}, streamWaitPartLimit
		}
		return part, streamWaitPart
	case <-modelContext.Done():
		if requestContext.Err() != nil {
			return provider.StreamPart{}, streamWaitCanceled
		}
		return provider.StreamPart{}, streamWaitTotalTimeout
	case <-idle:
		return provider.StreamPart{}, streamWaitIdleTimeout
	}
}

type streamPartResult uint8

const (
	streamPartContinue streamPartResult = iota + 1
	streamPartFinished
	streamPartAdapterFailure
	streamPartWriterFailure
)

type streamState struct {
	metadataSeen bool
	textStarted  bool
	activeID     string
	usedIDs      map[string]struct{}
}

func newStreamState(limit int) *streamState {
	capacity := limit
	if capacity > 64 {
		capacity = 64
	}
	return &streamState{usedIDs: make(map[string]struct{}, capacity)}
}

func (h *handler) runStream(w http.ResponseWriter, requestContext, modelContext context.Context, cancel context.CancelFunc, stream <-chan provider.StreamPart, counter *streamPartCounter, idleTimer *time.Timer, modelID string) {
	part, waitResult := waitStreamPart(requestContext, modelContext, stream, counter, idleTimer.C)
	if waitResult != streamWaitPart {
		cancel()
		if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) != streamWriteSuccess {
			return
		}
		h.writeStreamTerminalForWait(w, waitResult)
		return
	}

	if modelID == "" || !utf8.ValidString(modelID) {
		cancel()
		if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) == streamWriteSuccess {
			h.writeStreamTerminalError(w, safeError{category: safeInternal})
		}
		return
	}

	if part.Type == provider.PartStreamStart {
		warnings, err := mapStreamWarnings(part.Warnings, h.limits.StreamFrameBytes)
		if err != nil {
			cancel()
			if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) == streamWriteSuccess {
				h.writeStreamTerminalError(w, safeError{category: safeInternal})
			}
			return
		}
		switch h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart, warnings: warnings}) {
		case streamWriteSuccess:
			idleTimer.Reset(h.limits.StreamIdleDuration)
		case streamWriteEncodingFailure:
			cancel()
			if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) == streamWriteSuccess {
				h.writeStreamTerminalError(w, safeError{category: safeInternal})
			}
			return
		default:
			return
		}
	} else {
		if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) != streamWriteSuccess {
			return
		}
		state := newStreamState(h.limits.StreamParts)
		result := h.processStreamPart(w, state, part, modelID)
		if h.handleStreamPartResult(w, cancel, result) {
			return
		}
		idleTimer.Reset(h.limits.StreamIdleDuration)
		h.consumeStreamParts(w, requestContext, modelContext, cancel, stream, counter, idleTimer, modelID, state)
		return
	}

	h.consumeStreamParts(w, requestContext, modelContext, cancel, stream, counter, idleTimer, modelID, newStreamState(h.limits.StreamParts))
}

func (h *handler) consumeStreamParts(w http.ResponseWriter, requestContext, modelContext context.Context, cancel context.CancelFunc, stream <-chan provider.StreamPart, counter *streamPartCounter, idleTimer *time.Timer, modelID string, state *streamState) {
	for {
		part, waitResult := waitStreamPart(requestContext, modelContext, stream, counter, idleTimer.C)
		if waitResult != streamWaitPart {
			cancel()
			h.writeStreamTerminalForWait(w, waitResult)
			return
		}
		result := h.processStreamPart(w, state, part, modelID)
		if h.handleStreamPartResult(w, cancel, result) {
			return
		}
		idleTimer.Reset(h.limits.StreamIdleDuration)
	}
}

func (h *handler) handleStreamPartResult(w http.ResponseWriter, cancel context.CancelFunc, result streamPartResult) bool {
	switch result {
	case streamPartContinue:
		return false
	case streamPartAdapterFailure:
		cancel()
		h.writeStreamTerminalError(w, safeError{category: safeInternal})
	}
	return true
}

func safeErrorFromStreamProvider(err error) (result safeError) {
	result = safeError{category: safeInternal}
	defer func() {
		if recover() != nil {
			result = safeError{category: safeInternal}
		}
	}()
	if isNilInterface(err) {
		return result
	}

	var apiError *provider.APICallError
	if errors.As(err, &apiError) {
		if isNilInterface(apiError) {
			return result
		}
		if apiError.StatusCode == 0 {
			switch {
			case errors.Is(err, context.Canceled):
				return safeError{category: safeCancellation}
			case errors.Is(err, context.DeadlineExceeded):
				return safeError{category: safeTimeout}
			}
			if transportError, ok := safeErrorFromTransport(err); ok {
				return transportError
			}
			return safeError{category: safeUpstream}
		}
		if apiError.StatusCode < 100 || apiError.StatusCode > 599 {
			return result
		}
		switch apiError.StatusCode {
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			return safeError{category: safeTimeout}
		case http.StatusTooManyRequests:
			return safeError{category: safeRateLimit}
		case http.StatusServiceUnavailable, 529:
			return safeError{category: safeOverload}
		}
		if apiError.StatusCode >= 400 && apiError.StatusCode < 500 {
			return safeError{category: safeFailedDependency}
		}
		return safeError{category: safeUpstream}
	}
	return safeErrorFromProvider(err)
}

func validStreamTimestamp(value time.Time) bool {
	if value.IsZero() {
		return true
	}
	year := value.UTC().Year()
	return year >= 0 && year <= 9999
}

func (h *handler) processStreamPart(w http.ResponseWriter, state *streamState, part provider.StreamPart, modelID string) streamPartResult {
	switch part.Type {
	case provider.PartResponseMeta:
		if state.metadataSeen || state.textStarted || !utf8.ValidString(part.ResponseID) || !validStreamTimestamp(part.Timestamp) {
			return streamPartAdapterFailure
		}
		event := streamEvent{typeName: provider.PartResponseMeta, id: part.ResponseID, modelID: modelID, timestamp: part.Timestamp}
		if result := h.emitStreamEvent(w, event); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.metadataSeen = true
		return streamPartContinue
	case provider.PartTextStart:
		if state.activeID != "" || part.ID == "" || !utf8.ValidString(part.ID) {
			return streamPartAdapterFailure
		}
		if _, exists := state.usedIDs[part.ID]; exists {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartTextStart, id: part.ID}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.usedIDs[part.ID] = struct{}{}
		state.activeID = part.ID
		state.textStarted = true
		return streamPartContinue
	case provider.PartTextDelta:
		if state.activeID == "" || part.ID != state.activeID {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartTextDelta, id: part.ID, delta: part.Delta}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		return streamPartContinue
	case provider.PartTextEnd:
		if state.activeID == "" || part.ID != state.activeID {
			return streamPartAdapterFailure
		}
		if result := h.emitStreamEvent(w, streamEvent{typeName: provider.PartTextEnd, id: part.ID}); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		state.activeID = ""
		return streamPartContinue
	case provider.PartError:
		if result := h.emitSafeStreamError(w, safeErrorFromStreamProvider(part.APICallError)); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		return streamPartContinue
	case provider.PartFinish:
		if state.activeID != "" || len(part.Warnings) != 0 || part.FinishReason == nil || part.Usage == nil {
			return streamPartAdapterFailure
		}
		if !validStreamFinishReason(*part.FinishReason) {
			return streamPartAdapterFailure
		}
		inputUsage, err := mapInputUsage(part.Usage.InputTokens)
		if err != nil {
			return streamPartAdapterFailure
		}
		outputUsage, err := mapOutputUsage(part.Usage.OutputTokens)
		if err != nil {
			return streamPartAdapterFailure
		}
		event := streamEvent{typeName: provider.PartFinish, finishReason: *part.FinishReason, inputUsage: inputUsage, outputUsage: outputUsage}
		if result := h.emitStreamEvent(w, event); result != streamWriteSuccess {
			if result == streamWriteEncodingFailure {
				return streamPartAdapterFailure
			}
			return streamPartWriterFailure
		}
		return streamPartFinished
	case provider.PartStreamStart:
		return streamPartAdapterFailure
	default:
		return streamPartAdapterFailure
	}
}

func validStreamFinishReason(reason provider.FinishReason) bool {
	return utf8.ValidString(reason.Raw) && validStreamUnifiedFinishReason(reason.Unified)
}

func validStreamUnifiedFinishReason(reason provider.UnifiedFinishReason) bool {
	switch reason {
	case provider.FinishReasonStop,
		provider.FinishReasonLength,
		provider.FinishReasonContentFilter,
		provider.FinishReasonToolCalls,
		provider.FinishReasonError,
		provider.FinishReasonOther:
		return true
	default:
		return false
	}
}

type streamWriteResult uint8

const (
	streamWriteSuccess streamWriteResult = iota + 1
	streamWriteEncodingFailure
	streamWriteWriterFailure
)

func (h *handler) emitStreamEvent(w http.ResponseWriter, event streamEvent) streamWriteResult {
	frame, ok := encodeStreamFrame(event, h.limits.StreamFrameBytes)
	if !ok {
		return streamWriteEncodingFailure
	}
	if !writeCompleteStreamFrame(w, frame) {
		return streamWriteWriterFailure
	}
	return streamWriteSuccess
}

func (h *handler) emitSafeStreamError(w http.ResponseWriter, value safeError) streamWriteResult {
	frame := streamErrorFrameForSafeError(value)
	if int64(len(frame)) > h.limits.StreamFrameBytes {
		return streamWriteEncodingFailure
	}
	if !writeCompleteStreamFrame(w, frame) {
		return streamWriteWriterFailure
	}
	return streamWriteSuccess
}

func streamErrorFrameForSafeError(value safeError) []byte {
	switch value.category {
	case safeRateLimit:
		return canonicalRateLimitStreamErrorFrame
	case safeOverload:
		return canonicalOverloadStreamErrorFrame
	case safeFailedDependency:
		return canonicalDependencyStreamErrorFrame
	case safeUpstream:
		return canonicalUpstreamStreamErrorFrame
	case safeTimeout:
		return canonicalTimeoutStreamErrorFrame
	case safeCancellation:
		return canonicalCancellationStreamErrorFrame
	default:
		return canonicalInternalStreamErrorFrame
	}
}

func (h *handler) writeStreamTerminalForWait(w http.ResponseWriter, result streamWaitResult) {
	switch result {
	case streamWaitCanceled:
		h.writeStreamTerminalError(w, safeError{category: safeCancellation})
	case streamWaitTotalTimeout, streamWaitIdleTimeout:
		h.writeStreamTerminalError(w, safeError{category: safeTimeout})
	default:
		h.writeStreamTerminalError(w, safeError{category: safeInternal})
	}
}

func (h *handler) writeStreamTerminalError(w http.ResponseWriter, value safeError) {
	h.emitSafeStreamError(w, value)
}

func (h *handler) startStreamDrain(stream <-chan provider.StreamPart, counter *streamPartCounter) {
	if stream == nil {
		return
	}
	go h.drainStream(stream, counter)
}

func (h *handler) drainStream(stream <-chan provider.StreamPart, counter *streamPartCounter) {
	deadline := time.Now().Add(h.limits.StreamDrainDuration)
	timer := time.NewTimer(h.limits.StreamDrainDuration)
	defer timer.Stop()
	for !counter.exceeded() && time.Now().Before(deadline) {
		select {
		case _, ok := <-stream:
			if !ok || !counter.take() {
				return
			}
		case <-timer.C:
			return
		}
	}
}
