package aisdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/grafana/ai-sdk/internal/ptr"
	"github.com/grafana/ai-sdk/provider"
)

const toolLoopAgentUserAgent = "ai-sdk-agent/tool-loop"

// AgentVersion identifies the version of the Agent interface implemented by an Agent.
type AgentVersion string

const (
	// AgentVersionV1 is the upstream-compatible Agent interface version.
	AgentVersionV1 AgentVersion = "agent-v1"
)

// Agent is a reusable LLM agent.
//
// Implementations wrap autonomous orchestration behind Generate and Stream while
// returning the same result types as GenerateText and StreamText. ToolLoopAgent
// implements this interface by delegating to the existing StreamText engine.
type Agent interface {
	Version() AgentVersion
	ID() string
	Tools() ToolSet
	Generate(ctx context.Context, opts ...AgentGenerateOption) (*GenerateTextResult, error)
	Stream(ctx context.Context, opts ...AgentStreamOption) *StreamTextResult
}

// ToolLoopAgent is the built-in Agent implementation backed by StreamText and GenerateText.
//
// ToolLoopAgent stores reusable settings and merges them with per-call options
// for each Generate or Stream call. It defaults to StepCountIs(20) when no stop
// condition is configured, while direct StreamText calls keep their one-step
// default. Reusable and per-call lifecycle callbacks compose in reusable-then-call
// order and inherit StreamText's concurrency behavior.
//
// Agent runtime context is a wrapper-level Go adaptation of upstream runtimeContext:
// the resolved value is propagated through PrepareStepState.Context and
// ToolExecutionOptions.Context. Non-nil PrepareStepResult.Context values carry
// forward to later steps; nil preserves the effective context because the current
// result shape cannot distinguish an omitted context from an intentional nil clear.
//
// The Agent user-agent marker is added only to provider.CallOptions headers;
// provider modules must honor call headers for it to reach the network. OpenAI
// Responses currently does not honor call headers, and provider default
// User-Agent append semantics require separate provider work.
type ToolLoopAgent struct {
	model               provider.LanguageModel
	id                  string
	settingsConfig      *streamConfig
	runtimeContext      any
	runtimeContextSet   bool
	configuredToolSet   ToolSet
	configuredToolSetOK bool
}

var _ Agent = (*ToolLoopAgent)(nil)

// ToolLoopAgentOption configures a ToolLoopAgent.
type ToolLoopAgentOption interface {
	applyToolLoopAgent(*toolLoopAgentConfig)
}

type toolLoopAgentConfig struct {
	id                string
	settings          []StreamOption
	runtimeContext    any
	runtimeContextSet bool
}

type toolLoopAgentOptionFunc func(*toolLoopAgentConfig)

func (f toolLoopAgentOptionFunc) applyToolLoopAgent(c *toolLoopAgentConfig) { f(c) }

// WithToolLoopAgentID sets the optional Agent ID.
func WithToolLoopAgentID(id string) ToolLoopAgentOption {
	return toolLoopAgentOptionFunc(func(c *toolLoopAgentConfig) { c.id = id })
}

// WithToolLoopAgentOptions adds reusable StreamText/GenerateText settings to a ToolLoopAgent.
//
// Shared options such as WithTools, WithInstructions, WithStopWhen, and callbacks
// apply to both Stream and Generate. Stream-only options such as OnChunk apply
// only to Stream calls.
func WithToolLoopAgentOptions(opts ...StreamOption) ToolLoopAgentOption {
	copied := append([]StreamOption(nil), opts...)
	return toolLoopAgentOptionFunc(func(c *toolLoopAgentConfig) {
		c.settings = append(c.settings, copied...)
	})
}

// WithToolLoopAgentRuntimeContext sets reusable Agent runtime context.
//
// The context is delivered to callbacks and tools through PrepareStepState.Context
// and ToolExecutionOptions.Context. A per-call Agent runtime context
// overrides this value only when supplied.
func WithToolLoopAgentRuntimeContext(value any) ToolLoopAgentOption {
	return toolLoopAgentOptionFunc(func(c *toolLoopAgentConfig) {
		c.runtimeContext = value
		c.runtimeContextSet = true
	})
}

