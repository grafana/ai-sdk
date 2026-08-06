package providerwirev4

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/grafana/ai-sdk/provider"
)

type streamPartDTO struct {
	Type             string              `json:"type"`
	ID               *string             `json:"id,omitempty"`
	Delta            *string             `json:"delta,omitempty"`
	ToolCallID       *string             `json:"toolCallId,omitempty"`
	ToolName         *string             `json:"toolName,omitempty"`
	Input            *string             `json:"input,omitempty"`
	ProviderExecuted bool                `json:"providerExecuted,omitempty"`
	IsError          bool                `json:"isError,omitempty"`
	Dynamic          *bool               `json:"dynamic,omitempty"`
	Preliminary      *bool               `json:"preliminary,omitempty"`
	Kind             *string             `json:"kind,omitempty"`
	ApprovalID       *string             `json:"approvalId,omitempty"`
	Signature        string              `json:"signature,omitempty"`
	SourceType       *string             `json:"sourceType,omitempty"`
	URL              string              `json:"url,omitempty"`
	Title            string              `json:"title,omitempty"`
	Data             json.RawMessage     `json:"data,omitempty"`
	MediaType        *string             `json:"mediaType,omitempty"`
	Filename         string              `json:"filename,omitempty"`
	Warnings         *[]warningDTO       `json:"warnings,omitempty"`
	ModelID          string              `json:"modelId,omitempty"`
	Provider         string              `json:"provider,omitempty"`
	Timestamp        *time.Time          `json:"timestamp,omitempty"`
	ResponseHeaders  map[string]string   `json:"responseHeaders,omitempty"`
	Usage            *usageDTO           `json:"usage,omitempty"`
	FinishReason     *finishReasonDTO    `json:"finishReason,omitempty"`
	RawValue         json.RawMessage     `json:"rawValue,omitempty"`
	Error            json.RawMessage     `json:"error,omitempty"`
	Result           json.RawMessage     `json:"result,omitempty"`
	ProviderMetadata providerMetadataDTO `json:"providerMetadata,omitempty"`
}

type apiCallErrorDTO struct {
	Message           string              `json:"message"`
	StatusCode        int                 `json:"statusCode"`
	URL               string              `json:"url,omitempty"`
	RequestBodyValues json.RawMessage     `json:"requestBodyValues,omitempty"`
	ResponseHeaders   map[string][]string `json:"responseHeaders,omitempty"`
	ResponseBody      string              `json:"responseBody,omitempty"`
	IsRetryable       bool                `json:"isRetryable"`
	Data              json.RawMessage     `json:"data,omitempty"`
}

// encodeStreamPartJSON encodes one canonical LanguageModelV4 stream part.
func encodeStreamPartJSON(part provider.StreamPart) ([]byte, error) {
	dto, err := encodeStreamPart(part)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return nil, fmt.Errorf("providerwirev4: encoding stream part: %w", err)
	}
	return data, nil
}

