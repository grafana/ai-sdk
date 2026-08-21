package openai

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"github.com/grafana/ai-sdk/internal/mediatype"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// convertUserMessage converts a user message to a single input message item.
func convertUserMessage(msg provider.Message, popts OpenAIResponsesOptions, providerOptionsName string) (responses.ResponseInputItemUnionParam, []provider.Warning, error) {
	var warnings []provider.Warning
	var content responses.ResponseInputMessageContentListParam

	for i, part := range msg.Content {
		switch part.Type {
		case provider.ContentPartTypeText:
			content = append(content, inputTextContent(part.Text, promptCacheBreakpoint(part.ProviderOptions, providerOptionsName)))

		case provider.ContentPartTypeFile:
			c, w, err := convertUserFilePart(part, i, popts, providerOptionsName)
			if err != nil {
				return responses.ResponseInputItemUnionParam{}, nil, err
			}
			warnings = append(warnings, w...)
			if c != nil {
				content = append(content, *c)
			}
		}
	}

	item := responses.ResponseInputItemParamOfInputMessage(content, "user")
	return item, warnings, nil
}

// convertUserFilePart converts a file content part to an input_image or
// input_file content param.
func convertUserFilePart(part provider.ContentPart, index int, popts OpenAIResponsesOptions, providerOptionsName string) (*responses.ResponseInputContentUnionParam, []provider.Warning, error) {
	var warnings []provider.Warning
	topLevel := topLevelMediaType(part.MediaType)
	detail := partImageDetail(part, providerOptionsName)

	if part.Data == nil {
		return nil, nil, fmt.Errorf("openai: file part has no data")
	}

	dataType, ok := part.Data.DataType()
	if !ok {
		return nil, nil, fmt.Errorf("openai: invalid file data")
	}

	if topLevel == "image" {
		img := responses.ResponseInputImageParam{}
		if detail != "" {
			img.Detail = responses.ResponseInputImageDetail(detail)
		}
		switch dataType {
		case provider.DataContentTypeReference:
			fileID, err := resolveFileReference(part.Data.Reference, providerOptionsName)
			if err != nil {
				return nil, nil, err
			}
			img.FileID = param.NewOpt(fileID)
		case provider.DataContentTypeURL:
			img.ImageURL = param.NewOpt(part.Data.URL)
		case provider.DataContentTypeData:
			mediaType, err := resolveFullMediaType(part)
			if err != nil {
				return nil, nil, err
			}
			data := part.Data.Base64
			if part.Data.Bytes != nil {
				data = base64.StdEncoding.EncodeToString(part.Data.Bytes)
			}
			img.ImageURL = param.NewOpt(dataURI(mediaType, data))
		case provider.DataContentTypeText:
			return nil, nil, fmt.Errorf("openai: text file parts are not supported")
		}
		if breakpoint := promptCacheBreakpoint(part.ProviderOptions, providerOptionsName); breakpoint != nil {
			img.SetExtraFields(map[string]any{"prompt_cache_breakpoint": breakpoint})
		}
		return &responses.ResponseInputContentUnionParam{OfInputImage: &img}, warnings, nil
	}

	// Non-image file.
	f := responses.ResponseInputFileParam{}
	switch dataType {
	case provider.DataContentTypeReference:
		fileID, err := resolveFileReference(part.Data.Reference, providerOptionsName)
		if err != nil {
			return nil, nil, err
		}
		f.FileID = param.NewOpt(fileID)
	case provider.DataContentTypeURL:
		f.FileURL = param.NewOpt(part.Data.URL)
	case provider.DataContentTypeData:
		mediaType, err := resolveFullMediaType(part)
		if err != nil {
			return nil, nil, err
		}
		if mediaType != "application/pdf" && (popts.PassThroughUnsupportedFiles == nil || !*popts.PassThroughUnsupportedFiles) {
			return nil, nil, fmt.Errorf("openai: file part media type %q is not supported", mediaType)
		}
		b64 := part.Data.Base64
		if part.Data.Bytes != nil {
			b64 = base64.StdEncoding.EncodeToString(part.Data.Bytes)
		}
		filename := fmt.Sprintf("part-%d", index)
		if mediaType == "application/pdf" {
			filename += ".pdf"
		}
		if part.FilePartFilename != nil {
			filename = *part.FilePartFilename
		}
		f.FileData = param.NewOpt(dataURI(mediaType, b64))
		f.Filename = param.NewOpt(filename)
	case provider.DataContentTypeText:
		return nil, nil, fmt.Errorf("openai: text file parts are not supported")
	}
	if breakpoint := promptCacheBreakpoint(part.ProviderOptions, providerOptionsName); breakpoint != nil {
		f.SetExtraFields(map[string]any{"prompt_cache_breakpoint": breakpoint})
	}
	return &responses.ResponseInputContentUnionParam{OfInputFile: &f}, warnings, nil
}

