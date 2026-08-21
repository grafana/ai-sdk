package providerrequest

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/grafana/ai-sdk/provider"
)

// Validate verifies the selected request roles and discriminated arms before a
// direct provider converts the request.
func Validate(options provider.CallOptions) error {
	for _, number := range []struct {
		name  string
		value *provider.LanguageModelNumber
	}{
		{name: "maxOutputTokens", value: options.MaxOutputTokens},
		{name: "topK", value: options.TopK},
		{name: "seed", value: options.Seed},
	} {
		if number.value == nil {
			continue
		}
		if _, ok := number.value.Int64(); ok {
			continue
		}
		if _, ok := number.value.Float64(); !ok {
			return fmt.Errorf("provider request %s is invalid", number.name)
		}
	}
	for messageIndex, message := range options.Prompt {
		if err := validateMessage(message); err != nil {
			return fmt.Errorf("provider request message %d: %w", messageIndex, err)
		}
	}
	for toolIndex, tool := range options.Tools {
		if err := validateTool(tool); err != nil {
			return fmt.Errorf("provider request tool %d: %w", toolIndex, err)
		}
	}
	if options.ToolChoice != nil {
		switch options.ToolChoice.Type {
		case provider.ToolChoiceTool:
		case provider.ToolChoiceAuto, provider.ToolChoiceNone, provider.ToolChoiceRequired:
			if options.ToolChoice.ToolName != "" {
				return errors.New("provider request tool choice has an inactive tool name")
			}
		default:
			return fmt.Errorf("provider request has unsupported tool choice %q", options.ToolChoice.Type)
		}
	}
	if options.ResponseFormat != nil {
		switch options.ResponseFormat.Type {
		case provider.ResponseFormatText:
			if len(options.ResponseFormat.Schema) > 0 || options.ResponseFormat.Name != nil || options.ResponseFormat.Description != nil {
				return errors.New("provider request text response format has inactive JSON fields")
			}
		case provider.ResponseFormatJSON:
		default:
			return fmt.Errorf("provider request has unsupported response format %q", options.ResponseFormat.Type)
		}
	}
	return nil
}

func validateMessage(message provider.Message) error {
	switch message.Role {
	case provider.RoleSystem, provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
	default:
		return fmt.Errorf("unsupported message role %q", message.Role)
	}
	if message.Role == provider.RoleSystem {
		if len(message.Content) != 1 || message.Content[0].Type != provider.ContentPartTypeText {
			return errors.New("system message must contain exactly one text part")
		}
	}
	for partIndex, part := range message.Content {
		if !contentTypeAllowed(message.Role, part.Type) {
			return fmt.Errorf("content part %d type %q is invalid for role %q", partIndex, part.Type, message.Role)
		}
		if err := validateContentPart(part); err != nil {
			return fmt.Errorf("content part %d: %w", partIndex, err)
		}
	}
	return nil
}

func contentTypeAllowed(role provider.Role, contentType provider.ContentPartType) bool {
	switch role {
	case provider.RoleSystem:
		return contentType == provider.ContentPartTypeText
	case provider.RoleUser:
		return contentType == provider.ContentPartTypeText || contentType == provider.ContentPartTypeFile
	case provider.RoleAssistant:
		switch contentType {
		case provider.ContentPartTypeText,
			provider.ContentPartTypeFile,
			provider.ContentPartTypeReasoning,
			provider.ContentPartTypeReasoningFile,
			provider.ContentPartTypeToolCall,
			provider.ContentPartTypeToolResult,
			provider.ContentPartTypeCustom,
			provider.ContentPartTypeToolApprovalRequest:
			return true
		}
	case provider.RoleTool:
		return contentType == provider.ContentPartTypeToolResult || contentType == provider.ContentPartTypeToolApprovalResponse
	}
	return false
}