// DecodeStreamPart strictly decodes one canonical LanguageModelV4 stream part.
func DecodeStreamPart(data []byte) (provider.StreamPart, error) {
	object, err := decodeObject(data, "stream part")
	if err != nil {
		return provider.StreamPart{}, err
	}
	variant, err := decodeRequiredString(object, "type", "stream part")
	if err != nil {
		return provider.StreamPart{}, err
	}
	for _, legacy := range []string{"source", "apiCallError", "fileData", "output", "responseId", "provider", "responseHeaders", "signature", "approved", "reason"} {
		if _, exists := object[legacy]; exists {
			return provider.StreamPart{}, fmt.Errorf("providerwirev4: legacy stream field %q is not supported", legacy)
		}
	}
	fields := []string{"type"}
	switch provider.StreamPartType(variant) {
	case provider.PartTextStart, provider.PartTextEnd, provider.PartReasoningStart, provider.PartReasoningEnd, provider.PartToolInputEnd:
		fields = append(fields, "id", "providerMetadata")
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolInputDelta:
		fields = append(fields, "id", "delta", "providerMetadata")
	case provider.PartToolInputStart:
		fields = append(fields, "id", "toolName", "providerExecuted", "dynamic", "title", "providerMetadata")
	case provider.PartToolCall:
		fields = append(fields, "toolCallId", "toolName", "input", "providerExecuted", "dynamic", "providerMetadata")
	case provider.PartToolResult:
		fields = append(fields, "toolCallId", "toolName", "result", "isError", "preliminary", "dynamic", "providerMetadata")
	case provider.PartSource:
		sourceType, err := decodeRequiredString(object, "sourceType", "stream source")
		if err != nil {
			return provider.StreamPart{}, err
		}
		switch provider.SourceType(sourceType) {
		case provider.SourceTypeURL:
			fields = append(fields, "sourceType", "id", "url", "title", "providerMetadata")
		case provider.SourceTypeDocument:
			fields = append(fields, "sourceType", "id", "title", "mediaType", "filename", "providerMetadata")
		default:
			return provider.StreamPart{}, fmt.Errorf("providerwirev4: unsupported source type %q", sourceType)
		}
	case provider.PartFile, provider.PartReasoningFile:
		if _, exists := object["filename"]; exists {
			return provider.StreamPart{}, errors.New("providerwirev4: stream file filename is not in LanguageModelV4")
		}
		fields = append(fields, "data", "mediaType", "providerMetadata")
	case provider.PartStreamStart:
		fields = append(fields, "warnings")
	case provider.PartResponseMeta:
		fields = append(fields, "id", "modelId", "timestamp")
	case provider.PartFinish:
		fields = append(fields, "usage", "finishReason", "providerMetadata")
	case provider.PartRaw:
		fields = append(fields, "rawValue")
	case provider.PartError:
		fields = append(fields, "error")
	case provider.PartToolApprovalRequest:
		fields = append(fields, "approvalId", "toolCallId", "providerMetadata")
	case provider.PartCustom:
		fields = append(fields, "kind", "providerMetadata")
	default:
		return provider.StreamPart{}, fmt.Errorf("providerwirev4: unsupported stream part type %q", variant)
	}
	nonNullFields := fields
	if provider.StreamPartType(variant) == provider.PartRaw {
		nonNullFields = []string{"type"}
	}
	if err := rejectNullFields(object, "stream part", nonNullFields...); err != nil {
		return provider.StreamPart{}, err
	}
	var dto streamPartDTO
	if err := decodeSelectedObject(object, &dto, fields...); err != nil {
		return provider.StreamPart{}, fmt.Errorf("providerwirev4: decoding stream part: %w", err)
	}
	return decodeStreamPart(variant, object, dto)
}

