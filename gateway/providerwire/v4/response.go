package providerwirev4

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"

	"github.com/grafana/ai-sdk/provider"
)

type wireGenerateResult struct {
	Content      []wireGenerateContent `json:"content"`
	FinishReason wireFinishReason      `json:"finishReason"`
	Usage        wireUsage             `json:"usage"`
	Response     *wireResponse         `json:"response,omitempty"`
	Warnings     []wireWarning         `json:"warnings"`
}

type wireGenerateContent struct {
	Type             provider.GenerateContentType `json:"type"`
	Text             *string                      `json:"text,omitempty"`
	Kind             string                       `json:"kind,omitempty"`
	MediaType        *string                      `json:"mediaType,omitempty"`
	Data             *wireGeneratedFileData       `json:"data,omitempty"`
	SourceType       provider.SourceType          `json:"sourceType,omitempty"`
	ID               *string                      `json:"id,omitempty"`
	URL              *string                      `json:"url,omitempty"`
	Title            *string                      `json:"title,omitempty"`
	Filename         string                       `json:"filename,omitempty"`
	ToolCallID       *string                      `json:"toolCallId,omitempty"`
	ToolName         *string                      `json:"toolName,omitempty"`
	Input            *string                      `json:"input,omitempty"`
	Result           json.RawMessage              `json:"result,omitempty"`
	IsError          *bool                        `json:"isError,omitempty"`
	Preliminary      *bool                        `json:"preliminary,omitempty"`
	ProviderExecuted *bool                        `json:"providerExecuted,omitempty"`
	Dynamic          *bool                        `json:"dynamic,omitempty"`
	ApprovalID       *string                      `json:"approvalId,omitempty"`
}

type wireGeneratedFileData struct {
	Type string  `json:"type"`
	Data *string `json:"data,omitempty"`
	URL  *string `json:"url,omitempty"`
}

type wireFinishReason struct {
	Unified provider.UnifiedFinishReason `json:"unified"`
	Raw     string                       `json:"raw,omitempty"`
}

type wireUsage struct {
	InputTokens  wireInputUsage  `json:"inputTokens"`
	OutputTokens wireOutputUsage `json:"outputTokens"`
}

type wireInputUsage struct {
	Total      *int `json:"total,omitempty"`
	NoCache    *int `json:"noCache,omitempty"`
	CacheRead  *int `json:"cacheRead,omitempty"`
	CacheWrite *int `json:"cacheWrite,omitempty"`
}

type wireOutputUsage struct {
	Total     *int `json:"total,omitempty"`
	Text      *int `json:"text,omitempty"`
	Reasoning *int `json:"reasoning,omitempty"`
}

type wireResponse struct {
	ID        string  `json:"id,omitempty"`
	Timestamp *string `json:"timestamp,omitempty"`
}

type wireWarning struct {
	Type    provider.WarningType `json:"type"`
	Feature *string              `json:"feature,omitempty"`
	Setting *string              `json:"setting,omitempty"`
	Message *string              `json:"message,omitempty"`
	Details string               `json:"details,omitempty"`
}

type responseSizeEstimate struct {
	total int64
}

func (e *responseSizeEstimate) add(size int64) error {
	if size < 0 || e.total > math.MaxInt64-size {
		return fmt.Errorf("response size overflow")
	}
	e.total += size
	return nil
}

func (e *responseSizeEstimate) addString(value string) error {
	length := int64(len(value))
	if length > (math.MaxInt64-2)/6 {
		return fmt.Errorf("response string size overflow")
	}
	return e.add(length*6 + 2)
}

func (e *responseSizeEstimate) addRawJSON(value json.RawMessage) error {
	size := int64(len(value))
	for index := 0; index < len(value); index++ {
		extra := int64(0)
		switch value[index] {
		case '<', '>', '&':
			extra = 5
		case 0xe2:
			if index+2 < len(value) && value[index+1] == 0x80 && (value[index+2] == 0xa8 || value[index+2] == 0xa9) {
				extra = 3
			}
		}
		if size > math.MaxInt64-extra {
			return fmt.Errorf("response raw JSON size overflow")
		}
		size += extra
	}
	return e.add(size)
}

func estimateGenerateResultUpperBound(result *provider.GenerateResult) (int64, error) {
	if result == nil {
		return 0, fmt.Errorf("nil generate result")
	}
	estimate := responseSizeEstimate{}
	if err := estimate.add(512); err != nil {
		return 0, err
	}
	if err := estimate.addString(string(result.FinishReason.Unified)); err != nil {
		return 0, err
	}
	if err := estimate.addString(result.FinishReason.Raw); err != nil {
		return 0, err
	}
	for i, part := range result.Content {
		if err := estimate.add(512); err != nil {
			return 0, err
		}
		if err := estimateGenerateContentSize(&estimate, part); err != nil {
			return 0, fmt.Errorf("content/%d: %w", i, err)
		}
	}
	for i, warning := range result.Warnings {
		if err := estimate.add(256); err != nil {
			return 0, err
		}
		if err := estimateWarningSize(&estimate, warning); err != nil {
			return 0, fmt.Errorf("warnings/%d: %w", i, err)
		}
	}
	if result.Response != nil {
		if err := estimate.addString(result.Response.ID); err != nil {
			return 0, err
		}
		if !result.Response.Timestamp.IsZero() {
			if err := estimate.add(64); err != nil {
				return 0, err
			}
		}
	}
	return estimate.total, nil
}

