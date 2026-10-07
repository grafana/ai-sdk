package v4

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/provider"
)

type streamFrameSink interface {
	writeFrame([]byte) bool
}

type streamEventSink interface {
	writeEvent(streamEvent, []byte) streamWriteResult
}

type guardOutputEntry struct {
	kind     provider.ContentPartType
	id, name string
	text     strings.Builder
	input    json.RawMessage
	isError  bool
}

type guardedFrames struct {
	frames      [][]byte
	output      []*guardOutputEntry
	texts       map[string]*guardOutputEntry
	tools       map[string]*strings.Builder
	used, limit int64
	failure     bool
}

func newGuardedFrames(limit int64) *guardedFrames {
	return &guardedFrames{limit: limit, texts: make(map[string]*guardOutputEntry), tools: make(map[string]*strings.Builder)}
}

func (*guardedFrames) Header() http.Header         { return nil }
func (s *guardedFrames) WriteHeader(int)           { s.failure = true }
func (s *guardedFrames) Write([]byte) (int, error) { s.failure = true; return 0, ErrGuardFailed }

func (s *guardedFrames) writeFrame(frame []byte) bool {
	cost := int64(len(frame))*4 + 512
	if cost > s.limit-s.used {
		s.failure = true
		return false
	}
	s.used += cost
	s.frames = append(s.frames, frame)
	return true
}

func (s *guardedFrames) writeEvent(event streamEvent, frame []byte) streamWriteResult {
	switch event.typeName {
	case provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd:
		if guardedReasoningMetadata(event.reasoningMetadata) != nil {
			s.failure = true
			return streamWriteEncodingFailure
		}
	}
	if !s.writeFrame(frame) {
		return streamWriteWriterFailure
	}
	switch event.typeName {
	case provider.PartTextStart, provider.PartReasoningStart:
		kind := provider.ContentPartTypeText
		if event.typeName == provider.PartReasoningStart {
			kind = provider.ContentPartTypeReasoning
		}
		entry := &guardOutputEntry{kind: kind}
		key := string(event.typeName) + event.id
		s.texts[key] = entry
		s.output = append(s.output, entry)
	case provider.PartTextDelta, provider.PartReasoningDelta:
		start := provider.PartTextStart
		if event.typeName == provider.PartReasoningDelta {
			start = provider.PartReasoningStart
		}
		entry := s.texts[string(start)+event.id]
		if entry == nil {
			s.failure = true
			return streamWriteEncodingFailure
		}
		entry.text.WriteString(event.delta)
	case provider.PartToolInputStart:
		s.tools[event.id] = &strings.Builder{}
	case provider.PartToolInputDelta:
		args := s.tools[event.id]
		if args == nil {
			s.failure = true
			return streamWriteEncodingFailure
		}
		args.WriteString(event.delta)
	case provider.PartToolCall:
		if !json.Valid([]byte(event.input)) {
			s.failure = true
			return streamWriteEncodingFailure
		}
		if args := s.tools[event.id]; args != nil && args.Len() > 0 && !sameGuardedArguments(args.String(), event.input) {
			s.failure = true
			return streamWriteEncodingFailure
		}
		s.output = append(s.output, &guardOutputEntry{kind: provider.ContentPartTypeToolCall, id: event.id, name: event.toolName, input: json.RawMessage(event.input)})
	case provider.PartToolResult:
		s.output = append(s.output, &guardOutputEntry{kind: provider.ContentPartTypeToolResult, id: event.id, name: event.toolName, input: append(json.RawMessage(nil), event.result...), isError: event.isError})
	case provider.PartReasoningFile, provider.PartSource:
		s.failure = true
		return streamWriteEncodingFailure
	}
	return streamWriteSuccess
}

func sameGuardedArguments(a, b string) bool {
	var ca, cb bytes.Buffer
	return json.Compact(&ca, []byte(a)) == nil && json.Compact(&cb, []byte(b)) == nil && bytes.Equal(ca.Bytes(), cb.Bytes())
}