func encodeStreamPart(part provider.StreamPart) (streamPartDTO, error) {
	if part.Signature != "" || part.Approved != nil || part.Reason != "" {
		return streamPartDTO{}, errors.New("providerwirev4: stream part contains private fields")
	}
	metadata, err := encodeProviderMetadata(part.ProviderMetadata)
	if err != nil {
		return streamPartDTO{}, err
	}
	dto := streamPartDTO{Type: string(part.Type)}
	requireID := func() error {
		if part.ID == "" {
			return fmt.Errorf("providerwirev4: stream part %q ID is required", part.Type)
		}
		dto.ID = &part.ID
		return nil
	}
	switch part.Type {
	case provider.PartTextStart, provider.PartTextEnd, provider.PartReasoningStart, provider.PartReasoningEnd,
		provider.PartToolInputEnd:
		if err := requireID(); err != nil {
			return streamPartDTO{}, err
		}
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolInputDelta:
		if err := requireID(); err != nil {
			return streamPartDTO{}, err
		}
		dto.Delta = &part.Delta
	case provider.PartToolInputStart:
		if err := requireID(); err != nil {
			return streamPartDTO{}, err
		}
		if part.ToolName == "" {
			return streamPartDTO{}, errors.New("providerwirev4: tool input start name is required")
		}
		dto.ToolName, dto.ProviderExecuted = &part.ToolName, part.ProviderExecuted
		dto.Dynamic, dto.Title = part.Dynamic, part.Title
	case provider.PartToolCall:
		if part.ToolCallID == "" || part.ToolName == "" {
			return streamPartDTO{}, errors.New("providerwirev4: stream tool call ID and name are required")
		}
		if err := validateJSONObject(json.RawMessage(part.Input), "stream tool call input"); err != nil {
			return streamPartDTO{}, err
		}
		dto.ToolCallID, dto.ToolName, dto.Input = &part.ToolCallID, &part.ToolName, &part.Input
		dto.ProviderExecuted, dto.Dynamic = part.ProviderExecuted, part.Dynamic
	case provider.PartToolResult:
		if part.ToolCallID == "" || part.ToolName == "" {
			return streamPartDTO{}, errors.New("providerwirev4: stream tool result ID and name are required")
		}
		if err := validateNonNullJSON(part.Result, "stream tool result"); err != nil {
			return streamPartDTO{}, err
		}
		dto.ToolCallID, dto.ToolName = &part.ToolCallID, &part.ToolName
		dto.Result, dto.IsError, dto.Preliminary, dto.Dynamic = append(json.RawMessage(nil), part.Result...), part.IsError, part.Preliminary, part.Dynamic
	case provider.PartSource:
		if part.Source == nil || part.Source.ID == "" {
			return streamPartDTO{}, errors.New("providerwirev4: stream source and ID are required")
		}
		sourceType := string(part.Source.SourceType)
		dto.SourceType, dto.ID = &sourceType, &part.Source.ID
		switch part.Source.SourceType {
		case provider.SourceTypeURL:
			if part.Source.URL == "" {
				return streamPartDTO{}, errors.New("providerwirev4: URL source URL is required")
			}
			dto.URL, dto.Title = part.Source.URL, part.Source.Title
		case provider.SourceTypeDocument:
			if part.Source.Title == "" || part.Source.MediaType == "" {
				return streamPartDTO{}, errors.New("providerwirev4: document source title and mediaType are required")
			}
			dto.Title, dto.MediaType, dto.Filename = part.Source.Title, &part.Source.MediaType, part.Source.Filename
		default:
			return streamPartDTO{}, fmt.Errorf("providerwirev4: unsupported source type %q", part.Source.SourceType)
		}
		dto.ProviderMetadata, err = encodeProviderMetadata(part.Source.ProviderMetadata)
		if err != nil {
			return streamPartDTO{}, err
		}
	case provider.PartFile, provider.PartReasoningFile:
		if part.Filename != "" {
			return streamPartDTO{}, fmt.Errorf("providerwirev4: stream %s filename is not in LanguageModelV4", part.Type)
		}
		data, err := encodeStreamFileData(part.Data)
		if err != nil {
			return streamPartDTO{}, err
		}
		dto.Data, err = json.Marshal(data)
		if err != nil {
			return streamPartDTO{}, err
		}
		dto.MediaType = &part.MediaType
	case provider.PartStreamStart:
		warnings, err := encodeWarnings(part.Warnings)
		if err != nil {
			return streamPartDTO{}, err
		}
		if warnings == nil {
			warnings = []warningDTO{}
		}
		dto.Warnings = &warnings
	case provider.PartResponseMeta:
		if part.Provider != "" || len(part.ResponseHeaders) > 0 {
			return streamPartDTO{}, errors.New("providerwirev4: response-metadata provider and headers are not in LanguageModelV4")
		}
		if part.ResponseID != "" {
			dto.ID = &part.ResponseID
		}
		dto.ModelID = part.ModelID
		if !part.Timestamp.IsZero() {
			value := part.Timestamp
			dto.Timestamp = &value
		}
	case provider.PartFinish:
		if part.Usage == nil || part.FinishReason == nil {
			return streamPartDTO{}, errors.New("providerwirev4: finish usage and reason are required")
		}
		usage, err := encodeUsage(*part.Usage)
		if err != nil {
			return streamPartDTO{}, err
		}
		reason := finishReasonDTO{Unified: string(part.FinishReason.Unified), Raw: part.FinishReason.Raw}
		if err := validateFinishReason(reason); err != nil {
			return streamPartDTO{}, err
		}
		dto.Usage, dto.FinishReason = &usage, &reason
	case provider.PartRaw:
		if err := validateJSON(part.RawValue, "raw stream value"); err != nil {
			return streamPartDTO{}, err
		}
		dto.RawValue = append(json.RawMessage(nil), part.RawValue...)
	case provider.PartError:
		if part.APICallError == nil {
			return streamPartDTO{}, errors.New("providerwirev4: error stream part requires APICallError")
		}
		errorDTO, err := encodeAPICallError(part.APICallError)
		if err != nil {
			return streamPartDTO{}, err
		}
		dto.Error, err = json.Marshal(errorDTO)
		if err != nil {
			return streamPartDTO{}, err
		}
	case provider.PartToolApprovalRequest:
		if part.ApprovalID == "" || part.ToolCallID == "" {
			return streamPartDTO{}, errors.New("providerwirev4: stream tool approval IDs are required")
		}
		dto.ApprovalID, dto.ToolCallID = &part.ApprovalID, &part.ToolCallID
	case provider.PartCustom:
		if err := validateQualifiedIdentifier(part.Kind, "stream custom kind"); err != nil {
			return streamPartDTO{}, err
		}
		dto.Kind = &part.Kind
	default:
		return streamPartDTO{}, fmt.Errorf("providerwirev4: unsupported stream part type %q", part.Type)
	}
	switch part.Type {
	case provider.PartTextStart, provider.PartTextDelta, provider.PartTextEnd,
		provider.PartReasoningStart, provider.PartReasoningDelta, provider.PartReasoningEnd,
		provider.PartToolInputStart, provider.PartToolInputDelta, provider.PartToolInputEnd,
		provider.PartToolCall, provider.PartToolResult, provider.PartFile, provider.PartReasoningFile,
		provider.PartFinish, provider.PartToolApprovalRequest, provider.PartCustom:
		dto.ProviderMetadata = metadata
	}
	return dto, nil
}

