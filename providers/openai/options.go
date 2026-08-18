package openai

import (
	"encoding/json"

	"github.com/openai/openai-go/v3/option"
)

// Option configures an OpenAI Responses model instance at construction time.
type Option func(*model)

// WithRequestOptions appends raw SDK request options (e.g., custom headers,
// base URL, HTTP client). Per-call CallOptions.Headers override configured
// headers with the same name. Used by tests to point the model at a
// replay server.
func WithRequestOptions(opts ...option.RequestOption) Option {
	return func(m *model) {
		m.requestOpts = append(m.requestOpts, opts...)
	}
}

// WithGenerateID overrides the ID generator used for synthesized identifiers
// (e.g., source/citation IDs, MCP approval dummy tool-call IDs). Tests inject a
// deterministic generator so conformance output is byte-stable.
func WithGenerateID(fn func() string) Option {
	return func(m *model) {
		if fn != nil {
			m.generateID = fn
		}
	}
}

// OpenAIResponsesOptions carries OpenAI Responses-specific configuration from
// CallOptions.ProviderOptions["openai"]. Field names and semantics mirror
// upstream Vercel AI SDK's openaiLanguageModelResponsesOptionsSchema.
type OpenAIResponsesOptions struct {
	// Conversation is an OpenAI Conversation id. Mutually exclusive with
	// PreviousResponseID.
	Conversation string `json:"conversation,omitempty"`
	// Include lists additional output data to request (e.g.
	// "reasoning.encrypted_content", "file_search_call.results",
	// "message.output_text.logprobs").
	Include []string `json:"include,omitempty"`
	// Instructions overrides the system/developer instructions, separate from
	// message history. Used when continuing via PreviousResponseID.
	Instructions string `json:"instructions,omitempty"`
	// Logprobs requests log probabilities. A bool true maps to the maximum top
	// logprobs; a number sets the count (1..20).
	Logprobs *LogprobsOption `json:"logprobs,omitempty"`
	// MaxToolCalls limits the total number of built-in tool invocations.
	MaxToolCalls *int64 `json:"maxToolCalls,omitempty"`
	// Metadata is a key-value map persisted with stored responses.
	Metadata map[string]string `json:"metadata,omitempty"`
	// ParallelToolCalls controls whether tools may run in parallel.
	ParallelToolCalls *bool `json:"parallelToolCalls,omitempty"`
	// PreviousResponseID continues a previous response without resending history.
	PreviousResponseID string `json:"previousResponseId,omitempty"`
	// PromptCacheKey buckets similar requests for prompt caching.
	PromptCacheKey string `json:"promptCacheKey,omitempty"`
	// PromptCacheRetention is "in_memory" (default) or "24h".
	PromptCacheRetention string `json:"promptCacheRetention,omitempty"`
	// PromptCacheOptions configures GPT-5.6+ prompt cache behavior.
	PromptCacheOptions *PromptCacheOptions `json:"promptCacheOptions,omitempty"`
	// ReasoningEffort controls reasoning effort ("none","minimal","low",
	// "medium","high","xhigh","max"). Validated server-side.
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// ReasoningMode controls GPT-5.6 reasoning work mode ("standard","pro").
	ReasoningMode string `json:"reasoningMode,omitempty"`
	// ReasoningContext controls GPT-5.6 access to prior reasoning items.
	ReasoningContext string `json:"reasoningContext,omitempty"`
	// ReasoningSummary controls reasoning summary output ("auto","concise",
	// "detailed").
	ReasoningSummary string `json:"reasoningSummary,omitempty"`
	// SafetyIdentifier is a stable identifier for end users.
	SafetyIdentifier string `json:"safetyIdentifier,omitempty"`
	// ServiceTier is "auto","flex","priority","fast","default".
	ServiceTier string `json:"serviceTier,omitempty"`
	// Store controls whether the response is persisted. Defaults to true.
	Store *bool `json:"store,omitempty"`
	// PassThroughUnsupportedFiles passes unsupported file parts through rather
	// than erroring.
	PassThroughUnsupportedFiles *bool `json:"passThroughUnsupportedFiles,omitempty"`
	// StrictJSONSchema controls strict JSON schema validation. Defaults to true.
	StrictJSONSchema *bool `json:"strictJsonSchema,omitempty"`
	// TextVerbosity is "low","medium","high".
	TextVerbosity string `json:"textVerbosity,omitempty"`
	// Truncation is "auto" or "disabled".
	Truncation string `json:"truncation,omitempty"`
	// User is a stable end-user identifier (deprecated upstream in favor of
	// SafetyIdentifier/PromptCacheKey).
	User string `json:"user,omitempty"`
	// SystemMessageMode overrides system message handling ("system",
	// "developer","remove"). Auto-detected when unset.
	SystemMessageMode string `json:"systemMessageMode,omitempty"`
	// ForceReasoning forces reasoning-model handling regardless of model id.
	ForceReasoning *bool `json:"forceReasoning,omitempty"`
	// AllowedTools restricts and optionally requires a subset of tools.
	AllowedTools *AllowedToolsOption `json:"allowedTools,omitempty"`
	// ContextManagement configures server-side compaction.
	ContextManagement []ContextManagementEntry `json:"contextManagement,omitempty"`
}

// ProviderKey returns the provider namespace key.
func (OpenAIResponsesOptions) ProviderKey() string { return "openai" }