// NewToolLoopAgent constructs a ToolLoopAgent for model.
//
// Construction records settings only and does not call the provider.
func NewToolLoopAgent(model provider.LanguageModel, opts ...ToolLoopAgentOption) *ToolLoopAgent {
	cfg := toolLoopAgentConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyToolLoopAgent(&cfg)
		}
	}

	built := buildStreamConfig(cfg.settings)
	settingsConfig := cloneStreamConfig(built)
	tools := cloneToolSet(settingsConfig.tools)

	return &ToolLoopAgent{
		model:               model,
		id:                  cfg.id,
		settingsConfig:      settingsConfig,
		runtimeContext:      cfg.runtimeContext,
		runtimeContextSet:   cfg.runtimeContextSet,
		configuredToolSet:   tools,
		configuredToolSetOK: settingsConfig.tools != nil,
	}
}

// Version returns agent-v1.
func (a *ToolLoopAgent) Version() AgentVersion { return AgentVersionV1 }

// ID returns the optional Agent ID.
func (a *ToolLoopAgent) ID() string {
	if a == nil {
		return ""
	}
	return a.id
}

// Tools returns a copy of the reusable tool set configured on the Agent.
func (a *ToolLoopAgent) Tools() ToolSet {
	if a == nil || !a.configuredToolSetOK {
		return nil
	}
	return cloneToolSet(a.configuredToolSet)
}

// AgentStreamOption configures an Agent Stream call.
type AgentStreamOption interface {
	applyAgentStream(*agentCallConfig)
}

// AgentGenerateOption configures an Agent Generate call.
type AgentGenerateOption interface {
	applyAgentGenerate(*agentCallConfig)
}

// AgentOption configures both Agent Stream and Agent Generate calls.
type AgentOption interface {
	AgentStreamOption
	AgentGenerateOption
}

type agentCallConfig struct {
	streamOptions     []StreamOption
	generateOptions   []GenerateOption
	runtimeContext    any
	runtimeContextSet bool
}

type agentOptionFunc struct {
	stream   func(*agentCallConfig)
	generate func(*agentCallConfig)
}

func (f agentOptionFunc) applyAgentStream(c *agentCallConfig) {
	if f.stream != nil {
		f.stream(c)
	}
}

func (f agentOptionFunc) applyAgentGenerate(c *agentCallConfig) {
	if f.generate != nil {
		f.generate(c)
	}
}

// WithAgentOptions applies shared StreamText/GenerateText options to an Agent call.
func WithAgentOptions(opts ...Option) AgentOption {
	streamOpts := make([]StreamOption, len(opts))
	generateOpts := make([]GenerateOption, len(opts))
	for i, opt := range opts {
		streamOpts[i] = opt
		generateOpts[i] = opt
	}
	return agentOptionFunc{
		stream:   func(c *agentCallConfig) { c.streamOptions = append(c.streamOptions, streamOpts...) },
		generate: func(c *agentCallConfig) { c.generateOptions = append(c.generateOptions, generateOpts...) },
	}
}

// WithAgentStreamOptions applies StreamText options to an Agent Stream call.
func WithAgentStreamOptions(opts ...StreamOption) AgentStreamOption {
	copied := append([]StreamOption(nil), opts...)
	return agentOptionFunc{stream: func(c *agentCallConfig) { c.streamOptions = append(c.streamOptions, copied...) }}
}

// WithAgentGenerateOptions applies GenerateText options to an Agent Generate call.
func WithAgentGenerateOptions(opts ...GenerateOption) AgentGenerateOption {
	copied := append([]GenerateOption(nil), opts...)
	return agentOptionFunc{generate: func(c *agentCallConfig) { c.generateOptions = append(c.generateOptions, copied...) }}
}

