package bedrock

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/grafana/ai-sdk/internal/anthropicschema"
	"github.com/grafana/ai-sdk/provider"
)

// requestMeta carries flags built during request preparation that the
// response/stream decoder needs to interpret the model's reply.
type requestMeta struct {
	// usesJSONResponseTool indicates the synthetic `json` tool was injected
	// to fulfill a ResponseFormat=json request. The response decoder will
	// translate the resulting tool call into the final text output.
	usesJSONResponseTool bool
	usesJSONInstruction  bool
	// isMistral matches the model id for Mistral, which influences tool
	// call id normalization both on the request and response side.
	isMistral bool
}

// buildRequest translates `provider.CallOptions` into a Converse request
// body. Returns the request shape, collected warnings, and per-request
// metadata used during response decoding.
func buildRequest(modelID string, opts provider.CallOptions) (*converseInput, []provider.Warning, requestMeta, error) {
	var warnings []provider.Warning
	meta := requestMeta{isMistral: isMistralModel(modelID)}

	// Resolve Bedrock provider options (legacy `bedrock` key honored). A
	// malformed option is a hard error (matching the anthropic provider).
	bo, _, err := readBedrockOptions(opts.ProviderOptions)
	if err != nil {
		return nil, warnings, meta, err
	}
	isAnthropic := isAnthropicRequest(modelID, bo.ReasoningConfig)
	bo.ReasoningConfig = resolveReasoningConfig(modelID, opts.Reasoning, bo.ReasoningConfig, isAnthropic, &warnings)
	anthropicOptions, err := readAnthropicProviderOptions(opts.ProviderOptions)
	if err != nil {
		return nil, warnings, meta, err
	}

	// Convert the prompt. We must know whether tools are active to decide
	// whether to strip tool content. Pre-prepare tools first so we know.
	hasAnyToolsHint := len(opts.Tools) > 0
	if opts.ResponseFormat != nil && opts.ResponseFormat.Type == provider.ResponseFormatJSON && len(opts.ResponseFormat.Schema) > 0 {
		// JSON response either injects the synthetic tool (non-native) or
		// uses native output_config -- either way callers benefit from tool
		// content being preserved in the prompt.
		hasAnyToolsHint = true
	}

	converted, promptWarnings, err := convertPrompt(opts.Prompt, meta.isMistral, hasAnyToolsHint)
	warnings = append(warnings, promptWarnings...)
	if err != nil {
		return nil, warnings, meta, err
	}

	// Tools.
	pt := prepareTools(opts.Tools, opts.ToolChoice, modelID, anthropicOptions.DisableParallelToolUse)
	warnings = append(warnings, pt.warnings...)

	// Whether extended thinking is enabled (Anthropic only). Used both for the
	// native-structured-output gate and inference-config adjustments below.
	isThinkingEnabled := bo.ReasoningConfig != nil &&
		(bo.ReasoningConfig.Type == "enabled" || bo.ReasoningConfig.Type == "adaptive")

	// ResponseFormat handling. Some Anthropic models reject native
	// output_config.format even when their model family otherwise supports it.
	// Opus 4.7/4.8 use an instruction when user tools are present so those tools
	// remain selectable; other non-native cases use the synthetic json tool.
	useNativeStructuredOutput := false
	if opts.ResponseFormat != nil && opts.ResponseFormat.Type == provider.ResponseFormatJSON {
		if len(opts.ResponseFormat.Schema) == 0 {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "responseFormat",
				Details: "Bedrock requires a schema for JSON response format; ignoring",
			})
		} else if isAnthropic && !rejectsNativeStructuredOutput(modelID) && (supportsNativeStructuredOutput(modelID) || isThinkingEnabled) {
			useNativeStructuredOutput = true
		} else if isAnthropic && usesJSONInstructionForStructuredOutput(modelID) && len(opts.Tools) > 0 {
			converted.System = injectJSONInstruction(converted.System, opts.ResponseFormat.Schema)
			meta.usesJSONInstruction = true
		} else {
			pt = injectJSONResponseTool(pt, opts.ResponseFormat.Schema)
			meta.usesJSONResponseTool = true
		}
	} else if opts.ResponseFormat != nil && opts.ResponseFormat.Type != provider.ResponseFormatText {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "responseFormat",
			Details: fmt.Sprintf("Bedrock does not support response format %q", opts.ResponseFormat.Type),
		})
	}

	// Inference config (scalar sampling params).
	inf, infWarnings := buildInferenceConfig(opts, bo, isAnthropic)
	warnings = append(warnings, infWarnings...)

	// additionalModelRequestFields construction.
	addRequestFields := map[string]any{}

	// Anthropic-specific thinking/effort/beta pass-throughs.
	addWarn := applyAnthropicPassThroughs(addRequestFields, &inf, bo, isAnthropic)
	warnings = append(warnings, addWarn...)

	// OpenAI / Nova effort routing (non-Anthropic, model-prefix gated).
	applyNonAnthropicEffort(addRequestFields, modelID, bo, isAnthropic)

	// Native structured output goes through additionalModelRequestFields.
	if useNativeStructuredOutput {
		var schema map[string]any
		if err := json.Unmarshal(opts.ResponseFormat.Schema, &schema); err != nil {
			return nil, warnings, meta, fmt.Errorf("bedrock: parsing response format schema: %w", err)
		}
		ensureMap(addRequestFields, "output_config")["format"] = map[string]any{
			"type":   "json_schema",
			"schema": anthropicschema.Sanitize(schema),
		}
	}

	// Merge in the caller's pass-through fields while preserving derived nested
	// fields such as output_config.format/effort.
	for k, v := range bo.AdditionalModelRequestFields {
		mergeAdditionalModelRequestField(addRequestFields, k, v)
	}

	// Merge in additionalTools (Anthropic provider-tool tool_choice).
	for k, v := range pt.additionalTools {
		addRequestFields[k] = v
	}

	// Anthropic beta propagation.
	if bo.AnthropicBeta != nil || len(pt.betas) > 0 {
		betas := append([]string{}, bo.AnthropicBeta...)
		for b := range pt.betas {
			betas = append(betas, b)
		}
		addRequestFields["anthropic_beta"] = betas
	}

	// Service tier (typed pass-through).
	var st *serviceTier
	if bo.ServiceTier != "" {
		st = &serviceTier{Type: bo.ServiceTier}
	}

	// additionalModelResponseFieldPaths: upstream sets this only for
	// Anthropic models so the response decoder can pick up
	// delta.stop_sequence from messageStop metadata.
	var addRespPaths []string
	if isAnthropic {
		addRespPaths = []string{"/delta/stop_sequence"}
	}

	if !hasActiveTools(pt) {
		converted.Messages, warnings = filterToolContentFromMessages(converted.Messages, warnings)
	}

	// Match upstream: system is always present, defaulting to an empty array.
	systemBlocks := converted.System
	if systemBlocks == nil {
		systemBlocks = []systemContentBlock{}
	}

	out := &converseInput{
		passthrough:                       bo.topLevelPassthrough(),
		System:                            systemBlocks,
		Messages:                          converted.Messages,
		InferenceConfig:                   inf,
		ToolConfig:                        pt.toolConfig,
		AdditionalModelRequestFields:      compactMap(addRequestFields),
		AdditionalModelResponseFieldPaths: addRespPaths,
		ServiceTier:                       st,
	}

	return out, warnings, meta, nil
}

