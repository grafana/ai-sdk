package agentobservability

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/internal/streamusage"
	"github.com/grafana/ai-sdk/provider"
)

// StreamRecorder accumulates an agento11y.Generation from a sequence of
// provider.StreamPart events. It is the streaming counterpart to
// MapGenerateResult: the caller observes each part as it flows through the
// stream channel, and at end-of-stream Generation() returns the final
// payload to hand to agento11y.GenerationRecorder.SetResult.
//
// A StreamRecorder is owned by a single recording goroutine. Observe is not
// safe for concurrent invocation; Generation() / FirstChunkAt() are safe to
// call once the recording goroutine has stopped feeding observations.
type StreamRecorder struct {
	seed   agento11y.GenerationStart
	params provider.CallOptions

	mu sync.Mutex

	// firstChunkAt is recorded the first time Observe is called with a
	// payload-bearing part. The recorder publishes this through
	// FirstChunkAt so the middleware can call recorder.SetFirstTokenAt.
	firstChunkAt time.Time

	// Per-step accumulators. Streams may emit multiple text/reasoning blocks
	// keyed by an ID (text-start / text-end frame them); we maintain a map so
	// concurrent IDs accumulate correctly.
	texts                map[string]*streamTextAcc
	reasonings           map[string]*streamTextAcc
	toolCalls            []*streamToolCallAcc
	activeToolCalls      map[string]int
	mediaParts           []agento11y.Part
	outputOrder          []streamOutputRef
	toolResults          []streamToolResultAcc
	toolResultByIdentity map[streamToolIdentity]int

	// Aggregated stream state.
	finishReason  *provider.FinishReason
	usage         streamusage.Aggregator
	responseID    string
	responseModel string
	response      generationModelIdentity

	// Error captured from a PartError event mid-stream. The middleware
	// surfaces this through GenerationRecorder.SetCallError rather than
	// embedding it inside the Generation itself.
	callError error
}

type streamOutputRef struct {
	partType   provider.StreamPartType
	id         string
	toolIndex  int
	mediaIndex int
}

type streamToolIdentity struct {
	id   string
	name string
}

type streamTextAcc struct {
	text strings.Builder
}

type streamToolCallAcc struct {
	id               string
	name             string
	input            strings.Builder
	providerExecuted bool
	anthropicType    string
	// final captures the consolidated input when a PartToolCall event fires
	// (some providers emit the complete input there rather than via deltas).
	final json.RawMessage
}

type streamToolResultAcc struct {
	id               string
	name             string
	result           json.RawMessage
	isError          bool
	preliminary      bool
	providerExecuted bool
	anthropicType    string
}

// NewStreamRecorder constructs a StreamRecorder seeded with the
// GenerationStart and the CallOptions used for the call. The seed copies are
// folded into Generation() at end-of-stream alongside the accumulated output.
func NewStreamRecorder(start agento11y.GenerationStart, params provider.CallOptions) *StreamRecorder {
	return &StreamRecorder{
		seed:                 start,
		params:               params,
		texts:                map[string]*streamTextAcc{},
		reasonings:           map[string]*streamTextAcc{},
		activeToolCalls:      map[string]int{},
		toolResultByIdentity: map[streamToolIdentity]int{},
	}
}

// FirstChunkAt returns the timestamp of the first observed payload-bearing
// part, or the zero time when nothing has been observed yet.
func (r *StreamRecorder) FirstChunkAt() time.Time {
	if r == nil {
		return time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.firstChunkAt
}

// CallError returns the error captured from a PartError event, if any. The
// middleware reads it after the stream closes and forwards it to
// GenerationRecorder.SetCallError.
func (r *StreamRecorder) CallError() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callError
}