// WithAgentPrompt sets a user text prompt for an Agent call.
func WithAgentPrompt(prompt string) AgentOption {
	return WithAgentOptions(WithModelMessages(provider.UserText(prompt)))
}

// WithAgentMessages sets UI messages for an Agent call.
func WithAgentMessages(messages ...UIMessage) AgentOption {
	return WithAgentOptions(WithMessages(messages...))
}

// WithAgentModelMessages sets provider model messages for an Agent call.
func WithAgentModelMessages(messages ...provider.Message) AgentOption {
	return WithAgentOptions(WithModelMessages(messages...))
}

// WithAgentRuntimeContext sets per-call Agent runtime context.
func WithAgentRuntimeContext(value any) AgentOption {
	return agentOptionFunc{
		stream: func(c *agentCallConfig) {
			c.runtimeContext = value
			c.runtimeContextSet = true
		},
		generate: func(c *agentCallConfig) {
			c.runtimeContext = value
			c.runtimeContextSet = true
		},
	}
}

// Stream starts an Agent stream call.
func (a *ToolLoopAgent) Stream(ctx context.Context, opts ...AgentStreamOption) *StreamTextResult {
	call := buildAgentStreamCall(opts)
	settingsCfg := cloneStreamConfig(a.settingsConfig)
	callCfg := buildStreamConfig(call.streamOptions)
	cfg := mergeStreamConfig(settingsCfg, callCfg)
	a.finalizeConfig(cfg, call.runtimeContext, call.runtimeContextSet)
	return streamTextWithConfig(ctx, a.model, cfg)
}

// Generate runs an Agent call to completion.
func (a *ToolLoopAgent) Generate(ctx context.Context, opts ...AgentGenerateOption) (*GenerateTextResult, error) {
	call := buildAgentGenerateCall(opts)
	settingsCfg := cloneStreamConfig(a.settingsConfig)
	callGenerateCfg := buildGenerateConfig(call.generateOptions)
	effectiveTimeout := settingsCfg.timeout
	if callGenerateCfg.timeoutSet {
		effectiveTimeout = callGenerateCfg.timeout
	}
	timeoutWarnings := generateTimeoutWarnings(effectiveTimeout)
	callCfg := callGenerateCfg.toStreamConfig()
	cfg := mergeStreamConfig(settingsCfg, callCfg)
	cfg.onChunk = nil
	cfg.onAbort = nil
	cfg.includeRawChunks = nil
	cfg.parseOutputOnNonStop = false
	cfg.timeout.FirstChunk = 0
	cfg.timeout.Chunk = 0
	a.finalizeConfig(cfg, call.runtimeContext, call.runtimeContextSet)

	result := streamTextWithConfig(ctx, a.model, cfg)
	for range result.FullStream() {
	}
	result.Wait()
	if err := result.Err(); err != nil {
		return nil, err
	}
	if err := result.abortError(); err != nil {
		return nil, err
	}
	generateResult := streamResultToGenerateResult(result)
	generateResult.Warnings = append(timeoutWarnings, generateResult.Warnings...)
	return generateResult, nil
}

func buildAgentStreamCall(opts []AgentStreamOption) agentCallConfig {
	cfg := agentCallConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyAgentStream(&cfg)
		}
	}
	return cfg
}

func buildAgentGenerateCall(opts []AgentGenerateOption) agentCallConfig {
	cfg := agentCallConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyAgentGenerate(&cfg)
		}
	}
	return cfg
}

func (a *ToolLoopAgent) finalizeConfig(cfg *streamConfig, callRuntimeContext any, callRuntimeContextSet bool) {
	if len(cfg.stopWhen) == 0 {
		cfg.stopWhen = []StopCondition{StepCountIs(20)}
	}
	cfg.headers = appendAgentUserAgent(cloneStringMap(cfg.headers))

	runtimeContext := a.runtimeContext
	runtimeContextSet := a.runtimeContextSet
	if callRuntimeContextSet {
		runtimeContext = callRuntimeContext
		runtimeContextSet = true
	}
	if runtimeContextSet {
		cfg.runtimeContext = runtimeContext
	}
}

