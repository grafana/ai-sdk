package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"

	"github.com/grafana/ai-sdk/provider"
)

// streamBufferSize matches the Anthropic provider's channel buffer size so
// streamed events can land without head-of-line blocking on slow consumers.
const streamBufferSize = 64

// blockKind tracks the per-content-block-index type so contentBlockStop can
// emit the matching `*End` event.
type blockKind int

const (
	blockKindText blockKind = iota
	blockKindReasoning
	blockKindTool
)

// blockState holds per-block state used while accumulating streaming
// content. Tool blocks accumulate JSON fragments until contentBlockStop so
// we can emit a single PartToolCall after streaming all input deltas.
type blockState struct {
	kind            blockKind
	toolCallID      string
	toolName        string
	jsonText        string
	redactedContent string
	// isJSONResponseTool flips the stream conversion to emit text deltas
	// (rather than tool input deltas) when the synthetic json tool is in use.
	isJSONResponseTool bool
}

// streamConsumer holds the rolling state used while translating Bedrock
// event-stream events into provider stream parts.
type streamConsumer struct {
	out             chan<- provider.StreamPart
	meta            requestMeta
	modelID         string
	warnings        []provider.Warning
	blocks          map[int]*blockState
	finish          provider.FinishReason
	finishKnown     bool
	usage           *provider.Usage
	providerMD      provider.ProviderMetadata
	jsonRespEmitted bool
	jsonExtractor   *jsonObjectTextExtractor
	stopSequence    *string
	emittedFinish   bool
	// errorEmitted is set when an exception or transport error was already
	// turned into a PartError; emitFinish is skipped in that case.
	errorEmitted bool
	// includeRaw mirrors CallOptions.IncludeRawChunks: when set, a PartRaw is
	// emitted for every decoded frame before it is interpreted (matching
	// upstream's includeRawChunks behavior).
	includeRaw bool
}

// errStreamTerminatedByException is a sentinel returned from the frame
// callback after we emit a PartError for a Bedrock exception. The runStream
// loop uses it to stop consumption without treating the situation as a
// transport-level failure.
var errStreamTerminatedByException = &exceptionTerminatedError{}

type exceptionTerminatedError struct{}

func (e *exceptionTerminatedError) Error() string { return "bedrock: stream terminated by exception" }

// runStream consumes the AWS event-stream body from r and emits
// provider.StreamPart events on out. The function does not close out (the
// caller owns lifecycle so a final close happens regardless of which path
// terminated the stream).
//
// All errors that arise while reading the stream are converted into a final
// PartError event with `*provider.APICallError`. The caller can drain out
// to read everything; closing happens in the caller's defer.
func (m *model) runStream(ctx context.Context, body io.Reader, responseHeaders map[string][]string, meta requestMeta, warnings []provider.Warning, includeRaw bool, out chan<- provider.StreamPart) {
	// Emit a stream-start event with warnings collected during request build
	// (matches upstream behavior). Response metadata follows.
	if !sendStreamPart(ctx, out, provider.StreamPart{
		Type:     provider.PartStreamStart,
		Warnings: warnings,
	}) {
		return
	}

	if md := buildResponseMetadata(responseHeaders, m.modelID); md != nil {
		if !sendStreamPart(ctx, out, provider.StreamPart{
			Type:            provider.PartResponseMeta,
			ResponseID:      md.ID,
			ModelID:         md.ModelID,
			Provider:        md.Provider,
			Timestamp:       md.Timestamp,
			ResponseHeaders: md.Headers,
		}) {
			return
		}
	}

	c := &streamConsumer{
		out:        out,
		meta:       meta,
		modelID:    m.modelID,
		warnings:   warnings,
		blocks:     make(map[int]*blockState),
		includeRaw: includeRaw,
	}
	if meta.usesJSONInstruction {
		c.jsonExtractor = &jsonObjectTextExtractor{}
	}

	err := decodeEventStream(body, func(hdr frameHeader, payload []byte) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return c.handleFrame(hdr, payload)
	})
	switch {
	case err == nil:
		// Stream completed normally; emit the finish event.
		c.emitFinish(ctx)
	case ctx.Err() != nil:
		// Context cancelled mid-stream. Channel will be closed by the
		// caller; emit nothing extra.
	case err == errStreamTerminatedByException:
		// Exception frame already emitted a PartError and set the finish
		// reason to error. Upstream still emits a terminal finish chunk so
		// consumers can flush usage/finish state, so we do the same.
		c.emitFinish(ctx)
	default:
		// Transport-level failure (incomplete frame, CRC mismatch, etc.).
		// Synthesize a retryable APICallError so the orchestration layer
		// can decide whether to retry.
		retryable := true
		sendStreamPart(ctx, out, provider.StreamPart{
			Type: provider.PartError,
			APICallError: provider.NewAPICallError(provider.APICallErrorOptions{
				Message:     fmt.Sprintf("bedrock: stream decode failure: %v", err),
				IsRetryable: &retryable,
				Cause:       err,
			}),
		})
	}
}

