package providerwirev4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

type generateResultDTO struct {
	Content          []json.RawMessage    `json:"content"`
	FinishReason     finishReasonDTO      `json:"finishReason"`
	Usage            usageDTO             `json:"usage"`
	ProviderMetadata providerMetadataDTO  `json:"providerMetadata,omitempty"`
	Warnings         []warningDTO         `json:"warnings"`
	Request          *requestMetadataDTO  `json:"request,omitempty"`
	Response         *generateResponseDTO `json:"response,omitempty"`
}

type requestMetadataDTO struct {
	Body json.RawMessage `json:"body,omitempty"`
}

type generateResponseDTO struct {
	ID        string            `json:"id,omitempty"`
	ModelID   string            `json:"modelId,omitempty"`
	Timestamp *time.Time        `json:"timestamp,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      json.RawMessage   `json:"body,omitempty"`
}

type generateContentDTO struct {
	Type             string              `json:"type"`
	Text             *string             `json:"text,omitempty"`
	Kind             *string             `json:"kind,omitempty"`
	ApprovalID       *string             `json:"approvalId,omitempty"`
	ToolCallID       *string             `json:"toolCallId,omitempty"`
	ToolName         *string             `json:"toolName,omitempty"`
	Input            *string             `json:"input,omitempty"`
	Result           json.RawMessage     `json:"result,omitempty"`
	IsError          bool                `json:"isError,omitempty"`
	Preliminary      *bool               `json:"preliminary,omitempty"`
	ProviderExecuted bool                `json:"providerExecuted,omitempty"`
	Dynamic          *bool               `json:"dynamic,omitempty"`
	SourceType       *string             `json:"sourceType,omitempty"`
	ID               *string             `json:"id,omitempty"`
	URL              *string             `json:"url,omitempty"`
	Title            *string             `json:"title,omitempty"`
	Data             json.RawMessage     `json:"data,omitempty"`
	MediaType        *string             `json:"mediaType,omitempty"`
	Filename         string              `json:"filename,omitempty"`
	ProviderMetadata providerMetadataDTO `json:"providerMetadata,omitempty"`
}

// encodeGenerateResultJSON encodes one canonical LanguageModelV4 generate result.
func encodeGenerateResultJSON(result *provider.GenerateResult) ([]byte, error) {
	if result == nil {
		return nil, errors.New("providerwirev4: nil generate result")
	}
	dto, err := encodeGenerateResult(result)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: encoding generate result: %w", err)
	}
	return data, nil
}

// DecodeGenerateResult strictly decodes one canonical LanguageModelV4 generate
// result.
func DecodeGenerateResult(data []byte) (*provider.GenerateResult, error) {
	object, err := decodeObject(data, "generate result")
	if err != nil {
		return nil, err
	}
	for _, field := range []string{"content", "finishReason", "usage", "warnings"} {
		if _, err := requireField(object, field, "generate result"); err != nil {
			return nil, err
		}
	}
	if err := rejectNullFields(object, "generate result", "providerMetadata", "request", "response"); err != nil {
		return nil, err
	}
	if responseRaw, exists := object["response"]; exists {
		response, err := decodeObject(responseRaw, "generate response metadata")
		if err != nil {
			return nil, err
		}
		if _, legacy := response["provider"]; legacy {
			return nil, errors.New("providerwirev4: response provider is not in LanguageModelV4")
		}
		if err := rejectNullFields(response, "generate response metadata", "id", "modelId", "timestamp", "headers"); err != nil {
			return nil, err
		}
		if headers, exists := response["headers"]; exists {
			if err := validateStringMap(headers, "generate response headers"); err != nil {
				return nil, err
			}
		}
	}
	var dto generateResultDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, fmt.Errorf("providerwirev4: decoding generate result: %w", err)
	}
	return decodeGenerateResult(dto)
}

func encodeGenerateResult(result *provider.GenerateResult) (generateResultDTO, error) {
	content := make([]json.RawMessage, len(result.Content))
	for i, part := range result.Content {
		dto, err := encodeGenerateContent(part)
		if err != nil {
			return generateResultDTO{}, fmt.Errorf("providerwirev4: encoding generate content %d: %w", i, err)
		}
		content[i], err = json.Marshal(dto)
		if err != nil {
			return generateResultDTO{}, err
		}
	}
	metadata, err := encodeProviderMetadata(result.ProviderMetadata)
	if err != nil {
		return generateResultDTO{}, err
	}
	warnings, err := encodeWarnings(result.Warnings)
	if err != nil {
		return generateResultDTO{}, err
	}
	if warnings == nil {
		warnings = []warningDTO{}
	}
	usage, err := encodeUsage(result.Usage)
	if err != nil {
		return generateResultDTO{}, err
	}
	dto := generateResultDTO{
		Content:      content,
		FinishReason: finishReasonDTO{Unified: string(result.FinishReason.Unified), Raw: result.FinishReason.Raw},
		Usage:        usage, ProviderMetadata: metadata, Warnings: warnings,
	}
	if err := validateFinishReason(dto.FinishReason); err != nil {
		return generateResultDTO{}, err
	}
	if result.Request != nil {
		if len(result.Request.Body) > 0 {
			if err := validateJSON(result.Request.Body, "request body metadata"); err != nil {
				return generateResultDTO{}, err
			}
		}
		dto.Request = &requestMetadataDTO{Body: append(json.RawMessage(nil), result.Request.Body...)}
	}
	if result.Response != nil {
		if result.Response.Provider != "" {
			return generateResultDTO{}, errors.New("providerwirev4: response provider is not in LanguageModelV4")
		}
		if len(result.Response.Body) > 0 {
			if err := validateJSON(result.Response.Body, "response body metadata"); err != nil {
				return generateResultDTO{}, err
			}
		}
		response := &generateResponseDTO{
			ID: result.Response.ID, ModelID: result.Response.ModelID,
			Headers: result.Response.Headers, Body: append(json.RawMessage(nil), result.Response.Body...),
		}
		if !result.Response.Timestamp.IsZero() {
			value := result.Response.Timestamp
			response.Timestamp = &value
		}
		dto.Response = response
	}
	return dto, nil
}

func decodeGenerateResult(dto generateResultDTO) (*provider.GenerateResult, error) {
	if dto.Content == nil || dto.Warnings == nil {
		return nil, errors.New("providerwirev4: generate result content and warnings must be arrays")
	}
	if err := validateFinishReason(dto.FinishReason); err != nil {
		return nil, err
	}
	content := make([]provider.GenerateContentPart, len(dto.Content))
	for i, part := range dto.Content {
		decoded, err := decodeGenerateContent(part)
		if err != nil {
			return nil, fmt.Errorf("providerwirev4: decoding generate content %d: %w", i, err)
		}
		content[i] = decoded
	}
	usage, err := decodeUsage(dto.Usage)
	if err != nil {
		return nil, err
	}
	metadata, err := decodeProviderMetadata(dto.ProviderMetadata)
	if err != nil {
		return nil, err
	}
	warnings, err := decodeWarnings(dto.Warnings)
	if err != nil {
		return nil, err
	}
	result := &provider.GenerateResult{
		Content:      content,
		FinishReason: provider.FinishReason{Unified: provider.UnifiedFinishReason(dto.FinishReason.Unified), Raw: dto.FinishReason.Raw},
		Usage:        usage, ProviderMetadata: metadata, Warnings: warnings,
	}
	if dto.Request != nil {
		if len(dto.Request.Body) > 0 {
			if err := validateJSON(dto.Request.Body, "request body metadata"); err != nil {
				return nil, err
			}
		}
		result.Request = &provider.RequestMetadata{Body: append(json.RawMessage(nil), dto.Request.Body...)}
	}
	if dto.Response != nil {
		if len(dto.Response.Body) > 0 {
			if err := validateJSON(dto.Response.Body, "response body metadata"); err != nil {
				return nil, err
			}
		}
		response := &provider.GenerateResponse{
			ResponseMetadata: provider.ResponseMetadata{ID: dto.Response.ID, ModelID: dto.Response.ModelID},
			Headers:          dto.Response.Headers, Body: append(json.RawMessage(nil), dto.Response.Body...),
		}
		if dto.Response.Timestamp != nil {
			response.Timestamp = *dto.Response.Timestamp
		}
		result.Response = response
	}
	return result, nil
}

func encodeGenerateContent(part provider.GenerateContentPart) (generateContentDTO, error) {
	metadata, err := encodeProviderMetadata(part.ProviderMetadata)
	if err != nil {
		return generateContentDTO{}, err
	}
	dto := generateContentDTO{Type: string(part.Type), ProviderMetadata: metadata}
	switch part.Type {
	case provider.ContentText, provider.ContentReasoning:
		dto.Text = &part.Text
	case provider.ContentToolCall:
		if part.ToolCallID == "" || part.ToolName == "" {
			return generateContentDTO{}, errors.New("providerwirev4: generated tool call ID and name are required")
		}
		if err := validateJSONObject(part.Input, "generated tool call input"); err != nil {
			return generateContentDTO{}, err
		}
		input := string(part.Input)
		dto.ToolCallID, dto.ToolName, dto.Input = &part.ToolCallID, &part.ToolName, &input
		dto.ProviderExecuted, dto.Dynamic = part.ProviderExecuted, part.Dynamic
	case provider.ContentToolResult:
		if part.ToolCallID == "" || part.ToolName == "" {
			return generateContentDTO{}, errors.New("providerwirev4: generated tool result ID and name are required")
		}
		if err := validateNonNullJSON(part.Result, "generated tool result"); err != nil {
			return generateContentDTO{}, err
		}
		dto.ToolCallID, dto.ToolName = &part.ToolCallID, &part.ToolName
		dto.Result = append(json.RawMessage(nil), part.Result...)
		dto.IsError, dto.Preliminary, dto.Dynamic = part.IsError, part.Preliminary, part.Dynamic
	case provider.ContentSource:
		if part.ID == "" {
			return generateContentDTO{}, errors.New("providerwirev4: generated source ID is required")
		}
		sourceType := string(part.SourceType)
		dto.SourceType, dto.ID = &sourceType, &part.ID
		switch part.SourceType {
		case provider.SourceTypeURL:
			if part.URL == "" {
				return generateContentDTO{}, errors.New("providerwirev4: URL source URL is required")
			}
			dto.URL = &part.URL
			if part.Title != "" {
				dto.Title = &part.Title
			}
		case provider.SourceTypeDocument:
			if part.Title == "" || part.MediaType == "" {
				return generateContentDTO{}, errors.New("providerwirev4: document source title and mediaType are required")
			}
			dto.Title, dto.MediaType = &part.Title, &part.MediaType
			dto.Filename = part.Filename
		default:
			return generateContentDTO{}, fmt.Errorf("providerwirev4: unsupported source type %q", part.SourceType)
		}
	case provider.ContentFile, provider.ContentReasoningFile:
		if part.Filename != "" {
			return generateContentDTO{}, fmt.Errorf("providerwirev4: generated %s filename is not in LanguageModelV4", part.Type)
		}
		data, err := encodeData(part.Data, false)
		if err != nil {
			return generateContentDTO{}, err
		}
		dto.Data, err = json.Marshal(data)
		if err != nil {
			return generateContentDTO{}, err
		}
		dto.MediaType = &part.MediaType
	case provider.ContentCustom:
		if err := validateQualifiedIdentifier(part.Kind, "generated custom kind"); err != nil {
			return generateContentDTO{}, err
		}
		dto.Kind = &part.Kind
	case provider.ContentToolApprovalRequest:
		if part.ApprovalID == "" || part.ToolCallID == "" {
			return generateContentDTO{}, errors.New("providerwirev4: generated tool approval IDs are required")
		}
		dto.ApprovalID, dto.ToolCallID = &part.ApprovalID, &part.ToolCallID
	default:
		return generateContentDTO{}, fmt.Errorf("providerwirev4: unsupported generate content type %q", part.Type)
	}
	return dto, nil
}

func decodeGenerateContent(data json.RawMessage) (provider.GenerateContentPart, error) {
	object, err := decodeObject(data, "generate content")
	if err != nil {
		return provider.GenerateContentPart{}, err
	}
	variant, err := decodeRequiredString(object, "type", "generate content")
	if err != nil {
		return provider.GenerateContentPart{}, err
	}
	fields := []string{"type", "providerMetadata"}
	switch provider.GenerateContentType(variant) {
	case provider.ContentText, provider.ContentReasoning:
		fields = append(fields, "text")
	case provider.ContentToolCall:
		fields = append(fields, "toolCallId", "toolName", "input", "providerExecuted", "dynamic")
	case provider.ContentToolResult:
		fields = append(fields, "toolCallId", "toolName", "result", "isError", "preliminary", "dynamic")
	case provider.ContentSource:
		sourceType, err := decodeRequiredString(object, "sourceType", "generate source")
		if err != nil {
			return provider.GenerateContentPart{}, err
		}
		switch provider.SourceType(sourceType) {
		case provider.SourceTypeURL:
			fields = append(fields, "sourceType", "id", "url", "title")
		case provider.SourceTypeDocument:
			fields = append(fields, "sourceType", "id", "title", "mediaType", "filename")
		default:
			return provider.GenerateContentPart{}, fmt.Errorf("providerwirev4: unsupported source type %q", sourceType)
		}
	case provider.ContentFile, provider.ContentReasoningFile:
		if _, exists := object["filename"]; exists {
			return provider.GenerateContentPart{}, errors.New("providerwirev4: generated file filename is not in LanguageModelV4")
		}
		fields = append(fields, "data", "mediaType")
	case provider.ContentCustom:
		fields = append(fields, "kind")
	case provider.ContentToolApprovalRequest:
		fields = append(fields, "approvalId", "toolCallId")
	default:
		return provider.GenerateContentPart{}, fmt.Errorf("providerwirev4: unsupported generate content type %q", variant)
	}
	if err := rejectNullFields(object, "generate content", fields...); err != nil {
		return provider.GenerateContentPart{}, err
	}
	var dto generateContentDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.GenerateContentPart{}, err
	}
	metadata, err := decodeProviderMetadata(dto.ProviderMetadata)
	if err != nil {
		return provider.GenerateContentPart{}, err
	}
	part := provider.GenerateContentPart{Type: provider.GenerateContentType(variant), ProviderMetadata: metadata}
	switch part.Type {
	case provider.ContentText, provider.ContentReasoning:
		if dto.Text == nil {
			return provider.GenerateContentPart{}, fmt.Errorf("providerwirev4: generated %s text is required", variant)
		}
		part.Text = *dto.Text
	case provider.ContentToolCall:
		if dto.ToolCallID == nil || dto.ToolName == nil || dto.Input == nil {
			return provider.GenerateContentPart{}, errors.New("providerwirev4: generated tool call ID, name, and input are required")
		}
		part.Input = json.RawMessage(*dto.Input)
		if err := validateJSONObject(part.Input, "generated tool call input"); err != nil {
			return provider.GenerateContentPart{}, err
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		part.ProviderExecuted, part.Dynamic = dto.ProviderExecuted, dto.Dynamic
	case provider.ContentToolResult:
		if dto.ToolCallID == nil || dto.ToolName == nil {
			return provider.GenerateContentPart{}, errors.New("providerwirev4: generated tool result ID and name are required")
		}
		if err := validateNonNullJSON(dto.Result, "generated tool result"); err != nil {
			return provider.GenerateContentPart{}, err
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		part.Result, part.IsError, part.Preliminary, part.Dynamic = append(json.RawMessage(nil), dto.Result...), dto.IsError, dto.Preliminary, dto.Dynamic
	case provider.ContentSource:
		if dto.SourceType == nil || dto.ID == nil {
			return provider.GenerateContentPart{}, errors.New("providerwirev4: generated source type and ID are required")
		}
		part.SourceType, part.ID = provider.SourceType(*dto.SourceType), *dto.ID
		if dto.URL != nil {
			part.URL = *dto.URL
		}
		if dto.Title != nil {
			part.Title = *dto.Title
		}
		if dto.MediaType != nil {
			part.MediaType = *dto.MediaType
		}
		part.Filename = dto.Filename
		switch part.SourceType {
		case provider.SourceTypeURL:
			if part.URL == "" {
				return provider.GenerateContentPart{}, errors.New("providerwirev4: URL source URL is required")
			}
			part.MediaType, part.Filename = "", ""
		case provider.SourceTypeDocument:
			if part.Title == "" || part.MediaType == "" {
				return provider.GenerateContentPart{}, errors.New("providerwirev4: document source title and mediaType are required")
			}
			part.URL = ""
		default:
			return provider.GenerateContentPart{}, fmt.Errorf("providerwirev4: unsupported source type %q", part.SourceType)
		}
	case provider.ContentFile, provider.ContentReasoningFile:
		if len(dto.Data) == 0 || dto.MediaType == nil {
			return provider.GenerateContentPart{}, fmt.Errorf("providerwirev4: generated %s data and mediaType are required", variant)
		}
		part.Data, err = decodeData(dto.Data, false)
		if err != nil {
			return provider.GenerateContentPart{}, err
		}
		part.MediaType = *dto.MediaType
	case provider.ContentCustom:
		if dto.Kind == nil {
			return provider.GenerateContentPart{}, errors.New("providerwirev4: generated custom kind is required")
		}
		if err := validateQualifiedIdentifier(*dto.Kind, "generated custom kind"); err != nil {
			return provider.GenerateContentPart{}, err
		}
		part.Kind = *dto.Kind
	case provider.ContentToolApprovalRequest:
		if dto.ApprovalID == nil || dto.ToolCallID == nil {
			return provider.GenerateContentPart{}, errors.New("providerwirev4: generated tool approval IDs are required")
		}
		part.ApprovalID, part.ToolCallID = *dto.ApprovalID, *dto.ToolCallID
	default:
		return provider.GenerateContentPart{}, fmt.Errorf("providerwirev4: unsupported generate content type %q", variant)
	}
	return part, nil
}

func validateNonNullJSON(value json.RawMessage, context string) error {
	if err := validateJSON(value, context); err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return fmt.Errorf("providerwirev4: %s must not be null", context)
	}
	return nil
}