func resolveFileReference(reference json.RawMessage, providerOptionsName string) (string, error) {
	var references map[string]string
	if err := json.Unmarshal(reference, &references); err != nil {
		return "", fmt.Errorf("openai: decoding file provider reference: %w", err)
	}
	fileID, ok := references[providerOptionsName]
	if !ok {
		return "", fmt.Errorf("openai: file reference has no %q provider entry", providerOptionsName)
	}
	return fileID, nil
}

// convertAssistantMessage converts an assistant message to input items.
func convertAssistantMessage(msg provider.Message, ctx inputConversionContext) ([]responses.ResponseInputItemUnionParam, []provider.Warning, error) {
	var items []responses.ResponseInputItemUnionParam
	var warnings []provider.Warning

	for _, part := range msg.Content {
		switch part.Type {
		case provider.ContentPartTypeText:
			po := ctx.partOptions(part)
			if ctx.hasConversation && po.ItemID != "" {
				continue
			}
			if ctx.store && po.ItemID != "" {
				items = append(items, itemReference(po.ItemID))
				continue
			}
			items = append(items, assistantOutputMessage(part.Text, po))

		case provider.ContentPartTypeToolCall:
			item, err := convertAssistantToolCall(part, ctx)
			if err != nil {
				return nil, nil, err
			}
			if item != nil {
				items = append(items, *item)
			}

		case provider.ContentPartTypeToolResult:
			item, itemWarnings, err := convertAssistantToolResult(part, ctx)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, itemWarnings...)
			if item != nil {
				items = append(items, *item)
			}

		case provider.ContentPartTypeReasoning:
			po := ctx.partOptions(part)
			if (ctx.hasConversation || ctx.hasPreviousResponseID) && po.ItemID != "" {
				continue
			}
			if po.ItemID != "" && ctx.store {
				items = append(items, itemReference(po.ItemID))
				continue
			}
			// Non-stored reasoning requires encrypted content for round-trip.
			if po.ReasoningEncryptedContent == nil {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnOther,
					Feature: "reasoning",
					Message: "non-OpenAI reasoning parts are not supported and are omitted",
				})
				continue
			}
			items = append(items, reasoningItem(po.ItemID, *po.ReasoningEncryptedContent, part.Text))
		}
	}

	return items, warnings, nil
}

// convertToolMessage converts a tool message (results / approval responses) to
// input items.
func convertToolMessage(msg provider.Message, ctx inputConversionContext) ([]responses.ResponseInputItemUnionParam, []provider.Warning, error) {
	var items []responses.ResponseInputItemUnionParam
	var warnings []provider.Warning

	for _, part := range msg.Content {
		switch part.Type {
		case provider.ContentPartTypeToolResult:
			item, itemWarnings, err := convertProviderToolResult(part, ctx)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, itemWarnings...)
			if item != nil {
				items = append(items, *item)
			}

		case provider.ContentPartTypeToolApprovalResponse:
			approvalID := part.ApprovalID
			if approvalID == "" {
				approvalID = ctx.partOptions(part).ApprovalID
			}
			if _, processed := ctx.processedApprovalIDs[approvalID]; processed {
				continue
			}
			ctx.processedApprovalIDs[approvalID] = struct{}{}
			if ctx.store && !ctx.hasConversation && !ctx.hasPreviousResponseID {
				items = append(items, itemReference(approvalID))
			}
			approved := part.Approved != nil && *part.Approved
			items = append(items, mcpApprovalResponse(approvalID, approved))
		}
	}

	return items, warnings, nil
}

