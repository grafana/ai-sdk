package providerwirev4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

type callOptionsDTO struct {
	Prompt           []json.RawMessage  `json:"prompt"`
	Tools            []toolDTO          `json:"tools,omitempty"`
	ToolChoice       *toolChoiceDTO     `json:"toolChoice,omitempty"`
	MaxOutputTokens  *int               `json:"maxOutputTokens,omitempty"`
	Temperature      *float64           `json:"temperature,omitempty"`
	TopP             *float64           `json:"topP,omitempty"`
	TopK             *int               `json:"topK,omitempty"`
	PresencePenalty  *float64           `json:"presencePenalty,omitempty"`
	FrequencyPenalty *float64           `json:"frequencyPenalty,omitempty"`
	StopSequences    []string           `json:"stopSequences,omitempty"`
	ResponseFormat   *responseFormatDTO `json:"responseFormat,omitempty"`
	Seed             *int               `json:"seed,omitempty"`
	Reasoning        *string            `json:"reasoning,omitempty"`
	IncludeRawChunks bool               `json:"includeRawChunks,omitempty"`
	Headers          map[string]string  `json:"headers,omitempty"`
	ProviderOptions  providerOptionsDTO `json:"providerOptions,omitempty"`
}

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

type toolDTO struct {
	Type            string                      `json:"type"`
	Name            string                      `json:"name"`
	Description     string                      `json:"description,omitempty"`
	InputSchema     json.RawMessage             `json:"inputSchema,omitempty"`
	InputExamples   []inputExampleDTO           `json:"inputExamples,omitempty"`
	Strict          *bool                       `json:"strict,omitempty"`
	ID              string                      `json:"id,omitempty"`
	Args            *map[string]json.RawMessage `json:"args,omitempty"`
	ProviderOptions providerOptionsDTO          `json:"providerOptions,omitempty"`
}

func (dto *toolDTO) UnmarshalJSON(data []byte) error {
	type toolAlias toolDTO
	object, err := decodeObject(data, "tool")
	if err != nil {
		return err
	}
	variant, err := decodeRequiredString(object, "type", "tool")
	if err != nil {
		return err
	}
	fields := []string{"type", "name"}
	switch provider.ToolType(variant) {
	case provider.ToolTypeFunction:
		fields = append(fields, "description", "inputSchema", "inputExamples", "strict", "providerOptions")
	case provider.ToolTypeProvider:
		if _, exists := object["providerOptions"]; exists {
			return errors.New("providerwirev4: provider tool providerOptions are not in LanguageModelV4")
		}
		fields = append(fields, "id", "args")
	default:
		return fmt.Errorf("providerwirev4: unsupported tool type %q", variant)
	}
	if err := rejectNullFields(object, "tool", fields...); err != nil {
		return err
	}
	var decoded toolAlias
	if err := decodeSelectedObject(object, &decoded, fields...); err != nil {
		return err
	}
	*dto = toolDTO(decoded)
	return nil
}

type inputExampleDTO struct {
	Input json.RawMessage `json:"input"`
}

type toolChoiceDTO struct {
	Type     string `json:"type"`
	ToolName string `json:"toolName,omitempty"`
}