func estimateGenerateContentSize(estimate *responseSizeEstimate, part provider.GenerateContentPart) error {
	if err := estimate.addString(string(part.Type)); err != nil {
		return err
	}
	switch part.Type {
	case provider.ContentText, provider.ContentReasoning:
		return estimate.addString(part.Text)
	case provider.ContentCustom:
		return estimate.addString(part.Kind)
	case provider.ContentFile, provider.ContentReasoningFile:
		if part.Data == nil {
			return fmt.Errorf("file data is required")
		}
		if err := estimate.addString(part.MediaType); err != nil {
			return err
		}
		return estimateGeneratedFileSize(estimate, *part.Data)
	case provider.ContentSource:
		for _, value := range []string{string(part.SourceType), part.ID, part.URL, part.Title, part.MediaType, part.Filename} {
			if err := estimate.addString(value); err != nil {
				return err
			}
		}
		return nil
	case provider.ContentToolCall:
		for _, value := range []string{part.ToolCallID, part.ToolName} {
			if err := estimate.addString(value); err != nil {
				return err
			}
		}
		return estimate.addString(string(part.Input))
	case provider.ContentToolResult:
		for _, value := range []string{part.ToolCallID, part.ToolName} {
			if err := estimate.addString(value); err != nil {
				return err
			}
		}
		return estimate.addRawJSON(part.Result)
	case provider.ContentToolApprovalRequest:
		if err := estimate.addString(part.ApprovalID); err != nil {
			return err
		}
		return estimate.addString(part.ToolCallID)
	default:
		return fmt.Errorf("unsupported generate content type %q", part.Type)
	}
}

func estimateGeneratedFileSize(estimate *responseSizeEstimate, data provider.DataContent) error {
	if err := data.Validate(); err != nil {
		return fmt.Errorf("invalid generated file data: %w", err)
	}
	switch {
	case data.Bytes != nil:
		length, err := base64EncodedSize(len(data.Bytes))
		if err != nil {
			return err
		}
		return estimate.add(length + 64)
	case data.Base64 != "":
		return estimate.add(int64(len(data.Base64)) + 64)
	case data.IsURL():
		if err := estimate.addString(data.URL); err != nil {
			return err
		}
		return estimate.add(64)
	default:
		return fmt.Errorf("generated file data is not representable")
	}
}

func base64EncodedSize(length int) (int64, error) {
	value := int64(length)
	if value > (math.MaxInt64/4)*3-2 {
		return 0, fmt.Errorf("base64 size overflow")
	}
	return ((value + 2) / 3) * 4, nil
}

func estimateWarningSize(estimate *responseSizeEstimate, warning provider.Warning) error {
	for _, value := range []string{string(warning.Type), warning.Feature, warning.Setting, warning.Message, warning.Details} {
		if err := estimate.addString(value); err != nil {
			return err
		}
	}
	return nil
}

func projectGenerateResult(result *provider.GenerateResult) (wireGenerateResult, error) {
	if result == nil {
		return wireGenerateResult{}, fmt.Errorf("nil generate result")
	}
	content := make([]wireGenerateContent, len(result.Content))
	for i, part := range result.Content {
		projected, err := projectGenerateContent(part)
		if err != nil {
			return wireGenerateResult{}, fmt.Errorf("content/%d: %w", i, err)
		}
		content[i] = projected
	}
	warnings := make([]wireWarning, len(result.Warnings))
	for i, warning := range result.Warnings {
		projected, err := projectWarning(warning)
		if err != nil {
			return wireGenerateResult{}, fmt.Errorf("warnings/%d: %w", i, err)
		}
		warnings[i] = projected
	}
	projected := wireGenerateResult{
		Content: content,
		FinishReason: wireFinishReason{
			Unified: result.FinishReason.Unified,
			Raw:     result.FinishReason.Raw,
		},
		Usage: wireUsage{
			InputTokens: wireInputUsage{
				Total:      result.Usage.InputTokens.Total,
				NoCache:    result.Usage.InputTokens.NoCache,
				CacheRead:  result.Usage.InputTokens.CacheRead,
				CacheWrite: result.Usage.InputTokens.CacheWrite,
			},
			OutputTokens: wireOutputUsage{
				Total:     result.Usage.OutputTokens.Total,
				Text:      result.Usage.OutputTokens.Text,
				Reasoning: result.Usage.OutputTokens.Reasoning,
			},
		},
		Warnings: warnings,
	}
	if result.Response != nil && (result.Response.ID != "" || !result.Response.Timestamp.IsZero()) {
		response := &wireResponse{ID: result.Response.ID}
		if !result.Response.Timestamp.IsZero() {
			timestamp := result.Response.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z")
			response.Timestamp = &timestamp
		}
		projected.Response = response
	}
	return projected, nil
}

