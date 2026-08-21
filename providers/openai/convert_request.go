package openai

import (
	"fmt"

	"github.com/grafana/ai-sdk/internal/providerrequest"
	"github.com/grafana/ai-sdk/provider"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

// buildResult threads conversion metadata from buildParams out to the response
// and stream converters.
type buildResult struct {
	// store is the resolved store flag (defaults true).
	store                  bool
	storeExplicitlyEnabled bool
	// providerOptionsName is "openai" (or "azure" for the Azure variant).
	providerOptionsName        string
	toolNameMapping            toolNameMapping
	webSearchToolName          string
	isShellProviderExecuted    bool
	approvalRequestToolCallIDs map[string]string
	// hasWebSearchTool indicates a web_search/web_search_preview tool is present.
	hasWebSearchTool bool
	// hasCodeInterpreterTool indicates a code_interpreter tool is present.
	hasCodeInterpreterTool bool
	// hasComputerTool indicates the client-executed computer tool is present.
	hasComputerTool   bool
	logprobsRequested bool
}

// buildParams converts provider.CallOptions into an OpenAI Responses request.
// It returns the request body, accumulated warnings, conversion metadata, and
// an error.
func buildParams(modelID string, opts provider.CallOptions) (responses.ResponseNewParams, []provider.Warning, buildResult, error) {
	if err := providerrequest.Validate(opts); err != nil {
		return responses.ResponseNewParams{}, nil, buildResult{}, fmt.Errorf("openai: invalid request: %w", err)
	}
	var warnings []provider.Warning

	caps := getModelCapabilities(modelID)
	popts, poptsName, err := resolveProviderOptions(opts)
	if err != nil {
		return responses.ResponseNewParams{}, nil, buildResult{}, err
	}

	store := true
	if popts.Store != nil {
		store = *popts.Store
	}

	systemMode := resolveSystemMessageMode(popts, caps)
	isReasoning := caps.isReasoningModel
	if popts.ForceReasoning != nil {
		isReasoning = *popts.ForceReasoning
	}

	br := buildResult{
		store:                      store,
		storeExplicitlyEnabled:     popts.Store != nil && *popts.Store,
		providerOptionsName:        poptsName,
		toolNameMapping:            newToolNameMapping(opts.Tools),
		approvalRequestToolCallIDs: approvalRequestToolCallIDMapping(opts.Prompt, poptsName),
	}

	body := responses.ResponseNewParams{
		Model: shared.ResponsesModel(modelID),
	}

	// Input conversion.
	inputCtx := newInputConversionContext(opts.Tools, br.toolNameMapping, store, poptsName, popts.Conversation != "", popts.PreviousResponseID != "")
	input, inputWarnings, err := convertInput(opts.Prompt, systemMode, popts, inputCtx)
	if err != nil {
		return responses.ResponseNewParams{}, nil, buildResult{}, err
	}
	warnings = append(warnings, inputWarnings...)
	body.Input = responses.ResponseNewParamsInputUnion{OfInputItemList: input}

	// Scalar params + unsupported-param warnings.
	scalarWarnings, err := applyScalarParams(&body, opts, caps, isReasoning, popts)
	warnings = append(warnings, scalarWarnings...)
	if err != nil {
		return responses.ResponseNewParams{}, warnings, buildResult{}, err
	}

	// Structured output.
	applyResponseFormat(&body, opts, popts)

	// Tools + tool choice.
	toolWarnings, err := prepareTools(&body, opts, popts, &br)
	if err != nil {
		return responses.ResponseNewParams{}, nil, buildResult{}, err
	}
	warnings = append(warnings, toolWarnings...)

	// Provider options -> request body.
	warnings = append(warnings, applyProviderOptions(&body, popts, isReasoning, caps)...)

	// include auto-population + reasoning block.
	applyIncludeAndReasoning(&body, opts, popts, isReasoning, store, &br)

	return body, warnings, br, nil
}

func approvalRequestToolCallIDMapping(prompt []provider.Message, providerOptionsName string) map[string]string {
	mapping := map[string]string{}
	for _, msg := range prompt {
		if msg.Role != provider.RoleAssistant {
			continue
		}
		for _, part := range msg.Content {
			switch part.Type {
			case provider.ContentPartTypeToolCall:
				po := openAIPartOptionsFor(part.ProviderOptions, providerOptionsName)
				if po.ApprovalRequestID != "" {
					mapping[po.ApprovalRequestID] = part.ToolCallID
				}
			case provider.ContentPartTypeToolApprovalRequest:
				if part.ApprovalID != "" && part.ToolCallID != "" {
					mapping[part.ApprovalID] = part.ToolCallID
				}
			}
		}
	}
	return mapping
}

// resolveProviderOptions parses the typed OpenAI provider options. The provider
// options name is "openai"; an "azure" fallback is parsed for parity.
func resolveProviderOptions(opts provider.CallOptions) (OpenAIResponsesOptions, string, error) {
	name := "openai"
	po, ok, err := provider.ResolveOption[OpenAIResponsesOptions](opts.ProviderOptions, name)
	if err != nil {
		return OpenAIResponsesOptions{}, name, err
	}
	if !ok {
		po, ok, err = provider.ResolveOption[OpenAIResponsesOptions](opts.ProviderOptions, "azure")
		if err != nil {
			return OpenAIResponsesOptions{}, "azure", err
		}
		if ok {
			name = "azure"
		}
	}
	if err := validateOpenAIResponsesOptions(po); err != nil {
		return OpenAIResponsesOptions{}, name, err
	}
	return po, name, nil
}

func validateOpenAIResponsesOptions(options OpenAIResponsesOptions) error {
	switch options.ServiceTier {
	case "", "auto", "flex", "priority", "fast", "default":
		return nil
	default:
		return fmt.Errorf("openai: invalid serviceTier %q", options.ServiceTier)
	}
}

// resolveSystemMessageMode determines how system messages are mapped.
func resolveSystemMessageMode(popts OpenAIResponsesOptions, caps modelCapabilities) string {
	if popts.SystemMessageMode != "" {
		return popts.SystemMessageMode
	}
	isReasoning := caps.isReasoningModel
	if popts.ForceReasoning != nil {
		isReasoning = *popts.ForceReasoning
	}
	if isReasoning {
		return "developer"
	}
	return caps.systemMessageMode
}

// applyScalarParams maps temperature/topP/maxOutputTokens and emits warnings for
// unsupported sampling parameters and capability-gated parameters.
func applyScalarParams(body *responses.ResponseNewParams, opts provider.CallOptions, caps modelCapabilities, isReasoning bool, popts OpenAIResponsesOptions) ([]provider.Warning, error) {
	var warnings []provider.Warning

	if opts.TopK != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "topK"})
	}
	if opts.Seed != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "seed"})
	}
	if opts.PresencePenalty != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "presencePenalty"})
	}
	if opts.FrequencyPenalty != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "frequencyPenalty"})
	}
	if len(opts.StopSequences) > 0 {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "stopSequences"})
	}

	if opts.MaxOutputTokens != nil {
		if err := applyMaxOutputTokens(body, *opts.MaxOutputTokens); err != nil {
			return warnings, err
		}
	}

	// Capability gating for temperature/topP on reasoning models.
	resolvedEffort := popts.ReasoningEffort
	if resolvedEffort == "" && opts.Reasoning != nil && *opts.Reasoning != provider.ReasoningProviderDefault {
		resolvedEffort = string(*opts.Reasoning)
	}
	allowNonReasoning := resolvedEffort == "none" && caps.supportsNonReasoningParameters

	if opts.Temperature != nil {
		if isReasoning && !allowNonReasoning {
			warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "temperature", Details: "temperature is not supported for reasoning models"})
		} else {
			body.Temperature = param.NewOpt(*opts.Temperature)
		}
	}
	if opts.TopP != nil {
		if isReasoning && !allowNonReasoning {
			warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "topP", Details: "topP is not supported for reasoning models"})
		} else {
			body.TopP = param.NewOpt(*opts.TopP)
		}
	}

	return warnings, nil
}