func hasActiveTools(pt preparedTools) bool {
	return pt.toolConfig != nil && len(pt.toolConfig.Tools) > 0
}

func mergeAdditionalModelRequestField(dst map[string]any, key string, value any) {
	existing, ok := dst[key]
	if !ok {
		dst[key] = value
		return
	}
	existingMap, ok := existing.(map[string]any)
	if !ok {
		return
	}
	valueMap, ok := value.(map[string]any)
	if !ok {
		return
	}
	for k, v := range valueMap {
		mergeAdditionalModelRequestField(existingMap, k, v)
	}
}

// buildInferenceConfig maps scalar CallOptions params to Converse
// inferenceConfig, with clamping and warnings for out-of-range values and
// unsupported params (frequency/presence penalties, seed).
func buildInferenceConfig(opts provider.CallOptions, bo BedrockOptions, isAnthropic bool) (*inferenceConfig, []provider.Warning) {
	var warnings []provider.Warning
	inf := &inferenceConfig{}

	if opts.MaxOutputTokens != nil {
		v := *opts.MaxOutputTokens
		inf.MaxTokens = &v
	}
	if opts.Temperature != nil {
		t := *opts.Temperature
		if t > 1 {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("%v exceeds bedrock maximum of 1.0. clamped to 1.0", t),
			})
			t = 1
		} else if t < 0 {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("%v is below bedrock minimum of 0. clamped to 0", t),
			})
			t = 0
		}
		inf.Temperature = &t
	}
	if opts.TopP != nil {
		v := *opts.TopP
		inf.TopP = &v
	}
	if opts.TopK != nil {
		v := *opts.TopK
		inf.TopK = &v
	}
	if len(opts.StopSequences) > 0 {
		inf.StopSequences = append([]string{}, opts.StopSequences...)
	}

	if opts.FrequencyPenalty != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "frequencyPenalty"})
	}
	if opts.PresencePenalty != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "presencePenalty"})
	}
	if opts.Seed != nil {
		warnings = append(warnings, provider.Warning{Type: provider.WarnUnsupported, Feature: "seed"})
	}

	// When thinking is enabled (Anthropic only), drop temperature/topP/topK
	// with warnings.
	if bo.ReasoningConfig != nil && (bo.ReasoningConfig.Type == "enabled" || bo.ReasoningConfig.Type == "adaptive") && isAnthropic {
		if inf.Temperature != nil {
			warnings = append(warnings, provider.Warning{
				Type: provider.WarnUnsupported, Feature: "temperature",
				Details: "temperature is not supported when thinking is enabled",
			})
			inf.Temperature = nil
		}
		if inf.TopP != nil {
			warnings = append(warnings, provider.Warning{
				Type: provider.WarnUnsupported, Feature: "topP",
				Details: "topP is not supported when thinking is enabled",
			})
			inf.TopP = nil
		}
		if inf.TopK != nil {
			warnings = append(warnings, provider.Warning{
				Type: provider.WarnUnsupported, Feature: "topK",
				Details: "topK is not supported when thinking is enabled",
			})
			inf.TopK = nil
		}
	}

	// Treat empty inferenceConfig as nil so it doesn't serialize.
	if inf.MaxTokens == nil && inf.Temperature == nil && inf.TopP == nil && inf.TopK == nil && len(inf.StopSequences) == 0 {
		return nil, warnings
	}
	return inf, warnings
}

