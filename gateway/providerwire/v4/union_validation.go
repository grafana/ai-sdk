package providerwirev4

import (
	"fmt"

	"github.com/grafana/ai-sdk/provider"
)

func rejectPopulatedFields(context string, fields map[string]bool, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		allowedSet[field] = struct{}{}
	}
	for field, populated := range fields {
		if !populated {
			continue
		}
		if _, ok := allowedSet[field]; !ok {
			return fmt.Errorf("providerwirev4: %s contains contradictory field %q", context, field)
		}
	}
	return nil
}

func validateToolFields(tool provider.Tool) error {
	fields := map[string]bool{
		"description":   len(tool.Description) > 0,
		"inputSchema":   len(tool.InputSchema) > 0,
		"inputExamples": len(tool.InputExamples) > 0,
		"strict":        tool.Strict != nil,
		"id":            tool.ID != "",
		"args":          tool.Args != nil,
	}
	switch tool.Type {
	case provider.ToolTypeFunction:
		return rejectPopulatedFields("function tool", fields, "description", "inputSchema", "inputExamples", "strict")
	case provider.ToolTypeProvider:
		return rejectPopulatedFields("provider tool", fields, "id", "args")
	default:
		return nil
	}
}

func validateContentPartFields(part provider.ContentPart) error {
	fields := map[string]bool{
		"text": part.Text != "", "data": part.Data != nil, "filename": part.Filename != "", "mediaType": part.MediaType != "",
		"kind": part.Kind != "", "sourceType": part.SourceType != "", "id": part.ID != "", "url": part.URL != "", "title": part.Title != "",
		"toolCallId": part.ToolCallID != "", "toolName": part.ToolName != "", "input": len(part.Input) > 0, "output": part.Output != nil,
		"providerExecuted": part.ProviderExecuted, "approvalId": part.ApprovalID != "", "signature": part.Signature != "",
		"isAutomatic": part.IsAutomatic, "approved": part.Approved != nil, "reason": part.Reason != "",
	}
	switch part.Type {
	case provider.ContentPartTypeText, provider.ContentPartTypeReasoning:
		return rejectPopulatedFields("content part", fields, "text")
	case provider.ContentPartTypeFile:
		return rejectPopulatedFields("content part", fields, "data", "filename", "mediaType")
	case provider.ContentPartTypeReasoningFile:
		return rejectPopulatedFields("content part", fields, "data", "mediaType")
	case provider.ContentPartTypeCustom:
		return rejectPopulatedFields("content part", fields, "kind")
	case provider.ContentPartTypeToolCall:
		return rejectPopulatedFields("content part", fields, "toolCallId", "toolName", "input", "providerExecuted")
	case provider.ContentPartTypeToolResult:
		return rejectPopulatedFields("content part", fields, "toolCallId", "toolName", "output")
	case provider.ContentPartTypeToolApprovalResponse:
		return rejectPopulatedFields("content part", fields, "approvalId", "approved", "reason")
	default:
		return nil
	}
}

func validateToolResultContentFields(content provider.ToolResultContentValue) error {
	fields := map[string]bool{
		"text": content.Text != "", "data": content.Data != nil,
		"mediaType": content.MediaType != "", "filename": content.Filename != "",
	}
	switch content.Type {
	case provider.ToolContentText:
		return rejectPopulatedFields("tool result content", fields, "text")
	case provider.ToolContentFile:
		return rejectPopulatedFields("tool result content", fields, "data", "mediaType", "filename")
	case provider.ToolContentCustom:
		return rejectPopulatedFields("tool result content", fields)
	default:
		return nil
	}
}

