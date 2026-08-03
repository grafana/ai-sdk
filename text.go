package aisdk

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
)

// SystemModelMessage represents a segment of the system prompt.
type SystemModelMessage struct {
	Content         string                   `json:"content"`
	ProviderOptions provider.ProviderOptions `json:"providerOptions,omitempty"`
}

// ContentPart is the interface for structured content in a StepResult.
// NOT the same as UIMessage Part -- this is the model output side.
// Consumers type-switch on: TextContent, ReasoningContent, SourceContent,
// FileContent, ReasoningFileContent, ToolCallContent, ToolApprovalRequestContent, ToolApprovalResponseContent,
// ToolResultContent.
type ContentPart interface {
	contentPart()
}

// TextContent represents generated text content.
type TextContent struct {
	Text             string                    `json:"text"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (TextContent) contentPart() {}

// ReasoningContent represents generated reasoning text.
type ReasoningContent struct {
	Text             string                    `json:"text"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (ReasoningContent) contentPart() {}

// SourceContent represents a reference source.
type SourceContent struct {
	Source Source `json:"source"`
}

func (SourceContent) contentPart() {}

// FileContent represents a generated file.
type FileContent struct {
	File             GeneratedFile             `json:"file"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (FileContent) contentPart() {}

// ReasoningFileContent represents a file generated as part of model reasoning.
type ReasoningFileContent struct {
	File             GeneratedFile             `json:"file"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (ReasoningFileContent) contentPart() {}

// ToolCallContent represents a tool call in the generated content.
type ToolCallContent struct {
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Input            json.RawMessage           `json:"input"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	Title            string                    `json:"title,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (ToolCallContent) contentPart() {}

// ToolApprovalRequestContent represents a request for approval before tool execution.
type ToolApprovalRequestContent struct {
	ToolApprovalRequest
}

func (ToolApprovalRequestContent) contentPart() {}

// ToolApprovalResponseContent represents an approval decision for a tool call.
type ToolApprovalResponseContent struct {
	ToolApprovalResponse
}

func (ToolApprovalResponseContent) contentPart() {}

// ToolResultContent represents a tool execution result in the generated content.
type ToolResultContent struct {
	ToolCallID       string                    `json:"toolCallId"`
	ToolName         string                    `json:"toolName"`
	Input            json.RawMessage           `json:"input"`
	Output           json.RawMessage           `json:"output"`
	ProviderExecuted bool                      `json:"providerExecuted,omitempty"`
	Dynamic          *bool                     `json:"dynamic,omitempty"`
	Preliminary      bool                      `json:"preliminary,omitempty"`
	Title            string                    `json:"title,omitempty"`
	ProviderMetadata provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

func (ToolResultContent) contentPart() {}

// ResponseMetadata carries metadata about the provider response.
//
// Messages holds the assistant + tool messages produced from this step's
// content via [ToResponseMessages]. It mirrors upstream's
// `result.response.messages` semantics — the next-call message tail that
// consumers can append to their prompt to continue the conversation. The
// field is in-process only (json:"-") and is not transported on any wire.
type ResponseMetadata struct {
	provider.ResponseMetadata
	Headers  map[string]string  `json:"headers,omitempty"`
	Messages []provider.Message `json:"-"`
}

// StepModel identifies which provider and model were used for a step.
type StepModel struct {
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
}

// StepType identifies the kind of orchestration step.
type StepType string

const (
	StepTypeInitial    StepType = "initial"
	StepTypeToolResult StepType = "tool-result"
)

// StepResult contains per-step data from the StreamText orchestration loop.
type StepResult struct {
	StepNumber            int               `json:"stepNumber"`
	StepType              StepType          `json:"stepType"`
	Model                 StepModel         `json:"model"`
	Text                  string            `json:"text"`
	ReasoningText         string            `json:"reasoningText,omitempty"`
	Reasoning             []ReasoningOutput `json:"reasoning,omitempty"`
	Content               []ContentPart     `json:"-"`
	responseContent       []provider.ContentPart
	ToolCalls             []ToolCall                `json:"toolCalls,omitempty"`
	ToolApprovalRequests  []ToolApprovalRequest     `json:"toolApprovalRequests,omitempty"`
	ToolApprovalResponses []ToolApprovalResponse    `json:"toolApprovalResponses,omitempty"`
	ToolResults           []ToolResult              `json:"toolResults,omitempty"`
	Files                 []GeneratedFile           `json:"files,omitempty"`
	Sources               []Source                  `json:"sources,omitempty"`
	FinishReason          provider.FinishReason     `json:"finishReason"`
	IsContinued           bool                      `json:"isContinued,omitempty"`
	Usage                 provider.Usage            `json:"usage"`
	Warnings              []provider.Warning        `json:"warnings,omitempty"`
	Request               provider.RequestMetadata  `json:"request,omitempty"`
	Response              ResponseMetadata          `json:"response,omitempty"`
	ProviderMetadata      provider.ProviderMetadata `json:"providerMetadata,omitempty"`
}

// OnStartState is passed to the OnStart callback.
type OnStartState struct{}

// OnStepStartState is passed to the OnStepStart callback.
type OnStepStartState struct {
	StepNumber int
	Model      StepModel
}

// OnStepFinishState is passed to the OnStepFinish callback.
type OnStepFinishState struct {
	StepResult
}

// OnFinishState is passed to the OnFinish callback after all steps complete.
type OnFinishState struct {
	StepResult
	Steps      []StepResult
	TotalUsage provider.Usage
}

// OnAbortState is passed to the OnAbort callback when the stream is cancelled.
type OnAbortState struct {
	Steps []StepResult
}

// OnChunkState is passed to the OnChunk callback.
type OnChunkState struct {
	Chunk TextStreamPart
}

// OnToolCallStartState is passed to the OnToolCallStart callback.
type OnToolCallStartState struct {
	StepNumber int
	Model      StepModel
	ToolCall   ToolCall
	Messages   []provider.Message
}

// OnToolCallFinishState is passed to the OnToolCallFinish callback.
// Either Output+Success=true or Error+Success=false.
type OnToolCallFinishState struct {
	StepNumber int
	Model      StepModel
	ToolCall   ToolCall
	Messages   []provider.Message
	DurationMs int64
	Success    bool
	Output     json.RawMessage
	Error      error
}

// StopConditionState is passed to stop condition functions.
type StopConditionState struct {
	Steps []StepResult
}

// StopCondition is a function that determines whether the step loop should stop.
type StopCondition func(state StopConditionState) bool

// StepCountIs returns a StopCondition that stops after n steps.
func StepCountIs(n int) StopCondition {
	return func(state StopConditionState) bool {
		return len(state.Steps) >= n
	}
}

// HasToolCall returns a StopCondition that stops when the named tool is called.
func HasToolCall(toolName string) StopCondition {
	return func(state StopConditionState) bool {
		if len(state.Steps) == 0 {
			return false
		}
		last := state.Steps[len(state.Steps)-1]
		for _, tc := range last.ToolCalls {
			if tc.ToolName == toolName {
				return true
			}
		}
		return false
	}
}

// PrepareStepState is passed to the PrepareStep callback.
type PrepareStepState struct {
	Steps      []StepResult
	StepNumber int
	Model      provider.LanguageModel
	// Messages contains the provider messages for the current step. PrepareStep
	// message overrides carry forward to later steps.
	Messages []provider.Message
	// InitialMessages contains the original input messages before configured
	// system messages and approval-generated tool results are added.
	InitialMessages []provider.Message
	// ResponseMessages contains response messages accumulated before the current
	// step, including approval-generated tool results from the initial input.
	ResponseMessages []provider.Message
}

// PrepareStepFunc is a callback that customizes each step before the provider call.
type PrepareStepFunc func(PrepareStepState) (*PrepareStepResult, error)

// PrepareStepResult contains per-step overrides from PrepareStep.
type PrepareStepResult struct {
	Model            provider.LanguageModel
	System           []SystemModelMessage
	ToolChoice       *provider.ToolChoice
	ActiveTools      []string
	Messages         []provider.Message
	ProviderOptions  provider.ProviderOptions
	Context          any
	MaxOutputTokens  *int
	Temperature      *float64
	TopP             *float64
	TopK             *int
	PresencePenalty  *float64
	FrequencyPenalty *float64
	StopSequences    []string
	Seed             *int
	Reasoning        *provider.ReasoningEffort
}