// applyAnthropicPassThroughs writes Anthropic-on-Bedrock specific knobs into
// additionalModelRequestFields. For non-Anthropic models that receive
// Anthropic-only options, it instead emits warnings.
func applyAnthropicPassThroughs(addFields map[string]any, inf **inferenceConfig, bo BedrockOptions, isAnthropic bool) []provider.Warning {
	var warnings []provider.Warning

	if bo.ReasoningConfig != nil {
		rc := bo.ReasoningConfig
		if !isAnthropic {
			if rc.BudgetTokens != nil {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "budgetTokens",
					Details: "budgetTokens applies only to Anthropic models on Bedrock and will be ignored for this model.",
				})
			}
			if rc.Type == "adaptive" {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "adaptive thinking",
					Details: "adaptive thinking type applies only to Anthropic models on Bedrock.",
				})
			}
		} else if rc.Type == "enabled" && rc.BudgetTokens != nil {
			budgetTokens := *rc.BudgetTokens
			addFields["thinking"] = map[string]any{"type": "enabled", "budget_tokens": budgetTokens}
			// Increase maxTokens by budget so the model has room for thinking
			// plus the actual reply. Upstream does the same; the user-facing
			// `MaxOutputTokens` stays unchanged in their accounting.
			if *inf == nil {
				*inf = &inferenceConfig{}
			}
			if (*inf).MaxTokens == nil {
				def := budgetTokens + 4096
				(*inf).MaxTokens = &def
			} else {
				sum := *(*inf).MaxTokens + budgetTokens
				(*inf).MaxTokens = &sum
			}
		} else if rc.Type == "adaptive" {
			m := map[string]any{"type": "adaptive"}
			if rc.Display != "" {
				m["display"] = rc.Display
			}
			addFields["thinking"] = m
		}

		if rc.MaxReasoningEffort != "" && isAnthropic {
			ensureMap(addFields, "output_config")["effort"] = rc.MaxReasoningEffort
		}
	}

	return warnings
}

