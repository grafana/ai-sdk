//go:build conformance

package conformance

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/output"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// --- Config ---

type Operation string

const (
	OperationStream   Operation = "stream"
	OperationGenerate Operation = "generate"
)

type Config struct {
	Operation         Operation                     `yaml:"operation,omitempty"`
	Model             string                        `yaml:"model"`
	System            string                        `yaml:"system,omitempty"`
	Prompt            string                        `yaml:"prompt,omitempty"`
	Messages          []MessageConfig               `yaml:"messages,omitempty"`
	UIMessages        []UIMessageConfig             `yaml:"uiMessages,omitempty"`
	StopWhenStepCount int                           `yaml:"stopWhenStepCount,omitempty"`
	ToolChoice        *ToolChoiceConfig             `yaml:"toolChoice,omitempty"`
	ActiveTools       []string                      `yaml:"activeTools,omitempty"`
	Reasoning         provider.ReasoningEffort      `yaml:"reasoning,omitempty"`
	Headers           map[string]string             `yaml:"headers,omitempty"`
	StreamOptions     *StreamOptionsConfig          `yaml:"streamOptions,omitempty"`
	ProviderOptions   map[string]any                `yaml:"providerOptions,omitempty"`
	Tools             map[string]ToolConfig         `yaml:"tools,omitempty"`
	ProviderTools     map[string]ProviderToolConfig `yaml:"providerTools,omitempty"`
	ResponseFormat    *ResponseFormatConfig         `yaml:"responseFormat,omitempty"`
	AssertOutputValue bool                          `yaml:"assertOutputValue,omitempty"`
	Approval          *ApprovalConfig               `yaml:"approval,omitempty"`
	Approvals         []ApprovalConfig              `yaml:"approvals,omitempty"`
	ExpectStreamError bool                          `yaml:"expectStreamError,omitempty"`
	SkipReason        string                        `yaml:"skipReason,omitempty"`
	MaxRetries        *int                          `yaml:"maxRetries,omitempty"`
}

type UIMessageConfig struct {
	ID    string           `json:"id" yaml:"id,omitempty"`
	Role  provider.Role    `json:"role" yaml:"role"`
	Parts []map[string]any `json:"parts" yaml:"parts"`
}

type MessageConfig struct {
	Role            provider.Role       `yaml:"role"`
	ContentText     string              `yaml:"-"`
	ContentParts    []MessagePartConfig `yaml:"-"`
	ProviderOptions map[string]any      `yaml:"providerOptions,omitempty"`
}

type MessagePartConfig struct {
	Type             provider.ContentPartType `yaml:"type"`
	Text             string                   `yaml:"text,omitempty"`
	Data             string                   `yaml:"data,omitempty"`
	URL              string                   `yaml:"url,omitempty"`
	MediaType        string                   `yaml:"mediaType,omitempty"`
	Filename         string                   `yaml:"filename,omitempty"`
	Reference        map[string]string        `yaml:"reference,omitempty"`
	ToolCallID       string                   `yaml:"toolCallId,omitempty"`
	ToolName         string                   `yaml:"toolName,omitempty"`
	ApprovalID       string                   `yaml:"approvalId,omitempty"`
	Input            any                      `yaml:"input,omitempty"`
	Output           any                      `yaml:"output,omitempty"`
	Approved         bool                     `yaml:"approved,omitempty"`
	Reason           string                   `yaml:"reason,omitempty"`
	IsAutomatic      bool                     `yaml:"isAutomatic,omitempty"`
	ProviderExecuted bool                     `yaml:"providerExecuted,omitempty"`
	ProviderOptions  map[string]any           `yaml:"providerOptions,omitempty"`
}

type ToolConfig struct {
	Description     string                 `yaml:"description"`
	InputSchema     any                    `yaml:"inputSchema"`
	MockResults     []any                  `yaml:"mockResults,omitempty"`
	MockError       string                 `yaml:"mockError,omitempty"`
	ModelOutput     *ToolModelOutputConfig `yaml:"modelOutput,omitempty"`
	ProviderOptions map[string]any         `yaml:"providerOptions,omitempty"`
	NeedsApproval   bool                   `yaml:"needsApproval,omitempty"`
	Strict          *bool                  `yaml:"strict,omitempty"`
}

type ToolModelOutputConfig struct {
	Type    provider.ToolResultOutputType `yaml:"type"`
	Text    string                        `yaml:"text,omitempty"`
	Content []ToolModelOutputContent      `yaml:"content,omitempty"`
}

type ToolModelOutputContent struct {
	Type      provider.ToolResultContentType `yaml:"type"`
	Text      string                         `yaml:"text,omitempty"`
	Data      string                         `yaml:"data,omitempty"`
	MediaType string                         `yaml:"mediaType,omitempty"`
	Filename  string                         `yaml:"filename,omitempty"`
}

type ToolChoiceConfig struct {
	Type     provider.ToolChoiceType `yaml:"type"`
	ToolName string                  `yaml:"toolName,omitempty"`
}

type StreamOptionsConfig struct {
	SendReasoning *bool `yaml:"sendReasoning,omitempty"`
	SendSources   *bool `yaml:"sendSources,omitempty"`
	SendFinish    *bool `yaml:"sendFinish,omitempty"`
	SendStart     *bool `yaml:"sendStart,omitempty"`
}

type ApprovalConfig struct {
	ToolCallID string `yaml:"toolCallId"`
	ToolName   string `yaml:"toolName"`
	ApprovalID string `yaml:"approvalId"`
	Approved   bool   `yaml:"approved"`
	Reason     string `yaml:"reason,omitempty"`
	Input      any    `yaml:"input,omitempty"`
}

func (mc *MessageConfig) UnmarshalYAML(value *yaml.Node) error {
	var aux struct {
		Role            provider.Role  `yaml:"role"`
		Content         yaml.Node      `yaml:"content"`
		ProviderOptions map[string]any `yaml:"providerOptions,omitempty"`
	}
	if err := value.Decode(&aux); err != nil {
		return err
	}
	mc.Role = aux.Role
	mc.ProviderOptions = aux.ProviderOptions
	switch aux.Content.Kind {
	case yaml.ScalarNode:
		mc.ContentText = aux.Content.Value
	case yaml.SequenceNode:
		var parts []MessagePartConfig
		if err := aux.Content.Decode(&parts); err != nil {
			return err
		}
		mc.ContentParts = parts
	default:
		return fmt.Errorf("unsupported message content node kind %d", aux.Content.Kind)
	}
	return nil
}

