package v4

import (
	"context"
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
	typeName provider.WarningType
	feature  string
	setting  string
	message  string
	details  string
}

type streamEvent struct {
	typeName     provider.StreamPartType
	warnings     []streamWarning
	id           string
	modelID      string
	delta        string
	timestamp    time.Time
	finishReason provider.FinishReason
	inputUsage   unaryTokenUsage
	outputUsage  unaryTokenUsage
}

func encodeStreamFrame(value streamEvent, limit int64) ([]byte, bool) {
	buffer := newBoundedDocument(limit)
	buffer.append("data: {")
	buffer.append(`"type":`)
	buffer.appendJSONString(string(value.typeName))
	switch value.typeName {
	case provider.PartStreamStart:
		buffer.append(`,"warnings":[`)
		for i, warning := range value.warnings {
			if i > 0 {
				buffer.append(",")
			}
			encodeStreamWarning(&buffer, warning)
		}
		buffer.append("]")
	case provider.PartResponseMeta:
		if value.id != "" {
			buffer.append(`,"id":`)
			buffer.appendJSONString(value.id)
		}
		buffer.append(`,"modelId":`)
		buffer.appendJSONString(value.modelID)
		if !value.timestamp.IsZero() {
			buffer.append(`,"timestamp":`)
			buffer.appendJSONString(value.timestamp.UTC().Format(time.RFC3339Nano))
		}
	case provider.PartTextStart, provider.PartTextEnd:
		buffer.append(`,"id":`)
		buffer.appendJSONString(value.id)
	case provider.PartTextDelta:
		buffer.append(`,"id":`)
		buffer.appendJSONString(value.id)
		buffer.append(`,"delta":`)
		buffer.appendJSONString(value.delta)
	case provider.PartFinish:
		buffer.append(`,"usage":{"inputTokens":`)
		encodeInputUsage(&buffer, value.inputUsage)
		buffer.append(`,"outputTokens":`)
		encodeOutputUsage(&buffer, value.outputUsage)
		buffer.append(`},"finishReason":{"unified":`)
		buffer.appendJSONString(string(value.finishReason.Unified))
		if value.finishReason.Raw != "" {
			buffer.append(`,"raw":`)
			buffer.appendJSONString(value.finishReason.Raw)
		}
		buffer.append("}")
	default:
		return nil, false
	}
	buffer.append("}\n\n")
	if buffer.overflow || buffer.invalid {
		return nil, false
	}
	return buffer.data, true
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
	if limit <= 0 || int64(len(warnings)) > limit/minimumMappedStreamWarningBytes {
		return nil, errInvalidStreamWarning
	}
	mapped := make([]streamWarning, 0, len(warnings))
	for _, warning := range warnings {
		switch warning.Type {
		case provider.WarnUnsupported:
			mapped = append(mapped, streamWarning{
				typeName: warning.Type,
				feature:  streamWarningUnsupportedFeature,
				details:  streamWarningUnsupportedDetails,
			})
		case provider.WarnCompatibility:
			mapped = append(mapped, streamWarning{
				typeName: warning.Type,
				feature:  streamWarningCompatibilityFeature,
				details:  streamWarningCompatibilityDetails,
			})
		case provider.WarnDeprecated:
			mapped = append(mapped, streamWarning{
				typeName: warning.Type,
				setting:  streamWarningDeprecatedSetting,
				message:  streamWarningDeprecatedMessage,
			})
		case provider.WarnOther:
			mapped = append(mapped, streamWarning{
				typeName: warning.Type,
				message:  streamWarningOtherMessage,
			})
		default:
			return nil, errInvalidStreamWarning
		}
	}
	return mapped, nil
}

func encodeStreamWarning(buffer *boundedDocument, warning streamWarning) {
	buffer.append(`{"type":`)
	buffer.appendJSONString(string(warning.typeName))
	switch warning.typeName {
	case provider.WarnUnsupported, provider.WarnCompatibility:
		buffer.append(`,"feature":`)
		buffer.appendJSONString(warning.feature)
		buffer.append(`,"details":`)
		buffer.appendJSONString(warning.details)
	case provider.WarnDeprecated:
		buffer.append(`,"setting":`)
		buffer.appendJSONString(warning.setting)
		buffer.append(`,"message":`)
		buffer.appendJSONString(warning.message)
	case provider.WarnOther:
		buffer.append(`,"message":`)
		buffer.appendJSONString(warning.message)
	}
	buffer.append("}")
}