func validateContentPart(part provider.ContentPart) error {
	valid := provider.ContentPart{Type: part.Type}
	switch part.Type {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		valid.Text = part.Text
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeFile:
		if part.Data == nil {
			return errors.New("file data is required")
		}
		if err := part.Data.Validate(); err != nil {
			return fmt.Errorf("invalid file data: %w", err)
		}
		valid.Data = part.Data
		valid.FilePartFilename = part.FilePartFilename
		valid.MediaType = part.MediaType
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeReasoningFile:
		if part.Data == nil {
			return errors.New("reasoning file data is required")
		}
		if err := part.Data.Validate(); err != nil {
			return fmt.Errorf("invalid reasoning file data: %w", err)
		}
		valid.Data = part.Data
		valid.MediaType = part.MediaType
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeToolCall:
		valid.ToolCallID = part.ToolCallID
		valid.ToolName = part.ToolName
		valid.Input = part.Input
		valid.ProviderExecuted = part.ProviderExecuted
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeToolResult:
		if part.Output == nil {
			return errors.New("tool result output is required")
		}
		if err := validateToolResultOutput(*part.Output); err != nil {
			return err
		}
		valid.ToolCallID = part.ToolCallID
		valid.ToolName = part.ToolName
		valid.Output = part.Output
		valid.ProviderExecuted = part.ProviderExecuted
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeCustom:
		valid.Kind = part.Kind
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeToolApprovalRequest:
		valid.ApprovalID = part.ApprovalID
		valid.ToolCallID = part.ToolCallID
		valid.ToolName = part.ToolName
		valid.Signature = part.Signature
		valid.IsAutomatic = part.IsAutomatic
		valid.ProviderOptions = part.ProviderOptions
	case provider.ContentPartTypeToolApprovalResponse:
		valid.ApprovalID = part.ApprovalID
		valid.ToolCallID = part.ToolCallID
		valid.ToolName = part.ToolName
		valid.Approved = part.Approved
		valid.Reason = part.Reason
		valid.ProviderExecuted = part.ProviderExecuted
		valid.ProviderOptions = part.ProviderOptions
	default:
		return fmt.Errorf("unsupported content type %q", part.Type)
	}
	if !reflect.DeepEqual(part, valid) {
		return fmt.Errorf("content type %q has inactive fields or invalid filename ownership", part.Type)
	}
	return nil
}

func validateToolResultOutput(output provider.ToolResultOutput) error {
	valid := provider.ToolResultOutput{Type: output.Type, ProviderOptions: output.ProviderOptions}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		valid.Text = output.Text
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		valid.JSON = output.JSON
	case provider.ToolOutputContent:
		valid.Content = output.Content
		for index, content := range output.Content {
			if err := validateToolResultContent(content); err != nil {
				return fmt.Errorf("tool result content %d: %w", index, err)
			}
		}
	case provider.ToolOutputExecutionDenied:
		valid.Reason = output.Reason
	default:
		return fmt.Errorf("unsupported tool result output type %q", output.Type)
	}
	if !reflect.DeepEqual(output, valid) {
		return fmt.Errorf("tool result output type %q has inactive fields", output.Type)
	}
	return nil
}

func validateToolResultContent(content provider.ToolResultContentValue) error {
	valid := provider.ToolResultContentValue{Type: content.Type}
	switch content.Type {
	case provider.ToolContentText:
		valid.Text = content.Text
		valid.ProviderOptions = content.ProviderOptions
	case provider.ToolContentFile:
		if content.Data == nil {
			return errors.New("tool result file data is required")
		}
		if err := content.Data.Validate(); err != nil {
			return fmt.Errorf("invalid tool result file data: %w", err)
		}
		valid.Data = content.Data
		valid.MediaType = content.MediaType
		valid.Filename = content.Filename
		valid.ProviderOptions = content.ProviderOptions
	case provider.ToolContentCustom:
		valid.ProviderOptions = content.ProviderOptions
	default:
		return fmt.Errorf("unsupported tool result content type %q", content.Type)
	}
	if !reflect.DeepEqual(content, valid) {
		return fmt.Errorf("tool result content type %q has inactive fields", content.Type)
	}
	return nil
}

func validateTool(tool provider.Tool) error {
	valid := provider.Tool{Type: tool.Type, Name: tool.Name}
	switch tool.Type {
	case provider.ToolTypeFunction:
		valid.Description = tool.Description
		valid.InputSchema = tool.InputSchema
		valid.InputExamples = tool.InputExamples
		valid.Strict = tool.Strict
		valid.ProviderOptions = tool.ProviderOptions
	case provider.ToolTypeProvider:
		valid.ID = tool.ID
		valid.Args = tool.Args
		valid.ProviderOptions = tool.ProviderOptions
	default:
		return fmt.Errorf("unsupported tool type %q", tool.Type)
	}
	if !reflect.DeepEqual(tool, valid) {
		return fmt.Errorf("tool type %q has inactive fields", tool.Type)
	}
	return nil
}
