package openai

import (
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// convertInput converts the prompt messages into the Responses input item
// array. systemMode controls system message handling; store drives item
// reference emission.
func convertInput(prompt []provider.Message, systemMode string, popts OpenAIResponsesOptions, ctx inputConversionContext) (responses.ResponseInputParam, []provider.Warning, error) {
	var input responses.ResponseInputParam
	var warnings []provider.Warning

	for _, msg := range prompt {
		switch msg.Role {
		case provider.RoleSystem:
			text := concatText(msg.Content)
			switch systemMode {
			case "remove":
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnOther,
					Feature: "system",
					Message: "system messages are removed for this model",
				})
			case "developer":
				input = append(input, systemMessageInput(text, responses.EasyInputMessageRoleDeveloper, msg.ProviderOptions, ctx.providerOptionsName))
			default: // "system"
				input = append(input, systemMessageInput(text, responses.EasyInputMessageRoleSystem, msg.ProviderOptions, ctx.providerOptionsName))
			}

		case provider.RoleUser:
			item, itemWarnings, err := convertUserMessage(msg, popts, ctx.providerOptionsName)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, itemWarnings...)
			input = append(input, item)

		case provider.RoleAssistant:
			items, itemWarnings, err := convertAssistantMessage(msg, ctx)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, itemWarnings...)
			input = append(input, items...)

		case provider.RoleTool:
			items, itemWarnings, err := convertToolMessage(msg, ctx)
			if err != nil {
				return nil, nil, err
			}
			warnings = append(warnings, itemWarnings...)
			input = append(input, items...)
		}
	}

	return input, warnings, nil
}

// concatText concatenates the text of all text content parts.
func concatText(parts []provider.ContentPart) string {
	var b []byte
	for _, p := range parts {
		if p.Type == provider.ContentPartTypeText {
			b = append(b, p.Text...)
		}
	}
	return string(b)
}

func systemMessageInput(text string, role responses.EasyInputMessageRole, opts provider.ProviderOptions, providerOptionsName string) responses.ResponseInputItemUnionParam {
	breakpoint := promptCacheBreakpoint(opts, providerOptionsName)
	if breakpoint == nil {
		return responses.ResponseInputItemParamOfMessage(text, role)
	}
	content := responses.ResponseInputMessageContentListParam{inputTextContent(text, breakpoint)}
	return responses.ResponseInputItemParamOfMessage(content, role)
}
