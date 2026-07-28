package openai

import (
	"encoding/json"

	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const topLogprobsMax = 20

// applyResponseFormat maps a JSON response format to the Responses text.format.
// A schema produces a json_schema format honoring strict/name/description; an
// absent schema produces a json_object format. textVerbosity is also applied.
func applyResponseFormat(body *responses.ResponseNewParams, opts provider.CallOptions, popts OpenAIResponsesOptions) {
	rf := opts.ResponseFormat

	var hasText bool
	text := responses.ResponseTextConfigParam{}

	if rf != nil && rf.Type == provider.ResponseFormatJSON {
		hasText = true
		if len(rf.Schema) > 0 {
			strict := true
			if popts.StrictJSONSchema != nil {
				strict = *popts.StrictJSONSchema
			}
			var schemaMap map[string]any
			_ = json.Unmarshal(rf.Schema, &schemaMap)
			name := rf.Name
			if name == "" {
				name = "response"
			}
			cfg := responses.ResponseFormatTextJSONSchemaConfigParam{
				Name:   name,
				Schema: schemaMap,
				Strict: param.NewOpt(strict),
			}
			if rf.Description != "" {
				cfg.Description = param.NewOpt(rf.Description)
			}
			text.Format = responses.ResponseFormatTextConfigUnionParam{OfJSONSchema: &cfg}
		} else {
			text.Format = responses.ResponseFormatTextConfigUnionParam{
				OfJSONObject: &shared.ResponseFormatJSONObjectParam{},
			}
		}
	}

	if popts.TextVerbosity != "" {
		hasText = true
		text.Verbosity = responses.ResponseTextConfigVerbosity(popts.TextVerbosity)
	}

	if hasText {
		body.Text = text
	}
}

// applyProviderOptions maps typed provider options onto the request body and
// emits warnings for conflicting or unsupported combinations.
func applyProviderOptions(body *responses.ResponseNewParams, popts OpenAIResponsesOptions, isReasoning bool, caps modelCapabilities) []provider.Warning {
	var warnings []provider.Warning

	if popts.Conversation != "" && popts.PreviousResponseID != "" {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "conversation",
			Details: "conversation and previousResponseId cannot be used together",
		})
	}

	if popts.PreviousResponseID != "" {
		body.PreviousResponseID = param.NewOpt(popts.PreviousResponseID)
	}
	if popts.Conversation != "" {
		body.Conversation = responses.ResponseNewParamsConversationUnion{
			OfString: param.NewOpt(popts.Conversation),
		}
	}
	if popts.Instructions != "" {
		body.Instructions = param.NewOpt(popts.Instructions)
	}
	if popts.MaxToolCalls != nil {
		body.MaxToolCalls = param.NewOpt(*popts.MaxToolCalls)
	}
	if popts.ParallelToolCalls != nil {
		body.ParallelToolCalls = param.NewOpt(*popts.ParallelToolCalls)
	}
	if popts.Store != nil {
		body.Store = param.NewOpt(*popts.Store)
	}
	if popts.User != "" {
		body.User = param.NewOpt(popts.User)
	}
	if popts.PromptCacheKey != "" {
		body.PromptCacheKey = param.NewOpt(popts.PromptCacheKey)
	}
	if popts.SafetyIdentifier != "" {
		body.SafetyIdentifier = param.NewOpt(popts.SafetyIdentifier)
	}
	if popts.PromptCacheRetention != "" {
		body.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetention(popts.PromptCacheRetention)
	}
	if popts.PromptCacheOptions != nil {
		body.SetExtraFields(map[string]any{"prompt_cache_options": popts.PromptCacheOptions})
	}
	if popts.Truncation != "" {
		body.Truncation = responses.ResponseNewParamsTruncation(popts.Truncation)
	}
	if len(popts.Metadata) > 0 {
		body.Metadata = shared.Metadata(popts.Metadata)
	}
	for _, cm := range popts.ContextManagement {
		entry := responses.ResponseNewParamsContextManagement{Type: cm.Type}
		if cm.CompactThreshold != nil {
			entry.CompactThreshold = param.NewOpt(*cm.CompactThreshold)
		}
		body.ContextManagement = append(body.ContextManagement, entry)
	}

	// Service tier with capability gating.
	if popts.ServiceTier != "" {
		switch popts.ServiceTier {
		case "flex":
			if !caps.supportsFlexProcessing {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "serviceTier",
					Details: "flex processing is only available for o3, o4-mini, and gpt-5 models",
				})
			} else {
				body.ServiceTier = responses.ResponseNewParamsServiceTier(popts.ServiceTier)
			}
		case "priority":
			if !caps.supportsPriorityProcessing {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "serviceTier",
					Details: "priority processing is only available for supported models and requires Enterprise access",
				})
			} else {
				body.ServiceTier = responses.ResponseNewParamsServiceTier(popts.ServiceTier)
			}
		default:
			body.ServiceTier = responses.ResponseNewParamsServiceTier(popts.ServiceTier)
		}
	}

	// Reasoning effort/summary warnings on non-reasoning models.
	if !isReasoning {
		if popts.ReasoningEffort != "" {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoningEffort",
				Details: "reasoningEffort is not supported for non-reasoning models",
			})
		}
		if popts.ReasoningSummary != "" {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoningSummary",
				Details: "reasoningSummary is not supported for non-reasoning models",
			})
		}
		if popts.ReasoningMode != "" {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoningMode",
				Details: "reasoningMode is not supported for non-reasoning models",
			})
		}
		if popts.ReasoningContext != "" {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoningContext",
				Details: "reasoningContext is not supported for non-reasoning models",
			})
		}
	}

	return warnings
}

