package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
)

// Option configures an Anthropic model instance.
type Option func(*model)

// WithRequestOptions appends raw SDK request options (e.g., custom headers, base URL).
func WithRequestOptions(opts ...option.RequestOption) Option {
	return func(m *model) {
		m.requestOpts = append(m.requestOpts, opts...)
	}
}

// StructuredOutputMode controls how Anthropic JSON response formats are requested.
type StructuredOutputMode string

const (
	StructuredOutputAuto     StructuredOutputMode = "auto"
	StructuredOutputFormat   StructuredOutputMode = "outputFormat"
	StructuredOutputJSONTool StructuredOutputMode = "jsonTool"
)

// AnthropicOptions carries Anthropic-specific configuration from
// CallOptions.ProviderOptions["anthropic"].
type AnthropicOptions struct {
	Thinking               *ThinkingConfig      `json:"thinking,omitempty"`
	StructuredOutputMode   StructuredOutputMode `json:"structuredOutputMode,omitempty"`
	DisableParallelToolUse *bool                `json:"disableParallelToolUse,omitempty"`
	Effort                 string               `json:"effort,omitempty"` // "low", "medium", "high", "xhigh", "max"
	Betas                  []string             `json:"betas,omitempty"`
	MCPServers             []MCPServer          `json:"mcpServers,omitempty"`
	TaskBudget             *TaskBudgetConfig    `json:"taskBudget,omitempty"`
	Container              *Container           `json:"container,omitempty"`
	// ToolStreaming controls whether function tools receive a default
	// `eager_input_streaming: true` on streaming requests. A nil value
	// is treated as true, matching upstream's `?? true` semantics.
	// Per-tool AnthropicToolOptions.EagerInputStreaming always wins over
	// this model-level default.
	ToolStreaming *bool `json:"toolStreaming,omitempty"`
}

func (AnthropicOptions) ProviderKey() string { return "anthropic" }

// MCPServer configures a remote MCP server for tool execution.
type MCPServer struct {
	Name               string                `json:"name"`
	URL                string                `json:"url"`
	AuthorizationToken string                `json:"authorizationToken,omitempty"`
	ToolConfiguration  *MCPToolConfiguration `json:"toolConfiguration,omitempty"`
}

// MCPToolConfiguration controls which tools are available from an MCP server.
type MCPToolConfiguration struct {
	Enabled      bool     `json:"enabled,omitempty"`
	AllowedTools []string `json:"allowedTools,omitempty"`
}

// Container configures Anthropic's code-execution container.
type Container struct {
	ID     string           `json:"id,omitempty"`
	Skills []ContainerSkill `json:"skills,omitempty"`
}

// ContainerSkill configures a skill loaded into an Anthropic container.
type ContainerSkill struct {
	Type    string `json:"type"`
	SkillID string `json:"skillId"`
	Version string `json:"version,omitempty"`
}

// ThinkingType identifies the thinking/reasoning mode.
type ThinkingType string

const (
	ThinkingEnabled  ThinkingType = "enabled"
	ThinkingDisabled ThinkingType = "disabled"
	ThinkingAdaptive ThinkingType = "adaptive"
)

// ThinkingDisplay controls whether thinking content is included in the
// adaptive-thinking response. Newer models (Opus 4.7+) omit thinking text by
// default; set this to ThinkingDisplaySummarized to receive reasoning output.
type ThinkingDisplay string

const (
	// ThinkingDisplaySummarized returns thinking content normally.
	ThinkingDisplaySummarized ThinkingDisplay = "summarized"
	// ThinkingDisplayOmitted redacts the thinking text but keeps a signature
	// for multi-turn continuity.
	ThinkingDisplayOmitted ThinkingDisplay = "omitted"
)

// ThinkingConfig controls the thinking/reasoning mode.
type ThinkingConfig struct {
	Type         ThinkingType `json:"type"`
	BudgetTokens int          `json:"budgetTokens,omitempty"` // required when Type == ThinkingEnabled
	// Display controls visibility of thinking content. Only honored when
	// Type == ThinkingAdaptive (mirrors upstream anthropic-language-model.ts).
	Display ThinkingDisplay `json:"display,omitempty"`
}

// TaskBudgetType identifies the task-budget unit.
type TaskBudgetType string

const (
	// TaskBudgetTokens scopes the budget to model tokens.
	TaskBudgetTokens TaskBudgetType = "tokens"
)

// TaskBudgetConfig describes an advisory token budget for agentic workflows.
// The budget informs the model of the total token budget for the current task
// so it can prioritize work and wind down gracefully. It does not enforce a
// hard limit.
type TaskBudgetConfig struct {
	Type TaskBudgetType `json:"type"`
	// Total is the total token budget for the session. Anthropic's API
	// requires this to be at least 20000 (validated server-side).
	Total int64 `json:"total"`
	// Remaining is the tokens left in the budget. When nil, the API defaults
	// it to Total.
	Remaining *int64 `json:"remaining,omitempty"`
}

// AnthropicToolOptions carries Anthropic-specific configuration from
// tool.ProviderOptions["anthropic"] on individual function tool definitions.
type AnthropicToolOptions struct {
	DeferLoading        *bool             `json:"deferLoading,omitempty"`
	AllowedCallers      []string          `json:"allowedCallers,omitempty"`
	EagerInputStreaming *bool             `json:"eagerInputStreaming,omitempty"`
	CacheControl        *CacheControlType `json:"cacheControl,omitempty"`
}

func (AnthropicToolOptions) ProviderKey() string { return "anthropic" }

// CacheControlType carries the cache control type and optional TTL.
type CacheControlType struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

// AnthropicCacheControl carries per-part cache control configuration.
// Used on message and content part ProviderOptions for cache control only.
type AnthropicCacheControl struct {
	CacheType string
	TTL       string
}

func (AnthropicCacheControl) ProviderKey() string { return "anthropic" }

// CacheControl returns a ProviderOption that configures cache control on a
// content part, message, or tool definition. Common usage:
//
//	provider.ContentPart{Type: provider.ContentPartTypeText,
//	    Text:            "cached context",
//	    ProviderOptions: provider.BuildProviderOptions(anthropic.CacheControl("ephemeral")),
//	}
func CacheControl(cacheType string) provider.ProviderOption {
	return AnthropicCacheControl{CacheType: cacheType}
}

// CacheControlWithTTL returns a ProviderOption with cache control type and TTL.
func CacheControlWithTTL(cacheType, ttl string) provider.ProviderOption {
	return AnthropicCacheControl{CacheType: cacheType, TTL: ttl}
}