// Observe folds a single stream event into the recorder. Unknown / unsupported
// event types are silently ignored; the accumulator is robust to provider
// implementations that emit fewer or more events than expected.
func (r *StreamRecorder) Observe(part provider.StreamPart) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.firstChunkAt.IsZero() && isPayloadPart(part) {
		r.firstChunkAt = time.Now().UTC()
	}
	r.usage.Observe(part)

	switch part.Type {
	case provider.PartTextStart:
		r.ensureText(part.ID)

	case provider.PartTextDelta:
		acc := r.ensureText(part.ID)
		acc.text.WriteString(part.Delta)

	case provider.PartTextEnd:
		// Nothing to do: the accumulator already holds the full text. Some
		// providers emit a trailing PartTextEnd without delta.

	case provider.PartReasoningStart:
		r.ensureReasoning(part.ID)

	case provider.PartReasoningDelta:
		acc := r.ensureReasoning(part.ID)
		acc.text.WriteString(part.Delta)

	case provider.PartToolInputStart:
		acc := r.startToolCall(streamToolCallID(part), part.ToolName)
		acc.providerExecuted = acc.providerExecuted || part.ProviderExecuted
		if anthropicType := anthropicTypeFromMetadata(part.ProviderMetadata); anthropicType != "" {
			acc.anthropicType = anthropicType
		}

	case provider.PartToolInputDelta:
		acc := r.currentToolCall(streamToolCallID(part), part.ToolName)
		acc.input.WriteString(part.Delta)

	case provider.PartToolInputEnd:
		// Caller may still emit a PartToolCall with the consolidated input.

	case provider.PartToolCall:
		acc := r.finishToolCall(streamToolCallID(part), part.ToolName)
		acc.providerExecuted = acc.providerExecuted || part.ProviderExecuted
		if anthropicType := anthropicTypeFromMetadata(part.ProviderMetadata); anthropicType != "" {
			acc.anthropicType = anthropicType
		}
		if part.Input != "" {
			acc.final = json.RawMessage(part.Input)
		}

	case provider.PartToolResult:
		r.observeToolResult(part)

	case provider.PartFile:
		if media, ok := streamFilePartToAgento11y(part, "file"); ok {
			r.markFirstChunk()
			r.outputOrder = append(r.outputOrder, streamOutputRef{partType: part.Type, mediaIndex: len(r.mediaParts)})
			r.mediaParts = append(r.mediaParts, media)
		}

	case provider.PartReasoningFile:
		if media, ok := streamFilePartToAgento11y(part, "reasoning_file"); ok {
			r.markFirstChunk()
			r.outputOrder = append(r.outputOrder, streamOutputRef{partType: part.Type, mediaIndex: len(r.mediaParts)})
			r.mediaParts = append(r.mediaParts, media)
		}

	case provider.PartFinish:
		if part.FinishReason != nil {
			fr := *part.FinishReason
			r.finishReason = &fr
		}

	case provider.PartResponseMeta:
		if part.ResponseID != "" {
			r.responseID = part.ResponseID
		}
		if part.ModelID != "" {
			r.responseModel = part.ModelID
		}
		if response := modelIdentityFromResponse(part.Provider, part.ModelID); response.complete() {
			r.response = response
		}

	case provider.PartError:
		if part.APICallError != nil {
			r.callError = part.APICallError
		}
	}
}

func (r *StreamRecorder) markFirstChunk() {
	if r.firstChunkAt.IsZero() {
		r.firstChunkAt = time.Now().UTC()
	}
}

func (r *StreamRecorder) observeToolResult(part provider.StreamPart) {
	preliminary := part.Preliminary != nil && *part.Preliminary
	result := streamToolResultAcc{
		id:               part.ToolCallID,
		name:             part.ToolName,
		result:           cloneRawJSON(part.Result),
		isError:          part.IsError,
		preliminary:      preliminary,
		providerExecuted: part.ProviderExecuted,
		anthropicType:    anthropicTypeFromMetadata(part.ProviderMetadata),
	}
	if part.ToolCallID == "" {
		r.toolResults = append(r.toolResults, result)
		return
	}
	identity := streamToolIdentity{id: part.ToolCallID, name: part.ToolName}
	if index, ok := r.toolResultByIdentity[identity]; ok {
		previous := r.toolResults[index]
		result.providerExecuted = result.providerExecuted || previous.providerExecuted
		if result.name == "" {
			result.name = previous.name
		}
		if result.anthropicType == "" {
			result.anthropicType = previous.anthropicType
		}
		r.toolResults[index] = result
		if !preliminary {
			delete(r.toolResultByIdentity, identity)
		}
		return
	}
	if preliminary {
		r.toolResultByIdentity[identity] = len(r.toolResults)
	}
	r.toolResults = append(r.toolResults, result)
}

