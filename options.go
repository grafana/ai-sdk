package aisdk

import (
	"time"

	"github.com/grafana/ai-sdk/internal/ptr"
	"github.com/grafana/ai-sdk/provider"
)

// TimeoutConfig configures timeout levels for StreamText and GenerateText.
// All fields default to zero (disabled).
type TimeoutConfig struct {
	Total      time.Duration
	Step       time.Duration
	FirstChunk time.Duration
	Chunk      time.Duration
}

// StreamOption configures a StreamText call.
type StreamOption interface {
	applyStream(*streamConfig)
	streamOption()
}

// GenerateOption configures a GenerateText call.
type GenerateOption interface {
	applyGenerate(*generateConfig)
	generateOption()
}

// Option is an option that applies to both StreamText and GenerateText.
type Option interface {
	StreamOption
	GenerateOption
}

// ToolOption configures tools for text generation and UI message conversion.
type ToolOption interface {
	Option
	ConvertOption
}

type baseConfig struct {
	messages           []UIMessage
	modelMessages      []provider.Message
	system             []SystemModelMessage
	tools              ToolSet
	toolChoice         *provider.ToolChoice
	activeTools        []string
	activeToolsSet     bool
	stopWhen           []StopCondition
	toolApproval       toolApprovalConfig
	toolApprovalSecret []byte

	maxRetries     *int
	timeout        TimeoutConfig
	timeoutSet     bool
	retryInitDelay float64
	retryBackoff   float64

	maxOutputTokens  *provider.LanguageModelNumber
	temperature      *float64
	topP             *float64
	topK             *provider.LanguageModelNumber
	presencePenalty  *float64
	frequencyPenalty *float64
	stopSequences    []string
	responseFormat   *provider.ResponseFormat
	seed             *provider.LanguageModelNumber
	providerOptions  provider.ProviderOptions
	headers          map[string]string
	generateID       func() string

	onStepFinish     func(OnStepFinishState)
	onFinish         func(OnFinishState)
	onError          func(error)
	onStart          func(OnStartState)
	onStepStart      func(OnStepStartState)
	onToolCallStart  func(OnToolCallStartState)
	onToolCallFinish func(OnToolCallFinishState)

	reasoning      *provider.ReasoningEffort
	prepareStep    PrepareStepFunc
	runtimeContext any
	output         Output
}

type streamConfig struct {
	baseConfig
	onChunk              func(OnChunkState)
	onAbort              func(OnAbortState)
	includeRawChunks     *bool
	parseOutputOnNonStop bool
}

type generateConfig struct {
	baseConfig
}

func (gc *generateConfig) toStreamConfig() *streamConfig {
	bc := gc.baseConfig
	bc.timeout.FirstChunk = 0
	bc.timeout.Chunk = 0
	return &streamConfig{baseConfig: bc}
}

func buildStreamConfig(opts []StreamOption) *streamConfig {
	cfg := &streamConfig{parseOutputOnNonStop: true}
	for _, opt := range opts {
		opt.applyStream(cfg)
	}
	return cfg
}

func buildGenerateConfig(opts []GenerateOption) *generateConfig {
	cfg := &generateConfig{}
	for _, opt := range opts {
		opt.applyGenerate(cfg)
	}
	return cfg
}

func generateTimeoutWarnings(timeout TimeoutConfig) []provider.Warning {
	var warnings []provider.Warning
	if timeout.FirstChunk > 0 {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "timeout.firstChunkMs",
			Details: "The firstChunkMs timeout is only supported by streaming functions.",
		})
	}
	if timeout.Chunk > 0 {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "timeout.chunkMs",
			Details: "The chunkMs timeout is only supported by streaming functions.",
		})
	}
	return warnings
}

// sharedOption implements Option (both StreamOption and GenerateOption).
type sharedOption struct {
	fn func(*baseConfig)
}

func (o sharedOption) applyStream(c *streamConfig)     { o.fn(&c.baseConfig) }
func (o sharedOption) applyGenerate(c *generateConfig) { o.fn(&c.baseConfig) }
func (sharedOption) streamOption()                     {}
func (sharedOption) generateOption()                   {}

// streamOnlyOption implements only StreamOption.
type streamOnlyOption struct {
	fn func(*streamConfig)
}

func (o streamOnlyOption) applyStream(c *streamConfig) { o.fn(c) }
func (streamOnlyOption) streamOption()                 {}

// generateOnlyOption implements only GenerateOption.
type generateOnlyOption struct {
	fn func(*generateConfig)
}