type responseFormatDTO struct {
	Type        string          `json:"type"`
	Schema      json.RawMessage `json:"schema,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
}

type toolResultOutputDTO struct {
	Type            string             `json:"type"`
	Value           json.RawMessage    `json:"value,omitempty"`
	Reason          string             `json:"reason,omitempty"`
	ProviderOptions providerOptionsDTO `json:"providerOptions,omitempty"`
}

type toolResultContentDTO struct {
	Type            string             `json:"type"`
	Text            *string            `json:"text,omitempty"`
	Data            json.RawMessage    `json:"data,omitempty"`
	MediaType       *string            `json:"mediaType,omitempty"`
	Filename        string             `json:"filename,omitempty"`
	ProviderOptions providerOptionsDTO `json:"providerOptions,omitempty"`
}

// EncodeCallOptions encodes canonical LanguageModelV4 call options without
// invoking provider CallOptions or nested polymorphic JSON methods.
func EncodeCallOptions(options provider.CallOptions) ([]byte, error) {
	dto, err := encodeCallOptions(options)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: encoding call options: %w", err)
	}
	return data, nil
}

func decodeCallOptionsJSON(data []byte) (provider.CallOptions, error) {
	object, err := decodeObject(data, "call options")
	if err != nil {
		return provider.CallOptions{}, err
	}
	if _, err := requireField(object, "prompt", "call options"); err != nil {
		return provider.CallOptions{}, err
	}
	if _, exists := object["abortSignal"]; exists {
		return provider.CallOptions{}, errors.New("providerwirev4: abortSignal is transport-private and is not supported")
	}
	if err := rejectNullFields(object, "call options", "tools", "toolChoice", "maxOutputTokens", "temperature", "topP", "topK", "presencePenalty", "frequencyPenalty", "stopSequences", "responseFormat", "seed", "reasoning", "includeRawChunks", "headers", "providerOptions"); err != nil {
		return provider.CallOptions{}, err
	}
	if raw, exists := object["stopSequences"]; exists {
		if err := validateStringArray(raw, "stopSequences"); err != nil {
			return provider.CallOptions{}, err
		}
	}
	if raw, exists := object["headers"]; exists {
		if err := validateStringMap(raw, "headers"); err != nil {
			return provider.CallOptions{}, err
		}
	}
	for field, context := range map[string]string{"toolChoice": "tool choice", "responseFormat": "response format"} {
		raw, exists := object[field]
		if !exists {
			continue
		}
		nested, err := decodeObject(raw, context)
		if err != nil {
			return provider.CallOptions{}, err
		}
		variant, err := decodeRequiredString(nested, "type", context)
		if err != nil {
			return provider.CallOptions{}, err
		}
		fields := []string{"type"}
		if field == "toolChoice" && provider.ToolChoiceType(variant) == provider.ToolChoiceTool {
			fields = append(fields, "toolName")
		}
		if field == "responseFormat" && provider.ResponseFormatType(variant) == provider.ResponseFormatJSON {
			fields = append(fields, "schema", "name", "description")
		}
		if err := rejectNullFields(nested, context, fields...); err != nil {
			return provider.CallOptions{}, err
		}
		selected := make(map[string]json.RawMessage, len(fields))
		for _, selectedField := range fields {
			if value, ok := nested[selectedField]; ok {
				selected[selectedField] = value
			}
		}
		object[field], err = json.Marshal(selected)
		if err != nil {
			return provider.CallOptions{}, err
		}
	}
	data, err = json.Marshal(object)
	if err != nil {
		return provider.CallOptions{}, err
	}
	var dto callOptionsDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return provider.CallOptions{}, fmt.Errorf("providerwirev4: decoding call options: %w", err)
	}
	return decodeCallOptions(dto)
}

func encodeCallOptions(options provider.CallOptions) (callOptionsDTO, error) {
	prompt := make([]json.RawMessage, len(options.Prompt))
	for i, message := range options.Prompt {
		encoded, err := encodeMessage(message)
		if err != nil {
			return callOptionsDTO{}, fmt.Errorf("providerwirev4: encoding prompt message %d: %w", i, err)
		}
		prompt[i] = encoded
	}
	tools := make([]toolDTO, len(options.Tools))
	for i, tool := range options.Tools {
		encoded, err := encodeTool(tool)
		if err != nil {
			return callOptionsDTO{}, fmt.Errorf("providerwirev4: encoding tool %d: %w", i, err)
		}
		tools[i] = encoded
	}
	providerOptions, err := encodeProviderOptions(options.ProviderOptions)
	if err != nil {
		return callOptionsDTO{}, err
	}
	providerOptions, err = cleanGatewayOptions(providerOptions)
	if err != nil {
		return callOptionsDTO{}, err
	}

	dto := callOptionsDTO{
		Prompt: prompt, Tools: tools,
		MaxOutputTokens: options.MaxOutputTokens, Temperature: options.Temperature,
		TopP: options.TopP, TopK: options.TopK, PresencePenalty: options.PresencePenalty,
		FrequencyPenalty: options.FrequencyPenalty, StopSequences: options.StopSequences,
		Seed: options.Seed, IncludeRawChunks: options.IncludeRawChunks,
		Headers: options.Headers, ProviderOptions: providerOptions,
	}
	if options.ToolChoice != nil {
		dto.ToolChoice = &toolChoiceDTO{Type: string(options.ToolChoice.Type)}
		if options.ToolChoice.Type == provider.ToolChoiceTool {
			dto.ToolChoice.ToolName = options.ToolChoice.ToolName
		}
		if err := validateToolChoice(*dto.ToolChoice); err != nil {
			return callOptionsDTO{}, err
		}
	}
	if options.ResponseFormat != nil {
		dto.ResponseFormat = &responseFormatDTO{Type: string(options.ResponseFormat.Type)}
		if options.ResponseFormat.Type == provider.ResponseFormatJSON {
			if len(options.ResponseFormat.Schema) > 0 {
				if err := validateJSONObject(options.ResponseFormat.Schema, "response format schema"); err != nil {
					return callOptionsDTO{}, err
				}
			}
			dto.ResponseFormat.Schema = append(json.RawMessage(nil), options.ResponseFormat.Schema...)
			dto.ResponseFormat.Name = options.ResponseFormat.Name
			dto.ResponseFormat.Description = options.ResponseFormat.Description
		}
		if err := validateResponseFormat(*dto.ResponseFormat); err != nil {
			return callOptionsDTO{}, err
		}
	}
	if options.Reasoning != nil {
		value := string(*options.Reasoning)
		dto.Reasoning = &value
		if err := validateReasoning(value); err != nil {
			return callOptionsDTO{}, err
		}
	}
	return dto, nil
}

func decodeCallOptions(dto callOptionsDTO) (provider.CallOptions, error) {
	prompt := make([]provider.Message, len(dto.Prompt))
	for i, message := range dto.Prompt {
		decoded, err := decodeMessage(message)
		if err != nil {
			return provider.CallOptions{}, fmt.Errorf("providerwirev4: decoding prompt message %d: %w", i, err)
		}
		prompt[i] = decoded
	}
	var tools []provider.Tool
	if dto.Tools != nil {
		tools = make([]provider.Tool, len(dto.Tools))
		for i, tool := range dto.Tools {
			decoded, err := decodeTool(tool)
			if err != nil {
				return provider.CallOptions{}, fmt.Errorf("providerwirev4: decoding tool %d: %w", i, err)
			}
			tools[i] = decoded
		}
	}
	encodedProviderOptions, err := cleanGatewayOptions(dto.ProviderOptions)
	if err != nil {
		return provider.CallOptions{}, err
	}
	providerOptions, err := decodeProviderOptions(encodedProviderOptions)
	if err != nil {
		return provider.CallOptions{}, err
	}

	options := provider.CallOptions{
		Prompt: prompt, Tools: tools,
		MaxOutputTokens: dto.MaxOutputTokens, Temperature: dto.Temperature,
		TopP: dto.TopP, TopK: dto.TopK, PresencePenalty: dto.PresencePenalty,
		FrequencyPenalty: dto.FrequencyPenalty, StopSequences: dto.StopSequences,
		Seed: dto.Seed, IncludeRawChunks: dto.IncludeRawChunks,
		Headers: dto.Headers, ProviderOptions: providerOptions,
	}
	if dto.ToolChoice != nil {
		if err := validateToolChoice(*dto.ToolChoice); err != nil {
			return provider.CallOptions{}, err
		}
		options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceType(dto.ToolChoice.Type), ToolName: dto.ToolChoice.ToolName}
	}
	if dto.ResponseFormat != nil {
		if err := validateResponseFormat(*dto.ResponseFormat); err != nil {
			return provider.CallOptions{}, err
		}
		if len(dto.ResponseFormat.Schema) > 0 {
			if err := validateJSONObject(dto.ResponseFormat.Schema, "response format schema"); err != nil {
				return provider.CallOptions{}, err
			}
		}
		options.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatType(dto.ResponseFormat.Type), Schema: append(json.RawMessage(nil), dto.ResponseFormat.Schema...), Name: dto.ResponseFormat.Name, Description: dto.ResponseFormat.Description}
	}
	if dto.Reasoning != nil {
		if err := validateReasoning(*dto.Reasoning); err != nil {
			return provider.CallOptions{}, err
		}
		value := provider.ReasoningEffort(*dto.Reasoning)
		options.Reasoning = &value
	}
	return options, nil
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
	fields := []string{"type", "providerOptions"}
	switch provider.ContentPartType(variant) {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		fields = append(fields, "text")
	case provider.ContentPartTypeFile:
		fields = append(fields, "data", "filename", "mediaType")
	case provider.ContentPartTypeReasoningFile:
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
		part.Data, err = decodeData(dto.Data, part.Type == provider.ContentPartTypeFile)
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

func encodeTool(tool provider.Tool) (toolDTO, error) {
	dto := toolDTO{Type: string(tool.Type), Name: tool.Name}
	switch tool.Type {
	case provider.ToolTypeFunction:
		providerOptions, err := encodeNestedProviderOptions(tool.ProviderOptions, "tool")
		if err != nil {
			return toolDTO{}, err
		}
		dto.Description, dto.Strict, dto.ProviderOptions = tool.Description, tool.Strict, providerOptions
		if tool.Name == "" {
			return toolDTO{}, errors.New("providerwirev4: function tool name is required")
		}
		if err := validateJSONObject(tool.InputSchema, "function tool input schema"); err != nil {
			return toolDTO{}, err
		}
		dto.InputSchema = append(json.RawMessage(nil), tool.InputSchema...)
		dto.InputExamples = make([]inputExampleDTO, len(tool.InputExamples))
		for i, example := range tool.InputExamples {
			if _, err := decodeObject(example.Input, "tool input example"); err != nil {
				return toolDTO{}, err
			}
			dto.InputExamples[i] = inputExampleDTO{Input: append(json.RawMessage(nil), example.Input...)}
		}
	case provider.ToolTypeProvider:
		if len(tool.ProviderOptions) > 0 {
			return toolDTO{}, errors.New("providerwirev4: provider tool providerOptions are not in LanguageModelV4")
		}
		if tool.Name == "" {
			return toolDTO{}, errors.New("providerwirev4: provider tool name is required")
		}
		if err := validateQualifiedIdentifier(tool.ID, "provider tool ID"); err != nil {
			return toolDTO{}, err
		}
		if tool.Args == nil {
			return toolDTO{}, errors.New("providerwirev4: provider tool args object is required")
		}
		dto.ID = tool.ID
		args := make(map[string]json.RawMessage, len(tool.Args))
		for key, value := range tool.Args {
			if err := validateJSON(value, fmt.Sprintf("provider tool argument %q", key)); err != nil {
				return toolDTO{}, err
			}
			args[key] = append(json.RawMessage(nil), value...)
		}
		dto.Args = &args
	default:
		return toolDTO{}, fmt.Errorf("providerwirev4: unsupported tool type %q", tool.Type)
	}
	return dto, nil
}

func decodeTool(dto toolDTO) (provider.Tool, error) {
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool")
	if err != nil {
		return provider.Tool{}, err
	}
	tool := provider.Tool{Type: provider.ToolType(dto.Type), Name: dto.Name}
	switch tool.Type {
	case provider.ToolTypeFunction:
		tool.Description, tool.Strict, tool.ProviderOptions = dto.Description, dto.Strict, providerOptions
		if tool.Name == "" {
			return provider.Tool{}, errors.New("providerwirev4: function tool name is required")
		}
		if err := validateJSONObject(dto.InputSchema, "function tool input schema"); err != nil {
			return provider.Tool{}, err
		}
		tool.InputSchema = append(json.RawMessage(nil), dto.InputSchema...)
		tool.InputExamples = make([]provider.InputExample, len(dto.InputExamples))
		for i, example := range dto.InputExamples {
			if _, err := decodeObject(example.Input, "tool input example"); err != nil {
				return provider.Tool{}, err
			}
			tool.InputExamples[i] = provider.InputExample{Input: append(json.RawMessage(nil), example.Input...)}
		}
	case provider.ToolTypeProvider:
		tool.ID = dto.ID
		if tool.Name == "" {
			return provider.Tool{}, errors.New("providerwirev4: provider tool name is required")
		}
		if err := validateQualifiedIdentifier(tool.ID, "provider tool ID"); err != nil {
			return provider.Tool{}, err
		}
		if dto.Args == nil {
			return provider.Tool{}, errors.New("providerwirev4: provider tool args object is required")
		}
		tool.Args = make(map[string]json.RawMessage, len(*dto.Args))
		for key, value := range *dto.Args {
			if err := validateJSON(value, fmt.Sprintf("provider tool argument %q", key)); err != nil {
				return provider.Tool{}, err
			}
			tool.Args[key] = append(json.RawMessage(nil), value...)
		}
	default:
		return provider.Tool{}, fmt.Errorf("providerwirev4: unsupported tool type %q", dto.Type)
	}
	return tool, nil
}

func validateToolChoice(choice toolChoiceDTO) error {
	switch provider.ToolChoiceType(choice.Type) {
	case provider.ToolChoiceAuto, provider.ToolChoiceNone, provider.ToolChoiceRequired:
	case provider.ToolChoiceTool:
		if choice.ToolName == "" {
			return errors.New("providerwirev4: tool choice toolName is required")
		}
	default:
		return fmt.Errorf("providerwirev4: unsupported tool choice %q", choice.Type)
	}
	return nil
}

func validateResponseFormat(format responseFormatDTO) error {
	switch provider.ResponseFormatType(format.Type) {
	case provider.ResponseFormatText:
	case provider.ResponseFormatJSON:
	case "":
		return errors.New("providerwirev4: response format type is required")
	default:
		return fmt.Errorf("providerwirev4: unsupported response format %q", format.Type)
	}
	return nil
}

func validateReasoning(value string) error {
	switch provider.ReasoningEffort(value) {
	case provider.ReasoningProviderDefault, provider.ReasoningNone, provider.ReasoningMinimal,
		provider.ReasoningLow, provider.ReasoningMedium, provider.ReasoningHigh, provider.ReasoningXHigh:
		return nil
	default:
		return fmt.Errorf("providerwirev4: unsupported reasoning effort %q", value)
	}
}

func encodeToolResultOutput(output provider.ToolResultOutput) (toolResultOutputDTO, error) {
	if output.Type == provider.ToolOutputContent && len(output.ProviderOptions) > 0 {
		return toolResultOutputDTO{}, errors.New("providerwirev4: content tool result output providerOptions are not in LanguageModelV4")
	}
	providerOptions, err := encodeNestedProviderOptions(output.ProviderOptions, "tool result output")
	if err != nil {
		return toolResultOutputDTO{}, err
	}
	dto := toolResultOutputDTO{Type: string(output.Type), ProviderOptions: providerOptions}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		dto.Value, err = json.Marshal(output.Text)
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		if err := validateJSON(output.JSON, "tool result JSON value"); err != nil {
			return toolResultOutputDTO{}, err
		}
		dto.Value = append(json.RawMessage(nil), output.JSON...)
	case provider.ToolOutputContent:
		if output.Content == nil {
			return toolResultOutputDTO{}, errors.New("providerwirev4: content tool result value is required")
		}
		content := make([]toolResultContentDTO, len(output.Content))
		for i, value := range output.Content {
			content[i], err = encodeToolResultContent(value)
			if err != nil {
				return toolResultOutputDTO{}, err
			}
		}
		dto.Value, err = json.Marshal(content)
	case provider.ToolOutputExecutionDenied:
		dto.Reason = output.Reason
	default:
		return toolResultOutputDTO{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", output.Type)
	}
	return dto, err
}

func decodeToolResultOutput(data json.RawMessage) (provider.ToolResultOutput, error) {
	object, err := decodeObject(data, "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	variant, err := decodeRequiredString(object, "type", "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	if _, legacy := object["text"]; legacy {
		return provider.ToolResultOutput{}, errors.New("providerwirev4: legacy split tool result fields are not supported")
	}
	if _, legacy := object["json"]; legacy {
		return provider.ToolResultOutput{}, errors.New("providerwirev4: legacy split tool result fields are not supported")
	}
	if _, legacy := object["content"]; legacy {
		return provider.ToolResultOutput{}, errors.New("providerwirev4: legacy split tool result fields are not supported")
	}
	fields := []string{"type"}
	outputType := provider.ToolResultOutputType(variant)
	switch outputType {
	case provider.ToolOutputText, provider.ToolOutputErrorText, provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		fields = append(fields, "value", "providerOptions")
	case provider.ToolOutputContent:
		fields = append(fields, "value")
		if raw, exists := object["providerOptions"]; exists {
			if options, objectErr := decodeObject(raw, "inactive content tool result provider options"); objectErr == nil {
				if _, reserved := options["gateway"]; reserved {
					return provider.ToolResultOutput{}, errors.New("providerwirev4: tool result output must not contain reserved provider option \"gateway\"")
				}
			}
		}
	case provider.ToolOutputExecutionDenied:
		fields = append(fields, "reason", "providerOptions")
	default:
		return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", variant)
	}
	if outputType != provider.ToolOutputContent {
		nonNullFields := []string{"providerOptions"}
		if outputType == provider.ToolOutputExecutionDenied {
			nonNullFields = append(nonNullFields, "reason")
		}
		if err := rejectNullFields(object, "tool result output", nonNullFields...); err != nil {
			return provider.ToolResultOutput{}, err
		}
	}
	var dto toolResultOutputDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.ToolResultOutput{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	output := provider.ToolResultOutput{Type: outputType, Reason: dto.Reason, ProviderOptions: providerOptions}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		value, err := requireField(object, "value", "tool result output")
		if err != nil || json.Unmarshal(value, &output.Text) != nil {
			return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: tool result %q value must be a string", variant)
		}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		value, err := requireJSONField(object, "value", "tool result output")
		if err != nil {
			return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: tool result %q value is required", variant)
		}
		output.JSON = append(json.RawMessage(nil), value...)
	case provider.ToolOutputContent:
		value, err := requireField(object, "value", "tool result output")
		if err != nil {
			return provider.ToolResultOutput{}, err
		}
		var content []json.RawMessage
		if err := json.Unmarshal(value, &content); err != nil || content == nil {
			return provider.ToolResultOutput{}, errors.New("providerwirev4: tool result content value must be an array")
		}
		output.Content = make([]provider.ToolResultContentValue, len(content))
		for i, item := range content {
			decoded, err := decodeToolResultContent(item)
			if err != nil {
				return provider.ToolResultOutput{}, err
			}
			output.Content[i] = decoded
		}
	case provider.ToolOutputExecutionDenied:
	default:
		return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", variant)
	}
	return output, nil
}

func encodeToolResultContent(content provider.ToolResultContentValue) (toolResultContentDTO, error) {
	providerOptions, err := encodeNestedProviderOptions(content.ProviderOptions, "tool result content")
	if err != nil {
		return toolResultContentDTO{}, err
	}
	dto := toolResultContentDTO{Type: string(content.Type), ProviderOptions: providerOptions}
	switch content.Type {
	case provider.ToolContentText:
		dto.Text = &content.Text
	case provider.ToolContentFile:
		data, err := encodeData(content.Data, true)
		if err != nil {
			return toolResultContentDTO{}, err
		}
		dto.Data, err = json.Marshal(data)
		if err != nil {
			return toolResultContentDTO{}, err
		}
		dto.MediaType, dto.Filename = &content.MediaType, content.Filename
	case provider.ToolContentCustom:
	default:
		return toolResultContentDTO{}, fmt.Errorf("providerwirev4: unsupported canonical tool result content type %q", content.Type)
	}
	return dto, nil
}

func decodeToolResultContent(data json.RawMessage) (provider.ToolResultContentValue, error) {
	object, err := decodeObject(data, "tool result content")
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	variant, err := decodeRequiredString(object, "type", "tool result content")
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	fields := []string{"type", "providerOptions"}
	switch provider.ToolResultContentType(variant) {
	case provider.ToolContentText:
		fields = append(fields, "text")
	case provider.ToolContentFile:
		fields = append(fields, "data", "mediaType", "filename")
	case provider.ToolContentCustom:
	case provider.ToolContentFileData, provider.ToolContentFileURL, provider.ToolContentFileReference:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: legacy tool result content type %q is not supported", variant)
	default:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: unsupported tool result content type %q", variant)
	}
	if err := rejectNullFields(object, "tool result content", fields...); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	var dto toolResultContentDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool result content")
	if err != nil {
		return provider.ToolResultContentValue{}, err
	}
	content := provider.ToolResultContentValue{Type: provider.ToolResultContentType(variant), ProviderOptions: providerOptions}
	switch content.Type {
	case provider.ToolContentText:
		if dto.Text == nil {
			return provider.ToolResultContentValue{}, errors.New("providerwirev4: tool result content text is required")
		}
		content.Text = *dto.Text
	case provider.ToolContentFile:
		if len(dto.Data) == 0 || dto.MediaType == nil {
			return provider.ToolResultContentValue{}, errors.New("providerwirev4: tool result file data and mediaType are required")
		}
		content.Data, err = decodeData(dto.Data, true)
		if err != nil {
			return provider.ToolResultContentValue{}, err
		}
		content.MediaType, content.Filename = *dto.MediaType, dto.Filename
	case provider.ToolContentCustom:
	case provider.ToolContentFileData, provider.ToolContentFileURL, provider.ToolContentFileReference:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: legacy tool result content type %q is not supported", variant)
	default:
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: unsupported tool result content type %q", variant)
	}
	return content, nil
}