// sendStreamPart sends part on ch unless ctx is cancelled. Returns false
// when the context is cancelled and the caller should bail.
func sendStreamPart(ctx context.Context, ch chan<- provider.StreamPart, part provider.StreamPart) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- part:
		return true
	}
}

func (c *streamConsumer) handleFrame(hdr frameHeader, payload []byte) error {
	// When IncludeRawChunks is set, emit the raw frame payload before
	// interpreting it (matching upstream, which enqueues a `raw` chunk for
	// every event before processing). Applies to every frame, including
	// exception and unknown event types.
	if c.includeRaw {
		if !sendStreamPart(context.Background(), c.out, provider.StreamPart{
			Type:     provider.PartRaw,
			RawValue: json.RawMessage(append([]byte(nil), payload...)),
		}) {
			return nil
		}
	}

	// Exception frames (message-type=exception) carry a Bedrock error JSON.
	if hdr.MessageType == "exception" {
		var berr converseError
		_ = json.Unmarshal(payload, &berr) // best-effort decode
		apiErr := bedrockExceptionToAPIError(hdr.ExceptionType, berr, payload)
		if !sendStreamPart(context.Background(), c.out, provider.StreamPart{
			Type:         provider.PartError,
			APICallError: apiErr,
		}) {
			return nil
		}
		c.errorEmitted = true
		// Match upstream: an error chunk flips the finish reason to "error".
		// The terminal finish event is still emitted by runStream.
		c.finish = provider.FinishReason{Unified: provider.FinishReasonError}
		c.finishKnown = true
		// Bedrock closes the stream after an exception; stop consuming via
		// our sentinel so runStream knows this is graceful termination
		// (not a transport failure).
		return errStreamTerminatedByException
	}

	// Event frames carry a JSON payload with one of the known event shapes
	// (messageStart, contentBlockStart, contentBlockDelta, contentBlockStop,
	// messageStop, metadata).
	switch hdr.EventType {
	case "messageStart":
		return c.handleMessageStart(payload)
	case "contentBlockStart":
		return c.handleContentBlockStart(payload)
	case "contentBlockDelta":
		return c.handleContentBlockDelta(payload)
	case "contentBlockStop":
		return c.handleContentBlockStop(payload)
	case "messageStop":
		return c.handleMessageStop(payload)
	case "metadata":
		return c.handleMetadata(payload)
	default:
		// Forward unknown events as raw stream parts for visibility. Skip when
		// includeRaw already emitted the raw frame above to avoid duplicates.
		if !c.includeRaw {
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:     provider.PartRaw,
				RawValue: json.RawMessage(append([]byte(nil), payload...)),
			})
		}
		return nil
	}
}

func (c *streamConsumer) handleMessageStart(_ []byte) error {
	// We already emitted PartResponseMeta on stream init. messageStart in the
	// Bedrock protocol primarily carries the role; we don't need to emit
	// anything extra for it.
	return nil
}

func (c *streamConsumer) handleContentBlockStart(payload []byte) error {
	var ev streamContentBlockStart
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decoding contentBlockStart: %w", err)
	}
	if ev.Start != nil && ev.Start.ToolUse != nil {
		tu := ev.Start.ToolUse
		toolCallID := normalizeToolCallID(tu.ToolUseID, c.meta.isMistral)
		isJSON := c.meta.usesJSONResponseTool && tu.Name == jsonResponseToolName
		c.blocks[ev.ContentBlockIndex] = &blockState{
			kind:               blockKindTool,
			toolCallID:         toolCallID,
			toolName:           tu.Name,
			isJSONResponseTool: isJSON,
		}
		if !isJSON {
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:       provider.PartToolInputStart,
				ID:         toolCallID,
				ToolName:   tu.Name,
				ToolCallID: toolCallID,
			})
		}
		return nil
	}
	// No tool use: this is the start of a text block. Bedrock omits a
	// dedicated text-start event so we wait until the first text delta to
	// record the block, matching upstream behavior. We could also pre-create
	// the block here for parity:
	c.blocks[ev.ContentBlockIndex] = &blockState{kind: blockKindText}
	_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
		Type: provider.PartTextStart,
		ID:   strconv.Itoa(ev.ContentBlockIndex),
	})
	return nil
}