func decodeStreamPart(variant string, object map[string]json.RawMessage, dto streamPartDTO) (provider.StreamPart, error) {
	part := provider.StreamPart{Type: provider.StreamPartType(variant)}
	metadata, err := decodeProviderMetadata(dto.ProviderMetadata)
	if err != nil {
		return provider.StreamPart{}, err
	}
	part.ProviderMetadata = metadata
	requireID := func() (string, error) {
		if dto.ID == nil || *dto.ID == "" {
			return "", fmt.Errorf("providerwirev4: stream part %q ID is required", variant)
		}
		return *dto.ID, nil
	}
	switch part.Type {
	case provider.PartTextStart, provider.PartTextEnd, provider.PartReasoningStart, provider.PartReasoningEnd,
		provider.PartToolInputEnd:
		part.ID, err = requireID()
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolInputDelta:
		part.ID, err = requireID()
		if err == nil && dto.Delta == nil {
			err = fmt.Errorf("providerwirev4: stream part %q delta is required", variant)
		}
		if dto.Delta != nil {
			part.Delta = *dto.Delta
		}
	case provider.PartToolInputStart:
		part.ID, err = requireID()
		if err == nil && (dto.ToolName == nil || *dto.ToolName == "") {
			err = errors.New("providerwirev4: tool input start name is required")
		}
		if dto.ToolName != nil {
			part.ToolName = *dto.ToolName
		}
		part.ProviderExecuted, part.Dynamic, part.Title = dto.ProviderExecuted, dto.Dynamic, dto.Title
	case provider.PartToolCall:
		if dto.ToolCallID == nil || dto.ToolName == nil || dto.Input == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: stream tool call ID, name, and input are required")
		}
		if err := validateJSONObject(json.RawMessage(*dto.Input), "stream tool call input"); err != nil {
			return provider.StreamPart{}, err
		}
		part.ToolCallID, part.ToolName, part.Input = *dto.ToolCallID, *dto.ToolName, *dto.Input
		part.ProviderExecuted, part.Dynamic = dto.ProviderExecuted, dto.Dynamic
	case provider.PartToolResult:
		if dto.ToolCallID == nil || dto.ToolName == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: stream tool result ID and name are required")
		}
		if err := validateNonNullJSON(dto.Result, "stream tool result"); err != nil {
			return provider.StreamPart{}, err
		}
		part.ToolCallID, part.ToolName = *dto.ToolCallID, *dto.ToolName
		part.Result, part.IsError, part.Preliminary, part.Dynamic = append(json.RawMessage(nil), dto.Result...), dto.IsError, dto.Preliminary, dto.Dynamic
	case provider.PartSource:
		if dto.SourceType == nil || dto.ID == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: stream source type and ID are required")
		}
		sourceType := provider.SourceType(*dto.SourceType)
		mediaType := ""
		if dto.MediaType != nil {
			mediaType = *dto.MediaType
		}
		switch sourceType {
		case provider.SourceTypeURL:
			if dto.URL == "" {
				return provider.StreamPart{}, errors.New("providerwirev4: URL source URL is required")
			}
			mediaType = ""
			dto.Filename = ""
		case provider.SourceTypeDocument:
			if dto.Title == "" || mediaType == "" {
				return provider.StreamPart{}, errors.New("providerwirev4: document source title and mediaType are required")
			}
			dto.URL = ""
		default:
			return provider.StreamPart{}, fmt.Errorf("providerwirev4: unsupported source type %q", sourceType)
		}
		part.Source = &provider.SourceInfo{SourceType: sourceType, ID: *dto.ID, URL: dto.URL, Title: dto.Title, MediaType: mediaType, Filename: dto.Filename, ProviderMetadata: metadata}
		part.ProviderMetadata = nil
	case provider.PartFile, provider.PartReasoningFile:
		if len(dto.Data) == 0 || dto.MediaType == nil {
			return provider.StreamPart{}, fmt.Errorf("providerwirev4: stream %s data and mediaType are required", variant)
		}
		part.Data, err = decodeStreamFileData(dto.Data)
		if err == nil {
			part.MediaType = *dto.MediaType
		}
	case provider.PartStreamStart:
		if _, exists := object["warnings"]; !exists || dto.Warnings == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: stream-start warnings are required")
		}
		part.Warnings, err = decodeWarnings(*dto.Warnings)
	case provider.PartResponseMeta:
		if dto.ID != nil {
			part.ResponseID = *dto.ID
		}
		part.ModelID = dto.ModelID
		if dto.Timestamp != nil {
			part.Timestamp = *dto.Timestamp
		}
	case provider.PartFinish:
		if dto.Usage == nil || dto.FinishReason == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: finish usage and reason are required")
		}
		if err := validateFinishReason(*dto.FinishReason); err != nil {
			return provider.StreamPart{}, err
		}
		usage, err := decodeUsage(*dto.Usage)
		if err != nil {
			return provider.StreamPart{}, err
		}
		part.Usage = &usage
		part.FinishReason = &provider.FinishReason{Unified: provider.UnifiedFinishReason(dto.FinishReason.Unified), Raw: dto.FinishReason.Raw}
	case provider.PartRaw:
		if err := validateJSON(dto.RawValue, "raw stream value"); err != nil {
			return provider.StreamPart{}, err
		}
		part.RawValue = append(json.RawMessage(nil), dto.RawValue...)
	case provider.PartError:
		value, err := requireField(object, "error", "error stream part")
		if err != nil {
			return provider.StreamPart{}, err
		}
		part.APICallError, err = decodeAPICallError(value)
		if err != nil {
			return provider.StreamPart{}, err
		}
	case provider.PartToolApprovalRequest:
		if dto.ApprovalID == nil || dto.ToolCallID == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: stream tool approval IDs are required")
		}
		part.ApprovalID, part.ToolCallID = *dto.ApprovalID, *dto.ToolCallID
	case provider.PartCustom:
		if dto.Kind == nil {
			return provider.StreamPart{}, errors.New("providerwirev4: stream custom kind is required")
		}
		if err := validateQualifiedIdentifier(*dto.Kind, "stream custom kind"); err != nil {
			return provider.StreamPart{}, err
		}
		part.Kind = *dto.Kind
	default:
		return provider.StreamPart{}, fmt.Errorf("providerwirev4: unsupported stream part type %q", variant)
	}
	if err != nil {
		return provider.StreamPart{}, err
	}
	return part, nil
}

