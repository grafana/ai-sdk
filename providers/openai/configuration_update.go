package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const (
	configurationUpdateAdjacent                 = "adjacent reasoning effort configuration updates"
	configurationUpdateNonEmptySystemMessage    = "message-level reasoningEffortUpdate requires empty system message content"
	configurationUpdateModelUnsupported         = "reasoningEffortUpdate is only supported by GPT-6 and later models"
	configurationUpdateConfigurationUnsupported = "reasoningEffortUpdate requires standard reasoning mode without automatic compaction or automatic truncation"
)

func (e OpenAIReasoningEffortUpdate) valid() bool {
	switch e {
	case OpenAIReasoningEffortUpdateNone, OpenAIReasoningEffortUpdateLow, OpenAIReasoningEffortUpdateMedium,
		OpenAIReasoningEffortUpdateHigh, OpenAIReasoningEffortUpdateXHigh, OpenAIReasoningEffortUpdateMax:
		return true
	default:
		return false
	}
}

func configurationUpdateUnsupportedReason(caps modelCapabilities, popts OpenAIResponsesOptions) string {
	if !caps.supportsConfigurationUpdate {
		return configurationUpdateModelUnsupported
	}
	if popts.ReasoningMode == "pro" || popts.ContextManagement != nil || popts.Truncation == "auto" {
		return configurationUpdateConfigurationUnsupported
	}
	return ""
}

func unsupportedReasoningEffortReason(modelID string, caps modelCapabilities, effort string) string {
	if effort == "" || caps.supportedReasoningEfforts == nil || slices.Contains(caps.supportedReasoningEfforts, effort) {
		return ""
	}
	return fmt.Sprintf("%s only supports the following reasoning efforts: %s", modelID, strings.Join(caps.supportedReasoningEfforts, ", "))
}

// systemMessageEffortUpdate returns the positioned update carried by a system
// message, resolving options from the active namespace with an OpenAI fallback.
func systemMessageEffortUpdate(msg provider.Message, ctx inputConversionContext) (OpenAIReasoningEffortUpdate, error) {
	update, ok, err := messageEffortUpdateOption(msg.ProviderOptions, ctx.providerOptionsName)
	if err == nil && !ok && ctx.providerOptionsName != providerName {
		update, _, err = messageEffortUpdateOption(msg.ProviderOptions, providerName)
	}
	if err != nil {
		return "", fmt.Errorf("openai: system message options: %w", err)
	}
	if update == "" {
		return "", nil
	}
	if !update.valid() {
		return "", fmt.Errorf("openai: invalid message-level reasoningEffortUpdate %q", update)
	}
	reason := ctx.configurationUpdateUnsupportedReason
	if concatText(msg.Content) != "" {
		reason = configurationUpdateNonEmptySystemMessage
	}
	if reason != "" {
		return "", fmt.Errorf("openai: message-level reasoningEffortUpdate: %s: %w", reason, provider.ErrUnsupportedFunctionality)
	}
	return update, nil
}

// messageEffortUpdateOption reads only reasoningEffortUpdate so unrelated part
// options keep their lenient handling on system messages.
func messageEffortUpdateOption(opts provider.ProviderOptions, key string) (OpenAIReasoningEffortUpdate, bool, error) {
	switch v := opts[key].(type) {
	case nil:
		return "", false, nil
	case OpenAIPartOptions:
		return v.ReasoningEffortUpdate, true, nil
	case provider.RawProviderOption:
		if bytes.Equal(bytes.TrimSpace(v.Raw), []byte("null")) {
			return "", false, nil
		}
		var options struct {
			ReasoningEffortUpdate OpenAIReasoningEffortUpdate `json:"reasoningEffortUpdate"`
		}
		if err := json.Unmarshal(v.Raw, &options); err != nil {
			return "", true, fmt.Errorf("unmarshaling %q option: %w", key, err)
		}
		return options.ReasoningEffortUpdate, true, nil
	default:
		return "", true, nil
	}
}

func configurationUpdateItem(effort OpenAIReasoningEffortUpdate) responses.ResponseInputItemUnionParam {
	return responses.ResponseInputItemUnionParam{OfConfigurationUpdate: &responses.ResponseConfigurationUpdateItemParam{
		Reasoning: responses.ResponseConfigurationUpdateItemParamReasoning{Effort: shared.ReasoningEffort(effort)},
	}}
}

func configurationUpdateEffort(item responses.ResponseInputItemUnionParam) (string, bool) {
	if item.OfConfigurationUpdate == nil {
		return "", false
	}
	return string(item.OfConfigurationUpdate.Reasoning.Effort), true
}

// applyConfigurationUpdates validates positioned updates against the model,
// prepends the request-level update when supported, and rejects adjacent
// updates that the Responses API does not accept.
func applyConfigurationUpdates(input responses.ResponseInputParam, modelID string, caps modelCapabilities, popts OpenAIResponsesOptions) (responses.ResponseInputParam, []provider.Warning, error) {
	for _, item := range input {
		if effort, ok := configurationUpdateEffort(item); ok {
			if reason := unsupportedReasoningEffortReason(modelID, caps, effort); reason != "" {
				return nil, nil, fmt.Errorf("openai: message-level reasoningEffortUpdate: %s: %w", reason, provider.ErrUnsupportedFunctionality)
			}
		}
	}

	var warnings []provider.Warning
	if update := popts.ReasoningEffortUpdate; update != "" {
		reason := configurationUpdateUnsupportedReason(caps, popts)
		if reason == "" {
			reason = unsupportedReasoningEffortReason(modelID, caps, string(update))
		}
		if reason != "" {
			warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "reasoningEffortUpdate", Details: reason})
		} else if !startsWithConfigurationUpdate(input, string(update)) {
			input = append(responses.ResponseInputParam{configurationUpdateItem(update)}, input...)
		}
	}

	for i := 1; i < len(input); i++ {
		if input[i-1].OfConfigurationUpdate != nil && input[i].OfConfigurationUpdate != nil {
			return nil, nil, fmt.Errorf("openai: %s: %w", configurationUpdateAdjacent, provider.ErrUnsupportedFunctionality)
		}
	}
	return input, warnings, nil
}

func startsWithConfigurationUpdate(input responses.ResponseInputParam, effort string) bool {
	if len(input) == 0 {
		return false
	}
	got, ok := configurationUpdateEffort(input[0])
	return ok && got == effort
}

// effectiveReasoningEffort returns the effort of the last configuration update
// in the input, falling back to the request-level effort.
func effectiveReasoningEffort(input responses.ResponseInputParam, resolvedEffort string) string {
	for i := len(input) - 1; i >= 0; i-- {
		if effort, ok := configurationUpdateEffort(input[i]); ok {
			return effort
		}
	}
	return resolvedEffort
}