func (r *StreamRecorder) ensureText(id string) *streamTextAcc {
	if acc, ok := r.texts[id]; ok {
		return acc
	}
	acc := &streamTextAcc{}
	r.texts[id] = acc
	r.outputOrder = append(r.outputOrder, streamOutputRef{partType: provider.PartTextStart, id: id})
	return acc
}

func (r *StreamRecorder) ensureReasoning(id string) *streamTextAcc {
	if acc, ok := r.reasonings[id]; ok {
		return acc
	}
	acc := &streamTextAcc{}
	r.reasonings[id] = acc
	r.outputOrder = append(r.outputOrder, streamOutputRef{partType: provider.PartReasoningStart, id: id})
	return acc
}

func streamToolCallID(part provider.StreamPart) string {
	if part.Type == provider.PartToolCall && part.ToolCallID != "" {
		return part.ToolCallID
	}
	if part.ID != "" {
		return part.ID
	}
	return part.ToolCallID
}

func (r *StreamRecorder) startToolCall(id, name string) *streamToolCallAcc {
	acc := r.appendToolCall(id, name)
	r.activeToolCalls[id] = len(r.toolCalls) - 1
	return acc
}

func (r *StreamRecorder) currentToolCall(id, name string) *streamToolCallAcc {
	if index, ok := r.activeToolCalls[id]; ok {
		acc := r.toolCalls[index]
		if acc.name == "" && name != "" {
			acc.name = name
		}
		return acc
	}
	return r.startToolCall(id, name)
}

func (r *StreamRecorder) finishToolCall(id, name string) *streamToolCallAcc {
	if index, ok := r.activeToolCalls[id]; ok {
		delete(r.activeToolCalls, id)
		acc := r.toolCalls[index]
		if acc.name == "" || name == "" || acc.name == name {
			if acc.name == "" {
				acc.name = name
			}
			return acc
		}
	}
	return r.appendToolCall(id, name)
}

func (r *StreamRecorder) appendToolCall(id, name string) *streamToolCallAcc {
	acc := &streamToolCallAcc{id: id, name: name}
	r.toolCalls = append(r.toolCalls, acc)
	r.outputOrder = append(r.outputOrder, streamOutputRef{
		partType:  provider.PartToolCall,
		toolIndex: len(r.toolCalls) - 1,
	})
	return acc
}