func encodeStreamFileData(data *provider.StreamFileData) (*dataDTO, error) {
	if data == nil {
		return nil, errors.New("providerwirev4: stream file data is required")
	}
	switch data.Type {
	case provider.StreamFileDataTypeData:
		if data.Bytes != nil && data.Base64 != "" {
			return nil, errors.New("providerwirev4: stream file data has ambiguous inline values")
		}
		value := data.Base64
		if data.Bytes != nil {
			value = base64.StdEncoding.EncodeToString(data.Bytes)
		}
		return &dataDTO{Type: "data", Data: &value}, nil
	case provider.StreamFileDataTypeURL:
		if data.URL == "" {
			return nil, errors.New("providerwirev4: stream file URL is required")
		}
		value := data.URL
		return &dataDTO{Type: "url", URL: &value}, nil
	default:
		return nil, fmt.Errorf("providerwirev4: unsupported stream file data type %q", data.Type)
	}
}

func decodeStreamFileData(data json.RawMessage) (*provider.StreamFileData, error) {
	object, err := decodeObject(data, "stream file data")
	if err != nil {
		return nil, err
	}
	variant, err := decodeRequiredString(object, "type", "stream file data")
	if err != nil {
		return nil, err
	}
	switch variant {
	case "data":
		value, err := decodeRequiredString(object, "data", "stream file data")
		if err != nil {
			return nil, err
		}
		return &provider.StreamFileData{Type: provider.StreamFileDataTypeData, Base64: value}, nil
	case "url":
		value, err := decodeRequiredString(object, "url", "stream file data")
		if err != nil || value == "" {
			return nil, errors.New("providerwirev4: stream file URL is required")
		}
		return &provider.StreamFileData{Type: provider.StreamFileDataTypeURL, URL: value}, nil
	default:
		return nil, fmt.Errorf("providerwirev4: unsupported stream file data type %q", variant)
	}
}