var errInvalidStreamWarning = errors.New("providerwire v4: invalid stream warning")

type setupOwner uint32

const (
	setupPending setupOwner = iota
	setupHandler
	setupAbandoned
)

type streamOutcome struct {
	result *provider.StreamResult
	err    error
}

type streamSetupHandoff struct {
	outcome      streamOutcome
	ready        chan struct{}
	decided      chan struct{}
	owner        atomic.Uint32
	drainStarted atomic.Bool
}

func newStreamSetupHandoff() *streamSetupHandoff {
	return &streamSetupHandoff{ready: make(chan struct{}), decided: make(chan struct{})}
}

func (h *streamSetupHandoff) decide(owner setupOwner) bool {
	if !h.owner.CompareAndSwap(uint32(setupPending), uint32(owner)) {
		return false
	}
	close(h.decided)
	return true
}

func (h *streamSetupHandoff) startDrain(handler *handler, stream <-chan provider.StreamPart, counter *streamPartCounter) bool {
	if stream == nil || !h.drainStarted.CompareAndSwap(false, true) {
		return false
	}
	handler.startStreamDrain(stream, counter)
	return true
}

func decideStreamSetup(handoff *streamSetupHandoff, canceled, expired bool) setupOwner {
	owner := setupHandler
	if canceled || expired {
		owner = setupAbandoned
	}
	if !handoff.decide(owner) {
		return setupOwner(handoff.owner.Load())
	}
	return owner
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

	modelContext, cancel := context.WithCancel(requestContext)
	counter := newStreamPartCounter(h.limits.StreamParts)
	totalDeadline := h.clock.Now().Add(h.limits.ModelDuration)
	handoff := newStreamSetupHandoff()

	go func() {
		outcome := streamOutcome{}
		defer func() {
			if recover() != nil {
				outcome = streamOutcome{err: errModelInternal}
			}
			if outcome.result == nil && isNil(outcome.err) {
				outcome.err = errModelInternal
			}
			handoff.outcome = outcome
			close(handoff.ready)
			<-handoff.decided
			if setupOwner(handoff.owner.Load()) == setupAbandoned && outcome.result != nil {
				handoff.startDrain(h, outcome.result.Stream, counter)
			}
		}()
		outcome.result, outcome.err = model.DoStream(modelContext, options)
	}()

	ready := false
	canceled := false
	expired := false
	for !ready && !canceled && !expired {
		timer := h.clock.NewTimer(durationUntil(h.clock.Now(), totalDeadline))
		select {
		case <-handoff.ready:
		case <-requestContext.Done():
		case <-timer.C():
		}
		timer.Stop()
		select {
		case <-handoff.ready:
			ready = true
		default:
		}
		canceled = requestContext.Err() != nil
		expired = !h.clock.Now().Before(totalDeadline)
	}
	if !ready || canceled || expired {
		cancel()
		if decideStreamSetup(handoff, canceled, expired) != setupAbandoned {
			return
		}
		if canceled {
			h.writeSafeError(w, safeError{category: safeCancellation})
		} else {
			h.writeSafeError(w, safeError{category: safeTimeout})
		}
		return
	}
	if decideStreamSetup(handoff, false, false) != setupHandler {
		cancel()
		return
	}

	outcome := handoff.outcome
	if !isNil(outcome.err) || outcome.result == nil || outcome.result.Stream == nil {
		cancel()
		if outcome.result != nil {
			handoff.startDrain(h, outcome.result.Stream, counter)
		}
		if !isNil(outcome.err) {
			h.writeSafeError(w, safeErrorFromProvider(outcome.err))
		} else {
			h.writeSafeError(w, safeError{category: safeInternal})
		}
		return
	}

	idleDeadline := h.clock.Now().Add(h.limits.StreamIdleDuration)
	defer func() {
		cancel()
		handoff.startDrain(h, outcome.result.Stream, counter)
	}()
	if !commitStreamResponse(w) {
		return
	}
	h.runStream(w, requestContext, cancel, outcome.result.Stream, counter, totalDeadline, idleDeadline, modelID)
}