// Generation returns the final agento11y.Generation derived from the seed plus
// the accumulated stream state. Idempotent: callers may invoke it multiple
// times (e.g. once at end-of-stream and once on error fallback) and receive
// equivalent results.
func (r *StreamRecorder) Generation() agento11y.Generation {
	if r == nil {
		return agento11y.Generation{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compose Generation from CallOptions identically to MapGenerateResult,
	// using the accumulated output instead of result.Content.
	system, input := messagesToAgento11yWithTools(r.params.Prompt, r.params.Tools)
	controls := controlsFromCallOptions(r.params)

	output := r.buildOutput()
	usage, hasUsage := r.usage.Usage()
	metadata := mergeMetadata(r.seed.Metadata, metadataFromProviderOptions(r.params))
	if hasUsage {
		metadata = mergeMetadata(metadata, metadataFromUsage(usage))
	}

	gen := agento11y.Generation{
		UserID:              r.seed.UserID,
		AgentName:           r.seed.AgentName,
		AgentVersion:        r.seed.AgentVersion,
		ParentGenerationIDs: cloneStringSlice(r.seed.ParentGenerationIDs),
		SystemPrompt:        system,
		Input:               input,
		Output:              output,
		Tools:               toolsToAgento11y(r.params.Tools),
		MaxTokens:           controls.MaxTokens,
		Temperature:         controls.Temperature,
		TopP:                controls.TopP,
		ToolChoice:          controls.ToolChoice,
		ThinkingEnabled:     thinkingEnabledFromAnthropic(r.params.ProviderOptions),
		Tags:                cloneStringMap(r.seed.Tags),
		Metadata:            metadata,
	}
	if r.responseID != "" {
		gen.ResponseID = r.responseID
	}
	gen.ResponseModel = r.seed.Model.Name
	if r.responseModel != "" {
		gen.ResponseModel = r.responseModel
	}
	applyModelIdentity(&gen, modelIdentityFromStart(r.seed), r.response)

	if hasUsage {
		gen.Usage = usageToAgento11y(usage)
	}
	if r.finishReason != nil {
		gen.StopReason = finishReasonToAgento11yStop(*r.finishReason)
	}
	return gen
}

// buildOutput assembles a single assistant message plus optional tool messages
// for tool-result parts. Assistant parts retain the order in which their first
// provider events were observed.
func (r *StreamRecorder) buildOutput() []agento11y.Message {
	parts := make([]agento11y.Part, 0, len(r.outputOrder))

	for _, ref := range r.outputOrder {
		switch ref.partType {
		case provider.PartReasoningStart:
			acc := r.reasonings[ref.id]
			if acc.text.Len() == 0 {
				continue
			}
			part := agento11y.ThinkingPart(acc.text.String())
			part.Metadata.ProviderType = "thinking"
			parts = append(parts, part)

		case provider.PartTextStart:
			acc := r.texts[ref.id]
			if acc.text.Len() == 0 {
				continue
			}
			parts = append(parts, agento11y.TextPart(acc.text.String()))

		case provider.PartToolCall:
			acc := r.toolCalls[ref.toolIndex]
			var input json.RawMessage
			if len(acc.final) > 0 {
				input = normalizeJSONObject(acc.final)
			} else if acc.input.Len() > 0 {
				input = normalizeJSONObject(json.RawMessage(acc.input.String()))
			}
			part := agento11y.ToolCallPart(agento11y.ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				InputJSON: input,
			})
			part.Metadata.ProviderType = providerTypeForToolWithDefinitions(
				acc.providerExecuted,
				acc.name,
				acc.anthropicType,
				r.params.Tools,
			)
			parts = append(parts, part)

		case provider.PartFile, provider.PartReasoningFile:
			parts = append(parts, r.mediaParts[ref.mediaIndex])
		}
	}

	var output []agento11y.Message
	if len(parts) > 0 {
		output = append(output, agento11y.Message{
			Role:  agento11y.RoleAssistant,
			Parts: parts,
		})
	}

	for _, result := range r.toolResults {
		if result.preliminary {
			continue
		}
		out := agento11y.ToolResultPart(agento11y.ToolResult{
			ToolCallID:  result.id,
			Name:        result.name,
			IsError:     result.isError,
			ContentJSON: result.result,
		})
		providerExecuted := result.providerExecuted
		anthropicType := result.anthropicType
		if call, ok := r.matchToolCall(result.id, result.name); ok {
			providerExecuted = providerExecuted || call.providerExecuted
			if anthropicType == "" {
				anthropicType = call.anthropicType
			}
		}
		out.Metadata.ProviderType = providerTypeForToolResult(
			providerExecuted,
			result.name,
			anthropicType,
			r.params.Tools,
			result.result,
		)
		output = append(output, agento11y.Message{
			Role:  agento11y.RoleTool,
			Name:  result.name,
			Parts: []agento11y.Part{out},
		})
	}

	if len(output) == 0 {
		return nil
	}
	return output
}

func (r *StreamRecorder) matchToolCall(id, name string) (*streamToolCallAcc, bool) {
	var match *streamToolCallAcc
	for _, call := range r.toolCalls {
		if call.id != id || call.name != name {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = call
	}
	return match, match != nil
}

func isPayloadPart(part provider.StreamPart) bool {
	switch part.Type {
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolInputDelta:
		return part.Delta != ""
	case provider.PartToolCall:
		return true
	default:
		return false
	}
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