func resolveReasoningConfig(modelID string, reasoning *provider.ReasoningEffort, explicit *ReasoningConfig, isAnthropic bool, warnings *[]provider.Warning) *ReasoningConfig {
	if reasoning == nil || *reasoning == provider.ReasoningProviderDefault {
		return explicit
	}

	var resolved *ReasoningConfig
	if *reasoning == provider.ReasoningNone {
		if isAnthropic {
			resolved = &ReasoningConfig{Type: "disabled"}
		} else {
			resolved = cloneReasoningConfig(explicit)
		}
	} else {
		resolved = mergeReasoningConfig(deriveReasoningConfig(modelID, reasoning, isAnthropic, warnings), explicit)
	}
	if resolved != nil && resolved.Type == "disabled" {
		resolved.BudgetTokens = nil
		resolved.MaxReasoningEffort = ""
	}
	return resolved
}

func cloneReasoningConfig(config *ReasoningConfig) *ReasoningConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	if config.BudgetTokens != nil {
		budgetTokens := *config.BudgetTokens
		cloned.BudgetTokens = &budgetTokens
	}
	return &cloned
}

func mergeReasoningConfig(derived, explicit *ReasoningConfig) *ReasoningConfig {
	merged := cloneReasoningConfig(derived)
	if explicit == nil {
		return merged
	}
	if merged == nil {
		merged = &ReasoningConfig{}
	}
	if explicit.Type != "" {
		merged.Type = explicit.Type
	}
	if explicit.BudgetTokens != nil {
		budgetTokens := *explicit.BudgetTokens
		merged.BudgetTokens = &budgetTokens
	}
	if explicit.Display != "" {
		merged.Display = explicit.Display
	}
	if explicit.MaxReasoningEffort != "" {
		merged.MaxReasoningEffort = explicit.MaxReasoningEffort
	}
	return merged
}

func deriveReasoningConfig(modelID string, reasoning *provider.ReasoningEffort, isAnthropic bool, warnings *[]provider.Warning) *ReasoningConfig {
	if reasoning == nil || *reasoning == provider.ReasoningProviderDefault {
		return nil
	}
	if *reasoning == provider.ReasoningNone {
		if isAnthropic {
			return &ReasoningConfig{Type: "disabled"}
		}
		return nil
	}
	if isAnthropic {
		if supportsAdaptiveThinking(modelID) {
			effort, ok := reasoningEffort(*reasoning, warnings)
			if !ok {
				*warnings = append(*warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "reasoning",
					Details: fmt.Sprintf("reasoning %q is not supported by this model.", *reasoning),
				})
				return nil
			}
			return &ReasoningConfig{Type: "adaptive", MaxReasoningEffort: effort}
		}
		budget, ok := reasoningBudget(modelID, *reasoning)
		if !ok {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoning",
				Details: fmt.Sprintf("reasoning %q is not supported by this model.", *reasoning),
			})
			return nil
		}
		return &ReasoningConfig{Type: "enabled", BudgetTokens: intPtr(budget)}
	}
	effort, ok := reasoningEffort(*reasoning, warnings)
	if !ok {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "reasoning",
			Details: fmt.Sprintf("reasoning %q is not supported by this model.", *reasoning),
		})
		return nil
	}
	return &ReasoningConfig{MaxReasoningEffort: effort}
}