func (o generateOnlyOption) applyGenerate(c *generateConfig) { o.fn(c) }
func (generateOnlyOption) generateOption()                   {}

// --- Message options ---

// WithMessages sets the UI messages for the conversation.
func WithMessages(msgs ...UIMessage) Option {
	return sharedOption{fn: func(c *baseConfig) { c.messages = msgs }}
}

// WithModelMessages sets provider messages directly, bypassing UI message conversion.
func WithModelMessages(msgs ...provider.Message) Option {
	return sharedOption{fn: func(c *baseConfig) { c.modelMessages = msgs }}
}

// WithSystem sets a simple text system prompt.
func WithSystem(text string) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.system = []SystemModelMessage{{Content: text}}
	}}
}

// WithInstructions sets a simple text instruction prompt.
func WithInstructions(text string) Option {
	return WithSystem(text)
}

// WithSystemMessages sets multiple system prompt segments.
func WithSystemMessages(msgs ...SystemModelMessage) Option {
	return sharedOption{fn: func(c *baseConfig) { c.system = msgs }}
}

// --- Retry and timeout options ---

// WithMaxRetries sets the maximum number of retry attempts for transient
// errors. Default is 2 (up to 3 total attempts). Set to 0 to disable retry.
func WithMaxRetries(n int) Option {
	return sharedOption{fn: func(c *baseConfig) { c.maxRetries = &n }}
}

// WithTimeout sets the timeout configuration for the operation.
func WithTimeout(cfg TimeoutConfig) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.timeout = cfg
		c.timeoutSet = true
	}}
}

// --- Model parameter options ---

// WithTemperature sets the sampling temperature.
func WithTemperature(t float64) Option {
	return sharedOption{fn: func(c *baseConfig) { c.temperature = &t }}
}

// WithMaxOutputTokens sets the maximum number of output tokens.
func WithMaxOutputTokens(n int) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.maxOutputTokens = ptr.To(provider.LanguageModelNumberFromInt(n))
	}}
}

// WithTopP sets the top-p (nucleus) sampling parameter.
func WithTopP(p float64) Option {
	return sharedOption{fn: func(c *baseConfig) { c.topP = &p }}
}

// WithTopK sets the top-k sampling parameter.
func WithTopK(k int) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.topK = ptr.To(provider.LanguageModelNumberFromInt(k))
	}}
}

// WithSeed sets the random seed for deterministic sampling.
func WithSeed(s int) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.seed = ptr.To(provider.LanguageModelNumberFromInt(s))
	}}
}

// WithPresencePenalty sets the presence penalty.
func WithPresencePenalty(p float64) Option {
	return sharedOption{fn: func(c *baseConfig) { c.presencePenalty = &p }}
}

// WithFrequencyPenalty sets the frequency penalty.
func WithFrequencyPenalty(f float64) Option {
	return sharedOption{fn: func(c *baseConfig) { c.frequencyPenalty = &f }}
}

// WithStopSequences sets the stop sequences.
func WithStopSequences(seqs ...string) Option {
	return sharedOption{fn: func(c *baseConfig) { c.stopSequences = seqs }}
}

// WithGenerateID sets the ID generator used by orchestration-created IDs.
func WithGenerateID(fn func() string) Option {
	return sharedOption{fn: func(c *baseConfig) { c.generateID = fn }}
}

// --- Tool options ---

// WithTools sets the available tools.
func WithTools(tools ToolSet) ToolOption {
	return toolsOption{tools: tools}
}

type toolsOption struct {
	tools ToolSet
}

func (o toolsOption) applyStream(c *streamConfig)     { c.tools = o.tools }
func (o toolsOption) applyGenerate(c *generateConfig) { c.tools = o.tools }
func (o toolsOption) applyConvert(c *convertConfig)   { c.tools = o.tools }
func (toolsOption) streamOption()                     {}
func (toolsOption) generateOption()                   {}
func (toolsOption) convertOption()                    {}

// WithToolApproval sets call-level tool approval policy. It takes precedence
// over per-tool NeedsApproval configuration. If called more than once, the
// latest generic or per-tool policy replaces the previous one.
func WithToolApproval(policy ToolApprovalPolicyConfig) Option {
	return sharedOption{fn: func(c *baseConfig) {
		if policy != nil {
			policy.applyToolApproval(&c.toolApproval)
		}
	}}
}

// WithToolApprovalSecret sets the secret used to sign and verify tool approval requests.
func WithToolApprovalSecret(secret string) Option {
	return sharedOption{fn: func(c *baseConfig) {
		if secret != "" {
			c.toolApprovalSecret = []byte(secret)
		}
	}}
}

