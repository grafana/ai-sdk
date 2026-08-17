package providerwirev4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type messageDTO struct {
	Role            string             `json:"role"`
	Content         json.RawMessage    `json:"content"`
	ProviderOptions providerOptionsDTO `json:"providerOptions,omitempty"`
}

type contentPartDTO struct {
	Type             string             `json:"type"`
	Text             *string            `json:"text,omitempty"`
	Data             json.RawMessage    `json:"data,omitempty"`
	Filename         string             `json:"filename,omitempty"`
	MediaType        *string            `json:"mediaType,omitempty"`
	Kind             *string            `json:"kind,omitempty"`
	ToolCallID       *string            `json:"toolCallId,omitempty"`
	ToolName         *string            `json:"toolName,omitempty"`
	Input            json.RawMessage    `json:"input,omitempty"`
	Output           json.RawMessage    `json:"output,omitempty"`
	ProviderExecuted bool               `json:"providerExecuted,omitempty"`
	ApprovalID       *string            `json:"approvalId,omitempty"`
	Approved         *bool              `json:"approved,omitempty"`
	Reason           string             `json:"reason,omitempty"`
	ProviderOptions  providerOptionsDTO `json:"providerOptions,omitempty"`
}

func encodeMessage(message provider.Message) (json.RawMessage, error) {
	providerOptions, err := encodeNestedProviderOptions(message.ProviderOptions, "message")
	if err != nil {
		return nil, err
	}
	dto := messageDTO{Role: string(message.Role), ProviderOptions: providerOptions}
	switch message.Role {
	case provider.RoleSystem:
		var content bytes.Buffer
		for _, part := range message.Content {
			if err := validatePromptContentPrivacy(part); err != nil {
				return nil, err
			}
			if part.Type != provider.ContentPartTypeText || len(part.ProviderOptions) > 0 {
				return nil, errors.New("providerwirev4: system messages can contain only plain text")
			}
			content.WriteString(part.Text)
		}
		dto.Content, err = json.Marshal(content.String())
	case provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
		parts := make([]contentPartDTO, len(message.Content))
		for i, part := range message.Content {
			if !roleAllowsContent(message.Role, part.Type) {
				return nil, fmt.Errorf("providerwirev4: role %q does not allow content type %q", message.Role, part.Type)
			}
			parts[i], err = encodeContentPart(part)
			if err != nil {
				return nil, err
			}
		}
		dto.Content, err = json.Marshal(parts)
	default:
		return nil, fmt.Errorf("providerwirev4: unsupported message role %q", message.Role)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(dto)
}

func decodeMessage(data json.RawMessage) (provider.Message, error) {
	object, err := decodeObject(data, "message")
	if err != nil {
		return provider.Message{}, err
	}
	role, err := decodeRequiredString(object, "role", "message")
	if err != nil {
		return provider.Message{}, err
	}
	content, err := requireField(object, "content", "message")
	if err != nil {
		return provider.Message{}, err
	}
	if err := rejectUnknownFields(object, "message", "role", "content", "providerOptions"); err != nil {
		return provider.Message{}, err
	}
	if err := rejectNullFields(object, "message", "providerOptions"); err != nil {
		return provider.Message{}, err
	}
	var dto messageDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return provider.Message{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "message")
	if err != nil {
		return provider.Message{}, err
	}
	message := provider.Message{Role: provider.Role(role), ProviderOptions: providerOptions}
	switch message.Role {
	case provider.RoleSystem:
		var text string
		if err := json.Unmarshal(content, &text); err != nil {
			return provider.Message{}, errors.New("providerwirev4: system message content must be a string")
		}
		message.Content = []provider.ContentPart{{Type: provider.ContentPartTypeText, Text: text}}
	case provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
		var parts []json.RawMessage
		if err := json.Unmarshal(content, &parts); err != nil || parts == nil {
			return provider.Message{}, fmt.Errorf("providerwirev4: role %q content must be an array", role)
		}
		message.Content = make([]provider.ContentPart, len(parts))
		for i, part := range parts {
			decoded, err := decodeContentPart(part)
			if err != nil {
				return provider.Message{}, err
			}
			if !roleAllowsContent(message.Role, decoded.Type) {
				return provider.Message{}, fmt.Errorf("providerwirev4: role %q does not allow content type %q", role, decoded.Type)
			}
			message.Content[i] = decoded
		}
	default:
		return provider.Message{}, fmt.Errorf("providerwirev4: unsupported message role %q", role)
	}
	return message, nil
}

func roleAllowsContent(role provider.Role, contentType provider.ContentPartType) bool {
	switch role {
	case provider.RoleUser:
		return contentType == provider.ContentPartTypeText || contentType == provider.ContentPartTypeFile
	case provider.RoleAssistant:
		switch contentType {
		case provider.ContentPartTypeText, provider.ContentPartTypeFile, provider.ContentPartTypeCustom,
			provider.ContentPartTypeReasoning, provider.ContentPartTypeReasoningFile,
			provider.ContentPartTypeToolCall, provider.ContentPartTypeToolResult:
			return true
		}
	case provider.RoleTool:
		return contentType == provider.ContentPartTypeToolResult || contentType == provider.ContentPartTypeToolApprovalResponse
	}
	return false
}

func validatePromptContentPrivacy(part provider.ContentPart) error {
	if part.SourceType != "" || part.ID != "" || part.URL != "" || part.Title != "" || part.Signature != "" || part.IsAutomatic {
		return errors.New("providerwirev4: prompt content contains private fields")
	}
	return nil
}

func encodeContentPart(part provider.ContentPart) (contentPartDTO, error) {
	if err := validatePromptContentPrivacy(part); err != nil {
		return contentPartDTO{}, err
	}
	providerOptions, err := encodeNestedProviderOptions(part.ProviderOptions, "content part")
	if err != nil {
		return contentPartDTO{}, err
	}
	dto := contentPartDTO{Type: string(part.Type), ProviderOptions: providerOptions}
	switch part.Type {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		dto.Text = &part.Text
	case provider.ContentPartTypeFile, provider.ContentPartTypeReasoningFile:
		allowExtended := part.Type == provider.ContentPartTypeFile
		if !allowExtended && part.Filename != "" {
			return contentPartDTO{}, errors.New("providerwirev4: reasoning file filename is not in LanguageModelV4")
		}
		data, err := encodeData(part.Data, allowExtended)
		if err != nil {
			return contentPartDTO{}, err
		}
		dto.Data, err = json.Marshal(data)
		if err != nil {
			return contentPartDTO{}, err
		}
		dto.MediaType = &part.MediaType
		dto.Filename = part.Filename
	case provider.ContentPartTypeCustom:
		if err := validateQualifiedIdentifier(part.Kind, "custom content kind"); err != nil {
			return contentPartDTO{}, err
		}
		dto.Kind = &part.Kind
	case provider.ContentPartTypeToolCall:
		if part.ToolCallID == "" || part.ToolName == "" {
			return contentPartDTO{}, errors.New("providerwirev4: tool call ID and name are required")
		}
		if err := validateJSON(part.Input, "tool call input"); err != nil {
			return contentPartDTO{}, err
		}
		dto.ToolCallID, dto.ToolName = &part.ToolCallID, &part.ToolName
		dto.Input = append(json.RawMessage(nil), part.Input...)
		dto.ProviderExecuted = part.ProviderExecuted
	case provider.ContentPartTypeToolResult:
		if part.ToolCallID == "" || part.ToolName == "" || part.Output == nil {
			return contentPartDTO{}, errors.New("providerwirev4: tool result ID, name, and output are required")
		}
		dto.ToolCallID, dto.ToolName = &part.ToolCallID, &part.ToolName
		output, err := encodeToolResultOutput(*part.Output)
		if err != nil {
			return contentPartDTO{}, err
		}
		dto.Output, err = json.Marshal(output)
		if err != nil {
			return contentPartDTO{}, err
		}
	case provider.ContentPartTypeToolApprovalResponse:
		if part.ToolCallID != "" || part.ToolName != "" || part.ProviderExecuted {
			return contentPartDTO{}, errors.New("providerwirev4: tool approval response contains private fields")
		}
		if part.ApprovalID == "" || part.Approved == nil {
			return contentPartDTO{}, errors.New("providerwirev4: tool approval response ID and approved are required")
		}
		dto.ApprovalID, dto.Approved, dto.Reason = &part.ApprovalID, part.Approved, part.Reason
	default:
		return contentPartDTO{}, fmt.Errorf("providerwirev4: unsupported prompt content type %q", part.Type)
	}
	return dto, nil
}

func decodeContentPart(data json.RawMessage) (provider.ContentPart, error) {
	object, err := decodeObject(data, "content part")
	if err != nil {
		return provider.ContentPart{}, err
	}
	variant, err := decodeRequiredString(object, "type", "content part")
	if err != nil {
		return provider.ContentPart{}, err
	}
	for _, private := range []string{"sourceType", "id", "url", "title", "signature", "isAutomatic"} {
		if _, exists := object[private]; exists {
			return provider.ContentPart{}, fmt.Errorf("providerwirev4: private prompt content field %q is not supported", private)
		}
	}
	if err := rejectUnknownFields(object, "content part", "type", "text", "data", "filename", "mediaType", "kind", "toolCallId", "toolName", "input", "output", "providerExecuted", "approvalId", "approved", "reason", "providerOptions"); err != nil {
		return provider.ContentPart{}, err
	}
	fields := []string{"type", "providerOptions"}
	switch provider.ContentPartType(variant) {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		fields = append(fields, "text")
	case provider.ContentPartTypeFile:
		fields = append(fields, "data", "filename", "mediaType")
	case provider.ContentPartTypeReasoningFile:
		if _, exists := object["filename"]; exists {
			return provider.ContentPart{}, errors.New("providerwirev4: reasoning file filename is not in LanguageModelV4")
		}
		fields = append(fields, "data", "mediaType")
	case provider.ContentPartTypeCustom:
		fields = append(fields, "kind")
	case provider.ContentPartTypeToolCall:
		fields = append(fields, "toolCallId", "toolName", "input", "providerExecuted")
	case provider.ContentPartTypeToolResult:
		fields = append(fields, "toolCallId", "toolName", "output")
	case provider.ContentPartTypeToolApprovalResponse:
		for _, private := range []string{"toolCallId", "toolName", "providerExecuted"} {
			if _, exists := object[private]; exists {
				return provider.ContentPart{}, fmt.Errorf("providerwirev4: private tool approval field %q is not supported", private)
			}
		}
		fields = append(fields, "approvalId", "approved", "reason")
	default:
		return provider.ContentPart{}, fmt.Errorf("providerwirev4: unsupported prompt content type %q", variant)
	}
	nonNullFields := fields
	if provider.ContentPartType(variant) == provider.ContentPartTypeToolCall {
		nonNullFields = make([]string, 0, len(fields)-1)
		for _, field := range fields {
			if field != "input" {
				nonNullFields = append(nonNullFields, field)
			}
		}
	}
	if err := rejectNullFields(object, "content part", nonNullFields...); err != nil {
		return provider.ContentPart{}, err
	}
	var dto contentPartDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.ContentPart{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "content part")
	if err != nil {
		return provider.ContentPart{}, err
	}
	part := provider.ContentPart{Type: provider.ContentPartType(variant), ProviderOptions: providerOptions}
	switch part.Type {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		if dto.Text == nil {
			return provider.ContentPart{}, fmt.Errorf("providerwirev4: %s text is required", variant)
		}
		part.Text = *dto.Text
	case provider.ContentPartTypeFile, provider.ContentPartTypeReasoningFile:
		if len(dto.Data) == 0 || dto.MediaType == nil {
			return provider.ContentPart{}, fmt.Errorf("providerwirev4: %s data and mediaType are required", variant)
		}
		part.Data, err = decodeRequestData(dto.Data, part.Type == provider.ContentPartTypeFile)
		if err != nil {
			return provider.ContentPart{}, err
		}
		part.MediaType, part.Filename = *dto.MediaType, dto.Filename
	case provider.ContentPartTypeCustom:
		if dto.Kind == nil {
			return provider.ContentPart{}, errors.New("providerwirev4: custom content kind is required")
		}
		if err := validateQualifiedIdentifier(*dto.Kind, "custom content kind"); err != nil {
			return provider.ContentPart{}, err
		}
		part.Kind = *dto.Kind
	case provider.ContentPartTypeToolCall:
		if dto.ToolCallID == nil || *dto.ToolCallID == "" || dto.ToolName == nil || *dto.ToolName == "" {
			return provider.ContentPart{}, errors.New("providerwirev4: tool call ID and name are required")
		}
		if err := validateJSON(dto.Input, "tool call input"); err != nil {
			return provider.ContentPart{}, err
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		part.Input, part.ProviderExecuted = append(json.RawMessage(nil), dto.Input...), dto.ProviderExecuted
	case provider.ContentPartTypeToolResult:
		if dto.ToolCallID == nil || *dto.ToolCallID == "" || dto.ToolName == nil || *dto.ToolName == "" || len(dto.Output) == 0 {
			return provider.ContentPart{}, errors.New("providerwirev4: tool result ID, name, and output are required")
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		output, err := decodeToolResultOutput(dto.Output)
		if err != nil {
			return provider.ContentPart{}, err
		}
		part.Output = &output
	case provider.ContentPartTypeToolApprovalResponse:
		if dto.ApprovalID == nil || *dto.ApprovalID == "" || dto.Approved == nil {
			return provider.ContentPart{}, errors.New("providerwirev4: tool approval response ID and approved are required")
		}
		part.ApprovalID, part.Approved, part.Reason = *dto.ApprovalID, dto.Approved, dto.Reason
	default:
		return provider.ContentPart{}, fmt.Errorf("providerwirev4: unsupported prompt content type %q", variant)
	}
	return part, nil
}