// applyIncludeAndReasoning populates the include array (logprobs, web search
// sources, code interpreter outputs, encrypted reasoning) and the reasoning
// effort/summary block for reasoning models.
func applyIncludeAndReasoning(body *responses.ResponseNewParams, opts provider.CallOptions, popts OpenAIResponsesOptions, isReasoning, store bool, br *buildResult) {
	includes := map[responses.ResponseIncludable]bool{}
	for _, inc := range popts.Include {
		includes[responses.ResponseIncludable(inc)] = true
	}

	// Logprobs.
	var topLogprobs int64
	if popts.Logprobs != nil {
		if popts.Logprobs.Bool != nil && *popts.Logprobs.Bool {
			topLogprobs = topLogprobsMax
		} else if popts.Logprobs.Int != nil {
			topLogprobs = *popts.Logprobs.Int
		}
	}
	if topLogprobs > 0 {
		body.TopLogprobs = param.NewOpt(topLogprobs)
		includes[responses.ResponseIncludableMessageOutputTextLogprobs] = true
	}

	if br.hasWebSearchTool {
		includes[responses.ResponseIncludableWebSearchCallActionSources] = true
	}
	if br.hasCodeInterpreterTool {
		includes[responses.ResponseIncludableCodeInterpreterCallOutputs] = true
	}

	// When store is false on a reasoning model, include encrypted reasoning so
	// reasoning items round-trip statelessly.
	if !store && isReasoning {
		includes[responses.ResponseIncludableReasoningEncryptedContent] = true
	}

	if len(includes) > 0 {
		var list []responses.ResponseIncludable
		// Deterministic order is enforced by conformance object comparison
		// (arrays preserved); we append in a stable canonical order.
		for _, inc := range includeOrder {
			if includes[inc] {
				list = append(list, inc)
				delete(includes, inc)
			}
		}
		for inc := range includes {
			list = append(list, inc)
		}
		body.Include = list
	}

	// Reasoning effort + summary block.
	if isReasoning {
		effort := popts.ReasoningEffort
		if effort == "" && opts.Reasoning != nil && *opts.Reasoning != provider.ReasoningProviderDefault {
			effort = string(*opts.Reasoning)
		}
		summary := popts.ReasoningSummary
		if summary == "" && effort != "" && effort != "none" {
			summary = "detailed"
		}
		if effort != "" || summary != "" || popts.ReasoningMode != "" || popts.ReasoningContext != "" {
			r := shared.ReasoningParam{}
			if effort != "" {
				r.Effort = shared.ReasoningEffort(effort)
			}
			if summary != "" {
				r.Summary = shared.ReasoningSummary(summary)
			}
			extraFields := map[string]any{}
			if popts.ReasoningMode != "" {
				extraFields["mode"] = popts.ReasoningMode
			}
			if popts.ReasoningContext != "" {
				extraFields["context"] = popts.ReasoningContext
			}
			if len(extraFields) > 0 {
				r.SetExtraFields(extraFields)
			}
			body.Reasoning = r
		}
	}
}

// includeOrder defines the canonical ordering for include entries to keep
// request bodies stable across runs.
var includeOrder = []responses.ResponseIncludable{
	responses.ResponseIncludableReasoningEncryptedContent,
	responses.ResponseIncludableFileSearchCallResults,
	responses.ResponseIncludableWebSearchCallActionSources,
	responses.ResponseIncludableCodeInterpreterCallOutputs,
	responses.ResponseIncludableMessageOutputTextLogprobs,
	responses.ResponseIncludableMessageInputImageImageURL,
	responses.ResponseIncludableComputerCallOutputOutputImageURL,
}
