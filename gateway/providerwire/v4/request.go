package providerwirev4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/grafana/ai-sdk/gateway/runtime"
	"github.com/grafana/ai-sdk/provider"
)

// DecodedCall contains provider-bound options and separately extracted gateway
// controls.
type DecodedCall struct {
	// CallOptions contains provider-bound options with gateway controls removed.
	CallOptions provider.CallOptions
	// GatewayOptions contains the separately validated private gateway controls.
	GatewayOptions runtime.GatewayOptions
}

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
	Type            string                     `json:"type"`
	Name            string                     `json:"name"`
	Description     string                     `json:"description,omitempty"`
	InputSchema     json.RawMessage            `json:"inputSchema,omitempty"`
	InputExamples   []inputExampleDTO          `json:"inputExamples,omitempty"`
	Strict          *bool                      `json:"strict,omitempty"`
	ID              string                     `json:"id,omitempty"`
	Args            map[string]json.RawMessage `json:"args,omitempty"`
	ProviderOptions providerOptionsDTO         `json:"providerOptions,omitempty"`
	present         map[string]struct{}
}

func (dto toolDTO) MarshalJSON() ([]byte, error) {
	type toolAlias toolDTO
	if dto.Type != string(provider.ToolTypeProvider) {
		return json.Marshal(toolAlias(dto))
	}
	withoutArgs := dto
	withoutArgs.Args = nil
	return json.Marshal(struct {
		toolAlias
		Args map[string]json.RawMessage `json:"args"`
	}{toolAlias: toolAlias(withoutArgs), Args: dto.Args})
}

func (dto *toolDTO) UnmarshalJSON(data []byte) error {
	type toolAlias toolDTO
	var decoded toolAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	if err := rejectNullFields(object, "tool", "type", "name", "description", "inputSchema", "inputExamples", "strict", "id", "args", "providerOptions"); err != nil {
		return err
	}
	*dto = toolDTO(decoded)
	dto.present = make(map[string]struct{}, len(object))
	for field := range object {
		dto.present[field] = struct{}{}
	}
	return nil
}

