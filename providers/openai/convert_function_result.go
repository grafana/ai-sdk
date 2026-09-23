package openai

import (
	"encoding/base64"
	"fmt"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

func (c inputConversionContext) scalarResultBreakpoint(part provider.ContentPart) *PromptCacheBreakpoint {
	if part.Output == nil || part.Output.Type == provider.ToolOutputContent {
		return nil
	}
	if breakpoint := c.outputOptions(part.Output).PromptCacheBreakpoint; breakpoint != nil {
		return breakpoint
	}
	return c.partOptions(part).PromptCacheBreakpoint
}

func convertFunctionResultOutput(part provider.ContentPart, ctx inputConversionContext) (string, responses.ResponseFunctionCallOutputItemListParam, []provider.Warning, error) {
	if part.Output == nil || part.Output.Type != provider.ToolOutputContent {
		text := toolResultOutputString(part.Output, ctx.hasOutputSchema(part.ToolName))
		if breakpoint := ctx.scalarResultBreakpoint(part); breakpoint != nil {
			content := responses.ResponseInputTextContentParam{Text: text}
			content.SetExtraFields(map[string]any{"prompt_cache_breakpoint": breakpoint})
			return "", responses.ResponseFunctionCallOutputItemListParam{{OfInputText: &content}}, nil, nil
		}
		return text, nil, nil, nil
	}

	content := make(responses.ResponseFunctionCallOutputItemListParam, 0, len(part.Output.Content))
	var warnings []provider.Warning
	for _, value := range part.Output.Content {
		options := ctx.contentOptions(value)
		switch value.Type {
		case provider.ToolContentText:
			text := responses.ResponseInputTextContentParam{Text: value.Text}
			if options.PromptCacheBreakpoint != nil {
				text.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
			}
			content = append(content, responses.ResponseFunctionCallOutputItemUnionParam{OfInputText: &text})
		case provider.ToolContentFile:
			if value.Data == nil {
				warnings = append(warnings, provider.Warning{Type: provider.WarnOther, Message: "unsupported tool content part type: file with data type: unknown"})
				continue
			}
			image := topLevelMediaType(value.MediaType) == "image"
			data := value.Data
			switch {
			case len(data.Reference) > 0:
				fileID, err := resolveFileReference(data.Reference, ctx.providerOptionsName)
				if err != nil {
					return "", nil, nil, err
				}
				if image {
					item := responses.ResponseInputImageContentParam{FileID: param.NewOpt(fileID)}
					appendFunctionResultImage(&content, item, options)
				} else {
					item := responses.ResponseInputFileContentParam{FileID: param.NewOpt(fileID)}
					appendFunctionResultFile(&content, item, options)
				}
			case data.IsURL():
				if image {
					item := responses.ResponseInputImageContentParam{ImageURL: param.NewOpt(data.URL)}
					appendFunctionResultImage(&content, item, options)
				} else {
					item := responses.ResponseInputFileContentParam{FileURL: param.NewOpt(data.URL)}
					appendFunctionResultFile(&content, item, options)
				}
			case data.IsData():
				mediaType, err := resolveFullMediaType(provider.ContentPart{MediaType: value.MediaType, Data: data})
				if err != nil {
					return "", nil, nil, fmt.Errorf("openai: resolving tool result media type: %w", err)
				}
				encoded := data.Base64
				if data.Bytes != nil {
					encoded = base64.StdEncoding.EncodeToString(data.Bytes)
				}
				uri := dataURI(mediaType, encoded)
				if image {
					item := responses.ResponseInputImageContentParam{ImageURL: param.NewOpt(uri)}
					appendFunctionResultImage(&content, item, options)
				} else {
					filename := value.Filename
					if filename == "" {
						filename = "data"
					}
					item := responses.ResponseInputFileContentParam{FileData: param.NewOpt(uri), Filename: param.NewOpt(filename)}
					appendFunctionResultFile(&content, item, options)
				}
			default:
				dataType := "unknown"
				if data.Text != "" {
					dataType = "text"
				}
				warnings = append(warnings, provider.Warning{Type: provider.WarnOther, Message: fmt.Sprintf("unsupported tool content part type: file with data type: %s", dataType)})
			}
		default:
			warnings = append(warnings, provider.Warning{Type: provider.WarnOther, Message: fmt.Sprintf("unsupported tool content part type: %s", value.Type)})
		}
	}
	return "", content, warnings, nil
}

func appendFunctionResultImage(content *responses.ResponseFunctionCallOutputItemListParam, item responses.ResponseInputImageContentParam, options OpenAIPartOptions) {
	if options.ImageDetail != "" {
		item.Detail = responses.ResponseInputImageContentDetail(options.ImageDetail)
	}
	if options.PromptCacheBreakpoint != nil {
		item.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
	}
	*content = append(*content, responses.ResponseFunctionCallOutputItemUnionParam{OfInputImage: &item})
}

func appendFunctionResultFile(content *responses.ResponseFunctionCallOutputItemListParam, item responses.ResponseInputFileContentParam, options OpenAIPartOptions) {
	if options.PromptCacheBreakpoint != nil {
		item.SetExtraFields(map[string]any{"prompt_cache_breakpoint": options.PromptCacheBreakpoint})
	}
	*content = append(*content, responses.ResponseFunctionCallOutputItemUnionParam{OfInputFile: &item})
}
