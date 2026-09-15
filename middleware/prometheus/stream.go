package prometheus

import (
	"context"
	"time"

	"github.com/grafana/ai-sdk/internal/streamusage"
	"github.com/grafana/ai-sdk/provider"
)

const streamBufferSize = 64

type streamObservation struct {
	responseIdentity  identity
	finishReason      string
	usage             streamusage.Aggregator
	streamError       *outcome
	chunkCounts       map[string]int
	firstPayloadAfter *float64
	lastPayloadAt     time.Time
	recordPayloadGaps bool
	payloadGaps       []payloadGap
}

type payloadGap struct {
	chunkType string
	seconds   float64
}

func newStreamObservation(recordPayloadGaps bool) *streamObservation {
	return &streamObservation{
		finishReason:      finishReasonNone,
		chunkCounts:       map[string]int{},
		recordPayloadGaps: recordPayloadGaps,
	}
}

func (i *instrumentation) runStreamTee(
	ctx context.Context,
	upstream <-chan provider.StreamPart,
	tee chan<- provider.StreamPart,
	requested identity,
	start time.Time,
) {
	defer close(tee)
	defer i.collectors.inflight.WithLabelValues(operationStream, requested.provider, requested.model).Dec()

	obs := newStreamObservation(!i.config.disableStreamChunkMetrics)
	for {
		if i.config.streamDrainTimeout > 0 && ctx.Err() != nil {
			i.drainCanceledStream(upstream)
			i.finalizeStream(ctx, requested, obs, start, true)
			return
		}
		select {
		case <-ctx.Done():
			i.drainCanceledStream(upstream)
			i.finalizeStream(ctx, requested, obs, start, true)
			return
		case part, ok := <-upstream:
			receivedAt := time.Now()
			if !ok {
				i.finalizeStream(ctx, requested, obs, start, ctx.Err() != nil)
				return
			}
			obs.observe(part, start, receivedAt)

			select {
			case tee <- part:
			case <-ctx.Done():
				i.drainCanceledStream(upstream)
				i.finalizeStream(ctx, requested, obs, start, true)
				return
			}
		}
	}
}

func drainUntilClosed(upstream <-chan provider.StreamPart) {
	for range upstream {
	}
}

func (i *instrumentation) drainCanceledStream(upstream <-chan provider.StreamPart) {
	if i.config.streamDrainTimeout <= 0 {
		go drainUntilClosed(upstream)
		return
	}
	deadline := time.Now().Add(i.config.streamDrainTimeout)
	go drainStreamUntil(upstream, deadline)
}

func drainStreamUntil(upstream <-chan provider.StreamPart, deadline time.Time) {
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	for time.Now().Before(deadline) {
		select {
		case _, ok := <-upstream:
			if !ok {
				return
			}
		case <-timer.C:
			return
		}
	}
}

func (i *instrumentation) finalizeStream(ctx context.Context, requested identity, obs *streamObservation, start time.Time, canceled bool) {
	finalID := i.streamFinalIdentity(requested, obs.responseIdentity)
	out := outcome{status: statusSuccess, errorType: errorTypeNone, statusCode: statusCodeNone, finishReason: obs.finishReason}
	if canceled {
		if ctxOut, ok := classifyContext(ctx); ok {
			out = ctxOut
		}
	} else if obs.streamError != nil {
		out = *obs.streamError
		if out.finishReason == finishReasonNone && obs.finishReason != finishReasonNone {
			out.finishReason = obs.finishReason
		}
	}

	i.observeRequest(operationStream, finalID, out, time.Since(start).Seconds())
	if usage, ok := obs.usage.Usage(); ok {
		i.observeUsage(operationStream, finalID, usage)
	}
	if obs.firstPayloadAfter != nil {
		i.collectors.timeToFirstOutput.WithLabelValues(operationStream, finalID.provider, finalID.model, out.status).Observe(*obs.firstPayloadAfter)
	}
	if !i.config.disableStreamChunkMetrics {
		for chunkType, count := range obs.chunkCounts {
			if count > 0 {
				i.collectors.streamChunks.WithLabelValues(operationStream, finalID.provider, finalID.model, string(chunkType)).Add(float64(count))
			}
		}
		for _, gap := range obs.payloadGaps {
			i.collectors.interChunkDelay.WithLabelValues(operationStream, finalID.provider, finalID.model, gap.chunkType).Observe(gap.seconds)
		}
	}
}

func (o *streamObservation) observe(part provider.StreamPart, start, receivedAt time.Time) {
	chunkType := streamChunkTypeLabel(part.Type)
	o.chunkCounts[chunkType]++
	o.usage.Observe(part)

	switch part.Type {
	case provider.PartResponseMeta:
		if part.Provider != "" && part.ModelID != "" {
			o.responseIdentity = identity{provider: part.Provider, model: part.ModelID}
		}
	case provider.PartFinish:
		if part.FinishReason != nil {
			o.finishReason = finishReasonLabel(*part.FinishReason)
		}
	case provider.PartError:
		out := classifyStreamError(part.APICallError)
		o.streamError = &out
	}

	if !isPayloadBearing(part.Type) {
		return
	}
	elapsed := receivedAt.Sub(start).Seconds()
	if o.firstPayloadAfter == nil {
		o.firstPayloadAfter = &elapsed
	} else if o.recordPayloadGaps && !o.lastPayloadAt.IsZero() {
		o.payloadGaps = append(o.payloadGaps, payloadGap{chunkType: chunkType, seconds: receivedAt.Sub(o.lastPayloadAt).Seconds()})
	}
	o.lastPayloadAt = receivedAt
}

func streamChunkTypeLabel(partType provider.StreamPartType) string {
	switch partType {
	case provider.PartTextStart,
		provider.PartTextDelta,
		provider.PartTextEnd,
		provider.PartReasoningStart,
		provider.PartReasoningDelta,
		provider.PartReasoningEnd,
		provider.PartToolInputStart,
		provider.PartToolInputDelta,
		provider.PartToolInputEnd,
		provider.PartToolCall,
		provider.PartToolResult,
		provider.PartSource,
		provider.PartFile,
		provider.PartStreamStart,
		provider.PartResponseMeta,
		provider.PartFinish,
		provider.PartRaw,
		provider.PartError,
		provider.PartToolApprovalRequest,
		provider.PartCustom,
		provider.PartReasoningFile:
		return string(partType)
	default:
		return "other"
	}
}

func isPayloadBearing(partType provider.StreamPartType) bool {
	switch partType {
	case provider.PartTextDelta,
		provider.PartReasoningDelta,
		provider.PartToolInputDelta,
		provider.PartToolCall,
		provider.PartToolResult,
		provider.PartSource,
		provider.PartFile,
		provider.PartCustom,
		provider.PartReasoningFile,
		provider.PartToolApprovalRequest:
		return true
	default:
		return false
	}
}