func encodeAPICallError(apiErr *provider.APICallError) (apiCallErrorDTO, error) {
	if apiErr == nil {
		return apiCallErrorDTO{}, errors.New("providerwirev4: nil API call error")
	}
	for context, value := range map[string]json.RawMessage{"request body values": apiErr.RequestBodyValues, "error data": apiErr.Data} {
		if len(value) > 0 {
			if err := validateJSON(value, context); err != nil {
				return apiCallErrorDTO{}, err
			}
		}
	}
	return apiCallErrorDTO{Message: apiErr.Message, StatusCode: apiErr.StatusCode, URL: apiErr.URL,
		RequestBodyValues: append(json.RawMessage(nil), apiErr.RequestBodyValues...), ResponseHeaders: apiErr.ResponseHeaders,
		ResponseBody: apiErr.ResponseBody, IsRetryable: apiErr.IsRetryable, Data: append(json.RawMessage(nil), apiErr.Data...)}, nil
}

func decodeAPICallError(data json.RawMessage) (*provider.APICallError, error) {
	object, err := decodeObject(data, "stream API error")
	if err != nil {
		return nil, err
	}
	message, err := decodeRequiredString(object, "message", "stream API error")
	if err != nil {
		return nil, err
	}
	if _, err := requireField(object, "statusCode", "stream API error"); err != nil {
		return nil, err
	}
	if _, err := requireField(object, "isRetryable", "stream API error"); err != nil {
		return nil, err
	}
	if err := rejectNullFields(object, "stream API error", "url", "responseHeaders", "responseBody"); err != nil {
		return nil, err
	}
	if raw, exists := object["responseHeaders"]; exists {
		if err := validateHeaderValues(raw, "stream API error responseHeaders"); err != nil {
			return nil, err
		}
	}
	var dto apiCallErrorDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}
	for context, value := range map[string]json.RawMessage{"request body values": dto.RequestBodyValues, "error data": dto.Data} {
		if len(value) > 0 {
			if err := validateJSON(value, context); err != nil {
				return nil, err
			}
		}
	}
	retryable := dto.IsRetryable
	return provider.NewAPICallError(provider.APICallErrorOptions{Message: message, StatusCode: dto.StatusCode, URL: dto.URL,
		RequestBodyValues: append(json.RawMessage(nil), dto.RequestBodyValues...), ResponseHeaders: dto.ResponseHeaders,
		ResponseBody: dto.ResponseBody, IsRetryable: &retryable, Data: append(json.RawMessage(nil), dto.Data...)}), nil
}

func validateHeaderValues(data json.RawMessage, context string) error {
	object, err := decodeObject(data, context)
	if err != nil {
		return err
	}
	for key, value := range object {
		if err := validateStringArray(value, fmt.Sprintf("%s field %q", context, key)); err != nil {
			return err
		}
	}
	return nil
}
