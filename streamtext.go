package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

const (
	// defaultStreamBuffer is the channel buffer size for stream channels.
	defaultStreamBuffer = 256
)

// ErrNoOutputGenerated is returned when a model stream ends before producing output.
var ErrNoOutputGenerated = errors.New("aisdk: no output generated")

// StreamTextResult holds the result of a StreamText call.
// It provides both a streaming interface (FullStream) and blocking accessors
// that resolve after the stream completes.
type StreamTextResult struct {
	fullStream chan TextStreamPart
	done       chan struct{}
	consumed   atomic.Bool
	mu         sync.Mutex

	steps        []StepResult
	totalUsage   provider.Usage
	lastStep     *StepResult
	allWarnings  []provider.Warning
	provMeta     provider.ProviderMetadata
	lastResponse ResponseMetadata
	firstError   error
	abortCause   error

	outputValue any
	outputErr   error

	partialOutputStream *losslessStream[json.RawMessage]
	elementStream       *losslessStream[json.RawMessage]
	lastPartialJSON     string
	emittedElementCount int
}

// StreamText starts an LLM streaming call with multi-step tool execution.
// It returns immediately; consume events via FullStream() or convert to
// SSE via ToUIMessageStream(). Call Wait() or use blocking accessors
// to wait for completion.
func StreamText(ctx context.Context, model provider.LanguageModel, opts ...StreamOption) *StreamTextResult {
	cfg := buildStreamConfig(opts)
	return streamTextWithConfig(ctx, model, cfg)
}

func streamTextWithConfig(ctx context.Context, model provider.LanguageModel, cfg *streamConfig) *StreamTextResult {
	result := &StreamTextResult{
		fullStream: make(chan TextStreamPart, defaultStreamBuffer),
		done:       make(chan struct{}),
	}

	result.partialOutputStream = newLosslessStream[json.RawMessage]()
	result.elementStream = newLosslessStream[json.RawMessage]()
	if cfg.output == nil {
		result.partialOutputStream.close()
		result.elementStream.close()
	}

	go result.run(ctx, model, cfg)
	return result
}

type losslessStream[T any] struct {
	mu     sync.Mutex
	ready  *sync.Cond
	queue  []T
	output chan T
	closed bool
	start  sync.Once
}

func newLosslessStream[T any]() *losslessStream[T] {
	stream := &losslessStream[T]{output: make(chan T, defaultStreamBuffer)}
	stream.ready = sync.NewCond(&stream.mu)
	return stream
}

func (s *losslessStream[T]) send(value T) {
	s.mu.Lock()
	s.queue = append(s.queue, value)
	s.ready.Signal()
	s.mu.Unlock()
}

func (s *losslessStream[T]) close() {
	s.mu.Lock()
	s.closed = true
	s.ready.Broadcast()
	s.mu.Unlock()
}