// LogprobsOption accepts either a boolean or an integer count. A bare boolean
// true requests the maximum number of top logprobs; an integer sets the count.
type LogprobsOption struct {
	Bool *bool
	Int  *int64
}

// UnmarshalJSON decodes a bool or number into LogprobsOption.
func (l *LogprobsOption) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && (data[0] == 't' || data[0] == 'f') {
		var b bool
		if err := json.Unmarshal(data, &b); err != nil {
			return err
		}
		l.Bool = &b
		return nil
	}
	var n int64
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	l.Int = &n
	return nil
}

// MarshalJSON encodes a LogprobsOption back to a bool or number.
func (l LogprobsOption) MarshalJSON() ([]byte, error) {
	if l.Bool != nil {
		return json.Marshal(*l.Bool)
	}
	if l.Int != nil {
		return json.Marshal(*l.Int)
	}
	return []byte("null"), nil
}

// AllowedToolsOption restricts the model to a named subset of tools and
// optionally requires one of them. Overrides the request tool choice.
type AllowedToolsOption struct {
	ToolNames []string `json:"toolNames"`
	Mode      string   `json:"mode,omitempty"` // "auto" or "required"
}

// ContextManagementEntry configures a single server-side compaction policy.
type ContextManagementEntry struct {
	Type             string `json:"type"`
	CompactThreshold *int64 `json:"compactThreshold,omitempty"`
}

// PromptCacheOptions controls GPT-5.6+ prompt cache behavior.
type PromptCacheOptions struct {
	Mode string `json:"mode,omitempty"`
	TTL  string `json:"ttl,omitempty"`
}

// OpenAIAllowedCaller identifies a function-tool invocation context.
type OpenAIAllowedCaller string

const (
	// OpenAIAllowedCallerDirect permits direct model calls.
	OpenAIAllowedCallerDirect OpenAIAllowedCaller = "direct"
	// OpenAIAllowedCallerProgrammatic permits calls from a hosted program.
	OpenAIAllowedCallerProgrammatic OpenAIAllowedCaller = "programmatic"
)

// OpenAIToolOptions carries per-tool OpenAI options attached to a
// provider.Tool via ProviderOptions["openai"].
type OpenAIToolOptions struct {
	// DeferLoading defers function tool loading.
	DeferLoading *bool `json:"deferLoading,omitempty"`
	// AllowedCallers controls direct and programmatic invocation contexts.
	AllowedCallers []OpenAIAllowedCaller `json:"allowedCallers,omitempty"`
	// OutputSchema describes the JSON value encoded in string outputs.
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	// Namespace groups function tools under a shared namespace.
	Namespace *OpenAIToolNamespaceOptions `json:"namespace,omitempty"`
}

// ProviderKey returns the provider namespace key.
func (OpenAIToolOptions) ProviderKey() string { return "openai" }

// OpenAIToolNamespaceOptions groups function tools under an OpenAI namespace.
type OpenAIToolNamespaceOptions struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// OpenAIToolCallerType identifies the context that produced a tool call.
type OpenAIToolCallerType string

const (
	// OpenAIToolCallerDirect identifies a direct model invocation.
	OpenAIToolCallerDirect OpenAIToolCallerType = "direct"
	// OpenAIToolCallerProgram identifies a hosted-program invocation.
	OpenAIToolCallerProgram OpenAIToolCallerType = "program"
)

// OpenAIToolCaller carries programmatic tool-call correlation metadata.
type OpenAIToolCaller struct {
	Type     OpenAIToolCallerType `json:"type"`
	CallerID string               `json:"callerId,omitempty"`
}

// OpenAIPartOptions carries per-content-part OpenAI options used for
// round-tripping item references, reasoning, approvals, image detail, etc.
type OpenAIPartOptions struct {
	// ItemID is the OpenAI output item id used to emit item references.
	ItemID string `json:"itemId,omitempty"`
	// ReasoningEncryptedContent is the encrypted reasoning blob for stateless
	// continuation.
	ReasoningEncryptedContent *string `json:"reasoningEncryptedContent,omitempty"`
	// ApprovalRequestID maps an MCP approval request to a tool call.
	ApprovalRequestID string `json:"approvalRequestId,omitempty"`
	// ApprovalID identifies an approval response.
	ApprovalID string `json:"approvalId,omitempty"`
	// ImageDetail is the detail level for input images ("low","high","auto").
	ImageDetail string `json:"imageDetail,omitempty"`
	// PromptCacheBreakpoint marks this input part as an explicit cache breakpoint.
	PromptCacheBreakpoint *PromptCacheBreakpoint `json:"promptCacheBreakpoint,omitempty"`
	// Namespace is the function-call namespace.
	Namespace string `json:"namespace,omitempty"`
	// Caller identifies the direct or hosted-program invocation context.
	Caller *OpenAIToolCaller `json:"caller,omitempty"`
	// Phase is the message phase ("commentary","final_answer").
	Phase string `json:"phase,omitempty"`
}

// ProviderKey returns the provider namespace key.
func (OpenAIPartOptions) ProviderKey() string { return "openai" }

// PromptCacheBreakpoint marks a message/content part as an explicit cache breakpoint.
type PromptCacheBreakpoint struct {
	Mode string `json:"mode"`
}