func streamResultToGenerateResult(result *StreamTextResult) *GenerateTextResult {
	return &GenerateTextResult{
		Text:             result.Text(),
		Reasoning:        result.Reasoning(),
		ToolCalls:        result.ToolCalls(),
		ToolResults:      result.ToolResults(),
		Files:            result.Files(),
		Sources:          result.Sources(),
		FinishReason:     result.FinishReason(),
		Usage:            result.Usage(),
		TotalUsage:       result.TotalUsage(),
		Steps:            result.Steps(),
		Warnings:         result.Warnings(),
		Content:          result.Content(),
		Response:         result.Response(),
		ProviderMetadata: result.ProviderMetadata(),
		Output:           result.OutputValue(),
		OutputError:      result.OutputError(),
	}
}

func mergeStreamConfig(settings *streamConfig, call *streamConfig) *streamConfig {
	cfg := cloneStreamConfig(settings)
	mergeBaseConfig(&cfg.baseConfig, &call.baseConfig)

	if call.onChunk != nil {
		cfg.onChunk = mergeCallbacks1(cfg.onChunk, call.onChunk)
	}
	if call.onAbort != nil {
		cfg.onAbort = mergeCallbacks1(cfg.onAbort, call.onAbort)
	}
	if call.includeRawChunks != nil {
		cfg.includeRawChunks = ptr.Clone(call.includeRawChunks)
	}
	return cfg
}

func mergeBaseConfig(dst *baseConfig, call *baseConfig) {
	settingsCallbacks := baseCallbacks{
		onStart:          dst.onStart,
		onStepStart:      dst.onStepStart,
		onStepFinish:     dst.onStepFinish,
		onFinish:         dst.onFinish,
		onError:          dst.onError,
		onToolCallStart:  dst.onToolCallStart,
		onToolCallFinish: dst.onToolCallFinish,
	}

	if call.messages != nil {
		dst.messages = cloneUIMessages(call.messages)
		dst.modelMessages = nil
	}
	if call.modelMessages != nil {
		dst.modelMessages = cloneProviderMessages(call.modelMessages)
		dst.messages = nil
	}
	if call.system != nil {
		dst.system = cloneSystemMessages(call.system)
	}
	if call.tools != nil {
		dst.tools = cloneToolSet(call.tools)
	}
	if call.toolChoice != nil {
		dst.toolChoice = ptr.Clone(call.toolChoice)
	}
	if call.activeToolsSet {
		dst.activeTools = append([]string(nil), call.activeTools...)
		dst.activeToolsSet = true
	}
	if len(call.stopWhen) > 0 {
		dst.stopWhen = append([]StopCondition(nil), call.stopWhen...)
	}
	if call.toolApproval.generic != nil || call.toolApproval.tools != nil {
		dst.toolApproval = cloneToolApprovalConfig(call.toolApproval)
	}
	if call.maxRetries != nil {
		dst.maxRetries = ptr.Clone(call.maxRetries)
	}
	if call.timeoutSet {
		dst.timeout = call.timeout
		dst.timeoutSet = true
	}
	if call.retryInitDelay > 0 {
		dst.retryInitDelay = call.retryInitDelay
	}
	if call.retryBackoff > 0 {
		dst.retryBackoff = call.retryBackoff
	}
	if call.maxOutputTokens != nil {
		dst.maxOutputTokens = ptr.Clone(call.maxOutputTokens)
	}
	if call.temperature != nil {
		dst.temperature = ptr.Clone(call.temperature)
	}
	if call.topP != nil {
		dst.topP = ptr.Clone(call.topP)
	}
	if call.topK != nil {
		dst.topK = ptr.Clone(call.topK)
	}
	if call.presencePenalty != nil {
		dst.presencePenalty = ptr.Clone(call.presencePenalty)
	}
	if call.frequencyPenalty != nil {
		dst.frequencyPenalty = ptr.Clone(call.frequencyPenalty)
	}
	if call.stopSequences != nil {
		dst.stopSequences = append([]string(nil), call.stopSequences...)
	}
	if call.responseFormat != nil {
		dst.responseFormat = ptr.Clone(call.responseFormat)
	}
	if call.seed != nil {
		dst.seed = ptr.Clone(call.seed)
	}
	if call.providerOptions != nil {
		dst.providerOptions = mergeProviderOptions(dst.providerOptions, call.providerOptions)
	}
	if call.headers != nil {
		dst.headers = mergeStringMaps(dst.headers, call.headers)
	}
	if call.generateID != nil {
		dst.generateID = call.generateID
	}
	if call.reasoning != nil {
		dst.reasoning = ptr.Clone(call.reasoning)
	}
	if call.prepareStep != nil {
		dst.prepareStep = call.prepareStep
	}
	if call.output != nil {
		dst.output = call.output
	}

	dst.onStart = mergeCallbacks1(settingsCallbacks.onStart, call.onStart)
	dst.onStepStart = mergeCallbacks1(settingsCallbacks.onStepStart, call.onStepStart)
	dst.onStepFinish = mergeCallbacks1(settingsCallbacks.onStepFinish, call.onStepFinish)
	dst.onFinish = mergeCallbacks1(settingsCallbacks.onFinish, call.onFinish)
	dst.onError = mergeCallbacks1(settingsCallbacks.onError, call.onError)
	dst.onToolCallStart = mergeCallbacks1(settingsCallbacks.onToolCallStart, call.onToolCallStart)
	dst.onToolCallFinish = mergeCallbacks1(settingsCallbacks.onToolCallFinish, call.onToolCallFinish)
}