func (s *losslessStream[T]) stream() <-chan T {
	s.start.Do(func() {
		s.mu.Lock()
		if s.closed && len(s.queue) == 0 {
			close(s.output)
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		go func() {
			defer close(s.output)
			for {
				s.mu.Lock()
				for len(s.queue) == 0 && !s.closed {
					s.ready.Wait()
				}
				if len(s.queue) == 0 {
					s.mu.Unlock()
					return
				}
				next := s.queue[0]
				var zero T
				s.queue[0] = zero
				s.queue = s.queue[1:]
				s.mu.Unlock()

				s.output <- next
			}
		}()
	})
	return s.output
}

// consumeStream claims exclusive read access to the internal stream channel.
// The first call returns the live channel; subsequent calls return a closed
// (empty) channel. This prevents multiple consumers from splitting events.
func (r *StreamTextResult) consumeStream() <-chan TextStreamPart {
	if !r.consumed.CompareAndSwap(false, true) {
		ch := make(chan TextStreamPart)
		close(ch)
		return ch
	}
	return r.fullStream
}

// FullStream returns a channel of TextStreamPart events.
// The channel is closed when the stream completes.
// Only one consumer may read the stream; a second call to FullStream or
// ToUIMessageStream returns a closed (empty) channel.
func (r *StreamTextResult) FullStream() <-chan TextStreamPart {
	return r.consumeStream()
}

// Stream returns a channel of TextStreamPart events.
func (r *StreamTextResult) Stream() <-chan TextStreamPart {
	return r.FullStream()
}

// ToUIMessageStream converts the internal event stream to UIMessageChunk
// events suitable for SSE output to @ai-sdk/react hooks.
func (r *StreamTextResult) ToUIMessageStream(opts ...UIMessageStreamOption) <-chan UIMessageChunk {
	cfg := buildUIMessageStreamConfig(opts)
	out := make(chan UIMessageChunk, defaultStreamBuffer)
	go func() {
		defer close(out)

		messageID := responseUIMessageID(cfg)

		var assembledChunks []UIMessageChunk
		var finishReason provider.FinishReason
		var isAborted bool
		outcome := UIMessageStreamOutcome{Status: UIMessageStreamOutcomeUnknown}
		for part := range r.consumeStream() {
			switch part := part.(type) {
			case StreamFinish:
				if outcome.Status != UIMessageStreamOutcomeFailed && outcome.Status != UIMessageStreamOutcomeAborted {
					outcome = UIMessageStreamOutcome{Status: UIMessageStreamOutcomeCompleted}
				}
			case StreamAbort:
				if outcome.Status != UIMessageStreamOutcomeFailed {
					outcome = UIMessageStreamOutcome{Status: UIMessageStreamOutcomeAborted}
				}
			case StreamError:
				outcome = UIMessageStreamOutcome{Status: UIMessageStreamOutcomeFailed, Error: part.Error}
			}
			metadata := messageMetadataForPart(part, cfg)
			chunks := translateToChunksWithMetadata(part, cfg, metadata)
			if metadata != nil && !isStreamMessageMetadataCarrier(part) {
				chunks = append(chunks, UIMessageChunk{Type: ChunkMessageMetadata, MessageMetadata: metadata})
			}
			for _, chunk := range chunks {
				if chunk.Type == ChunkStart && messageID != "" && chunk.MessageID == "" {
					chunk.MessageID = messageID
				}
				if !shouldSendChunk(chunk, cfg) {
					continue
				}
				if chunk.Type == ChunkFinish && chunk.FinishReason != "" {
					finishReason = provider.FinishReason{Unified: provider.UnifiedFinishReason(chunk.FinishReason)}
				}
				if chunk.Type == ChunkAbort {
					isAborted = true
				}
				assembledChunks = append(assembledChunks, chunk)
				out <- chunk
			}
		}
		if cfg.onFinish != nil {
			respMsg := assembleResponseMessageForFinish(messageID, assembledChunks, cfg)
			isContinuation := false
			if last := lastOriginalAssistantMessage(cfg); last != nil {
				isContinuation = respMsg.ID == last.ID
			}
			messages := append([]UIMessage{}, cfg.originalMessages...)
			if isContinuation && len(messages) > 0 {
				messages = append(messages[:len(messages)-1], respMsg)
			} else {
				messages = append(messages, respMsg)
			}
			cfg.onFinish(UIMessageStreamOnFinishState{
				Messages:        messages,
				IsContinuation:  isContinuation,
				IsAborted:       isAborted || outcome.Status == UIMessageStreamOutcomeAborted,
				Outcome:         outcome,
				ResponseMessage: respMsg,
				FinishReason:    finishReason,
			})
		}
	}()
	return out
}

func responseUIMessageID(cfg uiMessageStreamConfig) string {
	if last := lastOriginalAssistantMessage(cfg); last != nil {
		return last.ID
	}
	if !cfg.hasOriginalMessages {
		if cfg.generateMessageID != nil {
			return cfg.generateMessageID()
		}
		return ""
	}
	genID := cfg.generateMessageID
	if genID == nil {
		genID = GenerateID
	}
	return genID()
}

func lastOriginalAssistantMessage(cfg uiMessageStreamConfig) *UIMessage {
	if !cfg.hasOriginalMessages || len(cfg.originalMessages) == 0 {
		return nil
	}
	last := cfg.originalMessages[len(cfg.originalMessages)-1]
	if last.Role != RoleAssistant {
		return nil
	}
	return &last
}

func assembleResponseMessageForFinish(messageID string, chunks []UIMessageChunk, cfg uiMessageStreamConfig) UIMessage {
	if last := lastOriginalAssistantMessage(cfg); last != nil {
		return assembleResponseMessageWithInitial(messageID, chunks, last)
	}
	return assembleResponseMessage(messageID, chunks)
}

func messageMetadataForPart(part TextStreamPart, cfg uiMessageStreamConfig) json.RawMessage {
	if cfg.messageMetadata == nil {
		return nil
	}
	return cfg.messageMetadata(part)
}

func isStreamMessageMetadataCarrier(part TextStreamPart) bool {
	switch part.(type) {
	case StreamStart, StreamFinish:
		return true
	default:
		return false
	}
}

func translateToChunks(part TextStreamPart, cfg uiMessageStreamConfig) []UIMessageChunk {
	return translateToChunksWithMetadata(part, cfg, messageMetadataForPart(part, cfg))
}

func translateToChunksWithMetadata(part TextStreamPart, cfg uiMessageStreamConfig, metadata json.RawMessage) []UIMessageChunk {
	switch p := part.(type) {
	case StreamStart:
		c := UIMessageChunk{Type: ChunkStart}
		if metadata != nil {
			c.MessageMetadata = metadata
		}
		return []UIMessageChunk{c}
	case StreamStartStep:
		return []UIMessageChunk{{Type: ChunkStartStep}}
	case StreamFinishStep:
		return []UIMessageChunk{{Type: ChunkFinishStep}}
	case StreamFinish:
		c := UIMessageChunk{Type: ChunkFinish}
		if p.FinishReason.Unified != "" {
			c.FinishReason = string(p.FinishReason.Unified)
		}
		if metadata != nil {
			c.MessageMetadata = metadata
		}
		return []UIMessageChunk{c}
	case StreamAbort:
		return []UIMessageChunk{{Type: ChunkAbort, Reason: p.Reason}}
	case StreamTextStart:
		return []UIMessageChunk{{Type: ChunkTextStart, ID: p.ID, ProviderMetadata: p.ProviderMetadata}}
	case StreamTextDelta:
		return []UIMessageChunk{{Type: ChunkTextDelta, ID: p.ID, Delta: p.Text, ProviderMetadata: p.ProviderMetadata}}
	case StreamTextEnd:
		return []UIMessageChunk{{Type: ChunkTextEnd, ID: p.ID, ProviderMetadata: p.ProviderMetadata}}
	case StreamReasoningStart:
		return []UIMessageChunk{{Type: ChunkReasoningStart, ID: p.ID, ProviderMetadata: p.ProviderMetadata}}
	case StreamReasoningDelta:
		return []UIMessageChunk{{Type: ChunkReasoningDelta, ID: p.ID, Delta: p.Text, ProviderMetadata: p.ProviderMetadata}}
	case StreamReasoningEnd:
		return []UIMessageChunk{{Type: ChunkReasoningEnd, ID: p.ID, ProviderMetadata: p.ProviderMetadata}}
	case StreamToolInputStart:
		return []UIMessageChunk{{Type: ChunkToolInputStart, ToolCallID: p.ID, ToolName: p.ToolName, ProviderExecuted: p.ProviderExecuted, Dynamic: p.Dynamic, Title: p.Title, ProviderMetadata: p.ProviderMetadata, ToolMetadata: toolMetadataFromProviderMetadata(p.ProviderMetadata)}}
	case StreamToolInputDelta:
		return []UIMessageChunk{{Type: ChunkToolInputDelta, ToolCallID: p.ID, InputTextDelta: p.Delta}}
	case StreamToolInputEnd:
		return nil // not sent to wire
	case StreamToolCall:
		if p.Invalid {
			dynamic := p.Dynamic
			if p.useUIDynamic {
				dynamic = p.uiDynamic
			}
			return []UIMessageChunk{{Type: ChunkToolInputError, ToolCallID: p.ToolCallID, ToolName: p.ToolName, Input: p.Input, ErrorText: errorText(p.Error, cfg), ProviderExecuted: p.ProviderExecuted, Dynamic: dynamic, Title: p.Title, ProviderMetadata: p.ProviderMetadata, ToolMetadata: toolMetadataFromProviderMetadata(p.ProviderMetadata)}}
		}
		return []UIMessageChunk{{Type: ChunkToolInputAvailable, ToolCallID: p.ToolCallID, ToolName: p.ToolName, Input: p.Input, ProviderExecuted: p.ProviderExecuted, Dynamic: p.Dynamic, Title: p.Title, ProviderMetadata: p.ProviderMetadata, ToolMetadata: toolMetadataFromProviderMetadata(p.ProviderMetadata)}}
	case StreamToolApprovalRequest:
		return []UIMessageChunk{{Type: ChunkToolApprovalRequest, ApprovalID: p.ApprovalID, ToolCallID: p.ToolCallID, Reason: p.Reason, IsAutomatic: p.IsAutomatic, Signature: p.Signature}}
	case StreamToolApprovalResponse:
		approved := p.Approved
		return []UIMessageChunk{{Type: ChunkToolApprovalResponse, ApprovalID: p.ApprovalID, Approved: approved, Reason: p.Reason, ProviderExecuted: p.ProviderExecuted, ProviderMetadata: p.ProviderMetadata}}
	case StreamToolOutputDenied:
		return []UIMessageChunk{{Type: ChunkToolOutputDenied, ToolCallID: p.ToolCallID}}
	case StreamToolResult:
		return []UIMessageChunk{{Type: ChunkToolOutputAvailable, ToolCallID: p.ToolCallID, Output: p.Output, ProviderExecuted: p.ProviderExecuted, Dynamic: p.Dynamic, Preliminary: p.Preliminary, ProviderMetadata: p.ProviderMetadata}}
	case StreamToolError:
		errText := errorText(p.Error, cfg)
		if p.ProviderExecuted {
			errText = p.Error.Error()
		}
		dynamic := p.Dynamic
		if p.useUIDynamic {
			dynamic = p.uiDynamic
		}
		return []UIMessageChunk{{Type: ChunkToolOutputError, ToolCallID: p.ToolCallID, ErrorText: errText, ProviderExecuted: p.ProviderExecuted, Dynamic: dynamic, ProviderMetadata: p.ProviderMetadata}}
	case StreamSource:
		src := p.Source
		if src.SourceType == provider.SourceTypeURL {
			return []UIMessageChunk{{Type: ChunkSourceURL, SourceID: src.ID, URL: src.URL, Title: src.Title, ProviderMetadata: src.ProviderMetadata}}
		}
		return []UIMessageChunk{{Type: ChunkSourceDocument, SourceID: src.ID, MediaType: src.MediaType, Title: src.Title, Filename: src.Filename, ProviderMetadata: src.ProviderMetadata}}
	case StreamFile:
		return []UIMessageChunk{{Type: ChunkFile, URL: p.File.DataURL(), MediaType: p.File.MediaType, ProviderMetadata: p.ProviderMetadata}}
	case StreamReasoningFile:
		return []UIMessageChunk{{Type: ChunkReasoningFile, URL: p.File.DataURL(), MediaType: p.File.MediaType, ProviderMetadata: p.ProviderMetadata}}
	case StreamError:
		return []UIMessageChunk{{Type: ChunkError, ErrorText: errorText(p.Error, cfg)}}
	case StreamRaw:
		return nil // not sent to wire
	case StreamCustom:
		return []UIMessageChunk{{Type: ChunkCustom, Kind: p.Kind, ProviderMetadata: p.ProviderMetadata}}
	default:
		return nil
	}
}

func errorText(err error, cfg uiMessageStreamConfig) string {
	if cfg.onError != nil {
		return cfg.onError(err)
	}
	return "An error occurred."
}

func toolMetadataFromProviderMetadata(meta provider.ProviderMetadata) json.RawMessage {
	if len(meta) == 0 {
		return nil
	}
	raw, ok := meta["toolMetadata"]
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make(json.RawMessage, len(raw))
	copy(out, raw)
	return out
}

func buildRetryConfig(cfg *baseConfig) retryConfig {
	rc := defaultRetryConfig
	if cfg.maxRetries != nil {
		v := *cfg.maxRetries
		if v < 0 {
			v = 0
		}
		rc.maxRetries = v
	}
	if cfg.retryInitDelay > 0 {
		rc.initialDelayMs = cfg.retryInitDelay
	}
	if cfg.retryBackoff > 0 {
		rc.backoffFactor = cfg.retryBackoff
	}
	return rc
}

func (r *StreamTextResult) run(ctx context.Context, model provider.LanguageModel, cfg *streamConfig) {
	defer close(r.fullStream)
	defer close(r.done)
	if cfg.output != nil {
		defer r.partialOutputStream.close()
		defer r.elementStream.close()
	}

	if cfg.timeout.Total > 0 {
		var totalCancel context.CancelFunc
		ctx, totalCancel = context.WithTimeout(ctx, cfg.timeout.Total)
		defer totalCancel()
	}

	var opCancel context.CancelFunc
	ctx, opCancel = context.WithCancel(ctx)
	defer opCancel()

	r.emit(StreamStart{})

	if cfg.onStart != nil {
		cfg.onStart(OnStartState{})
	}

	// Convert messages
	var msgs []provider.Message
	if cfg.modelMessages != nil {
		msgs = cfg.modelMessages
	} else if cfg.messages != nil {
		var err error
		msgs, err = ConvertToModelMessages(cfg.messages, WithTools(cfg.tools))
		if err != nil {
			r.emitError(err, cfg.onError)
			return
		}
	}

	initialMessages := cloneMessages(msgs)

	// Prepend system messages
	if len(cfg.system) > 0 {
		sysMsgs := systemToMessages(cfg.system)
		msgs = append(sysMsgs, msgs...)
	}

	preApprovalMessageCount := len(msgs)
	var err error
	msgs, err = r.resolveToolApprovals(ctx, cfg, msgs)
	if err != nil {
		r.emitError(err, cfg.onError)
		return
	}
	responseMessages := cloneMessages(msgs[preApprovalMessageCount:])

	// Stop conditions default
	stopWhen := cfg.stopWhen
	if len(stopWhen) == 0 {
		stopWhen = []StopCondition{StepCountIs(1)}
	}

	retryCfg := buildRetryConfig(&cfg.baseConfig)
	stepNum := 0
	currentRuntimeContext := cfg.runtimeContext
	usedTextPartIDs := make(map[string]struct{})
	usedReasoningPartIDs := make(map[string]struct{})

	for {
		stepNum++
		currentModel := model
		currentMsgs := msgs
		toolChoice := cfg.toolChoice
		activeTools := cfg.activeTools
		activeToolsSet := cfg.activeToolsSet
		providerOpts := cfg.providerOptions
		maxOutputTokens := cfg.maxOutputTokens
		temperature := cfg.temperature
		topP := cfg.topP
		topK := cfg.topK
		presencePenalty := cfg.presencePenalty
		frequencyPenalty := cfg.frequencyPenalty
		stopSequences := cfg.stopSequences
		seed := cfg.seed
		reasoning := cfg.reasoning
		stepContext := currentRuntimeContext

		// PrepareStep
		if cfg.prepareStep != nil {
			result, err := cfg.prepareStep(PrepareStepState{
				Steps:            r.steps,
				StepNumber:       len(r.steps),
				Model:            currentModel,
				Messages:         currentMsgs,
				InitialMessages:  initialMessages,
				ResponseMessages: responseMessages,
				Context:          currentRuntimeContext,
			})
			if err != nil {
				if ctx.Err() != nil && cfg.onAbort != nil {
					cfg.onAbort(OnAbortState{Steps: r.steps})
				}
				r.emitError(err, cfg.onError)
				return
			}
			if result != nil {
				if result.Model != nil {
					currentModel = result.Model
				}
				if result.System != nil {
					sysMsgs := systemToMessages(result.System)
					var nonSys []provider.Message
					for _, m := range currentMsgs {
						if m.Role != provider.RoleSystem {
							nonSys = append(nonSys, m)
						}
					}
					currentMsgs = append(sysMsgs, nonSys...)
				}
				if result.ToolChoice != nil {
					toolChoice = result.ToolChoice
				}
				if result.ActiveTools != nil {
					activeTools = result.ActiveTools
					activeToolsSet = true
				}
				if result.Messages != nil {
					currentMsgs = result.Messages
				}
				if result.ProviderOptions != nil {
					providerOpts, err = mergeStepProviderOptions(providerOpts, result.ProviderOptions)
					if err != nil {
						r.emitError(fmt.Errorf("aisdk: merging prepareStep provider options: %w", err), cfg.onError)
						return
					}
				}
				if result.MaxOutputTokens != nil {
					maxOutputTokens = result.MaxOutputTokens
				}
				if result.Temperature != nil {
					temperature = result.Temperature
				}
				if result.TopP != nil {
					topP = result.TopP
				}
				if result.TopK != nil {
					topK = result.TopK
				}
				if result.PresencePenalty != nil {
					presencePenalty = result.PresencePenalty
				}
				if result.FrequencyPenalty != nil {
					frequencyPenalty = result.FrequencyPenalty
				}
				if result.StopSequences != nil {
					stopSequences = result.StopSequences
				}
				if result.Seed != nil {
					seed = result.Seed
				}
				if result.Reasoning != nil {
					reasoning = result.Reasoning
				}
				if result.Context != nil {
					currentRuntimeContext = result.Context
					stepContext = currentRuntimeContext
				}
			}
		}

		if maxOutputTokens != nil && *maxOutputTokens < 1 {
			r.emitError(errors.New("aisdk: maxOutputTokens must be >= 1"), cfg.onError)
			return
		}

		stepModel := StepModel{
			Provider: currentModel.Provider(),
			ModelID:  currentModel.ModelID(),
		}

		if cfg.onStepStart != nil {
			cfg.onStepStart(OnStepStartState{StepNumber: stepNum, Model: stepModel})
		}

		// Build provider tools (sorted for deterministic order)
		provTools, toolWarnings := toolSetToProviderTools(cfg.tools)
		r.allWarnings = append(r.allWarnings, toolWarnings...)
		if activeToolsSet {
			provTools = filterProviderTools(provTools, activeTools)
		}
		if toolChoice == nil && len(provTools) > 0 {
			auto := provider.ToolChoice{Type: provider.ToolChoiceAuto}
			toolChoice = &auto
		}

		responseFormat := cfg.responseFormat
		if cfg.output != nil {
			responseFormat = cfg.output.ResponseFormat()
		}

		providerPrompt, err := sanitizePromptForProvider(currentMsgs)
		if err != nil {
			r.emitError(err, cfg.onError)
			return
		}

		callOpts := provider.CallOptions{
			Prompt:           providerPrompt,
			Tools:            provTools,
			ToolChoice:       toolChoice,
			MaxOutputTokens:  maxOutputTokens,
			Temperature:      temperature,
			TopP:             topP,
			TopK:             topK,
			PresencePenalty:  presencePenalty,
			FrequencyPenalty: frequencyPenalty,
			StopSequences:    stopSequences,
			ResponseFormat:   responseFormat,
			Seed:             seed,
			Reasoning:        reasoning,
			IncludeRawChunks: cfg.includeRawChunks,
			Headers:          cfg.headers,
			ProviderOptions:  providerOpts,
		}

		// Step timeout fires opCancel, which permanently aborts the entire
		// operation (not just this step). Retry backoff delays also consume
		// this budget, matching upstream's shared AbortController semantics.
		var stepTimer *time.Timer
		if cfg.timeout.Step > 0 {
			stepTimer = time.AfterFunc(cfg.timeout.Step, opCancel)
		}

		streamResult, err := retryWithExponentialBackoff(ctx, retryCfg, func() (*provider.StreamResult, error) {
			return currentModel.DoStream(ctx, callOpts)
		})
		if err != nil {
			if stepTimer != nil {
				stepTimer.Stop()
			}
			if ctx.Err() != nil && cfg.onAbort != nil {
				cfg.onAbort(OnAbortState{Steps: r.steps})
			}
			r.emitStreamError(err, cfg.onError)
			return
		}

		step, stepCompleted, stepTerminated, stepHasOutput, err := r.processStep(ctx, stepNum, stepModel, streamResult, cfg, stepContext, currentMsgs, opCancel, usedTextPartIDs, usedReasoningPartIDs)
		if stepTimer != nil {
			stepTimer.Stop()
		}
		if err != nil {
			r.emitError(err, cfg.onError)
			return
		}

		// Mid-stream cancellation: the context was canceled while reading
		// the provider stream. Fire onAbort, and only fire onFinish if
		// earlier steps completed (matching upstream recordedSteps semantics).
		if ctx.Err() != nil {
			if stepCompleted {
				r.mu.Lock()
				r.steps = append(r.steps, step)
				r.lastStep = &r.steps[len(r.steps)-1]
				r.lastResponse = step.Response
				r.provMeta = step.ProviderMetadata
				r.allWarnings = append(r.allWarnings, step.Warnings...)
				r.mu.Unlock()
				if cfg.onStepFinish != nil {
					cfg.onStepFinish(OnStepFinishState{StepResult: step})
				}
			}
			r.abort(ctx, cfg)
			if len(r.steps) > 0 && cfg.onFinish != nil {
				r.totalUsage = aggregateUsage(r.steps)
				last := r.steps[len(r.steps)-1]
				cfg.onFinish(OnFinishState{
					StepResult: last,
					Steps:      r.steps,
					TotalUsage: r.totalUsage,
				})
			}
			return
		}

		if !stepTerminated && !stepHasOutput {
			r.emitStreamError(fmt.Errorf("%w: model stream ended without a finish chunk", ErrNoOutputGenerated), cfg.onError)
			return
		}

		r.mu.Lock()
		r.steps = append(r.steps, step)
		r.lastStep = &r.steps[len(r.steps)-1]
		r.lastResponse = step.Response
		r.provMeta = step.ProviderMetadata
		r.allWarnings = append(r.allWarnings, step.Warnings...)
		r.mu.Unlock()

		if cfg.onStepFinish != nil {
			cfg.onStepFinish(OnStepFinishState{StepResult: step})
		}

		// Check if we should continue — only count client (non-provider-executed)
		// tool calls. Server-side tool calls (code_execution, web_search, etc.)
		// are fully resolved within the same API response and do not require a
		// follow-up request.
		hasClientToolCalls := false
		hasUnresolvedClientToolCalls := false
		hasUnresolvedExternal := false
		hasPendingApproval := false
		toolResultsByID := make(map[string]bool, len(step.ToolResults))
		for _, tr := range step.ToolResults {
			toolResultsByID[tr.ToolCallID] = true
		}
		approvalRequestsByID := make(map[string]bool, len(step.ToolApprovalRequests))
		for _, ar := range step.ToolApprovalRequests {
			approvalRequestsByID[ar.ToolCallID] = true
		}
		for _, tc := range step.ToolCalls {
			if !tc.ProviderExecuted {
				hasClientToolCalls = true
				if !toolResultsByID[tc.ToolCallID] {
					hasUnresolvedClientToolCalls = true
				}
				if !tc.Invalid {
					if tool, ok := cfg.tools[tc.ToolName]; ok {
						if tool.Execute == nil {
							hasUnresolvedExternal = true
						}
					}
					if approvalRequestsByID[tc.ToolCallID] && !toolResultsByID[tc.ToolCallID] {
						hasPendingApproval = true
					}
				}
			}
		}

		stopped := false
		for _, cond := range stopWhen {
			if cond(StopConditionState{Steps: r.steps}) {
				stopped = true
				break
			}
		}

		if !hasClientToolCalls || stopped || hasUnresolvedClientToolCalls || hasUnresolvedExternal || hasPendingApproval {
			if cfg.output != nil && (cfg.parseOutputOnNonStop || step.FinishReason.Unified == provider.FinishReasonStop || (step.FinishReason.Unified != provider.FinishReasonToolCalls && step.Text != "")) {
				outputVal, outputErr := cfg.output.ParseComplete(step.Text)
				r.mu.Lock()
				r.outputValue = outputVal
				r.outputErr = outputErr
				r.mu.Unlock()
			}

			r.totalUsage = aggregateUsage(r.steps)

			if cfg.onFinish != nil {
				cfg.onFinish(OnFinishState{
					StepResult: step,
					Steps:      r.steps,
					TotalUsage: r.totalUsage,
					Output:     r.outputValue,
				})
			}

			r.emit(StreamFinish{
				FinishReason: step.FinishReason,
				TotalUsage:   &r.totalUsage,
			})
			return
		}

		// Append tool results to messages for next step
		stepResponseMessages := step.Response.Messages
		msgs = appendToolResults(currentMsgs, step)
		nextResponseMessages := make([]provider.Message, 0, len(responseMessages)+len(stepResponseMessages))
		nextResponseMessages = append(nextResponseMessages, responseMessages...)
		responseMessages = append(nextResponseMessages, stepResponseMessages...)
	}
}

func (r *StreamTextResult) processStep(
	ctx context.Context,
	stepNum int,
	stepModel StepModel,
	streamResult *provider.StreamResult,
	cfg *streamConfig,
	stepContext any,
	currentMsgs []provider.Message,
	opCancel context.CancelFunc,
	usedTextPartIDs map[string]struct{},
	usedReasoningPartIDs map[string]struct{},
) (StepResult, bool, bool, bool, error) {
	step := StepResult{
		StepNumber: stepNum,
		StepType:   StepTypeInitial,
		Model:      stepModel,
	}
	if stepNum > 1 {
		step.StepType = StepTypeToolResult
	}

	var warnings []provider.Warning
	responseMeta := provider.ResponseMetadata{ID: generateConfigID(cfg), ModelID: stepModel.ModelID}
	var responseHeaders map[string]string
	var textBuilder strings.Builder
	var reasoningBlocks []ReasoningOutput
	type activeReasoningBlock struct {
		text             strings.Builder
		providerMetadata provider.ProviderMetadata
		outputIndex      int
	}
	activeReasoning := make(map[string]*activeReasoningBlock)
	var completed bool
	var terminated bool
	var hasOutput bool
	toolNameByID := make(map[string]string)
	toolTitleByID := make(map[string]string)
	responseTextIndex := make(map[string]int)
	responseReasoningIndex := make(map[string]int)
	textPartIDs := make(map[string]string)
	reasoningPartIDs := make(map[string]string)

	var outputTextChunkID string
	var outputTextChunk strings.Builder
	var outputTextMeta provider.ProviderMetadata
	var lastPublishedOutput string

	r.emit(StreamStartStep{Warnings: warnings, Request: streamResult.Request})

	firstChunkTimeout := cfg.timeout.FirstChunk
	var firstChunkTimer *time.Timer
	if firstChunkTimeout > 0 {
		firstChunkTimer = time.AfterFunc(firstChunkTimeout, opCancel)
		defer func() {
			if firstChunkTimer != nil {
				firstChunkTimer.Stop()
			}
		}()
	}

	chunkTimeout := cfg.timeout.Chunk
	var chunkTimer *time.Timer
	if chunkTimeout > 0 {
		defer func() {
			if chunkTimer != nil {
				chunkTimer.Stop()
			}
		}()
	}

	// Read from provider stream with context cancellation support.
loop:
	for {
		var part provider.StreamPart
		var ok bool
		select {
		case part, ok = <-streamResult.Stream:
			if !ok {
				break loop
			}
			if ctx.Err() != nil {
				break loop
			}
			if isSemanticOutputStreamPart(part) {
				if firstChunkTimer != nil {
					firstChunkTimer.Stop()
					firstChunkTimer = nil
				}
				if chunkTimeout > 0 {
					if chunkTimer == nil {
						chunkTimer = time.AfterFunc(chunkTimeout, opCancel)
					} else {
						chunkTimer.Reset(chunkTimeout)
					}
				}
			}
		case <-ctx.Done():
			break loop
		}

		if isSemanticOutputStreamPart(part) {
			hasOutput = true
		}

		switch part.Type {
		case provider.PartStreamStart:
			warnings = part.Warnings
			step.Warnings = warnings

		case provider.PartTextStart:
			responseTextIndex[part.ID] = len(step.responseContent)
			step.responseContent = append(step.responseContent, provider.ContentPart{
				Type:            provider.ContentPartTypeText,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
			streamPartID := reserveStreamPartID(usedTextPartIDs, part.ID, cfg)
			textPartIDs[part.ID] = streamPartID
			tsp := StreamTextStart{ID: streamPartID, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartTextDelta:
			streamPartID := mappedStreamPartID(textPartIDs, part.ID)
			if idx, ok := responseTextIndex[part.ID]; ok {
				step.responseContent[idx].Text += part.Delta
				if part.ProviderMetadata != nil {
					step.responseContent[idx].ProviderOptions = providerMetadataToOptions(part.ProviderMetadata)
				}
			}
			textBuilder.WriteString(part.Delta)

			if cfg.output != nil {
				if outputTextChunkID == "" {
					outputTextChunkID = streamPartID
				} else if streamPartID != outputTextChunkID {
					tsp := StreamTextDelta{ID: streamPartID, Text: part.Delta, ProviderMetadata: part.ProviderMetadata}
					r.emit(tsp)
					r.callOnChunk(cfg, tsp)
					continue
				}
				outputTextChunk.WriteString(part.Delta)
				if part.ProviderMetadata != nil {
					outputTextMeta = part.ProviderMetadata
				}
				if part.Delta == "" && part.ProviderMetadata != nil {
					tsp := StreamTextDelta{ID: streamPartID, ProviderMetadata: part.ProviderMetadata}
					r.emit(tsp)
					r.callOnChunk(cfg, tsp)
					continue
				}
				parsed := r.emitPartialOutput(cfg.output, textBuilder.String())
				if parsed != lastPublishedOutput && parsed != "" {
					lastPublishedOutput = parsed
					tsp := StreamTextDelta{ID: outputTextChunkID, Text: outputTextChunk.String(), ProviderMetadata: outputTextMeta}
					r.emit(tsp)
					r.callOnChunk(cfg, tsp)
					outputTextChunk.Reset()
				}
			} else {
				tsp := StreamTextDelta{ID: streamPartID, Text: part.Delta, ProviderMetadata: part.ProviderMetadata}
				r.emit(tsp)
				r.callOnChunk(cfg, tsp)
			}

		case provider.PartTextEnd:
			streamPartID := mappedStreamPartID(textPartIDs, part.ID)
			if cfg.output != nil && streamPartID == outputTextChunkID && outputTextChunk.Len() > 0 {
				tsp := StreamTextDelta{ID: outputTextChunkID, Text: outputTextChunk.String(), ProviderMetadata: outputTextMeta}
				r.emit(tsp)
				r.callOnChunk(cfg, tsp)
				outputTextChunk.Reset()
			}
			if idx, ok := responseTextIndex[part.ID]; ok && part.ProviderMetadata != nil {
				step.responseContent[idx].ProviderOptions = providerMetadataToOptions(part.ProviderMetadata)
			}
			tsp := StreamTextEnd{ID: streamPartID, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)
			delete(textPartIDs, part.ID)

		case provider.PartReasoningStart:
			responseReasoningIndex[part.ID] = len(step.responseContent)
			step.responseContent = append(step.responseContent, provider.ContentPart{
				Type:            provider.ContentPartTypeReasoning,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
			outputIndex := len(reasoningBlocks)
			reasoningBlocks = append(reasoningBlocks, ReasoningTextOutput{ProviderMetadata: part.ProviderMetadata})
			activeReasoning[part.ID] = &activeReasoningBlock{providerMetadata: part.ProviderMetadata, outputIndex: outputIndex}
			streamPartID := reserveStreamPartID(usedReasoningPartIDs, part.ID, cfg)
			reasoningPartIDs[part.ID] = streamPartID
			tsp := StreamReasoningStart{ID: streamPartID, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartReasoningDelta:
			streamPartID := mappedStreamPartID(reasoningPartIDs, part.ID)
			active := activeReasoning[part.ID]
			if active == nil {
				active = &activeReasoningBlock{outputIndex: len(reasoningBlocks)}
				reasoningBlocks = append(reasoningBlocks, ReasoningTextOutput{})
				activeReasoning[part.ID] = active
			}
			active.text.WriteString(part.Delta)
			if part.ProviderMetadata != nil {
				active.providerMetadata = part.ProviderMetadata
			}
			if idx, ok := responseReasoningIndex[part.ID]; ok {
				step.responseContent[idx].Text += part.Delta
				if active.providerMetadata != nil {
					step.responseContent[idx].ProviderOptions = providerMetadataToOptions(active.providerMetadata)
				}
			}
			tsp := StreamReasoningDelta{ID: streamPartID, Text: part.Delta, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartReasoningEnd:
			streamPartID := mappedStreamPartID(reasoningPartIDs, part.ID)
			active := activeReasoning[part.ID]
			if active == nil {
				active = &activeReasoningBlock{outputIndex: len(reasoningBlocks)}
				reasoningBlocks = append(reasoningBlocks, ReasoningTextOutput{})
			}
			if part.ProviderMetadata != nil {
				active.providerMetadata = part.ProviderMetadata
			}
			if idx, ok := responseReasoningIndex[part.ID]; ok {
				step.responseContent[idx].ProviderOptions = providerMetadataToOptions(active.providerMetadata)
			}
			reasoningBlocks[active.outputIndex] = ReasoningTextOutput{Text: active.text.String(), ProviderMetadata: active.providerMetadata}
			delete(activeReasoning, part.ID)
			tsp := StreamReasoningEnd{ID: streamPartID, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)
			delete(reasoningPartIDs, part.ID)

		case provider.PartToolInputStart:
			toolNameByID[part.ID] = part.ToolName
			if part.Title != "" {
				toolTitleByID[part.ID] = part.Title
			}
			tsp := StreamToolInputStart{
				ID: part.ID, ToolName: part.ToolName,
				ProviderExecuted: part.ProviderExecuted, Dynamic: isDynamic(part.ToolName, part.Dynamic, cfg.tools),
				Title: part.Title, ProviderMetadata: part.ProviderMetadata,
			}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)
			if t, ok := cfg.tools[part.ToolName]; ok && t.OnInputStart != nil {
				t.OnInputStart(ToolExecutionOptions{ToolCallID: part.ID, Context: stepContext})
			}

		case provider.PartToolInputDelta:
			toolName := part.ToolName
			if toolName == "" {
				toolName = toolNameByID[part.ID]
			}
			tsp := StreamToolInputDelta{ID: part.ID, Delta: part.Delta, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)
			if t, ok := cfg.tools[toolName]; ok && t.OnInputDelta != nil {
				t.OnInputDelta(part.Delta, ToolExecutionOptions{ToolCallID: part.ID, Context: stepContext})
			}

		case provider.PartToolInputEnd:
			tsp := StreamToolInputEnd{ID: part.ID, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartToolCall:
			r.handleToolCall(part, &step, cfg, toolTitleByID, stepContext)
			if len(step.ToolCalls) > 0 {
				tc := step.ToolCalls[len(step.ToolCalls)-1]
				input := tc.Input
				if tc.Invalid && !isJSONObject(input) {
					input = json.RawMessage(`{}`)
				}
				step.responseContent = append(step.responseContent, provider.ContentPart{
					Type:             provider.ContentPartTypeToolCall,
					ToolCallID:       tc.ToolCallID,
					ToolName:         tc.ToolName,
					Input:            input,
					ProviderExecuted: tc.ProviderExecuted,
					ProviderOptions:  providerMetadataToOptions(tc.ProviderMetadata),
				})
			}

		case provider.PartToolResult:
			if err := r.handleToolResult(part, &step, cfg); err != nil {
				return step, false, false, hasOutput, err
			}
			preliminary := part.Preliminary != nil && *part.Preliminary
			if !preliminary && len(step.ToolResults) > 0 {
				tr := step.ToolResults[len(step.ToolResults)-1]
				if tr.ProviderExecuted && !tr.Preliminary {
					step.responseContent = append(step.responseContent, provider.ContentPart{
						Type:             provider.ContentPartTypeToolResult,
						ToolCallID:       tr.ToolCallID,
						ToolName:         tr.ToolName,
						Output:           toolResultOutput(tr),
						ProviderExecuted: true,
						ProviderOptions:  providerMetadataToOptions(tr.ProviderMetadata),
					})
				}
			}

		case provider.PartSource:
			if part.Source != nil {
				src := Source{
					SourceType:       part.Source.SourceType,
					ID:               part.Source.ID,
					URL:              part.Source.URL,
					Title:            part.Source.Title,
					MediaType:        part.Source.MediaType,
					Filename:         part.Source.Filename,
					ProviderMetadata: part.Source.ProviderMetadata,
				}
				step.Sources = append(step.Sources, src)
				step.responseContent = append(step.responseContent, provider.ContentPart{
					Type:            provider.ContentPartTypeSource,
					SourceType:      src.SourceType,
					ID:              src.ID,
					URL:             src.URL,
					Title:           src.Title,
					MediaType:       src.MediaType,
					Filename:        src.Filename,
					ProviderOptions: providerMetadataToOptions(src.ProviderMetadata),
				})
				tsp := StreamSource{Source: src}
				r.emit(tsp)
				r.callOnChunk(cfg, tsp)
			}

		case provider.PartFile:
			gf := generatedFileFromStreamData(part.Data, part.MediaType)
			step.Files = append(step.Files, gf)
			step.responseContent = append(step.responseContent, provider.ContentPart{
				Type:            provider.ContentPartTypeFile,
				Data:            generatedFileData(gf),
				MediaType:       part.MediaType,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
			tsp := StreamFile{File: gf, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartReasoningFile:
			gf := generatedFileFromStreamData(part.Data, part.MediaType)
			reasoningBlocks = append(reasoningBlocks, ReasoningFileOutput{File: gf, ProviderMetadata: part.ProviderMetadata})
			step.responseContent = append(step.responseContent, provider.ContentPart{
				Type:            provider.ContentPartTypeReasoningFile,
				Data:            generatedFileData(gf),
				MediaType:       part.MediaType,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
			tsp := StreamReasoningFile{File: gf, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartResponseMeta:
			responseMeta = provider.ResponseMetadata{
				ID:        part.ResponseID,
				ModelID:   part.ModelID,
				Provider:  part.Provider,
				Timestamp: part.Timestamp,
			}
			responseHeaders = part.ResponseHeaders

		case provider.PartFinish:
			completed = true
			terminated = true
			if part.Usage != nil {
				step.Usage = *part.Usage
			}
			if part.FinishReason != nil {
				step.FinishReason = *part.FinishReason
			}
			step.ProviderMetadata = part.ProviderMetadata

		case provider.PartRaw:
			tsp := StreamRaw{RawValue: part.RawValue}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartToolApprovalRequest:
			// Providers only emit PartToolApprovalRequest for provider-executed
			// tool calls. We default ProviderExecuted to true and override from
			// the matching tool call (which should also be provider-executed)
			// so the captured ToolApprovalRequest mirrors the originating call.
			req := ToolApprovalRequest{
				ApprovalID:       part.ApprovalID,
				ToolCallID:       part.ToolCallID,
				ToolName:         part.ToolName,
				Signature:        part.Signature,
				Reason:           part.Reason,
				ProviderExecuted: true,
				ProviderMetadata: part.ProviderMetadata,
			}
			for _, tc := range step.ToolCalls {
				if tc.ToolCallID == part.ToolCallID {
					req.ToolName = tc.ToolName
					req.Input = tc.Input
					req.Dynamic = tc.Dynamic
					req.Title = tc.Title
					req.ProviderExecuted = tc.ProviderExecuted
					break
				}
			}
			if req.Signature == "" {
				if err := maybeSignToolApproval(cfg.toolApprovalSecret, &req); err != nil {
					return step, false, false, hasOutput, err
				}
			}
			step.ToolApprovalRequests = append(step.ToolApprovalRequests, req)
			step.responseContent = append(step.responseContent, provider.ContentPart{
				Type:            provider.ContentPartTypeToolApprovalRequest,
				ApprovalID:      req.ApprovalID,
				ToolCallID:      req.ToolCallID,
				ToolName:        req.ToolName,
				Signature:       req.Signature,
				Reason:          req.Reason,
				IsAutomatic:     req.IsAutomatic,
				ProviderOptions: providerMetadataToOptions(req.ProviderMetadata),
			})
			tsp := StreamToolApprovalRequest(req)
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartCustom:
			step.responseContent = append(step.responseContent, provider.ContentPart{
				Type:            provider.ContentPartTypeCustom,
				Kind:            part.Kind,
				ProviderOptions: providerMetadataToOptions(part.ProviderMetadata),
			})
			tsp := StreamCustom{Kind: part.Kind, ProviderMetadata: part.ProviderMetadata}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)

		case provider.PartError:
			terminated = true
			// Synthesize a non-retryable APICallError when a producer emits
			// a PartError without a populated APICallError. Wire boundaries
			// MUST always carry an *APICallError so the orchestration layer
			// can surface the error to consumers; a bare PartError means
			// "something failed but the producer did not specify what".
			partErr := part.APICallError
			if partErr == nil {
				partErr = provider.NewAPICallError(provider.APICallErrorOptions{
					Message: "PartError received without APICallError details (provider bug or wire decoding issue)",
				})
			}
			var partErrAsError error = partErr
			r.mu.Lock()
			if r.firstError == nil {
				r.firstError = partErrAsError
			}
			r.mu.Unlock()
			tsp := StreamError{Error: partErrAsError}
			r.emit(tsp)
			r.callOnChunk(cfg, tsp)
			callOnError(cfg.onError, partErrAsError)
		}
	}

	partialCompleted := !terminated && ctx.Err() == nil && hasOutput
	if cfg.output != nil && outputTextChunk.Len() > 0 && (completed || partialCompleted) {
		tsp := StreamTextDelta{ID: outputTextChunkID, Text: outputTextChunk.String(), ProviderMetadata: outputTextMeta}
		r.emit(tsp)
		r.callOnChunk(cfg, tsp)
	}

	step.Text = textBuilder.String()
	step.Reasoning = reasoningBlocks
	for _, output := range reasoningBlocks {
		if text, ok := output.(ReasoningTextOutput); ok {
			step.ReasoningText += text.Text
		}
	}

	if responseMeta.Timestamp.IsZero() {
		responseMeta.Timestamp = time.Now()
	}
	step.Response = ResponseMetadata{
		ResponseMetadata: responseMeta,
		Headers:          responseHeaders,
	}
	if streamResult.Request != nil {
		step.Request = *streamResult.Request
	}

	if partialCompleted {
		step.FinishReason = provider.FinishReason{Unified: provider.FinishReasonOther}
	}
	if terminated && !completed {
		step.FinishReason = provider.FinishReason{Unified: provider.FinishReasonError}
		step.Content = buildContent(step)
		step.Response.Messages = ToResponseMessages(buildResponseContent(step))
		r.emit(StreamFinishStep{
			Response:         &responseMeta,
			Usage:            &step.Usage,
			FinishReason:     step.FinishReason,
			ProviderMetadata: step.ProviderMetadata,
		})
	}
	if completed || partialCompleted {
		if isToolExecutionAllowedFinishReason(step.FinishReason.Unified) {
			if err := r.executeTools(ctx, &step, cfg, stepContext, currentMsgs); err != nil {
				return step, false, false, hasOutput, err
			}
		}
		step.Content = buildContent(step)
		// Populate Response.Messages with the next-call message tail
		// (mirrors upstream result.response.messages). The same ordering
		// is used by appendToolResults, so the orchestration loop and the
		// surfaced field stay in lockstep.
		step.Response.Messages = ToResponseMessages(buildResponseContent(step))
		r.emit(StreamFinishStep{
			Response:         &responseMeta,
			Usage:            &step.Usage,
			FinishReason:     step.FinishReason,
			ProviderMetadata: step.ProviderMetadata,
		})
	}

	return step, completed, terminated, hasOutput, nil
}

func isToolExecutionAllowedFinishReason(reason provider.UnifiedFinishReason) bool {
	return reason == provider.FinishReasonStop || reason == provider.FinishReasonToolCalls
}

func isSemanticOutputStreamPart(part provider.StreamPart) bool {
	switch part.Type {
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolInputDelta:
		return part.Delta != ""
	case provider.PartFile, provider.PartReasoningFile, provider.PartToolCall:
		return true
	default:
		return false
	}
}

func (r *StreamTextResult) handleToolCall(
	part provider.StreamPart,
	step *StepResult,
	cfg *streamConfig,
	toolTitleByID map[string]string,
	stepContext any,
) {
	var parsedInput json.RawMessage
	if strings.TrimSpace(part.Input) == "" {
		parsedInput = json.RawMessage(`{}`)
	} else {
		if !json.Valid([]byte(part.Input)) {
			input, _ := json.Marshal(string(part.Input))
			r.rejectToolCall(part, json.RawMessage(input), step, cfg, toolTitleByID, fmt.Errorf("invalid JSON input for tool %q", part.ToolName))
			return
		}
		parsedInput = json.RawMessage(part.Input)
	}

	if t, ok := cfg.tools[part.ToolName]; ok {
		if t.InputSchema.Compiled() != nil {
			if err := t.InputSchema.Validate(parsedInput); err != nil {
				r.rejectToolCall(part, parsedInput, step, cfg, toolTitleByID, fmt.Errorf("invalid input for tool %s: %w", part.ToolName, err))
				return
			}
		}
		if t.ValidateInput != nil {
			if valErr := t.ValidateInput(parsedInput); valErr != nil {
				r.rejectToolCall(part, parsedInput, step, cfg, toolTitleByID, fmt.Errorf("invalid input for tool %s: %w", part.ToolName, valErr))
				return
			}
		}
	}

	_, toolExists := cfg.tools[part.ToolName]
	if !toolExists && !part.ProviderExecuted {
		availableTools := make([]string, 0, len(cfg.tools))
		for name := range cfg.tools {
			availableTools = append(availableTools, name)
		}
		sort.Strings(availableTools)
		// Prefix matches upstream `NoSuchToolError`'s default error message
		// format in @ai-sdk/provider. Keeping it byte-identical lets
		// conformance fixtures compare without per-provider patching.
		errMsg := fmt.Sprintf("AI_NoSuchToolError: Model tried to call unavailable tool '%s'. Available tools: %s.", part.ToolName, strings.Join(availableTools, ", "))
		r.rejectToolCall(part, parsedInput, step, cfg, toolTitleByID, errors.New(errMsg))
		return
	}

	title := part.Title
	if title == "" {
		title = toolTitleByID[part.ToolCallID]
	}

	dynamic := isDynamic(part.ToolName, part.Dynamic, cfg.tools)

	tc := ToolCall{
		ToolCallID:       part.ToolCallID,
		ToolName:         part.ToolName,
		Input:            parsedInput,
		ProviderExecuted: part.ProviderExecuted,
		Dynamic:          dynamic,
		Title:            title,
		ProviderMetadata: part.ProviderMetadata,
	}
	step.ToolCalls = append(step.ToolCalls, tc)

	tsp := StreamToolCall{
		ToolCallID: part.ToolCallID, ToolName: part.ToolName,
		Input: parsedInput, ProviderExecuted: part.ProviderExecuted,
		Dynamic: dynamic, Title: title, ProviderMetadata: part.ProviderMetadata,
	}
	r.emit(tsp)
	r.callOnChunk(cfg, tsp)

	if t, ok := cfg.tools[part.ToolName]; ok && t.OnInputAvailable != nil {
		t.OnInputAvailable(parsedInput, ToolExecutionOptions{ToolCallID: part.ToolCallID, Context: stepContext})
	}
}

func (r *StreamTextResult) handleToolResult(
	part provider.StreamPart,
	step *StepResult,
	cfg *streamConfig,
) error {
	preliminary := part.Preliminary != nil && *part.Preliminary
	dynamic := part.Dynamic
	var input json.RawMessage
	for _, toolCall := range step.ToolCalls {
		if toolCall.ToolCallID == part.ToolCallID {
			input = toolCall.Input
			break
		}
	}

	tr := ToolResult{
		ToolCallID:       part.ToolCallID,
		ToolName:         part.ToolName,
		Input:            input,
		Output:           part.Result,
		IsError:          part.IsError,
		ProviderExecuted: true,
		Dynamic:          dynamic,
		Preliminary:      preliminary,
		ProviderMetadata: part.ProviderMetadata,
	}
	if part.IsError {
		tr.Error = toolResultError(part.Result)
		if !preliminary {
			step.ToolResults = append(step.ToolResults, tr)
		}
		tsp := StreamToolError{
			ToolCallID: part.ToolCallID, ToolName: part.ToolName,
			Input: input, Error: tr.Error, ProviderExecuted: true,
			Dynamic: dynamic, ProviderMetadata: part.ProviderMetadata,
		}
		r.emit(tsp)
		r.callOnChunk(cfg, tsp)
		return nil
	}

	tsp := StreamToolResult{
		ToolCallID: part.ToolCallID, ToolName: part.ToolName,
		Input: input, Output: part.Result, ProviderExecuted: true,
		Dynamic: dynamic, ProviderMetadata: part.ProviderMetadata,
	}
	r.emit(tsp)
	r.callOnChunk(cfg, tsp)
	if preliminary {
		return nil
	}

	if tool, ok := cfg.tools[part.ToolName]; ok && tool.ToModelOutput != nil {
		modelOutput, err := tool.ToModelOutput(ToolOutputContext{
			ToolCallID: part.ToolCallID,
			Input:      input,
			Output:     part.Result,
		})
		if err != nil {
			return fmt.Errorf("converting provider-executed tool result: %w", err)
		}
		tr.ModelOutput = modelOutput
	}
	step.ToolResults = append(step.ToolResults, tr)
	return nil
}

// toolExecOutcome holds the result of a single tool execution, including
// the stream event to emit. Events are collected during concurrent execution
// and emitted in declaration order afterwards to match the upstream TS SDK.
type toolExecOutcome struct {
	result ToolResult
	event  TextStreamPart // StreamToolResult or StreamToolError
}

// rejectToolCall records an invalid tool call and emits its call/error events.
func (r *StreamTextResult) rejectToolCall(
	part provider.StreamPart,
	input json.RawMessage,
	step *StepResult,
	cfg *streamConfig,
	toolTitleByID map[string]string,
	err error,
) {
	title := part.Title
	if title == "" {
		title = toolTitleByID[part.ToolCallID]
	}
	dynamicTrue := true
	uiDynamic := isDynamic(part.ToolName, &dynamicTrue, cfg.tools)

	tc := ToolCall{
		ToolCallID:       part.ToolCallID,
		ToolName:         part.ToolName,
		Input:            input,
		Invalid:          true,
		Error:            err,
		ProviderExecuted: part.ProviderExecuted,
		Dynamic:          &dynamicTrue,
		Title:            title,
		ProviderMetadata: part.ProviderMetadata,
	}
	step.ToolCalls = append(step.ToolCalls, tc)
	toolCallPart := StreamToolCall{
		ToolCallID:       part.ToolCallID,
		ToolName:         part.ToolName,
		Input:            input,
		Invalid:          true,
		Error:            err,
		ProviderExecuted: part.ProviderExecuted,
		Dynamic:          &dynamicTrue,
		Title:            title,
		ProviderMetadata: part.ProviderMetadata,
		uiDynamic:        uiDynamic,
		useUIDynamic:     true,
	}
	r.emit(toolCallPart)
	r.callOnChunk(cfg, toolCallPart)
	if part.ProviderExecuted {
		return
	}

	step.ToolResults = append(step.ToolResults, ToolResult{
		ToolCallID: part.ToolCallID,
		ToolName:   part.ToolName,
		Input:      input,
		IsError:    true,
		Error:      err,
		Dynamic:    &dynamicTrue,
		Title:      title,
		ModelOutput: &provider.ToolResultOutput{
			Type: provider.ToolOutputErrorText,
			Text: err.Error(),
		},
	})

	toolErrorPart := StreamToolError{
		ToolCallID:   part.ToolCallID,
		ToolName:     part.ToolName,
		Input:        input,
		Error:        err,
		Dynamic:      &dynamicTrue,
		Title:        title,
		uiDynamic:    uiDynamic,
		useUIDynamic: true,
	}
	r.emit(toolErrorPart)
	r.callOnChunk(cfg, toolErrorPart)
}

// executeTools runs after PartFinish and processes every tool call recorded
// during the stream loop. Compared to upstream
// create-execute-tools-transformation.ts, which emits each
// tool-approval-request/response immediately after its tool-call chunk, Go
// batches all approval/execution work here so the stream-read loop stays free
// of tool I/O. This produces a different in-step ordering on the wire (all
// tool-call chunks first, then all approval/result chunks) which is locked by
// TestStreamTextToolApproval_MixedBlockedAndUnblockedTools. Adjacency is not
// part of the upstream UI protocol contract, so consumers that care only about
// chunk identity (the documented contract) remain compatible.
func (r *StreamTextResult) executeTools(
	ctx context.Context,
	step *StepResult,
	cfg *streamConfig,
	stepContext any,
	currentMsgs []provider.Message,
) error {
	type executableTool struct {
		index int
		tc    ToolCall
		tool  Tool
	}

	var executable []executableTool
	for _, tc := range step.ToolCalls {
		if tc.Invalid {
			continue
		}
		tool, ok := cfg.tools[tc.ToolName]
		if !ok {
			continue
		}
		execOpts := ToolExecutionOptions{
			ToolCallID: tc.ToolCallID,
			Messages:   currentMsgs,
			Context:    stepContext,
		}
		decision, err := resolveToolApprovalDecision(cfg, tool, tc, execOpts)
		if err != nil {
			return err
		}
		switch decision.Status {
		case ToolApprovalNotApplicable:
			// execute normally below
		case ToolApprovalUserApproval:
			approvalID := generateConfigID(cfg)
			req := newToolApprovalRequest(approvalID, tc, decision.Reason, false)
			if err := maybeSignToolApproval(cfg.toolApprovalSecret, &req); err != nil {
				return err
			}
			step.ToolApprovalRequests = append(step.ToolApprovalRequests, req)
			r.emitToolApprovalRequest(cfg, req)
			continue
		case ToolApprovalApproved:
			approvalID := generateConfigID(cfg)
			req := newToolApprovalRequest(approvalID, tc, decision.Reason, true)
			if err := maybeSignToolApproval(cfg.toolApprovalSecret, &req); err != nil {
				return err
			}
			resp := newToolApprovalResponse(approvalID, tc, true, decision.Reason)
			step.ToolApprovalRequests = append(step.ToolApprovalRequests, req)
			step.ToolApprovalResponses = append(step.ToolApprovalResponses, resp)
			r.emitToolApprovalRequest(cfg, req)
			r.emitToolApprovalResponse(cfg, resp)
		case ToolApprovalDenied:
			approvalID := generateConfigID(cfg)
			req := newToolApprovalRequest(approvalID, tc, decision.Reason, true)
			if err := maybeSignToolApproval(cfg.toolApprovalSecret, &req); err != nil {
				return err
			}
			resp := newToolApprovalResponse(approvalID, tc, false, decision.Reason)
			step.ToolApprovalRequests = append(step.ToolApprovalRequests, req)
			step.ToolApprovalResponses = append(step.ToolApprovalResponses, resp)
			// Auto-deny stores the synthetic execution-denied result on the
			// step (so appendToolResults can surface it in the next prompt and
			// Content() consumers see it), but does NOT stream a
			// tool-output-denied event. Upstream
			// create-execute-tools-transformation.ts only emits the request +
			// response chunks for in-step auto-deny; tool-output-denied is
			// reserved for the resume-from-prior-message flow in
			// resolveToolApprovals.
			if !tc.ProviderExecuted {
				step.ToolResults = append(step.ToolResults, ToolResult{ToolCallID: tc.ToolCallID, ToolName: tc.ToolName, Input: tc.Input, ModelOutput: &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: decision.Reason}, Dynamic: tc.Dynamic, Title: tc.Title, ProviderMetadata: tc.ProviderMetadata})
			}
			r.emitToolApprovalRequest(cfg, req)
			r.emitToolApprovalResponse(cfg, resp)
			continue
		default:
			return fmt.Errorf("aisdk: unsupported tool approval status %q", decision.Status)
		}
		if tc.ProviderExecuted || tool.Execute == nil {
			continue
		}
		executable = append(executable, executableTool{
			index: len(executable),
			tc:    tc,
			tool:  tool,
		})
	}

	if len(executable) == 0 {
		return nil
	}

	outcomes := make([]toolExecOutcome, len(executable))

	var wg sync.WaitGroup
	wg.Add(len(executable))

	for _, et := range executable {
		go func(et executableTool) {
			defer wg.Done()
			r.executeSingleTool(ctx, step, cfg, et.tc, et.tool, stepContext, currentMsgs, outcomes, et.index)
		}(et)
	}

	wg.Wait()

	// Emit events and collect results in declaration order so that the
	// stream output is deterministic regardless of goroutine scheduling.
	for _, out := range outcomes {
		step.ToolResults = append(step.ToolResults, out.result)
		if out.event != nil {
			r.emit(out.event)
			r.callOnChunk(cfg, out.event)
		}
	}
	return nil
}

func newToolApprovalRequest(approvalID string, tc ToolCall, reason string, isAutomatic bool) ToolApprovalRequest {
	return ToolApprovalRequest{
		ApprovalID:       approvalID,
		ToolCallID:       tc.ToolCallID,
		ToolName:         tc.ToolName,
		Input:            tc.Input,
		Reason:           reason,
		ProviderExecuted: tc.ProviderExecuted,
		Dynamic:          tc.Dynamic,
		Title:            tc.Title,
		IsAutomatic:      isAutomatic,
		ProviderMetadata: tc.ProviderMetadata,
	}
}

func newToolApprovalResponse(approvalID string, tc ToolCall, approved bool, reason string) ToolApprovalResponse {
	return ToolApprovalResponse{
		ApprovalID:       approvalID,
		ToolCallID:       tc.ToolCallID,
		ToolName:         tc.ToolName,
		Approved:         approved,
		Reason:           reason,
		ProviderExecuted: tc.ProviderExecuted,
		Dynamic:          tc.Dynamic,
		Title:            tc.Title,
		ProviderMetadata: tc.ProviderMetadata,
	}
}

func (r *StreamTextResult) emitToolApprovalRequest(cfg *streamConfig, req ToolApprovalRequest) {
	tsp := StreamToolApprovalRequest(req)
	r.emit(tsp)
	r.callOnChunk(cfg, tsp)
}

func (r *StreamTextResult) emitToolApprovalResponse(cfg *streamConfig, resp ToolApprovalResponse) {
	tsp := StreamToolApprovalResponse(resp)
	r.emit(tsp)
	r.callOnChunk(cfg, tsp)
}

func (r *StreamTextResult) executeSingleTool(
	ctx context.Context,
	step *StepResult,
	cfg *streamConfig,
	tc ToolCall,
	tool Tool,
	stepContext any,
	currentMsgs []provider.Message,
	outcomes []toolExecOutcome,
	index int,
) {
	if cfg.onToolCallStart != nil {
		cfg.onToolCallStart(OnToolCallStartState{
			StepNumber: step.StepNumber,
			Model:      step.Model,
			ToolCall:   tc,
		})
	}

	start := time.Now()
	output, err := tool.Execute(ctx, tc.Input, ToolExecutionOptions{
		ToolCallID: tc.ToolCallID,
		Messages:   currentMsgs,
		Context:    stepContext,
	})
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		outcomes[index] = toolExecOutcome{
			result: ToolResult{
				ToolCallID:       tc.ToolCallID,
				ToolName:         tc.ToolName,
				Input:            tc.Input,
				IsError:          true,
				Error:            err,
				ProviderMetadata: tc.ProviderMetadata,
				Dynamic:          tc.Dynamic,
				Title:            tc.Title,
				ModelOutput: &provider.ToolResultOutput{
					Type: provider.ToolOutputErrorText,
					Text: err.Error(),
				},
			},
			event: StreamToolError{
				ToolCallID: tc.ToolCallID, ToolName: tc.ToolName,
				Input: tc.Input, Error: err, Dynamic: tc.Dynamic,
				Title: tc.Title, ProviderMetadata: tc.ProviderMetadata,
			},
		}

		if cfg.onToolCallFinish != nil {
			cfg.onToolCallFinish(OnToolCallFinishState{
				StepNumber: step.StepNumber,
				Model:      step.Model,
				ToolCall:   tc,
				DurationMs: durationMs,
				Success:    false,
				Error:      err,
			})
		}
		return
	}

	var modelOutput *provider.ToolResultOutput
	if tool.ToModelOutput != nil {
		mo, moErr := tool.ToModelOutput(ToolOutputContext{
			ToolCallID: tc.ToolCallID,
			Input:      tc.Input,
			Output:     output,
		})
		if moErr != nil {
			outcomes[index] = toolExecOutcome{
				result: ToolResult{
					ToolCallID:       tc.ToolCallID,
					ToolName:         tc.ToolName,
					Input:            tc.Input,
					IsError:          true,
					Error:            moErr,
					ProviderMetadata: tc.ProviderMetadata,
					Dynamic:          tc.Dynamic,
					Title:            tc.Title,
					ModelOutput: &provider.ToolResultOutput{
						Type: provider.ToolOutputErrorText,
						Text: moErr.Error(),
					},
				},
				event: StreamToolError{
					ToolCallID: tc.ToolCallID, ToolName: tc.ToolName,
					Input: tc.Input, Error: moErr, Dynamic: tc.Dynamic,
					Title: tc.Title, ProviderMetadata: tc.ProviderMetadata,
				},
			}

			if cfg.onToolCallFinish != nil {
				cfg.onToolCallFinish(OnToolCallFinishState{
					StepNumber: step.StepNumber,
					Model:      step.Model,
					ToolCall:   tc,
					DurationMs: durationMs,
					Success:    false,
					Error:      moErr,
				})
			}
			return
		}
		modelOutput = mo
	}

	outcomes[index] = toolExecOutcome{
		result: ToolResult{
			ToolCallID:       tc.ToolCallID,
			ToolName:         tc.ToolName,
			Input:            tc.Input,
			Output:           output,
			ModelOutput:      modelOutput,
			ProviderMetadata: tc.ProviderMetadata,
			Dynamic:          tc.Dynamic,
			Title:            tc.Title,
		},
		event: StreamToolResult{
			ToolCallID: tc.ToolCallID, ToolName: tc.ToolName,
			Input: tc.Input, Output: output, ProviderMetadata: tc.ProviderMetadata,
		},
	}

	if cfg.onToolCallFinish != nil {
		cfg.onToolCallFinish(OnToolCallFinishState{
			StepNumber: step.StepNumber,
			Model:      step.Model,
			ToolCall:   tc,
			DurationMs: durationMs,
			Success:    true,
			Output:     output,
		})
	}
}

func (r *StreamTextResult) emit(part TextStreamPart) {
	r.fullStream <- part
}

func (r *StreamTextResult) abort(ctx context.Context, cfg *streamConfig) {
	if cfg.onAbort != nil {
		cfg.onAbort(OnAbortState{Steps: r.steps})
	}
	part := StreamAbort{}
	if cause := context.Cause(ctx); cause != nil {
		r.mu.Lock()
		r.abortCause = cause
		r.mu.Unlock()
		part.Reason = cause.Error()
	}
	r.emit(part)
	r.callOnChunk(cfg, part)
}

func (r *StreamTextResult) emitPartialOutput(out Output, text string) string {
	v, ok := out.ParsePartial(text)
	if !ok {
		return ""
	}
	if elements, isSlice := v.([]json.RawMessage); isSlice {
		for i := r.emittedElementCount; i < len(elements); i++ {
			r.elementStream.send(elements[i])
		}
		r.emittedElementCount = len(elements)
	}

	var data json.RawMessage
	if raw, isRaw := v.(json.RawMessage); isRaw {
		data = raw
	} else {
		var err error
		data, err = json.Marshal(v)
		if err != nil {
			return ""
		}
	}
	canonical, err := canonicalPartialJSON(data)
	if err != nil || canonical == r.lastPartialJSON {
		return ""
	}
	r.lastPartialJSON = canonical
	r.partialOutputStream.send(data)
	return canonical
}

func canonicalPartialJSON(data json.RawMessage) (string, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func (r *StreamTextResult) emitError(err error, onError func(error)) {
	r.emitStreamError(err, onError)
	r.emit(StreamFinish{})
}

func (r *StreamTextResult) emitStreamError(err error, onError func(error)) {
	r.mu.Lock()
	if r.firstError == nil {
		r.firstError = err
	}
	r.mu.Unlock()
	r.emit(StreamError{Error: err})
	callOnError(onError, err)
}

func callOnError(onError func(error), err error) {
	if onError == nil {
		return
	}
	defer func() { _ = recover() }()
	onError(err)
}

type collectedToolApproval struct {
	request               provider.ContentPart
	response              provider.ContentPart
	toolCall              provider.ContentPart
	hasExistingToolResult bool
}

type invalidToolApproval struct {
	collectedToolApproval
	err error
}

func (r *StreamTextResult) resolveToolApprovals(ctx context.Context, cfg *streamConfig, msgs []provider.Message) ([]provider.Message, error) {
	approved, denied, err := collectToolApprovals(msgs)
	if err != nil {
		return nil, err
	}
	if len(approved) == 0 && len(denied) == 0 {
		return msgs, nil
	}

	// cloneMessages is shallow: only the slice header is copied. This is safe
	// because resolveToolApprovals only appends a new tool message and never
	// mutates existing message Content slices.
	resultMsgs := cloneMessages(msgs)

	localApproved := make([]collectedToolApproval, 0, len(approved))
	for _, approval := range approved {
		if !approval.toolCall.ProviderExecuted {
			localApproved = append(localApproved, approval)
		}
	}
	approved, demoted, invalid, err := validateApprovedToolApprovals(cfg, msgs, localApproved)
	if err != nil {
		return nil, err
	}
	denied = append(denied, demoted...)
	for _, approval := range denied {
		deniedEvent := StreamToolOutputDenied{ToolCallID: approval.toolCall.ToolCallID, ToolName: approval.toolCall.ToolName}
		r.emit(deniedEvent)
		r.callOnChunk(cfg, deniedEvent)
	}
	for _, approval := range invalid {
		toolCall := approval.toolCall
		dynamic := isDynamic(toolCall.ToolName, nil, cfg.tools)
		event := StreamToolError{
			ToolCallID:       toolCall.ToolCallID,
			ToolName:         toolCall.ToolName,
			Input:            toolCall.Input,
			Error:            approval.err,
			Dynamic:          dynamic,
			Title:            toolCall.Title,
			ProviderMetadata: optionsToProviderMetadata(toolCall.ProviderOptions),
		}
		r.emit(event)
		r.callOnChunk(cfg, event)
	}

	type executableApproval struct {
		index int
		tc    ToolCall
		tool  Tool
	}
	var executable []executableApproval
	for _, approval := range approved {
		if approval.toolCall.ProviderExecuted {
			continue
		}
		tool, ok := cfg.tools[approval.toolCall.ToolName]
		if !ok || tool.Execute == nil {
			return nil, &ToolNotExecutableError{ToolName: approval.toolCall.ToolName}
		}
		tc := ToolCall{
			ToolCallID:       approval.toolCall.ToolCallID,
			ToolName:         approval.toolCall.ToolName,
			Input:            approval.toolCall.Input,
			ProviderExecuted: approval.toolCall.ProviderExecuted,
			ProviderMetadata: optionsToProviderMetadata(approval.toolCall.ProviderOptions),
		}
		executable = append(executable, executableApproval{
			index: len(executable),
			tc:    tc,
			tool:  tool,
		})
	}

	// Resume tool execution mirrors upstream's Promise.all fan-out:
	// approved tools run concurrently, then events are emitted in declaration
	// order so the wire stream is deterministic. The synthetic StepResult uses
	// StepNumber 0 to signal "pre-step resume execution"; consumers that bucket
	// telemetry by step should treat it as a resume bucket, not as the first
	// model step (which uses StepNumber 1).
	outcomes := make([]toolExecOutcome, len(executable))
	if len(executable) > 0 {
		var wg sync.WaitGroup
		wg.Add(len(executable))
		resumeStep := &StepResult{StepNumber: 0, Model: StepModel{}}
		for _, et := range executable {
			go func(et executableApproval) {
				defer wg.Done()
				r.executeSingleTool(ctx, resumeStep, cfg, et.tc, et.tool, nil, msgs, outcomes, et.index)
			}(et)
		}
		wg.Wait()
	}

	approvedToolParts := make([]provider.ContentPart, 0, len(executable))
	for _, out := range outcomes {
		if out.event != nil {
			r.emit(out.event)
			r.callOnChunk(cfg, out.event)
		}
		approvedToolParts = append(approvedToolParts, provider.ContentPart{
			Type:       provider.ContentPartTypeToolResult,
			ToolCallID: out.result.ToolCallID,
			ToolName:   out.result.ToolName,
			Output:     toolResultOutput(out.result),
		})
	}

	invalidToolParts := make([]provider.ContentPart, 0, len(invalid))
	for _, approval := range invalid {
		invalidToolParts = append(invalidToolParts, provider.ContentPart{
			Type:       provider.ContentPartTypeToolResult,
			ToolCallID: approval.toolCall.ToolCallID,
			ToolName:   approval.toolCall.ToolName,
			Output: &provider.ToolResultOutput{
				Type: provider.ToolOutputErrorText,
				Text: approval.err.Error(),
			},
		})
	}

	deniedToolParts := make([]provider.ContentPart, 0, len(denied))
	for _, approval := range denied {
		if approval.toolCall.ProviderExecuted || approval.hasExistingToolResult {
			continue
		}
		deniedToolParts = append(deniedToolParts, provider.ContentPart{
			Type:       provider.ContentPartTypeToolResult,
			ToolCallID: approval.toolCall.ToolCallID,
			ToolName:   approval.toolCall.ToolName,
			Output: &provider.ToolResultOutput{
				Type:   provider.ToolOutputExecutionDenied,
				Reason: approval.response.Reason,
			},
		})
	}

	// Approved tool results go before invalid and synthetic execution-denied
	// results so the next-step prompt order matches upstream stream-text.ts.
	toolParts := append(approvedToolParts, invalidToolParts...)
	toolParts = append(toolParts, deniedToolParts...)
	if len(toolParts) > 0 {
		resultMsgs = append(resultMsgs, provider.NewToolMessage(toolParts...))
	}
	return resultMsgs, nil
}

func validateApprovedToolApprovals(cfg *streamConfig, msgs []provider.Message, approvals []collectedToolApproval) ([]collectedToolApproval, []collectedToolApproval, []invalidToolApproval, error) {
	approved := make([]collectedToolApproval, 0, len(approvals))
	var denied []collectedToolApproval
	var invalid []invalidToolApproval

	for _, approval := range approvals {
		if err := validateToolApprovalSignature(cfg.toolApprovalSecret, approval); err != nil {
			return nil, nil, nil, err
		}

		tool, ok := cfg.tools[approval.toolCall.ToolName]
		if ok && tool.Execute != nil {
			var validationErr error
			if tool.InputSchema.Compiled() != nil {
				if err := tool.InputSchema.Validate(approval.toolCall.Input); err != nil {
					validationErr = fmt.Errorf("invalid input for tool %s: %w", approval.toolCall.ToolName, err)
				}
			}
			if validationErr == nil && tool.ValidateInput != nil {
				if err := tool.ValidateInput(approval.toolCall.Input); err != nil {
					validationErr = fmt.Errorf("invalid input for tool %s: %w", approval.toolCall.ToolName, err)
				}
			}
			if validationErr != nil {
				invalid = append(invalid, invalidToolApproval{collectedToolApproval: approval, err: validationErr})
				continue
			}
		}

		if ok {
			decision, err := resolveToolApprovalDecision(cfg, tool, ToolCall{
				ToolCallID:       approval.toolCall.ToolCallID,
				ToolName:         approval.toolCall.ToolName,
				Input:            approval.toolCall.Input,
				ProviderExecuted: approval.toolCall.ProviderExecuted,
				ProviderMetadata: optionsToProviderMetadata(approval.toolCall.ProviderOptions),
			}, ToolExecutionOptions{
				ToolCallID: approval.toolCall.ToolCallID,
				Messages:   msgs,
			})
			if err != nil {
				return nil, nil, nil, err
			}
			if decision.Status == ToolApprovalDenied {
				approvedFalse := false
				approval.response.Approved = &approvedFalse
				if decision.Reason != "" {
					approval.response.Reason = decision.Reason
				}
				denied = append(denied, approval)
				continue
			}
		}

		approved = append(approved, approval)
	}

	return approved, denied, invalid, nil
}

func validateToolApprovalSignature(secret []byte, approval collectedToolApproval) error {
	if len(secret) == 0 {
		return nil
	}
	if approval.request.Signature == "" {
		return &InvalidToolApprovalSignatureError{
			ApprovalID: approval.request.ApprovalID,
			ToolCallID: approval.toolCall.ToolCallID,
			Reason:     "missing signature",
		}
	}
	valid, err := verifyToolApprovalSignature(secret, approval.request.Signature, approval.request.ApprovalID, approval.toolCall.ToolCallID, approval.toolCall.ToolName, approval.toolCall.Input)
	if err != nil {
		return err
	}
	if !valid {
		return &InvalidToolApprovalSignatureError{
			ApprovalID: approval.request.ApprovalID,
			ToolCallID: approval.toolCall.ToolCallID,
			Reason:     "invalid signature",
		}
	}
	return nil
}

func collectToolApprovals(msgs []provider.Message) ([]collectedToolApproval, []collectedToolApproval, error) {
	if len(msgs) == 0 || msgs[len(msgs)-1].Role != provider.RoleTool {
		return nil, nil, nil
	}
	last := msgs[len(msgs)-1]
	toolCallsByID := make(map[string]provider.ContentPart)
	requestsByApprovalID := make(map[string]provider.ContentPart)
	for _, msg := range msgs {
		if msg.Role != provider.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			switch part.Type {
			case provider.ContentPartTypeToolCall:
				toolCallsByID[part.ToolCallID] = part
			case provider.ContentPartTypeToolApprovalRequest:
				requestsByApprovalID[part.ApprovalID] = part
			}
		}
	}

	// Dedup only against the last tool message, matching upstream
	// collect-tool-approvals.ts. If a caller resends an approval response after
	// a tool-result has been written into an earlier tool message but not the
	// most recent one, the tool will execute again. This is intentional: the
	// last tool message is the one being acted upon.
	existingResults := make(map[string]provider.ContentPart)
	for _, part := range last.Content {
		if part.Type == provider.ContentPartTypeToolResult {
			existingResults[part.ToolCallID] = part
		}
	}

	var approved []collectedToolApproval
	var denied []collectedToolApproval
	for _, response := range last.Content {
		if response.Type != provider.ContentPartTypeToolApprovalResponse {
			continue
		}
		if response.Approved == nil {
			return nil, nil, &InvalidToolApprovalError{ApprovalID: response.ApprovalID, Reason: "approved is required"}
		}
		request, ok := requestsByApprovalID[response.ApprovalID]
		if !ok {
			return nil, nil, &InvalidToolApprovalError{ApprovalID: response.ApprovalID}
		}
		existingResult, hasExistingResult := existingResults[request.ToolCallID]
		if hasExistingResult && (*response.Approved || existingResult.Output == nil || existingResult.Output.Type != provider.ToolOutputExecutionDenied) {
			continue
		}
		toolCall, ok := toolCallsByID[request.ToolCallID]
		if !ok {
			return nil, nil, &ToolCallNotFoundForApprovalError{
				ToolCallID: request.ToolCallID,
				ApprovalID: request.ApprovalID,
			}
		}
		approval := collectedToolApproval{
			request:               request,
			response:              response,
			toolCall:              toolCall,
			hasExistingToolResult: hasExistingResult,
		}
		if *response.Approved {
			approved = append(approved, approval)
		} else {
			denied = append(denied, approval)
		}
	}
	return approved, denied, nil
}

func cloneMessages(msgs []provider.Message) []provider.Message {
	result := make([]provider.Message, len(msgs))
	copy(result, msgs)
	return result
}

func sanitizePromptForProvider(msgs []provider.Message) ([]provider.Message, error) {
	approvalToolCallIDs := make(map[string]string)
	for _, msg := range msgs {
		if msg.Role != provider.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			if part.Type == provider.ContentPartTypeToolApprovalRequest {
				approvalToolCallIDs[part.ApprovalID] = part.ToolCallID
			}
		}
	}

	approvedToolCallIDs := make(map[string]struct{})
	for _, msg := range msgs {
		if msg.Role != provider.RoleTool {
			continue
		}
		for _, part := range msg.Content {
			if part.Type != provider.ContentPartTypeToolApprovalResponse {
				continue
			}
			if toolCallID := approvalToolCallIDs[part.ApprovalID]; toolCallID != "" {
				approvedToolCallIDs[toolCallID] = struct{}{}
			}
		}
	}

	result := make([]provider.Message, 0, len(msgs))
	for _, msg := range msgs {
		if msg.Role != provider.RoleAssistant && msg.Role != provider.RoleTool {
			result = append(result, msg)
			continue
		}
		filtered := msg
		filtered.Content = make([]provider.ContentPart, 0, len(msg.Content))
		for _, part := range msg.Content {
			if msg.Role == provider.RoleAssistant && part.Type == provider.ContentPartTypeToolApprovalRequest {
				continue
			}
			if msg.Role == provider.RoleTool && part.Type == provider.ContentPartTypeToolApprovalResponse && !part.ProviderExecuted {
				continue
			}
			filtered.Content = append(filtered.Content, part)
		}
		if msg.Role != provider.RoleTool || len(filtered.Content) > 0 {
			result = append(result, filtered)
		}
	}

	unresolved := make(map[string]struct{})
	unresolvedOrder := make([]string, 0)
	addUnresolved := func(toolCallID string) {
		if _, ok := unresolved[toolCallID]; ok {
			return
		}
		unresolved[toolCallID] = struct{}{}
		unresolvedOrder = append(unresolvedOrder, toolCallID)
	}
	removeUnresolved := func(toolCallID string) {
		if _, ok := unresolved[toolCallID]; !ok {
			return
		}
		delete(unresolved, toolCallID)
		for i, unresolvedToolCallID := range unresolvedOrder {
			if unresolvedToolCallID == toolCallID {
				unresolvedOrder = append(unresolvedOrder[:i], unresolvedOrder[i+1:]...)
				return
			}
		}
	}
	removeApproved := func() {
		for toolCallID := range approvedToolCallIDs {
			removeUnresolved(toolCallID)
		}
	}
	missingResultsError := func() error {
		if len(unresolvedOrder) == 0 {
			return nil
		}
		return &MissingToolResultsError{ToolCallIDs: append([]string(nil), unresolvedOrder...)}
	}

	for _, msg := range result {
		switch msg.Role {
		case provider.RoleAssistant:
			for _, part := range msg.Content {
				if part.Type == provider.ContentPartTypeToolCall && !part.ProviderExecuted {
					addUnresolved(part.ToolCallID)
				}
			}
		case provider.RoleTool:
			for _, part := range msg.Content {
				if part.Type == provider.ContentPartTypeToolResult {
					removeUnresolved(part.ToolCallID)
				}
			}
		case provider.RoleUser, provider.RoleSystem:
			removeApproved()
			if err := missingResultsError(); err != nil {
				return nil, err
			}
		}
	}

	removeApproved()
	if err := missingResultsError(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *StreamTextResult) callOnChunk(cfg *streamConfig, tsp TextStreamPart) {
	if cfg.onChunk == nil {
		return
	}
	defer func() { _ = recover() }()
	cfg.onChunk(OnChunkState{Chunk: tsp})
}

// Wait blocks until the stream completes.
func (r *StreamTextResult) Wait() {
	<-r.done
}

// Text returns the generated text from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) Text() string {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.Text
	}
	return ""
}

// Reasoning returns the reasoning output from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) Reasoning() []ReasoningOutput {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.Reasoning
	}
	return nil
}

// ToolCalls returns the tool calls from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) ToolCalls() []ToolCall {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.ToolCalls
	}
	return nil
}

// ToolResults returns the tool results from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) ToolResults() []ToolResult {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.ToolResults
	}
	return nil
}

// Files returns the generated files from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) Files() []GeneratedFile {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.Files
	}
	return nil
}

// Sources returns the sources from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) Sources() []Source {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.Sources
	}
	return nil
}

