package aisdk

import (
	"context"
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

// ToolExecuteFunc is the signature for a tool's execution function.
// It receives the parsed input and returns the tool output as JSON.
type ToolExecuteFunc func(ctx context.Context, input json.RawMessage, opts ToolExecutionOptions) (json.RawMessage, error)

// ToolNeedsApprovalFunc decides whether a tool invocation requires user approval.
type ToolNeedsApprovalFunc func(input json.RawMessage, opts ToolExecutionOptions) (bool, error)

// ToolApprovalConfig configures whether a tool invocation requires approval.
type ToolApprovalConfig struct {
	Required bool
	Check    ToolNeedsApprovalFunc
}

// ToolApprovalStatus identifies the approval policy result for a tool call.
type ToolApprovalStatus string

const (
	ToolApprovalNotApplicable ToolApprovalStatus = "not-applicable"
	ToolApprovalUserApproval  ToolApprovalStatus = "user-approval"
	ToolApprovalApproved      ToolApprovalStatus = "approved"
	ToolApprovalDenied        ToolApprovalStatus = "denied"
)

// ToolApprovalDecision is the normalized approval policy result.
type ToolApprovalDecision struct {
	Status ToolApprovalStatus
	Reason string
}

// ToolApprovalOptions carries context to a generic approval policy function.
type ToolApprovalOptions struct {
	ToolCall ToolCall
	Tools    ToolSet
	Messages []provider.Message
	Context  any
}

// ToolApprovalFunc decides approval policy for any tool call in a request.
type ToolApprovalFunc func(ToolApprovalOptions) (ToolApprovalDecision, error)

// SingleToolApprovalFunc decides approval policy for a specific tool.
type SingleToolApprovalFunc func(input json.RawMessage, opts ToolExecutionOptions) (ToolApprovalDecision, error)

// ToolApprovalPolicy configures call-level approval for a single tool.
type ToolApprovalPolicy struct {
	Decision ToolApprovalDecision
	Check    SingleToolApprovalFunc
}

// ToolApprovalMap configures call-level approval policies by tool name.
type ToolApprovalMap map[string]ToolApprovalPolicy

type toolApprovalConfig struct {
	generic ToolApprovalFunc
	tools   ToolApprovalMap
}

// ToolApprovalPolicyConfig is accepted by WithToolApproval.
type ToolApprovalPolicyConfig interface {
	applyToolApproval(*toolApprovalConfig)
}

func (f ToolApprovalFunc) applyToolApproval(c *toolApprovalConfig) {
	c.generic = f
	c.tools = nil
}

func (m ToolApprovalMap) applyToolApproval(c *toolApprovalConfig) {
	c.generic = nil
	c.tools = m
}

// ApprovalPolicy returns a static call-level approval policy.
func ApprovalPolicy(status ToolApprovalStatus, reason ...string) ToolApprovalPolicy {
	policy := ToolApprovalPolicy{Decision: ToolApprovalDecision{Status: status}}
	if len(reason) > 0 {
		policy.Decision.Reason = reason[0]
	}
	return policy
}

// ApprovalPolicyFunc returns a dynamic call-level approval policy.
func ApprovalPolicyFunc(fn SingleToolApprovalFunc) ToolApprovalPolicy {
	return ToolApprovalPolicy{Check: fn}
}

// ApprovalRequired returns a static approval configuration that always requires approval.
func ApprovalRequired() *ToolApprovalConfig {
	return &ToolApprovalConfig{Required: true}
}

// ApprovalIf returns a dynamic approval configuration evaluated per tool call.
func ApprovalIf(fn ToolNeedsApprovalFunc) *ToolApprovalConfig {
	return &ToolApprovalConfig{Check: fn}
}

// ToolExecutionOptions carries context into tool execution.
type ToolExecutionOptions struct {
	ToolCallID string
	Messages   []provider.Message
	Context    any // user-defined context (experimental_context in AI SDK)
}

// ToolOutputContext is passed to Tool.ToModelOutput for converting tool output
// to the provider's expected format.
type ToolOutputContext struct {
	ToolCallID string
	Input      json.RawMessage
	Output     json.RawMessage
}

// UserToolType identifies the kind of user-facing tool.
type UserToolType string

const (
	UserToolFunction UserToolType = "function"
	UserToolProvider UserToolType = "provider"
	UserToolDynamic  UserToolType = "dynamic"
)

// Tool defines a tool that a language model can call.
//
// For function tools (Type "" or UserToolFunction), the tool is defined by the
// user with InputSchema, Execute, etc.
//
// For provider tools (Type UserToolProvider), the tool's schema and behavior
// are defined by the provider. Set ID to the provider tool identifier
// (e.g. "anthropic.web_search_20250305") and Args for tool-specific config.
// Callbacks (OnInputStart, OnInputAvailable, etc.) are still supported.
//
// Execute is optional: nil means tool calls are returned to the caller
// without execution (external/remote tool pattern).
type Tool struct {
	Type UserToolType               // "" or UserToolFunction for user-defined tools, UserToolProvider for provider tools
	ID   string                     // provider tool identifier (e.g. "anthropic.web_search_20250305"), only for provider tools
	Args map[string]json.RawMessage // provider-specific tool configuration, only for provider tools

	Description     string
	Title           string
	InputSchema     schema.Schema
	OutputSchema    schema.Schema
	InputExamples   []json.RawMessage
	Strict          *bool
	ProviderOptions provider.ProviderOptions

	Execute       ToolExecuteFunc
	NeedsApproval *ToolApprovalConfig
	ValidateInput func(input json.RawMessage) error
	ToModelOutput func(ToolOutputContext) (*provider.ToolResultOutput, error)

	OnInputStart     func(ToolExecutionOptions)
	OnInputDelta     func(inputTextDelta string, opts ToolExecutionOptions)
	OnInputAvailable func(input json.RawMessage, opts ToolExecutionOptions)
}

// ToolSet is a named collection of tools. Tools are keyed by name.
type ToolSet map[string]Tool

// ToolCall represents a complete tool call from the model.
type ToolCall struct {
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Input            json.RawMessage           `json:"input"`
	Invalid          bool                      `json:"-"`
	Error            error                     `json:"-"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	Title            string                    `json:"title,omitempty"`
}

// ToolResult represents a tool execution result.
type ToolResult struct {
	ToolCallID       string                     `json:"toolCallId"`
	ToolName         string                     `json:"toolName"`
	Input            json.RawMessage            `json:"input"`
	Output           json.RawMessage            `json:"output"`
	ModelOutput      *provider.ToolResultOutput `json:"-"`
	IsError          bool                       `json:"isError,omitempty"`
	ProviderExecuted bool                       `json:"providerExecuted,omitempty"`
	ProviderMetadata provider.ProviderMetadata  `json:"providerMetadata,omitempty"`
	Dynamic          *bool                      `json:"dynamic,omitempty"`
	Preliminary      bool                       `json:"preliminary,omitempty"`
	Title            string                     `json:"title,omitempty"`
}

// ToolApprovalRequest represents a request for user approval before executing a tool.
type ToolApprovalRequest struct {
	ApprovalID       string                    `json:"approvalId"`
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Input            json.RawMessage           `json:"input,omitempty"`
	Signature        string                    `json:"signature,omitempty"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	Title            string                    `json:"title,omitempty"`
	IsAutomatic      bool                      `json:"isAutomatic,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// ToolApprovalResponse represents a user's approval or denial of a tool request.
type ToolApprovalResponse struct {
	ApprovalID       string                    `json:"approvalId"`
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Approved         bool                      `json:"approved"`
	Reason           string                    `json:"reason,omitempty"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	Title            string                    `json:"title,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}