func durationUntil(now, deadline time.Time) time.Duration {
	duration := deadline.Sub(now)
	if duration < 0 {
		return 0
	}
	return duration
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

func streamPrecedence(canceled bool, now, totalDeadline, idleDeadline time.Time) streamWaitResult {
	if canceled {
		return streamWaitCanceled
	}
	if !now.Before(totalDeadline) {
		return streamWaitTotalTimeout
	}
	if !idleDeadline.IsZero() && !now.Before(idleDeadline) {
		return streamWaitIdleTimeout
	}
	return 0
}

func (h *handler) waitStreamPart(ctx context.Context, stream <-chan provider.StreamPart, counter *streamPartCounter, totalDeadline, idleDeadline time.Time) (provider.StreamPart, streamWaitResult) {
	for {
		now := h.clock.Now()
		if result := streamPrecedence(ctx.Err() != nil, now, totalDeadline, idleDeadline); result != 0 {
			return provider.StreamPart{}, result
		}
		deadline := totalDeadline
		if !idleDeadline.IsZero() && idleDeadline.Before(deadline) {
			deadline = idleDeadline
		}
		timer := h.clock.NewTimer(durationUntil(now, deadline))
		var part provider.StreamPart
		var ok bool
		selectedPart := false
		select {
		case part, ok = <-stream:
			selectedPart = true
		case <-ctx.Done():
		case <-timer.C():
		}
		timer.Stop()
		withinLimit := true
		if selectedPart && ok {
			withinLimit = counter.take()
		}
		if result := streamPrecedence(ctx.Err() != nil, h.clock.Now(), totalDeadline, idleDeadline); result != 0 {
			return provider.StreamPart{}, result
		}
		if !selectedPart {
			continue
		}
		if !ok {
			return provider.StreamPart{}, streamWaitClosed
		}
		if !withinLimit {
			return provider.StreamPart{}, streamWaitPartLimit
		}
		return part, streamWaitPart
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

func (h *handler) runStream(w http.ResponseWriter, ctx context.Context, cancel context.CancelFunc, stream <-chan provider.StreamPart, counter *streamPartCounter, totalDeadline, idleDeadline time.Time, modelID string) {
	part, waitResult := h.waitStreamPart(ctx, stream, counter, totalDeadline, idleDeadline)
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
		if !streamWarningCountFits(len(part.Warnings), h.limits.StreamFrameBytes) {
			cancel()
			if h.emitStreamEvent(w, streamEvent{typeName: provider.PartStreamStart}) == streamWriteSuccess {
				h.writeStreamTerminalError(w, safeError{category: safeInternal})
			}
			return
		}
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
			idleDeadline = h.clock.Now().Add(h.limits.StreamIdleDuration)
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
		idleDeadline = h.clock.Now().Add(h.limits.StreamIdleDuration)
		h.consumeStreamParts(w, ctx, cancel, stream, counter, totalDeadline, idleDeadline, modelID, state)
		return
	}

	h.consumeStreamParts(w, ctx, cancel, stream, counter, totalDeadline, idleDeadline, modelID, newStreamState(h.limits.StreamParts))
}

func (h *handler) consumeStreamParts(w http.ResponseWriter, ctx context.Context, cancel context.CancelFunc, stream <-chan provider.StreamPart, counter *streamPartCounter, totalDeadline, idleDeadline time.Time, modelID string, state *streamState) {
	for {
		part, waitResult := h.waitStreamPart(ctx, stream, counter, totalDeadline, idleDeadline)
		if waitResult != streamWaitPart {
			cancel()
			h.writeStreamTerminalForWait(w, waitResult)
			return
		}
		result := h.processStreamPart(w, state, part, modelID)
		if h.handleStreamPartResult(w, cancel, result) {
			return
		}
		idleDeadline = h.clock.Now().Add(h.limits.StreamIdleDuration)
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
	if isNil(err) {
		return result
	}

	var apiError *provider.APICallError
	if errors.As(err, &apiError) {
		if isNil(apiError) {
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
		if !validFinishReason(*part.FinishReason) {
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

func validFinishReason(reason provider.FinishReason) bool {
	return utf8.ValidString(reason.Raw) && validUnifiedFinishReason(reason.Unified)
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
	deadline := h.clock.Now().Add(h.limits.StreamDrainDuration)
	for {
		if !h.clock.Now().Before(deadline) || counter.exceeded() {
			return
		}
		timer := h.clock.NewTimer(durationUntil(h.clock.Now(), deadline))
		select {
		case _, ok := <-stream:
			timer.Stop()
			if !ok {
				return
			}
			withinLimit := counter.take()
			if !h.clock.Now().Before(deadline) || !withinLimit {
				return
			}
		case <-timer.C():
			timer.Stop()
			if !h.clock.Now().Before(deadline) {
				return
			}
		}
	}
}
