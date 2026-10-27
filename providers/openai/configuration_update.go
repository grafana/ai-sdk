package openai

import (
	"fmt"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const (
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
	if caps.supportedReasoningEfforts == nil || slices.Contains(caps.supportedReasoningEfforts, effort) {
		return ""
	}
	return fmt.Sprintf("%s only supports the following reasoning efforts: %s", modelID, strings.Join(caps.supportedReasoningEfforts, ", "))
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
func applyConfigurationUpdates(input responses.ResponseInputParam, modelID string, caps modelCapabilities, popts OpenAIResponsesOptions, unsupportedReason string) (responses.ResponseInputParam, []provider.Warning, error) {
	for _, item := range input {
		if effort, ok := configurationUpdateEffort(item); ok {
			if reason := unsupportedReasoningEffortReason(modelID, caps, effort); reason != "" {
				return nil, nil, fmt.Errorf("openai: message-level reasoningEffortUpdate: %s: %w", reason, provider.ErrUnsupportedFunctionality)
			}
		}
	}

	var warnings []provider.Warning
	if update := popts.ReasoningEffortUpdate; update != "" {
		reason := unsupportedReason
		if reason == "" {
			reason = unsupportedReasoningEffortReason(modelID, caps, string(update))
		}
		if reason != "" {
			warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "reasoningEffortUpdate", Details: reason})
		} else if len(input) == 0 || !isConfigurationUpdate(input[0], string(update)) {
			input = append(responses.ResponseInputParam{configurationUpdateItem(update)}, input...)
		}
	}

	for i := 1; i < len(input); i++ {
		if input[i-1].OfConfigurationUpdate != nil && input[i].OfConfigurationUpdate != nil {
			return nil, nil, fmt.Errorf("openai: adjacent reasoning effort configuration updates: %w", provider.ErrUnsupportedFunctionality)
		}
	}
	return input, warnings, nil
}

func isConfigurationUpdate(item responses.ResponseInputItemUnionParam, effort string) bool {
	got, ok := configurationUpdateEffort(item)
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
