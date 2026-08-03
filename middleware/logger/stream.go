package logger

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/internal/streamusage"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
)

const streamBuffer = 64

func (l *modelLogger) wrapStream(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
	started := l.opts.clock()
	callID := newCallID()
	l.log(ctx, EventStreamStart, l.opts.level, append(
		commonAttrs(callID, "stream", p.Model),
		requestSummaryAttrs(p.Params, l.opts.capture)...,
	)...)

	upstream, err := p.DoStream(ctx)
	if err != nil {
		duration := l.opts.clock().Sub(started)
		attrs := append(commonAttrs(callID, "stream", p.Model), terminalAttrs(duration, outcomeError)...)
		attrs = append(attrs, errorAttrs(err, l.opts.capture)...)
		l.log(ctx, EventStreamError, l.opts.errorLevel, attrs...)
		return nil, err
	}
	if upstream == nil {
		duration := l.opts.clock().Sub(started)
		attrs := append(commonAttrs(callID, "stream", p.Model), terminalAttrs(duration, outcomeSuccess)...)
		l.log(ctx, EventStreamFinish, l.opts.level, attrs...)
		return nil, nil
	}

	tee := make(chan provider.StreamPart, streamBuffer)
	result := &provider.StreamResult{
		Stream:   tee,
		Request:  upstream.Request,
		Response: upstream.Response,
	}
	go l.runStreamTee(ctx, streamTeeInput{
		started:  started,
		callID:   callID,
		model:    p.Model,
		request:  upstream.Request,
		response: upstream.Response,
		upstream: upstream.Stream,
		tee:      tee,
	})
	return result, nil
}

type streamTeeInput struct {
	started  time.Time
	callID   string
	model    provider.LanguageModel
	request  *provider.RequestMetadata
	response *provider.ResponseHeaders
	upstream <-chan provider.StreamPart
	tee      chan<- provider.StreamPart
}

func (l *modelLogger) runStreamTee(ctx context.Context, input streamTeeInput) {
	defer close(input.tee)

	summary := streamSummary{}
	cancelled := false

streamLoop:
	for {
		select {
		case part, ok := <-input.upstream:
			if !ok {
				break streamLoop
			}
			observedAt := l.opts.clock()
			summary.Observe(part, input.started, observedAt)
			if l.opts.logStreamParts {
				l.logStreamPart(ctx, input.callID, input.model, part, summary.total)
			}

			select {
			case input.tee <- part:
			case <-ctx.Done():
				cancelled = true
				drainAvailableStreamParts(input.upstream, &summary)
				break streamLoop
			}
		case <-ctx.Done():
			cancelled = true
			drainAvailableStreamParts(input.upstream, &summary)
			break streamLoop
		}
	}

	duration := l.opts.clock().Sub(input.started)
	outcome := outcomeSuccess
	if summary.firstErrSet {
		outcome = outcomeError
	} else if cancelled {
		outcome = outcomeForContextErr(ctx.Err())
	}
	attrs := append(terminalCommonAttrs(input.callID, "stream", input.model, summary.response), terminalAttrs(duration, outcome)...)
	attrs = append(attrs, summary.Attrs(l.opts.capture)...)
	if input.request != nil && l.opts.capture.RequestBody && len(input.request.Body) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.request.body", input.request.Body, l.opts.capture)
	}
	if input.response != nil && l.opts.capture.Headers && len(input.response.Headers) > 0 {
		attrs = append(attrs, slog.Any("ai_sdk.response.headers", cloneStringMap(input.response.Headers)))
	}

	if summary.firstErrSet {
		attrs = append(attrs, streamPartErrorAttrs(summary.firstErr, l.opts.capture)...)
		l.log(ctx, EventStreamError, l.opts.errorLevel, attrs...)
		return
	}
	if cancelled {
		attrs = append(attrs, errorAttrs(ctx.Err(), l.opts.capture)...)
		if outcome == outcomeTimeout {
			l.log(ctx, EventStreamError, l.opts.errorLevel, attrs...)
		} else {
			l.log(ctx, EventStreamCancelled, l.opts.level, attrs...)
		}
		return
	}
	l.log(ctx, EventStreamFinish, l.opts.level, attrs...)
}

func outcomeForContextErr(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return outcomeTimeout
	}
	return outcomeCancelled
}