// serializeToolCallArguments serializes tool-call input, mapping empty/undefined
// input to "{}" to match upstream behavior.
func serializeToolCallArguments(input []byte) string {
	if len(input) == 0 || string(input) == "null" {
		return "{}"
	}
	return string(input)
}

// toolResultOutputString renders a tool result output to its string form.
func toolResultOutputString(out *provider.ToolResultOutput, encodeTextAsJSON bool) string {
	if out == nil {
		return ""
	}
	switch out.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		if encodeTextAsJSON {
			encoded, _ := json.Marshal(out.Text)
			return string(encoded)
		}
		return out.Text
	case provider.ToolOutputExecutionDenied:
		reason := "Tool call execution denied."
		if out.Reason != nil {
			reason = *out.Reason
		}
		if encodeTextAsJSON {
			encoded, _ := json.Marshal(reason)
			return string(encoded)
		}
		return reason
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		return string(out.JSON)
	default:
		if len(out.JSON) > 0 {
			return string(out.JSON)
		}
		return out.Text
	}
}

func dataURI(mediaType, b64 string) string {
	return fmt.Sprintf("data:%s;base64,%s", mediaType, b64)
}

func topLevelMediaType(value string) string {
	before, _, _ := strings.Cut(mediaType(value), "/")
	return before
}

func mediaType(value string) string {
	mt, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return strings.ToLower(mt)
}

func resolveFullMediaType(part provider.ContentPart) (string, error) {
	mt := mediaType(part.MediaType)
	if isFullMediaType(mt) {
		return mt, nil
	}
	if part.Data == nil || part.Data.URL != "" {
		return "", fmt.Errorf("openai: file of media type %q must specify subtype since it is not passed as inline bytes", part.MediaType)
	}
	if detected := detectMediaType(part.Data, topLevelMediaType(part.MediaType)); detected != "" {
		return detected, nil
	}
	return "", fmt.Errorf("openai: file of media type %q must specify subtype since it could not be auto-detected", part.MediaType)
}

func isFullMediaType(value string) bool {
	_, subtype, ok := strings.Cut(mediaType(value), "/")
	return ok && subtype != "" && subtype != "*"
}

func detectMediaType(data *provider.DataContent, topLevel string) string {
	return mediatype.Detect(data.Bytes, data.Base64, topLevel)
}

func partImageDetail(part provider.ContentPart, providerOptionsName string) string {
	return openAIPartOptionsFor(part.ProviderOptions, providerOptionsName).ImageDetail
}

func inputTextContent(text string, breakpoint *PromptCacheBreakpoint) responses.ResponseInputContentUnionParam {
	inputText := responses.ResponseInputTextParam{Text: text}
	if breakpoint != nil {
		inputText.SetExtraFields(map[string]any{"prompt_cache_breakpoint": breakpoint})
	}
	return responses.ResponseInputContentUnionParam{OfInputText: &inputText}
}

func promptCacheBreakpoint(opts provider.ProviderOptions, providerOptionsName string) *PromptCacheBreakpoint {
	po := openAIPartOptionsFor(opts, providerOptionsName)
	return po.PromptCacheBreakpoint
}

func openAIPartOptionsFor(opts provider.ProviderOptions, providerOptionsName string) OpenAIPartOptions {
	po, ok, err := provider.ResolveOption[OpenAIPartOptions](opts, providerOptionsName)
	if err != nil || !ok {
		return OpenAIPartOptions{}
	}
	return po
}