func (s *guardedFrames) policyOutput() []provider.ContentPart {
	parts := make([]provider.ContentPart, 0, len(s.output))
	for _, entry := range s.output {
		switch entry.kind {
		case provider.ContentPartTypeText:
			parts = append(parts, provider.TextPart(entry.text.String()))
		case provider.ContentPartTypeReasoning:
			parts = append(parts, provider.ReasoningPart(entry.text.String()))
		case provider.ContentPartTypeToolCall:
			parts = append(parts, provider.ToolCallPart(entry.id, entry.name, entry.input))
		case provider.ContentPartTypeToolResult:
			kind := provider.ToolOutputJSON
			if entry.isError {
				kind = provider.ToolOutputErrorJSON
			}
			parts = append(parts, provider.ToolResultPart(entry.id, entry.name, &provider.ToolResultOutput{Type: kind, JSON: entry.input}))
		}
	}
	return parts
}

func (h *handler) serveGuardedStream(w http.ResponseWriter, ctx context.Context, resolved catalog.ResolvedModel, options provider.CallOptions) {
	modelContext, cancel := context.WithCancel(ctx)
	counter := newStreamPartCounter(h.limits.StreamParts)
	outcomes := make(chan streamOutcome)
	go func() {
		outcome := callStream(modelContext, resolved.Model, options)
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
	case <-ctx.Done():
		cancel()
		h.writeSafeError(w, safeErrorFromProvider(ctx.Err()))
		return
	}
	defer func() {
		cancel()
		if outcome.result != nil {
			h.startStreamDrain(outcome.result.Stream, counter)
		}
	}()
	if !isNilInterface(outcome.err) || outcome.result == nil || outcome.result.Stream == nil {
		if !isNilInterface(outcome.err) {
			h.writeSafeError(w, safeErrorFromProvider(outcome.err))
		} else {
			h.writeSafeError(w, safeError{category: safeInternal})
		}
		return
	}
	sink := newGuardedFrames(h.guardRetainedBytes / 2)
	state := newStreamState(h.limits.StreamParts)
	idle := time.NewTimer(h.limits.StreamIdleDuration)
	defer idle.Stop()
	started := false
	for {
		part, wait := waitStreamPart(ctx, modelContext, outcome.result.Stream, counter, idle.C)
		if wait != streamWaitPart {
			category := safeFailedDependency
			if ctx.Err() != nil {
				category = safeErrorFromProvider(ctx.Err()).category
			} else if wait == streamWaitIdleTimeout {
				category = safeTimeout
			}
			h.writeSafeError(w, safeError{category: category})
			return
		}
		if ctx.Err() != nil {
			h.writeSafeError(w, safeErrorFromProvider(ctx.Err()))
			return
		}
		if !started {
			started = true
			warnings := []responseWarning(nil)
			if part.Type == provider.PartStreamStart {
				var err error
				warnings, err = mapStreamWarnings(part.Warnings, h.limits.StreamFrameBytes)
				if err != nil {
					h.writeSafeError(w, safeError{category: safeInternal})
					return
				}
			}
			if h.emitStreamEvent(sink, streamEvent{typeName: provider.PartStreamStart, warnings: warnings}) != streamWriteSuccess {
				h.writeSafeError(w, safeError{category: safeFailedDependency})
				return
			}
			if part.Type == provider.PartStreamStart {
				idle.Reset(h.limits.StreamIdleDuration)
				continue
			}
		}
		result := h.processStreamPart(sink, state, part)
		if result == streamPartAdapterFailure || result == streamPartWriterFailure || sink.failure {
			h.writeSafeError(w, safeError{category: safeFailedDependency})
			return
		}
		if result == streamPartFinished {
			for _, tool := range state.tools {
				if tool.phase < toolCallEmitted {
					h.writeSafeError(w, safeError{category: safeFailedDependency})
					return
				}
			}
			cancel()
			err := h.guard.Postflight(ctx, resolved.ID, options, sink.policyOutput())
			if err == nil {
				err = ctx.Err()
			}
			if err != nil {
				h.writeSafeError(w, safeErrorFromGuard(err))
				return
			}
			if !commitStreamResponse(w) {
				return
			}
			for _, frame := range sink.frames {
				if ctx.Err() != nil || !writeCompleteStreamFrame(w, frame) {
					return
				}
			}
			return
		}
		idle.Reset(h.limits.StreamIdleDuration)
	}
}