func drainAvailableStreamParts(upstream <-chan provider.StreamPart, summary *streamSummary) {
	// Keep cancellation prompt: summarize already-buffered parts, but do not wait
	// for an idle upstream to close after ctx.Done(). Providers should observe
	// the same request context and stop producing after cancellation.
	for {
		select {
		case part, ok := <-upstream:
			if !ok {
				return
			}
			summary.ObserveDrained(part)
		default:
			return
		}
	}
}

func (l *modelLogger) logStreamPart(ctx context.Context, callID string, model provider.LanguageModel, part provider.StreamPart, index int) {
	attrs := commonAttrs(callID, "stream", model)
	attrs = append(attrs,
		slog.Int("ai_sdk.stream.part.index", index),
		slog.String("ai_sdk.stream.part.type", string(part.Type)),
	)
	attrs = append(attrs, streamPartCaptureAttrs(part, l.opts.capture)...)
	level := l.opts.partLevel
	if part.Type == provider.PartError {
		level = l.opts.errorLevel
		attrs = append(attrs, streamPartErrorAttrs(part.APICallError, l.opts.capture)...)
	}
	l.log(ctx, EventStreamPart, level, attrs...)
}

type streamSummary struct {
	total               int
	byType              map[provider.StreamPartType]int
	response            provider.ResponseMetadata
	usage               streamusage.Aggregator
	finish              *provider.FinishReason
	metadata            provider.ProviderMetadata
	firstContentLatency *time.Duration
	firstErr            *provider.APICallError
	firstErrSet         bool
}

func (s *streamSummary) Observe(part provider.StreamPart, started, observedAt time.Time) {
	s.observe(part)
	if s.firstContentLatency == nil && isFirstContentPart(part) {
		latency := observedAt.Sub(started)
		s.firstContentLatency = &latency
	}
}

func (s *streamSummary) ObserveDrained(part provider.StreamPart) {
	s.observe(part)
}

func (s *streamSummary) observe(part provider.StreamPart) {
	s.total++
	if s.byType == nil {
		s.byType = make(map[provider.StreamPartType]int)
	}
	s.byType[part.Type]++
	s.usage.Observe(part)

	switch part.Type {
	case provider.PartResponseMeta:
		s.response = provider.ResponseMetadata{
			ID:        part.ResponseID,
			ModelID:   part.ModelID,
			Provider:  part.Provider,
			Timestamp: part.Timestamp,
		}
	case provider.PartFinish:
		if part.FinishReason != nil {
			finish := *part.FinishReason
			s.finish = &finish
		}
		if len(part.ProviderMetadata) > 0 {
			s.metadata = part.ProviderMetadata
		}
	case provider.PartError:
		if !s.firstErrSet {
			s.firstErr = part.APICallError
			s.firstErrSet = true
		}
	}
}

func isFirstContentPart(part provider.StreamPart) bool {
	switch part.Type {
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolCall, provider.PartToolInputDelta:
		return true
	default:
		return false
	}
}

func (s streamSummary) Attrs(capture CaptureOptions) []slog.Attr {
	attrs := []slog.Attr{slog.Int("ai_sdk.stream.parts.count", s.total)}
	for _, typ := range []provider.StreamPartType{
		provider.PartTextDelta,
		provider.PartReasoningDelta,
		provider.PartToolCall,
		provider.PartToolResult,
		provider.PartError,
	} {
		attrs = append(attrs, slog.Int(streamPartCountKey(typ), s.byType[typ]))
	}
	attrs = append(attrs, responseMetadataAttrs(s.response)...)
	if s.firstContentLatency != nil {
		attrs = append(attrs, slog.Float64("ai_sdk.stream.time_to_first_content_ms", durationMs(*s.firstContentLatency)))
	}
	if usage, ok := s.usage.Usage(); ok {
		attrs = append(attrs, usageAttrs(usage)...)
	}
	if s.finish != nil {
		attrs = append(attrs, finishReasonAttrs(*s.finish)...)
	}
	if capture.ProviderMetadata && len(s.metadata) > 0 {
		attrs = appendJSONAttr(attrs, "ai_sdk.provider_metadata", s.metadata, capture)
	}
	return attrs
}

func streamPartCountKey(typ provider.StreamPartType) string {
	return "ai_sdk.stream.parts." + strings.ReplaceAll(string(typ), "-", "_") + ".count"
}