// FinishReason returns the finish reason from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) FinishReason() provider.FinishReason {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.FinishReason
	}
	return provider.FinishReason{}
}

// Usage returns the aggregate token usage across all steps.
// Blocks until the stream completes.
func (r *StreamTextResult) Usage() provider.Usage {
	return r.TotalUsage()
}

// TotalUsage returns the aggregate token usage across all steps.
// Blocks until the stream completes.
func (r *StreamTextResult) TotalUsage() provider.Usage {
	r.Wait()
	return r.totalUsage
}

// AggregateUsage returns the aggregate token usage across all steps.
// Blocks until the stream completes.
func (r *StreamTextResult) AggregateUsage() provider.Usage {
	return r.TotalUsage()
}

// Steps returns all step results.
// Blocks until the stream completes.
func (r *StreamTextResult) Steps() []StepResult {
	r.Wait()
	return r.steps
}

// Warnings returns all warnings accumulated across steps.
// Blocks until the stream completes.
func (r *StreamTextResult) Warnings() []provider.Warning {
	r.Wait()
	return r.allWarnings
}

// Content returns the structured content from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) Content() []ContentPart {
	r.Wait()
	if r.lastStep != nil {
		return r.lastStep.Content
	}
	return nil
}

// Response returns the response metadata from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) Response() ResponseMetadata {
	r.Wait()
	return r.lastResponse
}