func validateGenerateContentFields(part provider.GenerateContentPart) error {
	fields := map[string]bool{
		"id": part.ID != "", "kind": part.Kind != "", "text": part.Text != "", "approvalId": part.ApprovalID != "",
		"toolCallId": part.ToolCallID != "", "toolName": part.ToolName != "", "input": len(part.Input) > 0, "result": len(part.Result) > 0,
		"isError": part.IsError, "preliminary": part.Preliminary != nil, "providerExecuted": part.ProviderExecuted, "dynamic": part.Dynamic != nil,
		"sourceType": part.SourceType != "", "url": part.URL != "", "title": part.Title != "", "data": part.Data != nil,
		"mediaType": part.MediaType != "", "filename": part.Filename != "",
	}
	switch part.Type {
	case provider.ContentText, provider.ContentReasoning:
		return rejectPopulatedFields("generate content", fields, "text")
	case provider.ContentToolCall:
		return rejectPopulatedFields("generate content", fields, "toolCallId", "toolName", "input", "providerExecuted", "dynamic")
	case provider.ContentToolResult:
		return rejectPopulatedFields("generate content", fields, "toolCallId", "toolName", "result", "isError", "preliminary", "dynamic")
	case provider.ContentSource:
		return rejectPopulatedFields("generate content", fields, "id", "sourceType", "url", "title", "mediaType", "filename")
	case provider.ContentFile, provider.ContentReasoningFile:
		return rejectPopulatedFields("generate content", fields, "data", "mediaType")
	case provider.ContentCustom:
		return rejectPopulatedFields("generate content", fields, "kind")
	case provider.ContentToolApprovalRequest:
		return rejectPopulatedFields("generate content", fields, "approvalId", "toolCallId")
	default:
		return nil
	}
}

func validateStreamPartFields(part provider.StreamPart) error {
	fields := map[string]bool{
		"id": part.ID != "", "delta": part.Delta != "", "toolCallId": part.ToolCallID != "", "toolName": part.ToolName != "",
		"input": part.Input != "", "providerExecuted": part.ProviderExecuted, "isError": part.IsError, "dynamic": part.Dynamic != nil,
		"preliminary": part.Preliminary != nil, "title": part.Title != "", "result": len(part.Result) > 0, "kind": part.Kind != "",
		"approvalId": part.ApprovalID != "", "signature": part.Signature != "", "approved": part.Approved != nil, "reason": part.Reason != "",
		"source": part.Source != nil, "data": part.Data != nil, "mediaType": part.MediaType != "", "filename": part.Filename != "",
		"warnings": part.Warnings != nil, "responseId": part.ResponseID != "", "modelId": part.ModelID != "", "provider": part.Provider != "",
		"timestamp": !part.Timestamp.IsZero(), "responseHeaders": len(part.ResponseHeaders) > 0, "usage": part.Usage != nil,
		"finishReason": part.FinishReason != nil, "rawValue": len(part.RawValue) > 0, "apiCallError": part.APICallError != nil,
		"providerMetadata": len(part.ProviderMetadata) > 0,
	}
	metadata := "providerMetadata"
	switch part.Type {
	case provider.PartTextStart, provider.PartTextEnd, provider.PartReasoningStart, provider.PartReasoningEnd, provider.PartToolInputEnd:
		return rejectPopulatedFields("stream part", fields, "id", metadata)
	case provider.PartTextDelta, provider.PartReasoningDelta, provider.PartToolInputDelta:
		return rejectPopulatedFields("stream part", fields, "id", "delta", metadata)
	case provider.PartToolInputStart:
		return rejectPopulatedFields("stream part", fields, "id", "toolName", "providerExecuted", "dynamic", "title", metadata)
	case provider.PartToolCall:
		return rejectPopulatedFields("stream part", fields, "toolCallId", "toolName", "input", "providerExecuted", "dynamic", metadata)
	case provider.PartToolResult:
		return rejectPopulatedFields("stream part", fields, "toolCallId", "toolName", "result", "isError", "preliminary", "dynamic", metadata)
	case provider.PartSource:
		return rejectPopulatedFields("stream part", fields, "source")
	case provider.PartFile, provider.PartReasoningFile:
		return rejectPopulatedFields("stream part", fields, "data", "mediaType", metadata)
	case provider.PartStreamStart:
		return rejectPopulatedFields("stream part", fields, "warnings")
	case provider.PartResponseMeta:
		return rejectPopulatedFields("stream part", fields, "responseId", "modelId", "timestamp")
	case provider.PartFinish:
		return rejectPopulatedFields("stream part", fields, "usage", "finishReason", metadata)
	case provider.PartRaw:
		return rejectPopulatedFields("stream part", fields, "rawValue")
	case provider.PartError:
		return rejectPopulatedFields("stream part", fields, "apiCallError")
	case provider.PartToolApprovalRequest:
		return rejectPopulatedFields("stream part", fields, "approvalId", "toolCallId", metadata)
	case provider.PartCustom:
		return rejectPopulatedFields("stream part", fields, "kind", metadata)
	default:
		return nil
	}
}