func (dto toolDTO) has(field string) bool {
	_, ok := dto.present[field]
	return ok
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

// DecodeCallOptions strictly decodes canonical LanguageModelV4 call options
// and extracts the gateway provider-option namespace.
func DecodeCallOptions(data []byte) (DecodedCall, error) {
	object, err := decodeObject(data, "call options")
	if err != nil {
		return DecodedCall{}, err
	}
	if _, err := requireField(object, "prompt", "call options"); err != nil {
		return DecodedCall{}, err
	}
	if _, exists := object["abortSignal"]; exists {
		return DecodedCall{}, errors.New("providerwirev4: abortSignal is transport-private and is not supported")
	}
	if err := rejectNullFields(object, "call options", "tools", "toolChoice", "maxOutputTokens", "temperature", "topP", "topK", "presencePenalty", "frequencyPenalty", "stopSequences", "responseFormat", "seed", "reasoning", "includeRawChunks", "headers", "providerOptions"); err != nil {
		return DecodedCall{}, err
	}
	if raw, exists := object["stopSequences"]; exists {
		if err := validateStringArray(raw, "stopSequences"); err != nil {
			return DecodedCall{}, err
		}
	}
	if raw, exists := object["headers"]; exists {
		if err := validateStringMap(raw, "headers"); err != nil {
			return DecodedCall{}, err
		}
	}
	for field, context := range map[string]string{"toolChoice": "tool choice", "responseFormat": "response format"} {
		if raw, exists := object[field]; exists {
			nested, err := decodeObject(raw, context)
			if err != nil {
				return DecodedCall{}, err
			}
			if err := rejectNullFields(nested, context, "type", "toolName", "schema", "name", "description"); err != nil {
				return DecodedCall{}, err
			}
		}
	}
	var dto callOptionsDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return DecodedCall{}, fmt.Errorf("providerwirev4: decoding call options: %w", err)
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

	dto := callOptionsDTO{
		Prompt: prompt, Tools: tools,
		MaxOutputTokens: options.MaxOutputTokens, Temperature: options.Temperature,
		TopP: options.TopP, TopK: options.TopK, PresencePenalty: options.PresencePenalty,
		FrequencyPenalty: options.FrequencyPenalty, StopSequences: options.StopSequences,
		Seed: options.Seed, IncludeRawChunks: options.IncludeRawChunks,
		Headers: options.Headers, ProviderOptions: providerOptions,
	}
	if options.ToolChoice != nil {
		dto.ToolChoice = &toolChoiceDTO{Type: string(options.ToolChoice.Type), ToolName: options.ToolChoice.ToolName}
		if err := validateToolChoice(*dto.ToolChoice); err != nil {
			return callOptionsDTO{}, err
		}
	}
	if options.ResponseFormat != nil {
		if len(options.ResponseFormat.Schema) > 0 {
			if err := validateJSONObject(options.ResponseFormat.Schema, "response format schema"); err != nil {
				return callOptionsDTO{}, err
			}
		}
		dto.ResponseFormat = &responseFormatDTO{Type: string(options.ResponseFormat.Type), Schema: append(json.RawMessage(nil), options.ResponseFormat.Schema...), Name: options.ResponseFormat.Name, Description: options.ResponseFormat.Description}
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

func decodeCallOptions(dto callOptionsDTO) (DecodedCall, error) {
	prompt := make([]provider.Message, len(dto.Prompt))
	for i, message := range dto.Prompt {
		decoded, err := decodeMessage(message)
		if err != nil {
			return DecodedCall{}, fmt.Errorf("providerwirev4: decoding prompt message %d: %w", i, err)
		}
		prompt[i] = decoded
	}
	var tools []provider.Tool
	if dto.Tools != nil {
		tools = make([]provider.Tool, len(dto.Tools))
		for i, tool := range dto.Tools {
			decoded, err := decodeTool(tool)
			if err != nil {
				return DecodedCall{}, fmt.Errorf("providerwirev4: decoding tool %d: %w", i, err)
			}
			tools[i] = decoded
		}
	}
	gatewayOptions, providerOptions, err := extractGatewayOptions(dto.ProviderOptions)
	if err != nil {
		return DecodedCall{}, err
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
			return DecodedCall{}, err
		}
		options.ToolChoice = &provider.ToolChoice{Type: provider.ToolChoiceType(dto.ToolChoice.Type), ToolName: dto.ToolChoice.ToolName}
	}
	if dto.ResponseFormat != nil {
		if err := validateResponseFormat(*dto.ResponseFormat); err != nil {
			return DecodedCall{}, err
		}
		if len(dto.ResponseFormat.Schema) > 0 {
			if err := validateJSONObject(dto.ResponseFormat.Schema, "response format schema"); err != nil {
				return DecodedCall{}, err
			}
		}
		options.ResponseFormat = &provider.ResponseFormat{Type: provider.ResponseFormatType(dto.ResponseFormat.Type), Schema: append(json.RawMessage(nil), dto.ResponseFormat.Schema...), Name: dto.ResponseFormat.Name, Description: dto.ResponseFormat.Description}
	}
	if dto.Reasoning != nil {
		if err := validateReasoning(*dto.Reasoning); err != nil {
			return DecodedCall{}, err
		}
		value := provider.ReasoningEffort(*dto.Reasoning)
		options.Reasoning = &value
	}
	return DecodedCall{CallOptions: options, GatewayOptions: gatewayOptions}, nil
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
			if part.Type != provider.ContentPartTypeText || len(part.ProviderOptions) > 0 {
				return nil, errors.New("providerwirev4: system messages can contain only plain text")
			}
			if err := validateContentPartFields(part); err != nil {
				return nil, err
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

func encodeContentPart(part provider.ContentPart) (contentPartDTO, error) {
	if err := validateContentPartFields(part); err != nil {
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
	knownFields := []string{"text", "data", "filename", "mediaType", "kind", "sourceType", "id", "url", "title", "toolCallId", "toolName", "input", "output", "providerExecuted", "approvalId", "signature", "isAutomatic", "approved", "reason"}
	allowedFields := map[string][]string{
		"text": {"text"}, "reasoning": {"text"},
		"file": {"data", "filename", "mediaType"}, "reasoning-file": {"data", "mediaType"},
		"custom": {"kind"}, "tool-call": {"toolCallId", "toolName", "input", "providerExecuted"},
		"tool-result":            {"toolCallId", "toolName", "output"},
		"tool-approval-response": {"approvalId", "approved", "reason"},
	}
	allowed, supported := allowedFields[variant]
	if !supported {
		return provider.ContentPart{}, fmt.Errorf("providerwirev4: unsupported prompt content type %q", variant)
	}
	if err := rejectContradictoryFields(object, "content part", knownFields, allowed...); err != nil {
		return provider.ContentPart{}, err
	}
	if err := rejectNullFields(object, "content part", "text", "data", "filename", "mediaType", "kind", "sourceType", "id", "url", "title", "toolCallId", "toolName", "output", "providerExecuted", "approvalId", "signature", "isAutomatic", "approved", "reason", "providerOptions"); err != nil {
		return provider.ContentPart{}, err
	}
	var dto contentPartDTO
	if err := json.Unmarshal(data, &dto); err != nil {
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
		if dto.ToolCallID == nil || dto.ToolName == nil {
			return provider.ContentPart{}, errors.New("providerwirev4: tool call ID and name are required")
		}
		if err := validateJSON(dto.Input, "tool call input"); err != nil {
			return provider.ContentPart{}, err
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		part.Input, part.ProviderExecuted = append(json.RawMessage(nil), dto.Input...), dto.ProviderExecuted
	case provider.ContentPartTypeToolResult:
		if dto.ToolCallID == nil || dto.ToolName == nil || len(dto.Output) == 0 {
			return provider.ContentPart{}, errors.New("providerwirev4: tool result ID, name, and output are required")
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		output, err := decodeToolResultOutput(dto.Output)
		if err != nil {
			return provider.ContentPart{}, err
		}
		part.Output = &output
	case provider.ContentPartTypeToolApprovalResponse:
		if dto.ApprovalID == nil || dto.Approved == nil {
			return provider.ContentPart{}, errors.New("providerwirev4: tool approval response ID and approved are required")
		}
		part.ApprovalID, part.Approved, part.Reason = *dto.ApprovalID, dto.Approved, dto.Reason
	default:
		return provider.ContentPart{}, fmt.Errorf("providerwirev4: unsupported prompt content type %q", variant)
	}
	return part, nil
}

func encodeTool(tool provider.Tool) (toolDTO, error) {
	if err := validateToolFields(tool); err != nil {
		return toolDTO{}, err
	}
	providerOptions, err := encodeNestedProviderOptions(tool.ProviderOptions, "tool")
	if err != nil {
		return toolDTO{}, err
	}
	dto := toolDTO{Type: string(tool.Type), Name: tool.Name, Description: tool.Description, Strict: tool.Strict, ID: tool.ID, ProviderOptions: providerOptions}
	switch tool.Type {
	case provider.ToolTypeFunction:
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
		dto.Args = make(map[string]json.RawMessage, len(tool.Args))
		for key, value := range tool.Args {
			if err := validateJSON(value, fmt.Sprintf("provider tool argument %q", key)); err != nil {
				return toolDTO{}, err
			}
			dto.Args[key] = append(json.RawMessage(nil), value...)
		}
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
	tool := provider.Tool{Type: provider.ToolType(dto.Type), Name: dto.Name, Description: dto.Description, Strict: dto.Strict, ID: dto.ID, ProviderOptions: providerOptions}
	switch tool.Type {
	case provider.ToolTypeFunction:
		if dto.has("id") || dto.has("args") {
			return provider.Tool{}, errors.New("providerwirev4: function tool contains provider-tool fields")
		}
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
		if dto.has("providerOptions") {
			return provider.Tool{}, errors.New("providerwirev4: provider tool providerOptions are not in LanguageModelV4")
		}
		for _, field := range []string{"description", "inputSchema", "inputExamples", "strict"} {
			if dto.has(field) {
				return provider.Tool{}, errors.New("providerwirev4: provider tool contains function-tool fields")
			}
		}
		if tool.Name == "" {
			return provider.Tool{}, errors.New("providerwirev4: provider tool name is required")
		}
		if err := validateQualifiedIdentifier(tool.ID, "provider tool ID"); err != nil {
			return provider.Tool{}, err
		}
		if !dto.has("args") || dto.Args == nil {
			return provider.Tool{}, errors.New("providerwirev4: provider tool args object is required")
		}
		tool.Args = make(map[string]json.RawMessage, len(dto.Args))
		for key, value := range dto.Args {
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
		if choice.ToolName != "" {
			return errors.New("providerwirev4: non-tool tool choice must not contain toolName")
		}
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
		if len(format.Schema) > 0 || format.Name != "" || format.Description != "" {
			return errors.New("providerwirev4: text response format must not contain JSON fields")
		}
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
	providerOptions, err := encodeNestedProviderOptions(output.ProviderOptions, "tool result output")
	if err != nil {
		return toolResultOutputDTO{}, err
	}
	dto := toolResultOutputDTO{Type: string(output.Type), ProviderOptions: providerOptions}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		if len(output.JSON) > 0 || output.Content != nil || output.Reason != "" {
			return toolResultOutputDTO{}, errors.New("providerwirev4: text tool result contains contradictory fields")
		}
		dto.Value, err = json.Marshal(output.Text)
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		if output.Text != "" || output.Content != nil || output.Reason != "" {
			return toolResultOutputDTO{}, errors.New("providerwirev4: JSON tool result contains contradictory fields")
		}
		if err := validateJSON(output.JSON, "tool result JSON value"); err != nil {
			return toolResultOutputDTO{}, err
		}
		dto.Value = append(json.RawMessage(nil), output.JSON...)
	case provider.ToolOutputContent:
		if output.Text != "" || len(output.JSON) > 0 || output.Reason != "" {
			return toolResultOutputDTO{}, errors.New("providerwirev4: content tool result contains contradictory fields")
		}
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
		if output.Text != "" || len(output.JSON) > 0 || output.Content != nil {
			return toolResultOutputDTO{}, errors.New("providerwirev4: execution-denied output contains contradictory value fields")
		}
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
	knownFields := []string{"value", "reason"}
	if variant == string(provider.ToolOutputExecutionDenied) {
		if err := rejectContradictoryFields(object, "tool result output", knownFields, "reason"); err != nil {
			return provider.ToolResultOutput{}, err
		}
		if err := rejectNullFields(object, "tool result output", "reason", "providerOptions"); err != nil {
			return provider.ToolResultOutput{}, err
		}
	} else {
		if err := rejectContradictoryFields(object, "tool result output", knownFields, "value"); err != nil {
			return provider.ToolResultOutput{}, err
		}
		if err := rejectNullFields(object, "tool result output", "providerOptions"); err != nil {
			return provider.ToolResultOutput{}, err
		}
	}
	var dto toolResultOutputDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return provider.ToolResultOutput{}, err
	}
	providerOptions, err := decodeNestedProviderOptions(dto.ProviderOptions, "tool result output")
	if err != nil {
		return provider.ToolResultOutput{}, err
	}
	output := provider.ToolResultOutput{Type: provider.ToolResultOutputType(variant), Reason: dto.Reason, ProviderOptions: providerOptions}
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
		if _, exists := object["value"]; exists {
			return provider.ToolResultOutput{}, errors.New("providerwirev4: execution-denied output must not contain value")
		}
	default:
		return provider.ToolResultOutput{}, fmt.Errorf("providerwirev4: unsupported tool result output type %q", variant)
	}
	return output, nil
}

func encodeToolResultContent(content provider.ToolResultContentValue) (toolResultContentDTO, error) {
	if err := validateToolResultContentFields(content); err != nil {
		return toolResultContentDTO{}, err
	}
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
	knownFields := []string{"text", "data", "mediaType", "filename"}
	allowedFields := map[string][]string{
		"text": {"text"}, "file": {"data", "mediaType", "filename"}, "custom": {},
	}
	allowed, supported := allowedFields[variant]
	if !supported {
		return provider.ToolResultContentValue{}, fmt.Errorf("providerwirev4: unsupported tool result content type %q", variant)
	}
	if err := rejectContradictoryFields(object, "tool result content", knownFields, allowed...); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	if err := rejectNullFields(object, "tool result content", "text", "data", "mediaType", "filename", "providerOptions"); err != nil {
		return provider.ToolResultContentValue{}, err
	}
	var dto toolResultContentDTO
	if err := json.Unmarshal(data, &dto); err != nil {
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