func reasoningBudget(modelID string, reasoning provider.ReasoningEffort) (int, bool) {
	percentages := map[provider.ReasoningEffort]float64{
		provider.ReasoningMinimal: 0.02,
		provider.ReasoningLow:     0.1,
		provider.ReasoningMedium:  0.3,
		provider.ReasoningHigh:    0.6,
		provider.ReasoningXHigh:   0.9,
	}
	pct, ok := percentages[reasoning]
	if !ok {
		return 0, false
	}
	maxTokens := anthropicReasoningMaxOutputTokens(modelID)
	budget := int(float64(maxTokens)*pct + 0.5)
	if budget < 1024 {
		budget = 1024
	}
	if budget > maxTokens {
		budget = maxTokens
	}
	return budget, true
}

func reasoningEffort(reasoning provider.ReasoningEffort, warnings *[]provider.Warning) (string, bool) {
	var mapped string
	switch reasoning {
	case provider.ReasoningMinimal:
		mapped = "low"
	case provider.ReasoningLow:
		mapped = "low"
	case provider.ReasoningMedium:
		mapped = "medium"
	case provider.ReasoningHigh:
		mapped = "high"
	case provider.ReasoningXHigh:
		mapped = "max"
	default:
		return "", false
	}
	if mapped != string(reasoning) {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnCompatibility,
			Feature: "reasoning",
			Details: fmt.Sprintf("reasoning %q is not directly supported by this model. mapped to effort %q.", reasoning, mapped),
		})
	}
	return mapped, true
}

// applyNonAnthropicEffort routes effort hints to OpenAI/Nova-specific shapes.
func applyNonAnthropicEffort(addFields map[string]any, modelID string, bo BedrockOptions, isAnthropic bool) {
	if bo.ReasoningConfig == nil || bo.ReasoningConfig.MaxReasoningEffort == "" {
		return
	}
	if isAnthropic {
		return
	}
	effort := bo.ReasoningConfig.MaxReasoningEffort
	if isOpenAIModel(modelID) {
		if isOpenAIGPTOSSModel(modelID) {
			addFields["reasoning_effort"] = effort
		} else {
			ensureMap(addFields, "reasoning")["effort"] = effort
		}
		return
	}
	// Default to Nova-style reasoningConfig nesting for other model families.
	reasoningConfig := map[string]any{"maxReasoningEffort": effort}
	if bo.ReasoningConfig.Type != "" && bo.ReasoningConfig.Type != "adaptive" {
		reasoningConfig["type"] = bo.ReasoningConfig.Type
	}
	if bo.ReasoningConfig.Type == "enabled" && bo.ReasoningConfig.BudgetTokens != nil {
		reasoningConfig["budgetTokens"] = *bo.ReasoningConfig.BudgetTokens
	}
	addFields["reasoningConfig"] = reasoningConfig
}

func ensureMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	out := map[string]any{}
	m[key] = out
	return out
}

func injectJSONInstruction(system []systemContentBlock, schema json.RawMessage) []systemContentBlock {
	var compact bytes.Buffer
	if err := json.Compact(&compact, schema); err != nil {
		compact.Write(schema)
	}
	instruction := "JSON schema:\n" + compact.String() + "\nYou MUST answer with only a JSON object that matches the JSON schema above. Do not wrap it in markdown fences or include any other text."

	for i := range system {
		if system[i].CachePoint != nil {
			continue
		}
		if system[i].Text == "" {
			system[i].Text = instruction
		} else {
			system[i].Text += "\n\n" + instruction
		}
		return system
	}
	return append([]systemContentBlock{{Text: instruction}}, system...)
}

func compactMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	return m
}
