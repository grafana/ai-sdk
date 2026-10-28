package openai

import (
	"slices"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
)

// convertInput converts the prompt messages into the Responses input item
// array. systemMode controls system message handling; store drives item
// reference emission.
func convertInput(prompt []provider.Message, systemMode string, popts OpenAIResponsesOptions, ctx inputConversionContext) (responses.ResponseInputParam, []provider.Warning, error) {
	var input responses.ResponseInputParam
	var warnings []provider.Warning
	if ctx.hasConversation || ctx.hasPreviousResponseID {
		ctx.parallelResults = collectParallelResults(prompt, ctx.providerOptionsName)
		ctx.emittedParallelCalls = make(map[string]bool)
		ctx.emittedParallelResults = make(map[string]bool)
	}

	for _, msg := range prompt {
		switch msg.Role {
		case provider.RoleSystem:
			update, err := systemMessageEffortUpdate(msg, ctx)
			if err != nil {
				return nil, nil, err
			}
			if update != "" {
				input = append(input, configurationUpdateItem(update))
				continue
			}
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
			item, err := convertUserMessage(msg, popts, ctx.providerOptionsName)
			if err != nil {
				return nil, nil, err
			}
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

	if !ctx.store && slices.ContainsFunc(input, isReasoningWithoutEncryptedContent) {
		warnings = append(warnings, unstoredReasoningWithoutEncryptedContentWarning)
		input = slices.DeleteFunc(input, isReasoningWithoutEncryptedContent)
		if len(input) == 0 {
			input = nil
		}
	}

	return input, warnings, nil
}

var unstoredReasoningWithoutEncryptedContentWarning = provider.Warning{
	Type:    provider.WarnOther,
	Feature: "reasoning",
	Message: "reasoning parts without encrypted content are not supported when store is false and are omitted",
}

func isReasoningWithoutEncryptedContent(item responses.ResponseInputItemUnionParam) bool {
	return item.OfReasoning != nil && !item.OfReasoning.EncryptedContent.Valid()
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