// ProviderMetadata returns the provider metadata from the last step.
// Blocks until the stream completes.
func (r *StreamTextResult) ProviderMetadata() provider.ProviderMetadata {
	r.Wait()
	return r.provMeta
}

// OutputValue returns the parsed structured output value, or nil if no Output
// was specified or parsing failed. Blocks until the stream completes.
func (r *StreamTextResult) OutputValue() any {
	r.Wait()
	return r.outputValue
}

// OutputError returns the structured output validation error, or nil
// if validation succeeded or no Output was specified.
// Blocks until the stream completes.
func (r *StreamTextResult) OutputError() error {
	r.Wait()
	return r.outputErr
}

// PartialOutputStream returns a channel of partial JSON snapshots during
// streaming. Each emission is a json.RawMessage representing the partial parse
// of the accumulated text so far. Emissions are delivered exactly once and in
// order. The channel is closed after the stream finishes and all emissions are
// delivered. If no Output is set, returns a closed channel.
func (r *StreamTextResult) PartialOutputStream() <-chan json.RawMessage {
	return r.partialOutputStream.stream()
}

// ElementStream returns a channel of complete validated array elements
// during streaming. Only produces values when the Output is in array mode.
// Elements are delivered exactly once and in order. The channel is closed after
// the stream finishes and all elements are delivered.
func (r *StreamTextResult) ElementStream() <-chan json.RawMessage {
	return r.elementStream.stream()
}