func (c *streamConsumer) handleContentBlockDelta(payload []byte) error {
	var ev streamContentBlockDelta
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decoding contentBlockDelta: %w", err)
	}
	if ev.Delta == nil {
		return nil
	}
	idx := ev.ContentBlockIndex
	block := c.blocks[idx]

	// Text delta.
	if ev.Delta.Text != "" {
		if block == nil {
			block = &blockState{kind: blockKindText}
			c.blocks[idx] = block
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type: provider.PartTextStart,
				ID:   strconv.Itoa(idx),
			})
		}
		text := ev.Delta.Text
		if c.jsonExtractor != nil {
			text = c.jsonExtractor.process(text)
		}
		if text != "" {
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:  provider.PartTextDelta,
				ID:    strconv.Itoa(idx),
				Delta: text,
			})
		}
		return nil
	}

	// Tool input delta.
	if ev.Delta.ToolUse != nil {
		if block == nil || block.kind != blockKindTool {
			return nil
		}
		block.jsonText += ev.Delta.ToolUse.Input
		if !block.isJSONResponseTool {
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:  provider.PartToolInputDelta,
				ID:    block.toolCallID,
				Delta: ev.Delta.ToolUse.Input,
			})
		}
		return nil
	}

	// Reasoning content delta.
	if rc := ev.Delta.ReasoningContent; rc != nil {
		if block == nil {
			block = &blockState{kind: blockKindReasoning}
			c.blocks[idx] = block
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type: provider.PartReasoningStart,
				ID:   strconv.Itoa(idx),
			})
		}
		switch {
		case rc.Text != "":
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:  provider.PartReasoningDelta,
				ID:    strconv.Itoa(idx),
				Delta: rc.Text,
			})
		case rc.Signature != "":
			meta := jsonRawOrZero(ReasoningMetadata{Signature: rc.Signature})
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:  provider.PartReasoningDelta,
				ID:    strconv.Itoa(idx),
				Delta: "",
				ProviderMetadata: provider.ProviderMetadata{
					"amazonBedrock": meta,
					"bedrock":       meta,
				},
			})
		case rc.RedactedContent != "":
			block.redactedContent += rc.RedactedContent
		case rc.Data != "":
			meta := jsonRawOrZero(ReasoningMetadata{RedactedData: rc.Data})
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:  provider.PartReasoningDelta,
				ID:    strconv.Itoa(idx),
				Delta: "",
				ProviderMetadata: provider.ProviderMetadata{
					"amazonBedrock": meta,
					"bedrock":       meta,
				},
			})
		}
	}
	return nil
}

func (c *streamConsumer) handleContentBlockStop(payload []byte) error {
	var ev streamContentBlockStop
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decoding contentBlockStop: %w", err)
	}
	block := c.blocks[ev.ContentBlockIndex]
	if block == nil {
		return nil
	}
	switch block.kind {
	case blockKindText:
		_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
			Type: provider.PartTextEnd,
			ID:   strconv.Itoa(ev.ContentBlockIndex),
		})
	case blockKindReasoning:
		part := provider.StreamPart{
			Type: provider.PartReasoningEnd,
			ID:   strconv.Itoa(ev.ContentBlockIndex),
		}
		if block.redactedContent != "" {
			meta := jsonRawOrZero(ReasoningMetadata{RedactedContent: block.redactedContent})
			part.ProviderMetadata = provider.ProviderMetadata{
				"amazonBedrock": meta,
				"bedrock":       meta,
			}
		}
		_ = sendStreamPart(context.Background(), c.out, part)
	case blockKindTool:
		input := block.jsonText
		if input == "" {
			input = "{}"
		}
		if block.isJSONResponseTool {
			// JSON-response-tool collapse: emit text-start + text-delta +
			// text-end carrying the JSON. The runner side flips
			// finishReason to stop via meta.usesJSONResponseTool.
			c.jsonRespEmitted = true
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type: provider.PartTextStart, ID: strconv.Itoa(ev.ContentBlockIndex),
			})
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type: provider.PartTextDelta, ID: strconv.Itoa(ev.ContentBlockIndex), Delta: input,
			})
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type: provider.PartTextEnd, ID: strconv.Itoa(ev.ContentBlockIndex),
			})
		} else {
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type: provider.PartToolInputEnd, ID: block.toolCallID,
			})
			_ = sendStreamPart(context.Background(), c.out, provider.StreamPart{
				Type:       provider.PartToolCall,
				ToolCallID: block.toolCallID,
				ToolName:   block.toolName,
				Input:      input,
			})
		}
	}
	delete(c.blocks, ev.ContentBlockIndex)
	return nil
}

func (c *streamConsumer) handleMessageStop(payload []byte) error {
	var ev streamMessageStop
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decoding messageStop: %w", err)
	}
	c.finish = mapFinishReason(ev.StopReason, c.jsonRespEmitted)
	c.finishKnown = true
	if ev.AdditionalModelResponseFields != nil && ev.AdditionalModelResponseFields.Delta != nil {
		c.stopSequence = ev.AdditionalModelResponseFields.Delta.StopSequence
	}
	return nil
}