// WithToolApprovalSecretBytes sets the byte secret used to sign and verify tool approval requests.
func WithToolApprovalSecretBytes(secret []byte) Option {
	return sharedOption{fn: func(c *baseConfig) {
		if len(secret) > 0 {
			c.toolApprovalSecret = append([]byte(nil), secret...)
		}
	}}
}

// WithToolChoice sets the tool choice strategy.
func WithToolChoice(tc provider.ToolChoice) Option {
	return sharedOption{fn: func(c *baseConfig) { c.toolChoice = &tc }}
}

// WithActiveTools filters which tools are active for a call.
func WithActiveTools(names ...string) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.activeTools = names
		c.activeToolsSet = true
	}}
}

// WithStopWhen sets stop conditions for the multi-step loop.
func WithStopWhen(conditions ...StopCondition) Option {
	return sharedOption{fn: func(c *baseConfig) { c.stopWhen = conditions }}
}

// --- Provider integration options ---

// WithProviderOptions sets provider-specific options from typed values.
// Each value's ProviderKey() determines its map key; last value wins per key.
func WithProviderOptions(opts ...provider.ProviderOption) Option {
	return sharedOption{fn: func(c *baseConfig) {
		c.providerOptions = provider.BuildProviderOptions(opts...)
	}}
}

// WithHeaders sets additional request headers.
func WithHeaders(headers map[string]string) Option {
	return sharedOption{fn: func(c *baseConfig) { c.headers = headers }}
}

// WithResponseFormat sets the response format.
func WithResponseFormat(f provider.ResponseFormat) Option {
	return sharedOption{fn: func(c *baseConfig) { c.responseFormat = &f }}
}

// --- Shared callback options ---

// OnStart sets a callback invoked when the stream starts.
func OnStart(fn func(OnStartState)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onStart = fn }}
}

// OnStepStart sets a callback invoked at the start of each step.
func OnStepStart(fn func(OnStepStartState)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onStepStart = fn }}
}

// OnStepFinish sets a callback invoked when a step completes.
func OnStepFinish(fn func(OnStepFinishState)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onStepFinish = fn }}
}

// OnStepEnd sets a callback invoked when a step completes.
func OnStepEnd(fn func(OnStepFinishState)) Option {
	return OnStepFinish(fn)
}

// OnFinish sets a callback invoked once after all steps complete successfully.
func OnFinish(fn func(OnFinishState)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onFinish = fn }}
}

// OnError sets a callback invoked on errors.
func OnError(fn func(error)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onError = fn }}
}

// OnToolCallStart sets a callback invoked before a tool is executed.
func OnToolCallStart(fn func(OnToolCallStartState)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onToolCallStart = fn }}
}

// OnToolCallFinish sets a callback invoked after a tool finishes execution.
func OnToolCallFinish(fn func(OnToolCallFinishState)) Option {
	return sharedOption{fn: func(c *baseConfig) { c.onToolCallFinish = fn }}
}

// --- Stream-only options ---

// OnChunk sets a callback invoked for each streaming chunk. Only available for StreamText.
func OnChunk(fn func(OnChunkState)) StreamOption {
	return streamOnlyOption{fn: func(c *streamConfig) { c.onChunk = fn }}
}

// WithIncludeRawChunks enables raw provider chunks in the stream. Only available for StreamText.
func WithIncludeRawChunks() StreamOption {
	return streamOnlyOption{fn: func(c *streamConfig) {
		value := true
		c.includeRawChunks = &value
	}}
}

// OnAbort sets a callback invoked when the stream is cancelled via context. Only available for StreamText.
func OnAbort(fn func(OnAbortState)) StreamOption {
	return streamOnlyOption{fn: func(c *streamConfig) { c.onAbort = fn }}
}

// --- Model behavior options ---

// WithReasoning sets the reasoning effort level for the model.
func WithReasoning(level provider.ReasoningEffort) Option {
	return sharedOption{fn: func(c *baseConfig) { c.reasoning = &level }}
}

// --- Advanced options ---

// WithPrepareStep sets a per-step preparation callback.
func WithPrepareStep(fn PrepareStepFunc) Option {
	return sharedOption{fn: func(c *baseConfig) { c.prepareStep = fn }}
}

// WithOutput sets structured output processing.
func WithOutput(out Output) Option {
	return sharedOption{fn: func(c *baseConfig) { c.output = out }}
}