type ProviderToolConfig struct {
	ID              string         `yaml:"id"`
	Args            map[string]any `yaml:"args,omitempty"`
	InputSchema     any            `yaml:"inputSchema,omitempty"`
	ProviderOptions map[string]any `yaml:"providerOptions,omitempty"`
}

type ResponseFormatConfig struct {
	Type        string   `yaml:"type"`
	OutputMode  string   `yaml:"outputMode,omitempty"`
	Schema      any      `yaml:"schema"`
	Choices     []string `yaml:"choices,omitempty"`
	Name        string   `yaml:"name,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if cfg.Operation == "" {
		cfg.Operation = OperationStream
	}
	if cfg.Operation != OperationStream && cfg.Operation != OperationGenerate {
		return nil, fmt.Errorf("unknown operation %q", cfg.Operation)
	}
	if cfg.Operation == OperationGenerate {
		var unsupported []string
		if len(cfg.UIMessages) > 0 {
			unsupported = append(unsupported, "uiMessages")
		}
		if len(cfg.Tools) > 0 {
			unsupported = append(unsupported, "tools")
		}
		if len(cfg.ProviderTools) > 0 {
			unsupported = append(unsupported, "providerTools")
		}
		if cfg.ToolChoice != nil {
			unsupported = append(unsupported, "toolChoice")
		}
		if len(cfg.ActiveTools) > 0 {
			unsupported = append(unsupported, "activeTools")
		}
		if cfg.Reasoning != "" {
			unsupported = append(unsupported, "reasoning")
		}
		if cfg.StreamOptions != nil {
			unsupported = append(unsupported, "streamOptions")
		}
		if cfg.Approval != nil {
			unsupported = append(unsupported, "approval")
		}
		if len(cfg.Approvals) > 0 {
			unsupported = append(unsupported, "approvals")
		}
		if cfg.AssertOutputValue {
			unsupported = append(unsupported, "assertOutputValue")
		}
		if cfg.ExpectStreamError {
			unsupported = append(unsupported, "expectStreamError")
		}
		if cfg.MaxRetries != nil {
			unsupported = append(unsupported, "maxRetries")
		}
		if cfg.StopWhenStepCount > 1 {
			unsupported = append(unsupported, "stopWhenStepCount")
		}
		if len(unsupported) > 0 {
			return nil, fmt.Errorf("operation generate does not support: %s", strings.Join(unsupported, ", "))
		}
	}
	if cfg.StopWhenStepCount == 0 {
		cfg.StopWhenStepCount = 1
	}
	return &cfg, nil
}

func (cfg *Config) BuildResponseFormat() (*provider.ResponseFormat, error) {
	if cfg.ResponseFormat == nil {
		return nil, nil
	}
	schema, err := json.Marshal(cfg.ResponseFormat.Schema)
	if err != nil {
		return nil, fmt.Errorf("marshaling response format schema: %w", err)
	}
	return &provider.ResponseFormat{
		Type:        provider.ResponseFormatType(cfg.ResponseFormat.Type),
		Schema:      schema,
		Name:        cfg.ResponseFormat.Name,
		Description: cfg.ResponseFormat.Description,
	}, nil
}

func (cfg *Config) BuildOutput() (aisdk.Output, error) {
	if cfg.ResponseFormat == nil {
		return nil, nil
	}
	var opts []output.ObjectOption
	if cfg.ResponseFormat.Name != "" {
		opts = append(opts, output.WithName(cfg.ResponseFormat.Name))
	}
	if cfg.ResponseFormat.Description != "" {
		opts = append(opts, output.WithDescription(cfg.ResponseFormat.Description))
	}
	switch cfg.ResponseFormat.OutputMode {
	case "", "object":
		s, err := cfg.outputSchema()
		if err != nil {
			return nil, err
		}
		return output.Object[any](s, opts...)
	case "array":
		s, err := cfg.outputSchema()
		if err != nil {
			return nil, err
		}
		return output.Array[any](s, opts...)
	case "choice":
		return output.ChoiceWithOptions(cfg.ResponseFormat.Choices, opts...)
	case "json":
		return output.JSON(opts...), nil
	default:
		return nil, fmt.Errorf("unknown responseFormat outputMode %q", cfg.ResponseFormat.OutputMode)
	}
}

func (cfg *Config) outputSchema() (schema.Schema, error) {
	raw, err := json.Marshal(cfg.ResponseFormat.Schema)
	if err != nil {
		return schema.Schema{}, fmt.Errorf("marshaling output schema: %w", err)
	}
	s, err := schema.SchemaFromJSON(raw)
	if err != nil {
		return schema.Schema{}, fmt.Errorf("compiling output schema: %w", err)
	}
	return s, nil
}

func (cfg *Config) BuildProviderOptions() ([]provider.ProviderOption, error) {
	if cfg.ProviderOptions == nil {
		return nil, nil
	}
	var result []provider.ProviderOption
	for k, v := range cfg.ProviderOptions {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling provider option %q: %w", k, err)
		}
		result = append(result, provider.RawProviderOption{Key: k, Raw: data})
	}
	return result, nil
}

func (cfg *Config) BuildToolChoice() *provider.ToolChoice {
	if cfg.ToolChoice == nil {
		return nil
	}
	return &provider.ToolChoice{
		Type:     cfg.ToolChoice.Type,
		ToolName: cfg.ToolChoice.ToolName,
	}
}

func (cfg *Config) BuildUIMessageStreamOptions() []aisdk.UIMessageStreamOption {
	if cfg.StreamOptions == nil {
		return nil
	}
	var opts []aisdk.UIMessageStreamOption
	if cfg.StreamOptions.SendReasoning != nil {
		opts = append(opts, aisdk.WithUIMessageStreamReasoning(*cfg.StreamOptions.SendReasoning))
	}
	if cfg.StreamOptions.SendSources != nil {
		opts = append(opts, aisdk.WithUIMessageStreamSources(*cfg.StreamOptions.SendSources))
	}
	if cfg.StreamOptions.SendFinish != nil {
		opts = append(opts, aisdk.WithUIMessageStreamFinish(*cfg.StreamOptions.SendFinish))
	}
	if cfg.StreamOptions.SendStart != nil {
		opts = append(opts, aisdk.WithUIMessageStreamStart(*cfg.StreamOptions.SendStart))
	}
	return opts
}

func (cfg *Config) BuildToolSet() (aisdk.ToolSet, error) {
	if cfg.Tools == nil && cfg.ProviderTools == nil {
		return nil, nil
	}
	tools := make(aisdk.ToolSet, len(cfg.Tools)+len(cfg.ProviderTools))
	for name, tc := range cfg.Tools {
		tool, err := tc.buildTool(name)
		if err != nil {
			return nil, fmt.Errorf("building tool %q: %w", name, err)
		}
		tools[name] = tool
	}
	for name, ptc := range cfg.ProviderTools {
		tool, err := ptc.buildTool()
		if err != nil {
			return nil, fmt.Errorf("building provider tool %q: %w", name, err)
		}
		tools[name] = tool
	}
	return tools, nil
}

func (cfg *Config) buildStreamOptions(messages []provider.Message, tools aisdk.ToolSet, providerOpts []provider.ProviderOption, stopConditions []aisdk.StopCondition, out aisdk.Output, responseFormat *provider.ResponseFormat) []aisdk.StreamOption {
	streamOpts := []aisdk.StreamOption{
		aisdk.WithModelMessages(messages...),
		aisdk.WithTools(tools),
		aisdk.WithStopWhen(stopConditions...),
		aisdk.WithProviderOptions(providerOpts...),
		aisdk.WithGenerateID(conformanceIDGenerator("id")),
	}
	if cfg.System != "" {
		streamOpts = append(streamOpts, aisdk.WithSystem(cfg.System))
	}
	if toolChoice := cfg.BuildToolChoice(); toolChoice != nil {
		streamOpts = append(streamOpts, aisdk.WithToolChoice(*toolChoice))
	}
	if len(cfg.ActiveTools) > 0 {
		streamOpts = append(streamOpts, aisdk.WithActiveTools(cfg.ActiveTools...))
	}
	if cfg.Reasoning != "" {
		streamOpts = append(streamOpts, aisdk.WithReasoning(cfg.Reasoning))
	}
	if len(cfg.Headers) > 0 {
		streamOpts = append(streamOpts, aisdk.WithHeaders(cfg.Headers))
	}
	if out != nil {
		streamOpts = append(streamOpts, aisdk.WithOutput(out))
	} else if responseFormat != nil {
		streamOpts = append(streamOpts, aisdk.WithResponseFormat(*responseFormat))
	}
	return streamOpts
}

func (tc *ToolConfig) buildTool(name string) (aisdk.Tool, error) {
	paramsRaw, err := json.Marshal(tc.InputSchema)
	if err != nil {
		return aisdk.Tool{}, fmt.Errorf("marshaling parameters: %w", err)
	}
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(paramsRaw))
	if err != nil {
		return aisdk.Tool{}, fmt.Errorf("compiling input schema: %w", err)
	}

	var providerOpts provider.ProviderOptions
	if tc.ProviderOptions != nil {
		providerOpts = make(provider.ProviderOptions, len(tc.ProviderOptions))
		for k, v := range tc.ProviderOptions {
			data, err := json.Marshal(v)
			if err != nil {
				return aisdk.Tool{}, fmt.Errorf("marshaling tool provider option %q: %w", k, err)
			}
			providerOpts[k] = provider.RawProviderOption{Key: k, Raw: data}
		}
	}

	t := aisdk.Tool{
		Description:     tc.Description,
		InputSchema:     inputSchema,
		ProviderOptions: providerOpts,
		Strict:          tc.Strict,
	}

	if tc.MockError != "" {
		t.Execute = func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			return nil, fmt.Errorf("%s", tc.MockError)
		}
	} else if len(tc.MockResults) > 0 {
		mockResults := tc.MockResults

		// Track tool call IDs in stream order via OnInputAvailable (called
		// sequentially during streaming, before concurrent Execute calls).
		// This lets Execute return the correct mock result for each tool
		// call regardless of goroutine scheduling order.
		var mu sync.Mutex
		callOrder := make([]string, 0, len(mockResults))
		fallbackIdx := 0

		t.OnInputAvailable = func(_ json.RawMessage, opts aisdk.ToolExecutionOptions) {
			mu.Lock()
			defer mu.Unlock()
			callOrder = append(callOrder, opts.ToolCallID)
		}

		t.Execute = func(_ context.Context, _ json.RawMessage, opts aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			mu.Lock()
			idx := -1
			for i, id := range callOrder {
				if id == opts.ToolCallID {
					idx = i
					break
				}
			}
			if idx < 0 {
				idx = fallbackIdx
				fallbackIdx++
			}
			mu.Unlock()

			if idx < 0 || idx >= len(mockResults) {
				return nil, fmt.Errorf("no mock result for tool %q call %q (idx=%d, have %d results)", name, opts.ToolCallID, idx, len(mockResults))
			}
			result, err := json.Marshal(mockResults[idx])
			if err != nil {
				return nil, fmt.Errorf("conformance: marshaling mock result: %w", err)
			}
			return result, nil
		}
	}
	if tc.ModelOutput != nil {
		content := make([]provider.ToolResultContentValue, len(tc.ModelOutput.Content))
		for i, value := range tc.ModelOutput.Content {
			contentType := value.Type
			if contentType == provider.ToolContentFileData {
				contentType = provider.ToolContentFile
			}
			content[i] = provider.ToolResultContentValue{
				Type:      contentType,
				Text:      value.Text,
				MediaType: value.MediaType,
				Filename:  value.Filename,
			}
			if value.Type == provider.ToolContentFileData {
				data := provider.Base64DataContent(value.Data)
				content[i].Data = &data
			}
		}
		modelOutput := &provider.ToolResultOutput{
			Type:    tc.ModelOutput.Type,
			Text:    tc.ModelOutput.Text,
			Content: content,
		}
		t.ToModelOutput = func(aisdk.ToolOutputContext) (*provider.ToolResultOutput, error) {
			return modelOutput, nil
		}
	}
	if tc.NeedsApproval {
		t.NeedsApproval = aisdk.ApprovalRequired()
	}

	return t, nil
}

func (cfg *Config) BuildUIMessages() ([]aisdk.UIMessage, error) {
	messages := make([]aisdk.UIMessage, len(cfg.UIMessages))
	for i, message := range cfg.UIMessages {
		data, err := json.Marshal(message)
		if err != nil {
			return nil, fmt.Errorf("marshaling UI message %d: %w", i, err)
		}
		if err := json.Unmarshal(data, &messages[i]); err != nil {
			return nil, fmt.Errorf("unmarshaling UI message %d: %w", i, err)
		}
	}
	return messages, nil
}

func (cfg *Config) BuildMessages(prompt string) ([]provider.Message, error) {
	if len(cfg.Messages) > 0 {
		return cfg.buildConfiguredMessages()
	}

	messages := make([]provider.Message, 0, 4)

	approvals := cfg.approvalConfigs()
	if len(approvals) == 0 {
		return []provider.Message{provider.UserText(prompt)}, nil
	}
	assistantParts := make([]provider.ContentPart, 0, len(approvals)*2)
	toolParts := make([]provider.ContentPart, 0, len(approvals))
	for _, approval := range approvals {
		input := json.RawMessage(`{}`)
		if approval.Input != nil {
			data, err := json.Marshal(approval.Input)
			if err != nil {
				return nil, fmt.Errorf("marshaling approval input: %w", err)
			}
			input = data
		}
		assistantParts = append(assistantParts,
			provider.ToolCallPart(approval.ToolCallID, approval.ToolName, input),
			provider.ToolApprovalRequestPart(approval.ApprovalID, approval.ToolCallID, false),
		)
		toolParts = append(toolParts, provider.ToolApprovalResponsePart(approval.ApprovalID, approval.Approved, approval.Reason))
	}
	messages = append(messages,
		provider.UserText(prompt),
		provider.NewAssistantMessage(assistantParts...),
		provider.NewToolMessage(toolParts...),
	)
	return messages, nil
}

func (cfg *Config) buildConfiguredMessages() ([]provider.Message, error) {
	messages := make([]provider.Message, 0, len(cfg.Messages))
	for _, mc := range cfg.Messages {
		switch mc.Role {
		case provider.RoleSystem, provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
		default:
			return nil, fmt.Errorf("unsupported configured message role %q", mc.Role)
		}
		providerOpts, err := rawProviderOptions(mc.ProviderOptions)
		if err != nil {
			return nil, fmt.Errorf("building message provider options: %w", err)
		}
		parts, err := mc.buildContentParts()
		if err != nil {
			return nil, err
		}
		messages = append(messages, provider.Message{
			Role:            mc.Role,
			Content:         parts,
			ProviderOptions: providerOpts,
		})
	}
	return messages, nil
}

func (mc *MessageConfig) buildContentParts() ([]provider.ContentPart, error) {
	if mc.ContentParts == nil {
		return []provider.ContentPart{provider.TextPart(mc.ContentText)}, nil
	}
	parts := make([]provider.ContentPart, 0, len(mc.ContentParts))
	for _, partConfig := range mc.ContentParts {
		providerOpts, err := rawProviderOptions(partConfig.ProviderOptions)
		if err != nil {
			return nil, fmt.Errorf("building message part provider options: %w", err)
		}
		var part provider.ContentPart
		switch partConfig.Type {
		case provider.ContentPartTypeText:
			part = provider.TextPart(partConfig.Text)
		case provider.ContentPartTypeReasoning:
			part = provider.ReasoningPart(partConfig.Text)
		case provider.ContentPartTypeFile:
			data := provider.DataContent{Base64: partConfig.Data}
			if partConfig.URL != "" {
				data = provider.DataContent{URL: partConfig.URL}
			} else if partConfig.Reference != nil {
				reference, err := json.Marshal(partConfig.Reference)
				if err != nil {
					return nil, fmt.Errorf("marshaling configured file reference: %w", err)
				}
				data = provider.DataContent{Reference: reference}
			}
			part = provider.FilePart(partConfig.MediaType, data)
			part.Filename = partConfig.Filename
		case provider.ContentPartTypeToolCall:
			input, err := json.Marshal(partConfig.Input)
			if err != nil {
				return nil, fmt.Errorf("marshaling configured tool input: %w", err)
			}
			part = provider.ToolCallPart(partConfig.ToolCallID, partConfig.ToolName, input)
			part.ProviderExecuted = partConfig.ProviderExecuted
		case provider.ContentPartTypeToolResult:
			output, err := json.Marshal(partConfig.Output)
			if err != nil {
				return nil, fmt.Errorf("marshaling configured tool output: %w", err)
			}
			var toolOutput provider.ToolResultOutput
			if err := json.Unmarshal(output, &toolOutput); err != nil {
				return nil, fmt.Errorf("parsing configured tool output: %w", err)
			}
			part = provider.ToolResultPart(partConfig.ToolCallID, partConfig.ToolName, &toolOutput)
		case provider.ContentPartTypeToolApprovalRequest:
			part = provider.ToolApprovalRequestPart(partConfig.ApprovalID, partConfig.ToolCallID, partConfig.IsAutomatic)
		case provider.ContentPartTypeToolApprovalResponse:
			part = provider.ToolApprovalResponsePart(partConfig.ApprovalID, partConfig.Approved, partConfig.Reason)
			part.ProviderExecuted = partConfig.ProviderExecuted
		default:
			return nil, fmt.Errorf("unsupported configured message part type %q", partConfig.Type)
		}
		part.ProviderOptions = providerOpts
		parts = append(parts, part)
	}
	return parts, nil
}

func rawProviderOptions(raw map[string]any) (provider.ProviderOptions, error) {
	if raw == nil {
		return nil, nil
	}
	providerOpts := make(provider.ProviderOptions, len(raw))
	for k, v := range raw {
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshaling provider option %q: %w", k, err)
		}
		providerOpts[k] = provider.RawProviderOption{Key: k, Raw: data}
	}
	return providerOpts, nil
}

func (cfg *Config) approvalConfigs() []ApprovalConfig {
	if len(cfg.Approvals) > 0 {
		return cfg.Approvals
	}
	if cfg.Approval != nil {
		return []ApprovalConfig{*cfg.Approval}
	}
	return nil
}

func (ptc *ProviderToolConfig) buildTool() (aisdk.Tool, error) {
	var inputSchema schema.Schema
	if ptc.InputSchema != nil {
		raw, err := json.Marshal(ptc.InputSchema)
		if err != nil {
			return aisdk.Tool{}, fmt.Errorf("marshaling provider tool input schema: %w", err)
		}
		inputSchema, err = schema.SchemaFromJSON(raw)
		if err != nil {
			return aisdk.Tool{}, fmt.Errorf("compiling provider tool input schema: %w", err)
		}
	}

	var args map[string]json.RawMessage
	if ptc.Args != nil {
		args = make(map[string]json.RawMessage, len(ptc.Args))
		for k, v := range ptc.Args {
			data, err := json.Marshal(v)
			if err != nil {
				return aisdk.Tool{}, fmt.Errorf("marshaling provider tool arg %q: %w", k, err)
			}
			args[k] = data
		}
	}
	var providerOpts provider.ProviderOptions
	if ptc.ProviderOptions != nil {
		providerOpts = make(provider.ProviderOptions, len(ptc.ProviderOptions))
		for k, v := range ptc.ProviderOptions {
			data, err := json.Marshal(v)
			if err != nil {
				return aisdk.Tool{}, fmt.Errorf("marshaling provider tool provider option %q: %w", k, err)
			}
			providerOpts[k] = provider.RawProviderOption{Key: k, Raw: data}
		}
	}
	return aisdk.Tool{
		Type:            aisdk.UserToolProvider,
		ID:              ptc.ID,
		Args:            args,
		InputSchema:     inputSchema,
		ProviderOptions: providerOpts,
	}, nil
}

// --- Replay Server ---

type ReplayServer struct {
	Server           *httptest.Server
	providerName     string
	counter          atomic.Int32
	fixtures         [][]byte
	generateResponse []byte
	framing          Framing
	requestsMu       sync.Mutex
	requests         []RequestSnapshot
}

// NewReplayServer creates a replay server using the default SSE framing used by
// Anthropic and Grafana provider-wire fixtures.
func NewReplayServer(fixtureDir string, providerName string) (*ReplayServer, error) {
	return NewReplayServerWithFraming(fixtureDir, providerName, SSEFraming{})
}

// NewReplayServerWithFraming creates a replay server with a pluggable wire
// framing strategy. Use this for Bedrock fixtures (BedrockFraming) so the
// response body matches the AWS Smithy event-stream binary format the Go
// Bedrock provider expects.
func NewReplayServerWithFraming(fixtureDir string, providerName string, framing Framing) (*ReplayServer, error) {
	rs := &ReplayServer{providerName: providerName, framing: framing}

	generatePath := filepath.Join(fixtureDir, "input.response.json")
	if data, err := os.ReadFile(generatePath); err == nil {
		rs.generateResponse = data
	} else {
		singlePath := filepath.Join(fixtureDir, "input.chunks.txt")
		if data, err := os.ReadFile(singlePath); err == nil {
			rs.fixtures = append(rs.fixtures, data)
		} else {
			for i := 1; ; i++ {
				path := filepath.Join(fixtureDir, fmt.Sprintf("input-%d.chunks.txt", i))
				data, err := os.ReadFile(path)
				if err != nil {
					if i == 1 {
						return nil, fmt.Errorf("no fixture files found in %s", fixtureDir)
					}
					break
				}
				rs.fixtures = append(rs.fixtures, data)
			}
		}
	}

	rs.Server = httptest.NewServer(http.HandlerFunc(rs.handler))
	return rs, nil
}

func (rs *ReplayServer) handler(w http.ResponseWriter, r *http.Request) {
	idx := int(rs.counter.Add(1)) - 1
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("reading request body: %v", err), http.StatusInternalServerError)
		return
	}
	snapshot, err := newRequestSnapshot(rs.providerName, r, body)
	if err != nil {
		http.Error(w, fmt.Sprintf("capturing request snapshot: %v", err), http.StatusBadRequest)
		return
	}
	rs.addRequest(snapshot)

	if rs.generateResponse != nil {
		if idx > 0 {
			http.Error(w, "generate fixture already served", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rs.generateResponse)
		return
	}

	if idx >= len(rs.fixtures) {
		http.Error(w, fmt.Sprintf("no more fixtures (request %d, have %d)", idx+1, len(rs.fixtures)), http.StatusInternalServerError)
		return
	}
	fixture := rs.fixtures[idx]

	framing := rs.framing
	if framing == nil {
		framing = SSEFraming{}
	}

	w.Header().Set("Content-Type", framing.ContentType())
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	framing.WriteFixture(w, fixture)
}

func (rs *ReplayServer) Close() {
	rs.Server.Close()
}

func (rs *ReplayServer) RequestCount() int {
	return int(rs.counter.Load())
}

func (rs *ReplayServer) Requests() []RequestSnapshot {
	rs.requestsMu.Lock()
	defer rs.requestsMu.Unlock()
	return append([]RequestSnapshot(nil), rs.requests...)
}

func (rs *ReplayServer) addRequest(snapshot RequestSnapshot) {
	rs.requestsMu.Lock()
	defer rs.requestsMu.Unlock()
	rs.requests = append(rs.requests, snapshot)
}

// --- Request Snapshots ---

type RequestSnapshot struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

const redactedHeaderValue = "<redacted>"

var secretRequestHeaders = map[string]bool{
	"authorization":     true,
	"x-api-key":         true,
	"api-key":           true,
	"anthropic-api-key": true,
}

var requestHeaderAllowlists = map[string]map[string]bool{
	"anthropic": {
		"content-type":      true,
		"anthropic-version": true,
		"anthropic-beta":    true,
		"authorization":     true,
		"x-api-key":         true,
	},
	"openai": {
		"content-type":        true,
		"authorization":       true,
		"openai-beta":         true,
		"openai-organization": true,
		"openai-project":      true,
		"x-ai-sdk-test":       true,
	},
	// Bedrock authenticates via SigV4, so authorization, x-amz-*, host,
	// user-agent, and content-length are all volatile and intentionally
	// excluded. "accept" is also excluded: upstream relies on the JS fetch
	// default ("*/*") which carries no behavioral signal, while the Go client
	// sends none. Only content-type is asserted; the request body carries the
	// meaningful conformance signal.
	"bedrock": {
		"content-type": true,
	},
	"openai-compatible": {
		"authorization": true,
		"content-type":  true,
	},
}

func newRequestSnapshot(providerName string, r *http.Request, body []byte) (RequestSnapshot, error) {
	decoded, err := decodeJSONBody(body)
	if err != nil {
		return RequestSnapshot{}, err
	}
	return RequestSnapshot{
		Method: strings.ToUpper(r.Method),
		// EscapedPath preserves percent-encoding (e.g. ":" -> "%3A" in Bedrock
		// model IDs) so the captured path matches what was sent on the wire and
		// what the upstream TypeScript snapshot records. For paths without
		// escapable characters this is identical to r.URL.Path.
		Path:    r.URL.EscapedPath(),
		Headers: normalizeRequestHeaders(providerName, r.Header),
		Body:    decoded,
	}, nil
}

func decodeJSONBody(body []byte) (any, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decoding request JSON body: %w", err)
	}
	return normalizeJSONValue("", decoded), nil
}

func normalizeJSONValue(key string, value any) any {
	switch v := value.(type) {
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = normalizeJSONValue("", item)
		}
		if key == "tools" {
			sort.SliceStable(result, func(i, j int) bool {
				return toolSortKey(result[i]) < toolSortKey(result[j])
			})
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(v))
		for k, item := range v {
			result[k] = normalizeJSONValue(k, item)
		}
		if result["type"] == "tool_result" {
			if content, ok := result["content"]; ok {
				result["content"] = normalizeToolResultContent(content)
			}
		}
		if toolResult, ok := result["toolResult"].(map[string]any); ok {
			normalizeBedrockToolResultContent(toolResult)
		}
		// OpenAI function_call_output carries the tool result as a JSON string;
		// parse it so object field ordering is compared insensitively, matching
		// the upstream serialization which preserves tool-result key order.
		if result["type"] == "function_call_output" {
			if out, ok := result["output"].(string); ok {
				result["output"] = parseJSONIfPossible(out)
			}
		}
		if result["type"] == "web_search_result" && result["page_age"] == nil {
			delete(result, "page_age")
		}
		return result
	default:
		return value
	}
}

func normalizeToolResultContent(content any) any {
	if text, ok := content.(string); ok {
		return normalizeToolResultText(text)
	}
	blocks, ok := content.([]any)
	if !ok || len(blocks) != 1 {
		return content
	}
	block, ok := blocks[0].(map[string]any)
	if !ok || block["type"] != "text" {
		return content
	}
	text, ok := block["text"].(string)
	if !ok {
		return content
	}
	return normalizeToolResultText(text)
}

func normalizeBedrockToolResultContent(toolResult map[string]any) {
	blocks, ok := toolResult["content"].([]any)
	if !ok || len(blocks) != 1 {
		return
	}
	block, ok := blocks[0].(map[string]any)
	if !ok {
		return
	}
	text, ok := block["text"].(string)
	if !ok {
		return
	}
	block["text"] = normalizeToolResultText(text)
}

func normalizeToolResultText(text string) any {
	const upstreamPrefix = "AI_InvalidToolInputError: Invalid input for tool "
	const goPrefix = "invalid input for tool "

	var detail string
	switch {
	case strings.HasPrefix(text, upstreamPrefix):
		detail = strings.TrimPrefix(text, upstreamPrefix)
	case strings.HasPrefix(text, goPrefix):
		detail = strings.TrimPrefix(text, goPrefix)
	default:
		return parseJSONIfPossible(text)
	}
	toolName, _, ok := strings.Cut(detail, ":")
	if !ok || toolName == "" {
		return text
	}
	return "invalid input for tool " + toolName + ": <validator-diagnostics>"
}

func parseJSONIfPossible(text string) any {
	decoded, err := decodeJSONBody([]byte(text))
	if err != nil {
		return text
	}
	return decoded
}

func toolSortKey(value any) string {
	tool, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if spec, ok := tool["toolSpec"].(map[string]any); ok {
		return fmt.Sprintf("%v\x00%v", spec["name"], "toolSpec")
	}
	return fmt.Sprintf("%v\x00%v", tool["name"], tool["type"])
}

func normalizeRequestHeaders(providerName string, headers http.Header) map[string]string {
	allowlist := requestHeaderAllowlists[providerName]
	result := make(map[string]string)
	for name, values := range headers {
		key := strings.ToLower(name)
		if !allowlist[key] {
			continue
		}
		value := strings.TrimSpace(strings.Join(values, ", "))
		if value == "" {
			continue
		}
		if key == "anthropic-beta" {
			value = normalizeBetaHeader(value)
		}
		if secretRequestHeaders[key] {
			value = redactedHeaderValue
		}
		result[key] = value
	}
	return result
}

func normalizeBetaHeader(value string) string {
	parts := strings.Split(value, ",")
	betas := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			betas = append(betas, part)
		}
	}
	sort.Strings(betas)
	return strings.Join(betas, ",")
}

// --- Test Case Discovery ---

type TestCase struct {
	Name     string
	Dir      string
	Provider string
}

func conformanceIDGenerator(prefix string) func() string {
	var counter atomic.Int64
	return func() string {
		return fmt.Sprintf("%s-%d", prefix, counter.Add(1)-1)
	}
}

func DiscoverTestCases(t *testing.T, providerDir string) []TestCase {
	t.Helper()
	var cases []TestCase
	providerName := providerNameFromDir(t, providerDir)

	for _, category := range []string{"upstream", "recorded"} {
		catDir := filepath.Join(providerDir, category)
		entries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(catDir, entry.Name())
			configPath := filepath.Join(dir, "config.yaml")
			if _, err := os.Stat(configPath); err != nil {
				continue
			}
			cases = append(cases, TestCase{
				Name:     fmt.Sprintf("%s/%s", category, entry.Name()),
				Dir:      dir,
				Provider: providerName,
			})
		}
	}

	return cases
}

func providerNameFromDir(t *testing.T, providerDir string) string {
	t.Helper()
	clean := filepath.Clean(providerDir)
	if clean != "." {
		return filepath.Base(clean)
	}
	wd, err := os.Getwd()
	require.NoError(t, err, "getting conformance provider directory")
	return filepath.Base(wd)
}

// --- Provider Factory ---

type ProviderFactory func(baseURL string, cfg *Config) (provider.LanguageModel, error)

type TestServer struct {
	BaseURL      string
	RequestCount func() int
	Requests     func() []RequestSnapshot
	Close        func()
}

type TestServerFactory func(t *testing.T, tc TestCase) (*TestServer, error)

// --- Test Runner ---

func RunTestCase(t *testing.T, tc TestCase, factory ProviderFactory) {
	t.Helper()
	RunTestCaseWithServer(t, tc, factory, defaultTestServerFactory)
}

func RunTestCaseWithServer(t *testing.T, tc TestCase, factory ProviderFactory, serverFactory TestServerFactory) {
	t.Helper()

	cfg, err := LoadConfig(filepath.Join(tc.Dir, "config.yaml"))
	require.NoError(t, err, "loading config")
	if cfg.SkipReason != "" {
		require.True(t, strings.HasPrefix(tc.Name, "upstream/"), "skipReason is only valid for imported upstream fixtures")
		t.Skip(cfg.SkipReason)
	}

	ts, err := serverFactory(t, tc)
	require.NoError(t, err, "creating replay server")
	defer ts.Close()

	model, err := factory(ts.BaseURL, cfg)
	require.NoError(t, err, "creating provider")

	tools, err := cfg.BuildToolSet()
	require.NoError(t, err, "building tools")

	providerOpts, err := cfg.BuildProviderOptions()
	require.NoError(t, err, "building provider options")

	responseFormat, err := cfg.BuildResponseFormat()
	require.NoError(t, err, "building response format")

	prompt := cfg.Prompt
	if prompt == "" {
		prompt = "test"
	}

	var messages []provider.Message
	if len(cfg.UIMessages) > 0 {
		uiMessages, err := cfg.BuildUIMessages()
		require.NoError(t, err, "building UI messages")
		messages, err = aisdk.ConvertToModelMessages(uiMessages, aisdk.WithTools(tools))
		require.NoError(t, err, "converting UI messages")
	} else {
		messages, err = cfg.BuildMessages(prompt)
		require.NoError(t, err, "building messages")
	}

	if cfg.Operation == OperationGenerate {
		require.Equal(t, "bedrock", tc.Provider, "operation generate is currently supported only for Bedrock")
		runGenerateTestCase(t, tc, cfg, ts, model, messages, providerOpts, responseFormat)
		return
	}

	out, err := cfg.BuildOutput()
	require.NoError(t, err, "building output")
	stopConditions := []aisdk.StopCondition{aisdk.StepCountIs(cfg.StopWhenStepCount)}
	streamOpts := cfg.buildStreamOptions(messages, tools, providerOpts, stopConditions, out, responseFormat)
	if cfg.MaxRetries != nil {
		streamOpts = append(streamOpts, aisdk.WithMaxRetries(*cfg.MaxRetries))
	}
	result := aisdk.StreamText(t.Context(), model, streamOpts...)

	uiStream := result.ToUIMessageStream(cfg.BuildUIMessageStreamOptions()...)

	var actual []map[string]any
	for chunk := range uiStream {
		data, err := json.Marshal(chunk)
		require.NoError(t, err, "marshaling chunk")

		var parsed map[string]any
		require.NoError(t, json.Unmarshal(data, &parsed), "parsing marshaled chunk")
		actual = append(actual, parsed)
	}

	expected, err := LoadExpected(filepath.Join(tc.Dir, "expected.jsonl"))
	require.NoError(t, err, "loading expected output")

	require.Positive(t, ts.RequestCount(), "replay server received no requests — check provider URL configuration")
	if cfg.ExpectStreamError {
		require.Error(t, result.Err(), "expected stream error")
	} else {
		require.NoError(t, result.Err(), "stream error")
	}
	CompareChunks(t, expected, actual)

	// Request-input assertions are opt-in per fixture: when a fixture ships an
	// expected-requests.jsonl snapshot, the provider request is compared
	// semantically. Fixtures without the file (e.g. provider suites that have
	// not regenerated request snapshots yet) skip this assertion.
	expectedRequestsPath := filepath.Join(tc.Dir, "expected-requests.jsonl")
	if _, statErr := os.Stat(expectedRequestsPath); statErr == nil {
		expectedRequests, err := LoadExpectedRequests(expectedRequestsPath)
		require.NoError(t, err, "loading expected request inputs")
		require.NotNil(t, ts.Requests, "test server does not expose captured request inputs")
		CompareRequestSnapshots(t, expectedRequests, ts.Requests())
	}

	expectedUsagePath := filepath.Join(tc.Dir, "expected-usage.json")
	if expectedUsage, readErr := os.ReadFile(expectedUsagePath); readErr == nil {
		steps := result.Steps()
		actualUsage := make([]provider.Usage, len(steps))
		for i, step := range steps {
			actualUsage[i] = step.Usage
		}
		actualJSON, err := json.Marshal(actualUsage)
		require.NoError(t, err, "marshaling actual usage")
		require.JSONEq(t, string(expectedUsage), string(actualJSON), "usage mismatch")
	} else if !os.IsNotExist(readErr) {
		require.NoError(t, readErr, "loading expected usage")
	}

	expectedObjectPath := filepath.Join(tc.Dir, "expected-object.json")
	if _, err := os.Stat(expectedObjectPath); err == nil {
		compareOutputObject(t, expectedObjectPath, result.OutputValue(), actual, cfg.AssertOutputValue)
	}
}

type generateResultSnapshot struct {
	Content          []provider.GenerateContentPart `json:"content"`
	FinishReason     provider.FinishReason          `json:"finishReason"`
	Usage            provider.Usage                 `json:"usage"`
	ProviderMetadata provider.ProviderMetadata      `json:"providerMetadata,omitempty"`
	Warnings         []provider.Warning             `json:"warnings,omitempty"`
}

func runGenerateTestCase(
	t *testing.T,
	tc TestCase,
	cfg *Config,
	ts *TestServer,
	model provider.LanguageModel,
	messages []provider.Message,
	providerOpts []provider.ProviderOption,
	responseFormat *provider.ResponseFormat,
) {
	t.Helper()

	if cfg.System != "" {
		messages = append(
			[]provider.Message{provider.NewSystemMessage(cfg.System)},
			messages...,
		)
	}
	callOpts := provider.CallOptions{
		Prompt:          messages,
		ResponseFormat:  responseFormat,
		Headers:         cfg.Headers,
		ProviderOptions: provider.BuildProviderOptions(providerOpts...),
	}

	result, err := model.DoGenerate(t.Context(), callOpts)
	require.NoError(t, err, "generate error")
	require.Positive(t, ts.RequestCount(), "replay server received no requests — check provider URL configuration")

	metadata := make(provider.ProviderMetadata)
	if bedrockMetadata, ok := result.ProviderMetadata["bedrock"]; ok {
		metadata["bedrock"] = bedrockMetadata
	}
	if len(metadata) == 0 {
		metadata = nil
	}
	actual := generateResultSnapshot{
		Content:          result.Content,
		FinishReason:     result.FinishReason,
		Usage:            result.Usage,
		ProviderMetadata: metadata,
		Warnings:         result.Warnings,
	}
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err, "marshaling generate result")
	expectedJSON, err := os.ReadFile(filepath.Join(tc.Dir, "expected-generate.json"))
	require.NoError(t, err, "loading expected generate result")
	require.JSONEq(t, string(expectedJSON), string(actualJSON), "generate result mismatch")

	expectedRequests, err := LoadExpectedRequests(filepath.Join(tc.Dir, "expected-requests.jsonl"))
	require.NoError(t, err, "loading expected request inputs")
	CompareRequestSnapshots(t, expectedRequests, ts.Requests())
}

func defaultTestServerFactory(_ *testing.T, tc TestCase) (*TestServer, error) {
	rs, err := NewReplayServer(tc.Dir, tc.Provider)
	if err != nil {
		return nil, err
	}
	return &TestServer{
		BaseURL:      rs.Server.URL,
		RequestCount: rs.RequestCount,
		Requests:     rs.Requests,
		Close:        rs.Close,
	}, nil
}

// BedrockTestServerFactory builds a TestServer using the Bedrock binary
// event-stream framing. Bedrock-aware Go conformance tests pass this factory to
// RunTestCaseWithServer so the replay server emits the wire format the Go
// Bedrock provider expects while still capturing request snapshots.
func BedrockTestServerFactory(_ *testing.T, tc TestCase) (*TestServer, error) {
	rs, err := NewReplayServerWithFraming(tc.Dir, tc.Provider, BedrockFraming{})
	if err != nil {
		return nil, err
	}
	return &TestServer{
		BaseURL:      rs.Server.URL,
		RequestCount: rs.RequestCount,
		Requests:     rs.Requests,
		Close:        rs.Close,
	}, nil
}

func compareOutputObject(t *testing.T, expectedPath string, actualObj any, chunks []map[string]any, assertOutputValue bool) {
	t.Helper()

	expectedData, err := os.ReadFile(expectedPath)
	require.NoError(t, err, "reading expected-object.json")

	var expectedObj any
	require.NoError(t, json.Unmarshal(expectedData, &expectedObj), "parsing expected-object.json")

	if actualObj == nil && !assertOutputValue {
		var textBuilder string
		for _, chunk := range chunks {
			if chunk["type"] == "text-delta" {
				if delta, ok := chunk["delta"].(string); ok {
					textBuilder += delta
				}
			}
		}
		require.NotEmpty(t, textBuilder, "no text-delta chunks found for output object comparison")
		require.NoError(t, json.Unmarshal([]byte(textBuilder), &actualObj), "parsing concatenated text deltas as JSON")
	}

	require.Equal(t, expectedObj, actualObj, "output object mismatch")
}

// --- Expected Output ---

func LoadExpected(path string) ([]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading expected output: %w", err)
	}

	var chunks []map[string]any
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var chunk map[string]any
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return nil, fmt.Errorf("parsing expected chunk: %w", err)
		}
		chunks = append(chunks, chunk)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning expected output: %w", err)
	}

	return chunks, nil
}

func LoadExpectedRequests(path string) ([]RequestSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading expected request inputs: %w", err)
	}

	var requests []RequestSnapshot
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var request RequestSnapshot
		dec := json.NewDecoder(strings.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&request); err != nil {
			return nil, fmt.Errorf("parsing expected request input: %w", err)
		}
		request.Body = normalizeJSONValue("", request.Body)
		requests = append(requests, request)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning expected request inputs: %w", err)
	}

	return requests, nil
}

// --- Comparison ---

func CompareChunks(t *testing.T, expected, actual []map[string]any) {
	t.Helper()

	minLen := len(expected)
	if len(actual) < minLen {
		minLen = len(actual)
	}

	for i := 0; i < minLen; i++ {
		expJSON, _ := json.MarshalIndent(expected[i], "", "  ")
		actJSON, _ := json.MarshalIndent(actual[i], "", "  ")
		require.Equalf(t, expected[i], actual[i],
			"chunk %d mismatch\nexpected:\n%s\nactual:\n%s", i, expJSON, actJSON)
	}

	require.Equalf(t, len(expected), len(actual),
		"chunk count mismatch: expected %d, got %d", len(expected), len(actual))
}

func CompareRequestSnapshots(t *testing.T, expected, actual []RequestSnapshot) {
	t.Helper()

	minLen := len(expected)
	if len(actual) < minLen {
		minLen = len(actual)
	}

	for i := 0; i < minLen; i++ {
		require.Equalf(t, expected[i].Method, actual[i].Method, "request %d method mismatch", i)
		require.Equalf(t, expected[i].Path, actual[i].Path, "request %d path mismatch", i)
		require.Equalf(t, expected[i].Headers, actual[i].Headers, "request %d headers mismatch", i)

		expJSON, _ := json.MarshalIndent(expected[i].Body, "", "  ")
		actJSON, _ := json.MarshalIndent(actual[i].Body, "", "  ")
		require.Equalf(t, expected[i].Body, actual[i].Body,
			"request %d body mismatch\nexpected:\n%s\nactual:\n%s", i, expJSON, actJSON)
	}

	require.Equalf(t, len(expected), len(actual),
		"request count mismatch: expected %d, got %d", len(expected), len(actual))
}