func (c *streamConsumer) handleMetadata(payload []byte) error {
	var ev streamMetadata
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("decoding metadata: %w", err)
	}
	if ev.Usage != nil {
		u := convertUsage(ev.Usage)
		c.usage = &u
	}
	// Build provider metadata under amazonBedrock + bedrock when any of
	// trace/performanceConfig/serviceTier/cache details are present.
	payloadMap := map[string]any{}
	if len(ev.Trace) > 0 {
		var t any
		if json.Unmarshal(ev.Trace, &t) == nil {
			payloadMap["trace"] = t
		}
	}
	if len(ev.PerformanceConfig) > 0 {
		var pc any
		if json.Unmarshal(ev.PerformanceConfig, &pc) == nil {
			payloadMap["performanceConfig"] = pc
		}
	}
	if len(ev.ServiceTier) > 0 {
		var st any
		if json.Unmarshal(ev.ServiceTier, &st) == nil {
			payloadMap["serviceTier"] = st
		}
	}
	if ev.Usage != nil {
		usagePayload := map[string]any{}
		if ev.Usage.CacheWriteInputTokens != nil {
			usagePayload["cacheWriteInputTokens"] = *ev.Usage.CacheWriteInputTokens
		}
		if len(ev.Usage.CacheDetails) > 0 {
			var cd any
			if json.Unmarshal(ev.Usage.CacheDetails, &cd) == nil {
				usagePayload["cacheDetails"] = cd
			}
		}
		if len(usagePayload) > 0 {
			payloadMap["usage"] = usagePayload
		}
	}
	if len(payloadMap) > 0 {
		encoded := jsonRawOrZero(payloadMap)
		if c.providerMD == nil {
			c.providerMD = provider.ProviderMetadata{}
		}
		c.providerMD["amazonBedrock"] = encoded
		c.providerMD["bedrock"] = encoded
	}
	return nil
}

// emitFinish flushes the final finish event with the accumulated finish
// reason, usage, and provider metadata. Called once per stream.
func (c *streamConsumer) emitFinish(ctx context.Context) {
	c.emittedFinish = true
	if !c.finishKnown {
		c.finish = provider.FinishReason{Unified: provider.FinishReasonOther}
	}

	// JSON-response-tool collapse augments provider metadata with the marker
	// so consumers know the text came from the synthetic json tool. Mirrors
	// upstream's flush: when triggered, stopSequence is always written (null
	// or value).
	if c.jsonRespEmitted || c.stopSequence != nil {
		extras := map[string]any{}
		if c.jsonRespEmitted {
			extras["isJsonResponseFromTool"] = true
		}
		extras["stopSequence"] = c.stopSequence
		// Merge with any existing metadata.
		if c.providerMD == nil {
			c.providerMD = provider.ProviderMetadata{}
		}
		existing := map[string]any{}
		if raw, ok := c.providerMD["amazonBedrock"]; ok {
			_ = json.Unmarshal(raw, &existing)
		}
		for k, v := range extras {
			existing[k] = v
		}
		encoded := jsonRawOrZero(existing)
		c.providerMD["amazonBedrock"] = encoded
		c.providerMD["bedrock"] = encoded
	}

	usage := c.usage
	if usage == nil {
		usage = &provider.Usage{}
	}
	_ = sendStreamPart(ctx, c.out, provider.StreamPart{
		Type:             provider.PartFinish,
		FinishReason:     &c.finish,
		Usage:            usage,
		ProviderMetadata: c.providerMD,
	})
}

// bedrockExceptionToAPIError builds an APICallError from a Bedrock exception
// frame. The exception type drives `IsRetryable` (throttling and 5xx are
// retryable; validation/runtime are not).
func bedrockExceptionToAPIError(exceptionType string, body converseError, raw []byte) *provider.APICallError {
	retryable := false
	statusCode := 500
	switch exceptionType {
	case "throttlingException":
		retryable = true
		statusCode = 429
	case "internalServerException", "modelStreamErrorException":
		retryable = true
		statusCode = 500
	case "serviceUnavailableException":
		retryable = true
		statusCode = 503
	case "validationException":
		retryable = false
		statusCode = 400
	default:
		// Unknown exception types: default to non-retryable.
		retryable = false
	}
	msg := body.Message
	if msg == "" {
		msg = exceptionType
		if exceptionType == "" {
			msg = "bedrock stream exception"
		}
	}
	return provider.NewAPICallError(provider.APICallErrorOptions{
		Message:      msg,
		StatusCode:   statusCode,
		ResponseBody: string(raw),
		IsRetryable:  &retryable,
	})
}