type baseCallbacks struct {
	onStart          func(OnStartState)
	onStepStart      func(OnStepStartState)
	onStepFinish     func(OnStepFinishState)
	onFinish         func(OnFinishState)
	onError          func(error)
	onToolCallStart  func(OnToolCallStartState)
	onToolCallFinish func(OnToolCallFinishState)
}

func cloneStreamConfig(src *streamConfig) *streamConfig {
	if src == nil {
		return &streamConfig{}
	}
	base := cloneBaseConfig(src.baseConfig)
	return &streamConfig{
		baseConfig:           base,
		onChunk:              src.onChunk,
		onAbort:              src.onAbort,
		includeRawChunks:     src.includeRawChunks,
		parseOutputOnNonStop: src.parseOutputOnNonStop,
	}
}

func cloneBaseConfig(src baseConfig) baseConfig {
	cfg := src
	cfg.messages = cloneUIMessages(src.messages)
	cfg.modelMessages = cloneProviderMessages(src.modelMessages)
	cfg.system = cloneSystemMessages(src.system)
	cfg.tools = cloneToolSet(src.tools)
	cfg.activeTools = append([]string(nil), src.activeTools...)
	cfg.stopWhen = append([]StopCondition(nil), src.stopWhen...)
	cfg.toolApproval = cloneToolApprovalConfig(src.toolApproval)
	cfg.stopSequences = append([]string(nil), src.stopSequences...)
	cfg.providerOptions = cloneProviderOptions(src.providerOptions)
	cfg.headers = cloneStringMap(src.headers)
	cfg.toolChoice = ptr.Clone(src.toolChoice)
	cfg.maxRetries = ptr.Clone(src.maxRetries)
	cfg.maxOutputTokens = ptr.Clone(src.maxOutputTokens)
	cfg.temperature = ptr.Clone(src.temperature)
	cfg.topP = ptr.Clone(src.topP)
	cfg.topK = ptr.Clone(src.topK)
	cfg.presencePenalty = ptr.Clone(src.presencePenalty)
	cfg.frequencyPenalty = ptr.Clone(src.frequencyPenalty)
	cfg.responseFormat = ptr.Clone(src.responseFormat)
	cfg.seed = ptr.Clone(src.seed)
	cfg.reasoning = ptr.Clone(src.reasoning)
	return cfg
}