func projectGenerateContent(part provider.GenerateContentPart) (wireGenerateContent, error) {
	projected := wireGenerateContent{Type: part.Type}
	switch part.Type {
	case provider.ContentText, provider.ContentReasoning:
		text := part.Text
		projected.Text = &text
	case provider.ContentCustom:
		if part.Kind == "" {
			return wireGenerateContent{}, fmt.Errorf("custom kind is required")
		}
		projected.Kind = part.Kind
	case provider.ContentFile, provider.ContentReasoningFile:
		if part.Data == nil {
			return wireGenerateContent{}, fmt.Errorf("file data is required")
		}
		data, err := projectGeneratedFileData(*part.Data)
		if err != nil {
			return wireGenerateContent{}, err
		}
		projected.MediaType, projected.Data = stringPointer(part.MediaType), &data
	case provider.ContentSource:
		projected.SourceType, projected.ID = part.SourceType, stringPointer(part.ID)
		switch part.SourceType {
		case provider.SourceTypeURL:
			projected.URL = stringPointer(part.URL)
			if part.Title != "" {
				title := part.Title
				projected.Title = &title
			}
		case provider.SourceTypeDocument:
			projected.MediaType, projected.Filename = stringPointer(part.MediaType), part.Filename
			title := part.Title
			projected.Title = &title
		default:
			return wireGenerateContent{}, fmt.Errorf("unsupported source type %q", part.SourceType)
		}
	case provider.ContentToolCall:
		if len(part.Input) == 0 {
			return wireGenerateContent{}, fmt.Errorf("tool call input is required")
		}
		if _, err := validateStrictJSON(part.Input); err != nil {
			return wireGenerateContent{}, fmt.Errorf("invalid tool call input: %w", err)
		}
		input := string(part.Input)
		projected.ToolCallID, projected.ToolName, projected.Input = stringPointer(part.ToolCallID), stringPointer(part.ToolName), &input
		if part.ProviderExecuted {
			value := true
			projected.ProviderExecuted = &value
		}
		projected.Dynamic = part.Dynamic
	case provider.ContentToolResult:
		if len(part.Result) == 0 || string(part.Result) == "null" {
			return wireGenerateContent{}, fmt.Errorf("tool result is required and must be non-null")
		}
		if _, err := validateStrictJSON(part.Result); err != nil {
			return wireGenerateContent{}, fmt.Errorf("invalid tool result: %w", err)
		}
		projected.ToolCallID, projected.ToolName = stringPointer(part.ToolCallID), stringPointer(part.ToolName)
		projected.Result = cloneRaw(part.Result)
		if part.IsError {
			value := true
			projected.IsError = &value
		}
		projected.Preliminary, projected.Dynamic = part.Preliminary, part.Dynamic
	case provider.ContentToolApprovalRequest:
		projected.ApprovalID, projected.ToolCallID = stringPointer(part.ApprovalID), stringPointer(part.ToolCallID)
	default:
		return wireGenerateContent{}, fmt.Errorf("unsupported generate content type %q", part.Type)
	}
	return projected, nil
}

func projectGeneratedFileData(data provider.DataContent) (wireGeneratedFileData, error) {
	if err := data.Validate(); err != nil {
		return wireGeneratedFileData{}, fmt.Errorf("invalid generated file data: %w", err)
	}
	if data.Bytes != nil {
		encoded := base64.StdEncoding.EncodeToString(data.Bytes)
		return wireGeneratedFileData{Type: "data", Data: &encoded}, nil
	}
	if data.Base64 != "" {
		if _, err := base64.StdEncoding.DecodeString(data.Base64); err != nil {
			return wireGeneratedFileData{}, fmt.Errorf("invalid generated base64 data: %w", err)
		}
		encoded := data.Base64
		return wireGeneratedFileData{Type: "data", Data: &encoded}, nil
	}
	if data.IsURL() {
		url := data.URL
		return wireGeneratedFileData{Type: "url", URL: &url}, nil
	}
	return wireGeneratedFileData{}, fmt.Errorf("generated file data is not representable")
}

func projectWarning(warning provider.Warning) (wireWarning, error) {
	projected := wireWarning{Type: warning.Type}
	switch warning.Type {
	case provider.WarnUnsupported, provider.WarnCompatibility:
		projected.Feature, projected.Details = stringPointer(warning.Feature), warning.Details
	case provider.WarnDeprecated:
		projected.Setting, projected.Message = stringPointer(warning.Setting), stringPointer(warning.Message)
	case provider.WarnOther:
		projected.Message = stringPointer(warning.Message)
	default:
		return wireWarning{}, fmt.Errorf("unsupported warning type %q", warning.Type)
	}
	return projected, nil
}

func stringPointer(value string) *string {
	return &value
}