func streamPartCaptureAttrs(part provider.StreamPart, capture CaptureOptions) []slog.Attr {
	attrs := make([]slog.Attr, 0, 6)
	if shouldCaptureStreamPart(part, capture) {
		sanitized := sanitizeStreamPart(part, capture)
		attrs = appendJSONAttr(attrs, "ai_sdk.stream.part", sanitized, capture)
	}
	switch part.Type {
	case provider.PartTextDelta:
		if capture.Outputs && part.Delta != "" {
			attrs = append(attrs, slog.String("ai_sdk.stream.text", boundString(part.Delta, capture.MaxStringLen)))
		}
	case provider.PartReasoningDelta:
		if capture.Reasoning && part.Delta != "" {
			attrs = append(attrs, slog.String("ai_sdk.stream.reasoning", boundString(part.Delta, capture.MaxStringLen)))
		}
	case provider.PartToolInputDelta, provider.PartToolCall:
		if capture.ToolInputs && part.Input != "" {
			attrs = append(attrs, slog.String("ai_sdk.stream.tool.input", boundString(part.Input, capture.MaxStringLen)))
		}
	case provider.PartToolResult:
		if capture.ToolOutputs && len(part.Result) > 0 {
			attrs = appendJSONAttr(attrs, "ai_sdk.stream.tool.output", part.Result, capture)
		}
	case provider.PartRaw:
		if capture.RawChunks && len(part.RawValue) > 0 {
			attrs = appendJSONAttr(attrs, "ai_sdk.stream.raw", part.RawValue, capture)
		}
	}
	return attrs
}

func shouldCaptureStreamPart(part provider.StreamPart, capture CaptureOptions) bool {
	if capture.ProviderMetadata && len(part.ProviderMetadata) > 0 {
		return true
	}
	switch part.Type {
	case provider.PartTextDelta:
		return capture.Outputs
	case provider.PartReasoningDelta:
		return capture.Reasoning
	case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall, provider.PartToolApprovalRequest:
		return capture.ToolInputs
	case provider.PartToolResult:
		return capture.ToolOutputs
	case provider.PartFile, provider.PartReasoningFile:
		return capture.Files
	case provider.PartRaw:
		return capture.RawChunks
	case provider.PartCustom:
		return capture.RawChunks && len(part.RawValue) > 0
	default:
		return false
	}
}

func sanitizeStreamPart(part provider.StreamPart, capture CaptureOptions) provider.StreamPart {
	part.APICallError = nil
	if part.Type == provider.PartCustom {
		return sanitizeCustomStreamPart(part, capture)
	}
	if !capture.Files {
		part.Source = nil
	}
	if !capture.Outputs {
		switch part.Type {
		case provider.PartTextStart, provider.PartTextDelta, provider.PartTextEnd:
			part.ID = ""
			part.Delta = ""
		}
	}
	if !capture.Reasoning {
		switch part.Type {
		case provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd:
			part.ID = ""
			part.Delta = ""
		}
	}
	if !capture.ToolInputs {
		switch part.Type {
		case provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd, provider.PartToolCall, provider.PartToolApprovalRequest:
			part.ToolCallID = ""
			part.ToolName = ""
			part.Input = ""
			part.Kind = ""
			part.ApprovalID = ""
			part.Signature = ""
			part.Approved = nil
			part.Reason = ""
		}
	}
	if !capture.ToolOutputs && part.Type == provider.PartToolResult {
		part.ToolCallID = ""
		part.ToolName = ""
		part.Result = nil
		part.Reason = ""
	}
	if !capture.Files {
		switch part.Type {
		case provider.PartFile, provider.PartReasoningFile:
			part.Data = nil
			part.Filename = ""
			part.MediaType = ""
			part.Title = ""
		case provider.PartSource:
			part.Title = ""
		}
	}
	if !capture.RawChunks {
		part.RawValue = nil
	}
	if !capture.Headers {
		part.ResponseHeaders = nil
	}
	if !capture.ProviderMetadata {
		part.ProviderMetadata = nil
	}
	return part
}

func sanitizeCustomStreamPart(part provider.StreamPart, capture CaptureOptions) provider.StreamPart {
	sanitized := provider.StreamPart{Type: part.Type}
	if capture.RawChunks {
		sanitized.RawValue = part.RawValue
	}
	if capture.ProviderMetadata {
		sanitized.ProviderMetadata = part.ProviderMetadata
	}
	return sanitized
}