// Err returns the first error encountered during streaming, or nil.
// Blocks until the stream completes.
func (r *StreamTextResult) Err() error {
	r.Wait()
	return r.firstError
}

func (r *StreamTextResult) abortError() error {
	r.Wait()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.abortCause
}

// Helpers

func filterProviderTools(tools []provider.Tool, activeNames []string) []provider.Tool {
	nameSet := make(map[string]bool, len(activeNames))
	for _, n := range activeNames {
		nameSet[n] = true
	}
	var filtered []provider.Tool
	for _, t := range tools {
		if t.Type != provider.ToolTypeFunction && t.Type != provider.ToolTypeProvider {
			continue
		}
		if nameSet[t.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func generateConfigID(cfg *streamConfig) string {
	if cfg != nil && cfg.generateID != nil {
		return cfg.generateID()
	}
	return GenerateID()
}

func reserveStreamPartID(used map[string]struct{}, providerID string, cfg *streamConfig) string {
	if _, exists := used[providerID]; !exists {
		used[providerID] = struct{}{}
		return providerID
	}

	generatedID := generateConfigID(cfg)
	uniqueID := generatedID
	for suffix := 1; ; suffix++ {
		if _, exists := used[uniqueID]; !exists {
			used[uniqueID] = struct{}{}
			return uniqueID
		}
		uniqueID = fmt.Sprintf("%s-%d", generatedID, suffix)
	}
}

func mappedStreamPartID(active map[string]string, providerID string) string {
	if id, ok := active[providerID]; ok {
		return id
	}
	return providerID
}

func isApprovalNeeded(tool Tool, input json.RawMessage, opts ToolExecutionOptions) (bool, error) {
	if tool.NeedsApproval == nil {
		return false, nil
	}
	if tool.NeedsApproval.Check != nil {
		return tool.NeedsApproval.Check(input, opts)
	}
	return tool.NeedsApproval.Required, nil
}

func resolveToolApprovalDecision(cfg *streamConfig, tool Tool, tc ToolCall, opts ToolExecutionOptions) (ToolApprovalDecision, error) {
	if cfg != nil {
		if cfg.toolApproval.generic != nil {
			decision, err := cfg.toolApproval.generic(ToolApprovalOptions{ToolCall: tc, Tools: cfg.tools, Messages: opts.Messages, Context: opts.Context})
			return normalizeToolApprovalDecision(decision), err
		}
		if cfg.toolApproval.tools != nil {
			if policy, ok := cfg.toolApproval.tools[tc.ToolName]; ok {
				if policy.Check != nil {
					decision, err := policy.Check(tc.Input, opts)
					return normalizeToolApprovalDecision(decision), err
				}
				return normalizeToolApprovalDecision(policy.Decision), nil
			}
		}
	}
	needed, err := isApprovalNeeded(tool, tc.Input, opts)
	if err != nil {
		return ToolApprovalDecision{}, err
	}
	if needed {
		return ToolApprovalDecision{Status: ToolApprovalUserApproval}, nil
	}
	return ToolApprovalDecision{Status: ToolApprovalNotApplicable}, nil
}

func normalizeToolApprovalDecision(decision ToolApprovalDecision) ToolApprovalDecision {
	if decision.Status == "" {
		decision.Status = ToolApprovalNotApplicable
	}
	return decision
}

func aggregateUsage(steps []StepResult) provider.Usage {
	var totalInput, totalOutput int
	for _, s := range steps {
		if s.Usage.InputTokens.Total != nil {
			totalInput += *s.Usage.InputTokens.Total
		}
		if s.Usage.OutputTokens.Total != nil {
			totalOutput += *s.Usage.OutputTokens.Total
		}
	}
	return provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: &totalInput},
		OutputTokens: provider.OutputTokenUsage{Total: &totalOutput},
	}
}

// appendToolResults appends the messages produced from the given step onto
// msgs, returning a new slice. It delegates the part-shape conversion to
// the public [ToResponseMessages] helper so that both the internal
// orchestration loop and external multi-step consumers share one
// implementation. Reasoning blocks (with their ProviderMetadata, including
// Anthropic's extended-thinking signature) are preserved across rounds.
func appendToolResults(msgs []provider.Message, step StepResult) []provider.Message {
	result := make([]provider.Message, len(msgs))
	copy(result, msgs)
	result = append(result, ToResponseMessages(buildResponseContent(step))...)
	return result
}

// buildResponseContent reconciles a step's outputs into the
// []provider.ContentPart input expected by [ToResponseMessages]. When the step
// has recorded provider content, that sequence is preserved and locally
// generated approvals, files, approval responses, and tool results are appended.
// Otherwise, manually constructed step state uses deterministic grouped order.
// Invalid tool-call inputs with non-object payloads are collapsed to {} so the
// model sees syntactically valid JSON on the next call.
func buildResponseContent(step StepResult) []provider.ContentPart {
	if len(step.responseContent) > 0 {
		existingApprovalRequests := make(map[string]bool, len(step.ToolApprovalRequests))
		for _, part := range step.responseContent {
			if part.Type == provider.ContentPartTypeToolApprovalRequest {
				existingApprovalRequests[part.ApprovalID] = true
			}
		}

		parts := make([]provider.ContentPart, 0, len(step.responseContent)+len(step.ToolApprovalRequests)+len(step.Files)+len(step.ToolApprovalResponses)+len(step.ToolResults))
		parts = append(parts, step.responseContent...)
		for _, ar := range step.ToolApprovalRequests {
			if existingApprovalRequests[ar.ApprovalID] {
				continue
			}
			parts = append(parts, provider.ContentPart{
				Type:            provider.ContentPartTypeToolApprovalRequest,
				ApprovalID:      ar.ApprovalID,
				ToolCallID:      ar.ToolCallID,
				ToolName:        ar.ToolName,
				Signature:       ar.Signature,
				Reason:          ar.Reason,
				IsAutomatic:     ar.IsAutomatic,
				ProviderOptions: providerMetadataToOptions(ar.ProviderMetadata),
			})
		}
		recordedFiles := 0
		for _, part := range step.responseContent {
			if part.Type == provider.ContentPartTypeFile {
				recordedFiles++
			}
		}
		for _, f := range step.Files[recordedFiles:] {
			parts = append(parts, provider.ContentPart{
				Type:      provider.ContentPartTypeFile,
				Data:      generatedFileData(f),
				MediaType: f.MediaType,
			})
		}
		for _, ar := range step.ToolApprovalResponses {
			approved := ar.Approved
			parts = append(parts, provider.ContentPart{
				Type:             provider.ContentPartTypeToolApprovalResponse,
				ApprovalID:       ar.ApprovalID,
				ToolCallID:       ar.ToolCallID,
				ToolName:         ar.ToolName,
				Approved:         &approved,
				Reason:           ar.Reason,
				ProviderExecuted: ar.ProviderExecuted,
				ProviderOptions:  providerMetadataToOptions(ar.ProviderMetadata),
			})
		}
		for _, tr := range step.ToolResults {
			if tr.ProviderExecuted || tr.Preliminary {
				continue
			}
			parts = append(parts, provider.ContentPart{
				Type:             provider.ContentPartTypeToolResult,
				ToolCallID:       tr.ToolCallID,
				ToolName:         tr.ToolName,
				Output:           toolResultOutput(tr),
				ProviderExecuted: false,
				ProviderOptions:  providerMetadataToOptions(tr.ProviderMetadata),
			})
		}
		return parts
	}

	var parts []provider.ContentPart

	for _, output := range step.Reasoning {
		switch reasoning := output.(type) {
		case ReasoningTextOutput:
			parts = append(parts, provider.ContentPart{
				Type:            provider.ContentPartTypeReasoning,
				Text:            reasoning.Text,
				ProviderOptions: providerMetadataToOptions(reasoning.ProviderMetadata),
			})
		case ReasoningFileOutput:
			parts = append(parts, provider.ContentPart{
				Type:            provider.ContentPartTypeReasoningFile,
				Data:            generatedFileData(reasoning.File),
				MediaType:       reasoning.File.MediaType,
				ProviderOptions: providerMetadataToOptions(reasoning.ProviderMetadata),
			})
		}
	}

	if step.Text != "" {
		parts = append(parts, provider.ContentPart{
			Type: provider.ContentPartTypeText,
			Text: step.Text,
		})
	}

	for _, f := range step.Files {
		parts = append(parts, provider.ContentPart{
			Type:      provider.ContentPartTypeFile,
			Data:      generatedFileData(f),
			MediaType: f.MediaType,
		})
	}

	// Index provider-executed results by ToolCallID so each call can be
	// emitted immediately followed by its result. Pre-grouped step state
	// (ToolCalls then ToolResults) loses the natural streaming order, so
	// we re-establish call/result adjacency here rather than relying on
	// [ToResponseMessages] to rewrite the input.
	inlineResults := make(map[string]ToolResult)
	for _, tr := range step.ToolResults {
		if tr.ProviderExecuted && !tr.Preliminary {
			inlineResults[tr.ToolCallID] = tr
		}
	}
	approvalRequests := make(map[string][]ToolApprovalRequest, len(step.ToolApprovalRequests))
	for _, ar := range step.ToolApprovalRequests {
		approvalRequests[ar.ToolCallID] = append(approvalRequests[ar.ToolCallID], ar)
	}

	for _, tc := range step.ToolCalls {
		input := tc.Input
		if tc.Invalid && !isJSONObject(input) {
			input = json.RawMessage(`{}`)
		}
		parts = append(parts, provider.ContentPart{
			Type:             provider.ContentPartTypeToolCall,
			ToolCallID:       tc.ToolCallID,
			ToolName:         tc.ToolName,
			Input:            input,
			ProviderExecuted: tc.ProviderExecuted,
			ProviderOptions:  providerMetadataToOptions(tc.ProviderMetadata),
		})
		for _, ar := range approvalRequests[tc.ToolCallID] {
			parts = append(parts, provider.ContentPart{
				Type:            provider.ContentPartTypeToolApprovalRequest,
				ApprovalID:      ar.ApprovalID,
				ToolCallID:      ar.ToolCallID,
				ToolName:        ar.ToolName,
				Signature:       ar.Signature,
				Reason:          ar.Reason,
				IsAutomatic:     ar.IsAutomatic,
				ProviderOptions: providerMetadataToOptions(ar.ProviderMetadata),
			})
		}
		if tr, ok := inlineResults[tc.ToolCallID]; ok {
			parts = append(parts, provider.ContentPart{
				Type:             provider.ContentPartTypeToolResult,
				ToolCallID:       tr.ToolCallID,
				ToolName:         tr.ToolName,
				Output:           toolResultOutput(tr),
				ProviderExecuted: true,
				ProviderOptions:  providerMetadataToOptions(tr.ProviderMetadata),
			})
		}
	}

	// Approval responses populated here come from in-step automatic
	// approve/deny decisions in executeTools. Resume-resolved approvals
	// (handled by resolveToolApprovals before the first model call) intentionally
	// do not populate step.ToolApprovalResponses, so this loop never duplicates
	// the resume-handled responses already written into the prompt.
	for _, ar := range step.ToolApprovalResponses {
		approved := ar.Approved
		parts = append(parts, provider.ContentPart{
			Type:             provider.ContentPartTypeToolApprovalResponse,
			ApprovalID:       ar.ApprovalID,
			ToolCallID:       ar.ToolCallID,
			ToolName:         ar.ToolName,
			Approved:         &approved,
			Reason:           ar.Reason,
			ProviderExecuted: ar.ProviderExecuted,
			ProviderOptions:  providerMetadataToOptions(ar.ProviderMetadata),
		})
	}
	for _, tr := range step.ToolResults {
		if tr.ProviderExecuted || tr.Preliminary {
			continue
		}
		parts = append(parts, provider.ContentPart{
			Type:             provider.ContentPartTypeToolResult,
			ToolCallID:       tr.ToolCallID,
			ToolName:         tr.ToolName,
			Output:           toolResultOutput(tr),
			ProviderExecuted: false,
			ProviderOptions:  providerMetadataToOptions(tr.ProviderMetadata),
		})
	}

	return parts
}

func toolResultOutput(tr ToolResult) *provider.ToolResultOutput {
	if tr.ModelOutput != nil {
		return tr.ModelOutput
	}
	if tr.IsError {
		return &provider.ToolResultOutput{
			Type: provider.ToolOutputErrorJSON,
			JSON: tr.Output,
		}
	}
	var text string
	if err := json.Unmarshal(tr.Output, &text); err == nil {
		return &provider.ToolResultOutput{
			Type: provider.ToolOutputText,
			Text: text,
		}
	}
	return &provider.ToolResultOutput{
		Type: provider.ToolOutputJSON,
		JSON: tr.Output,
	}
}

func toolResultError(result json.RawMessage) error {
	var text string
	if err := json.Unmarshal(result, &text); err == nil {
		return errors.New(text)
	}
	return errors.New(string(result))
}

func isJSONObject(data json.RawMessage) bool {
	for _, b := range data {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

func isDynamic(toolName string, providerDynamic *bool, tools map[string]Tool) *bool {
	t, known := tools[toolName]
	if !known {
		if providerDynamic != nil {
			return providerDynamic
		}
		dynamic := false
		return &dynamic
	}
	if t.Type == UserToolDynamic {
		d := true
		return &d
	}
	return nil
}

func buildContent(step StepResult) []ContentPart {
	if len(step.responseContent) > 0 {
		return buildRecordedContent(step)
	}
	return buildGroupedContent(step)
}

func buildRecordedContent(step StepResult) []ContentPart {
	toolCalls := make(map[string]ToolCall, len(step.ToolCalls))
	for _, call := range step.ToolCalls {
		toolCalls[call.ToolCallID] = call
	}
	toolResults := make(map[string][]ToolResult, len(step.ToolResults))
	for _, result := range step.ToolResults {
		if !result.Preliminary {
			toolResults[result.ToolCallID] = append(toolResults[result.ToolCallID], result)
		}
	}
	approvalRequests := make(map[string]ToolApprovalRequest, len(step.ToolApprovalRequests))
	for _, request := range step.ToolApprovalRequests {
		approvalRequests[request.ApprovalID] = request
	}

	var parts []ContentPart
	seenCalls := make(map[string]bool, len(step.ToolCalls))
	seenResults := make(map[string]int, len(step.ToolResults))
	seenRequests := make(map[string]bool, len(step.ToolApprovalRequests))
	for _, recorded := range step.responseContent {
		switch recorded.Type {
		case provider.ContentPartTypeText:
			parts = append(parts, TextContent{Text: recorded.Text, ProviderMetadata: optionsToProviderMetadata(recorded.ProviderOptions)})
		case provider.ContentPartTypeReasoning:
			parts = append(parts, ReasoningContent{Text: recorded.Text, ProviderMetadata: optionsToProviderMetadata(recorded.ProviderOptions)})
		case provider.ContentPartTypeFile:
			parts = append(parts, FileContent{
				File:             generatedFileFromDataContent(recorded.Data, recorded.MediaType),
				ProviderMetadata: optionsToProviderMetadata(recorded.ProviderOptions),
			})
		case provider.ContentPartTypeReasoningFile:
			parts = append(parts, ReasoningFileContent{
				File:             generatedFileFromDataContent(recorded.Data, recorded.MediaType),
				ProviderMetadata: optionsToProviderMetadata(recorded.ProviderOptions),
			})
		case provider.ContentPartTypeSource:
			parts = append(parts, SourceContent{Source: Source{
				SourceType:       recorded.SourceType,
				ID:               recorded.ID,
				URL:              recorded.URL,
				Title:            recorded.Title,
				MediaType:        recorded.MediaType,
				Filename:         recorded.Filename,
				ProviderMetadata: optionsToProviderMetadata(recorded.ProviderOptions),
			}})
		case provider.ContentPartTypeCustom:
			parts = append(parts, CustomContent{
				Kind:             recorded.Kind,
				ProviderMetadata: optionsToProviderMetadata(recorded.ProviderOptions),
			})
		case provider.ContentPartTypeToolCall:
			call, ok := toolCalls[recorded.ToolCallID]
			if !ok {
				continue
			}
			seenCalls[call.ToolCallID] = true
			parts = append(parts, toolCallContent(call))
		case provider.ContentPartTypeToolResult:
			index := seenResults[recorded.ToolCallID]
			results := toolResults[recorded.ToolCallID]
			if index >= len(results) {
				continue
			}
			seenResults[recorded.ToolCallID] = index + 1
			parts = append(parts, toolResultContent(results[index]))
		case provider.ContentPartTypeToolApprovalRequest:
			request, ok := approvalRequests[recorded.ApprovalID]
			if !ok {
				continue
			}
			seenRequests[request.ApprovalID] = true
			parts = append(parts, ToolApprovalRequestContent{ToolApprovalRequest: request})
		}
	}

	for _, call := range step.ToolCalls {
		if !seenCalls[call.ToolCallID] {
			parts = append(parts, toolCallContent(call))
		}
	}
	for _, request := range step.ToolApprovalRequests {
		if !seenRequests[request.ApprovalID] {
			parts = append(parts, ToolApprovalRequestContent{ToolApprovalRequest: request})
		}
	}
	for _, response := range step.ToolApprovalResponses {
		parts = append(parts, ToolApprovalResponseContent{ToolApprovalResponse: response})
	}
	for _, result := range step.ToolResults {
		if result.Preliminary {
			continue
		}
		index := seenResults[result.ToolCallID]
		if index > 0 {
			seenResults[result.ToolCallID] = index - 1
			continue
		}
		parts = append(parts, toolResultContent(result))
	}
	return parts
}

func buildGroupedContent(step StepResult) []ContentPart {
	var parts []ContentPart
	for _, output := range step.Reasoning {
		switch reasoning := output.(type) {
		case ReasoningTextOutput:
			parts = append(parts, ReasoningContent(reasoning))
		case ReasoningFileOutput:
			parts = append(parts, ReasoningFileContent(reasoning))
		}
	}
	if step.Text != "" {
		parts = append(parts, TextContent{Text: step.Text})
	}
	requestsByToolCallID := make(map[string][]ToolApprovalRequest, len(step.ToolApprovalRequests))
	for _, ar := range step.ToolApprovalRequests {
		requestsByToolCallID[ar.ToolCallID] = append(requestsByToolCallID[ar.ToolCallID], ar)
	}
	responsesByToolCallID := make(map[string][]ToolApprovalResponse, len(step.ToolApprovalResponses))
	for _, ar := range step.ToolApprovalResponses {
		responsesByToolCallID[ar.ToolCallID] = append(responsesByToolCallID[ar.ToolCallID], ar)
	}
	resultsByToolCallID := make(map[string][]ToolResult, len(step.ToolResults))
	for _, tr := range step.ToolResults {
		if !tr.Preliminary {
			resultsByToolCallID[tr.ToolCallID] = append(resultsByToolCallID[tr.ToolCallID], tr)
		}
	}
	seenToolCallID := make(map[string]bool, len(step.ToolCalls))
	for _, tc := range step.ToolCalls {
		seenToolCallID[tc.ToolCallID] = true
		parts = append(parts, toolCallContent(tc))
		for _, ar := range requestsByToolCallID[tc.ToolCallID] {
			parts = append(parts, ToolApprovalRequestContent{ToolApprovalRequest: ar})
		}
		for _, ar := range responsesByToolCallID[tc.ToolCallID] {
			parts = append(parts, ToolApprovalResponseContent{ToolApprovalResponse: ar})
		}
		for _, tr := range resultsByToolCallID[tc.ToolCallID] {
			parts = append(parts, toolResultContent(tr))
		}
	}
	for _, ar := range step.ToolApprovalRequests {
		if seenToolCallID[ar.ToolCallID] {
			continue
		}
		parts = append(parts, ToolApprovalRequestContent{ToolApprovalRequest: ar})
	}
	for _, ar := range step.ToolApprovalResponses {
		if seenToolCallID[ar.ToolCallID] {
			continue
		}
		parts = append(parts, ToolApprovalResponseContent{ToolApprovalResponse: ar})
	}
	for _, tr := range step.ToolResults {
		if tr.Preliminary || seenToolCallID[tr.ToolCallID] {
			continue
		}
		parts = append(parts, toolResultContent(tr))
	}
	for _, src := range step.Sources {
		parts = append(parts, SourceContent{Source: src})
	}
	for _, f := range step.Files {
		parts = append(parts, FileContent{File: f})
	}
	return parts
}

func toolCallContent(call ToolCall) ToolCallContent {
	return ToolCallContent{
		ToolCallID:       call.ToolCallID,
		ToolName:         call.ToolName,
		Input:            call.Input,
		Invalid:          call.Invalid,
		Error:            call.Error,
		ProviderExecuted: call.ProviderExecuted,
		Dynamic:          call.Dynamic,
		Title:            call.Title,
		ProviderMetadata: call.ProviderMetadata,
	}
}

func toolResultContent(result ToolResult) ContentPart {
	if result.IsError {
		var errorValue any = result.Error
		if result.ProviderExecuted && len(result.Output) > 0 {
			errorValue = append(json.RawMessage(nil), result.Output...)
		} else if errorValue == nil {
			errorValue = toolResultError(result.Output)
		}
		return ToolErrorContent{
			ToolCallID:       result.ToolCallID,
			ToolName:         result.ToolName,
			Input:            result.Input,
			Error:            errorValue,
			ProviderExecuted: result.ProviderExecuted,
			Dynamic:          result.Dynamic,
			Title:            result.Title,
			ProviderMetadata: result.ProviderMetadata,
		}
	}
	return ToolResultContent{
		ToolCallID:       result.ToolCallID,
		ToolName:         result.ToolName,
		Input:            result.Input,
		Output:           result.Output,
		ProviderExecuted: result.ProviderExecuted,
		Dynamic:          result.Dynamic,
		Preliminary:      result.Preliminary,
		Title:            result.Title,
		ProviderMetadata: result.ProviderMetadata,
	}
}