func cloneToolSet(tools ToolSet) ToolSet {
	if tools == nil {
		return nil
	}
	out := make(ToolSet, len(tools))
	for name, tool := range tools {
		out[name] = tool
	}
	return out
}

func cloneToolApprovalConfig(src toolApprovalConfig) toolApprovalConfig {
	out := toolApprovalConfig{generic: src.generic}
	if src.tools != nil {
		out.tools = make(ToolApprovalMap, len(src.tools))
		for name, policy := range src.tools {
			out.tools[name] = policy
		}
	}
	return out
}

func cloneUIMessages(messages []UIMessage) []UIMessage {
	if messages == nil {
		return nil
	}
	out := make([]UIMessage, len(messages))
	copy(out, messages)
	return out
}

func cloneProviderMessages(messages []provider.Message) []provider.Message {
	if messages == nil {
		return nil
	}
	out := make([]provider.Message, len(messages))
	copy(out, messages)
	return out
}

func cloneSystemMessages(messages []SystemModelMessage) []SystemModelMessage {
	if messages == nil {
		return nil
	}
	out := make([]SystemModelMessage, len(messages))
	copy(out, messages)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeStringMaps(base map[string]string, override map[string]string) map[string]string {
	if base == nil && override == nil {
		return nil
	}
	out := cloneStringMap(base)
	if out == nil {
		out = make(map[string]string, len(override))
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func cloneProviderOptions(in provider.ProviderOptions) provider.ProviderOptions {
	if in == nil {
		return nil
	}
	out := make(provider.ProviderOptions, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeProviderOptions(base provider.ProviderOptions, override provider.ProviderOptions) provider.ProviderOptions {
	if base == nil && override == nil {
		return nil
	}
	out := cloneProviderOptions(base)
	if out == nil {
		out = make(provider.ProviderOptions, len(override))
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func mergeCallbacks1[T any](first func(T), second func(T)) func(T) {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(state T) {
		first(state)
		second(state)
	}
}

func appendAgentUserAgent(headers map[string]string) map[string]string {
	if headers == nil {
		headers = map[string]string{}
	}
	key := findHeaderKey(headers, "User-Agent")
	if key == "" {
		key = "User-Agent"
	}
	current := headers[key]
	if current == "" {
		headers[key] = toolLoopAgentUserAgent
		return headers
	}
	if strings.Contains(current, toolLoopAgentUserAgent) {
		return headers
	}
	headers[key] = current + " " + toolLoopAgentUserAgent
	return headers
}

func findHeaderKey(headers map[string]string, name string) string {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return ""
}

// CreateAgentUIStream creates a UIMessageChunk stream from an Agent and UI message history.
//
// The helper validates the current Go UI message model before starting the
// provider stream, converts UI messages to model messages, calls Agent.Stream,
// and returns the existing ToUIMessageStream output with original messages
// preserved for response assembly.
func CreateAgentUIStream(ctx context.Context, agent Agent, messages []UIMessage, opts ...UIMessageStreamOption) (<-chan UIMessageChunk, error) {
	if agent == nil {
		return nil, fmt.Errorf("aisdk: agent is nil")
	}
	if err := validateAgentUIMessages(messages, agent.Tools()); err != nil {
		return nil, err
	}
	modelMessages, err := ConvertToModelMessages(messages, WithTools(agent.Tools()))
	if err != nil {
		return nil, err
	}
	result := agent.Stream(ctx, WithAgentModelMessages(modelMessages...))
	streamOpts := append([]UIMessageStreamOption{}, opts...)
	streamOpts = append(streamOpts, WithUIMessageStreamOriginalMessages(messages...))
	return result.ToUIMessageStream(streamOpts...), nil
}

// WriteAgentUIStream creates an Agent UI stream and writes it as SSE.
func WriteAgentUIStream(w http.ResponseWriter, ctx context.Context, agent Agent, messages []UIMessage, opts ...UIMessageStreamOption) error {
	stream, err := CreateAgentUIStream(ctx, agent, messages, opts...)
	if err != nil {
		return err
	}
	return PipeUIMessageStreamToResponse(w, stream)
}

// PipeAgentUIStreamToResponse writes an already-created Agent UI stream as SSE.
func PipeAgentUIStreamToResponse(w http.ResponseWriter, stream <-chan UIMessageChunk) error {
	return PipeUIMessageStreamToResponse(w, stream)
}

func validateAgentUIMessages(messages []UIMessage, tools ToolSet) error {
	for msgIdx, msg := range messages {
		for partIdx, part := range msg.Parts {
			switch p := part.(type) {
			case ToolInvocationPart:
				if err := validateAgentToolInvocation(toolPartFields(p), false, tools); err != nil {
					return fmt.Errorf("aisdk: validating UI message %d part %d: %w", msgIdx, partIdx, err)
				}
			case DynamicToolUIPart:
				if err := validateAgentToolInvocation(toolPartFields(p), true, tools); err != nil {
					return fmt.Errorf("aisdk: validating UI message %d part %d: %w", msgIdx, partIdx, err)
				}
			}
		}
	}
	return nil
}

func validateAgentToolInvocation(part toolPartFields, dynamic bool, tools ToolSet) error {
	if part.ToolCallID == "" {
		return fmt.Errorf("tool invocation has empty tool call ID")
	}
	if part.ToolName == "" {
		return fmt.Errorf("tool invocation has empty tool name")
	}
	if !dynamic && !part.ProviderExecuted {
		if _, ok := tools[part.ToolName]; !ok {
			return fmt.Errorf("tool %q is not configured on agent", part.ToolName)
		}
	}
	if !isKnownToolInvocationState(part.State) {
		return fmt.Errorf("unknown tool invocation state %q", part.State)
	}
	if toolInvocationStateRequiresInput(part.State) && len(part.Input) == 0 {
		return fmt.Errorf("tool invocation %q in state %q is missing input", part.ToolCallID, part.State)
	}
	switch part.State {
	case ToolStateOutputAvailable:
		if len(part.Output) == 0 {
			return fmt.Errorf("tool invocation %q in state %q is missing output", part.ToolCallID, part.State)
		}
	case ToolStateOutputError:
		if part.ErrorText == "" {
			return fmt.Errorf("tool invocation %q in state %q is missing error text", part.ToolCallID, part.State)
		}
	case ToolStateOutputDenied:
		// No extra fields are required by the current Go model.
	case ToolStateApprovalResponded:
		if part.Approval == nil || part.Approval.ID == "" || part.Approval.Approved == nil {
			return fmt.Errorf("tool invocation %q in state %q is missing approval response", part.ToolCallID, part.State)
		}
	case ToolStateApprovalRequested:
		if part.Approval == nil || part.Approval.ID == "" {
			return fmt.Errorf("tool invocation %q in state %q is missing approval request", part.ToolCallID, part.State)
		}
	}
	return nil
}

func toolInvocationStateRequiresInput(state ToolInvocationState) bool {
	switch state {
	case ToolStateInputAvailable,
		ToolStateApprovalRequested,
		ToolStateApprovalResponded,
		ToolStateOutputAvailable,
		ToolStateOutputDenied:
		return true
	default:
		return false
	}
}

func isKnownToolInvocationState(state ToolInvocationState) bool {
	switch state {
	case ToolStateInputStreaming,
		ToolStateInputAvailable,
		ToolStateApprovalRequested,
		ToolStateApprovalResponded,
		ToolStateOutputAvailable,
		ToolStateOutputError,
		ToolStateOutputDenied:
		return true
	default:
		return false
	}
}
