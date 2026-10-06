package bedrock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

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
	return buildRequestWithFamily(modelID, "", opts)
}

func buildRequestWithFamily(modelID string, family ModelFamily, opts provider.CallOptions) (*converseInput, []provider.Warning, requestMeta, error) {
	var warnings []provider.Warning
	meta := requestMeta{isMistral: isMistralModel(modelID)}
	if err := provider.ValidateFileInputs(opts.Prompt); err != nil {
		return nil, warnings, meta, fmt.Errorf("bedrock: invalid file input: %w", err)
	}

	// Resolve Bedrock provider options (legacy `bedrock` key honored). A
	// malformed option is a hard error (matching the anthropic provider).
	bo, _, err := readBedrockOptions(opts.ProviderOptions)
	if err != nil {
		return nil, warnings, meta, err
	}
	anthropicOpts, err := readAnthropicCallOptions(opts.ProviderOptions)
	if err != nil {
		return nil, warnings, meta, err
	}
	mode := bo.StructuredOutputMode
	if mode == "" {
		mode = anthropicOpts.StructuredOutputMode
	}
	if mode == "" {
		mode = StructuredOutputModeAuto
	}
	if mode != StructuredOutputModeAuto && mode != StructuredOutputModeOutputFormat && mode != StructuredOutputModeJSONTool {
		return nil, warnings, meta, fmt.Errorf("bedrock: unsupported structuredOutputMode %q", mode)
	}
	hasBudgetTokens := bo.ReasoningConfig != nil && (bo.ReasoningConfig.budgetTokensPresent || bo.ReasoningConfig.BudgetTokens != 0)
	isAnthropic := isAnthropicModelForCall(modelID, family, hasBudgetTokens)
	bo.ReasoningConfig = resolveReasoningConfig(modelID, isAnthropic, opts.Reasoning, bo.ReasoningConfig, &warnings)

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

	// Whether extended thinking is enabled (Anthropic only). Used both for the
	// native-structured-output gate and inference-config adjustments below.
	isThinkingEnabled := bo.ReasoningConfig != nil &&
		(bo.ReasoningConfig.Type == "enabled" || bo.ReasoningConfig.Type == "adaptive" || bo.ReasoningConfig.Type == reasoningTypeBetweenTools)

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
		} else if isAnthropic && (mode == StructuredOutputModeOutputFormat || (mode == StructuredOutputModeAuto && !rejectsNativeStructuredOutput(modelID) && (supportsStructuredOutputCapability(modelID) || isThinkingEnabled || family == ModelFamilyAnthropic))) {
			useNativeStructuredOutput = true
		} else if isAnthropic && (rejectsForcedToolUse(modelID) || (mode != StructuredOutputModeJSONTool && usesJSONInstructionForStructuredOutput(modelID) && len(opts.Tools) > 0)) {
			// The JSON response tool needs forced tool use, so models that
			// reject it always get the instruction (upstream
			// amazon-bedrock-chat-language-model.ts, 5.0.99).
			converted.System = injectJSONInstruction(converted.System, opts.ResponseFormat.Schema)
			meta.usesJSONInstruction = true
		} else {
			meta.usesJSONResponseTool = true
		}
	} else if opts.ResponseFormat != nil && opts.ResponseFormat.Type != provider.ResponseFormatText {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "responseFormat",
			Details: fmt.Sprintf("Bedrock does not support response format %q", opts.ResponseFormat.Type),
		})
	}

	tools := opts.Tools
	toolChoice := opts.ToolChoice
	if meta.usesJSONResponseTool {
		tools = append(append([]provider.Tool(nil), tools...), provider.Tool{
			Type: provider.ToolTypeFunction, Name: jsonResponseToolName,
			Description: "Respond with a JSON object.", InputSchema: opts.ResponseFormat.Schema,
		})
		toolChoice = &provider.ToolChoice{Type: provider.ToolChoiceRequired}
	}
	pt, err := prepareTools(tools, toolChoice, modelID, isAnthropic, anthropicOpts.DisableParallelToolUse)
	if err != nil {
		return nil, warnings, meta, err
	}
	warnings = append(warnings, pt.warnings...)

	// Inference config (scalar sampling params).
	inf, infWarnings := buildInferenceConfig(modelID, opts, isAnthropic, bo)
	warnings = append(warnings, infWarnings...)

	// additionalModelRequestFields construction.
	addRequestFields := map[string]any{}

	// Anthropic-specific thinking/effort/beta pass-throughs.
	addWarn := applyAnthropicPassThroughs(addRequestFields, &inf, isAnthropic, bo)
	warnings = append(warnings, addWarn...)

	// OpenAI / Nova effort routing (non-Anthropic, model-prefix gated).
	applyNonAnthropicEffort(addRequestFields, modelID, isAnthropic, bo)

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
	if mode == StructuredOutputModeJSONTool {
		if original, ok := addRequestFields["output_config"].(map[string]any); ok {
			config := maps.Clone(original)
			addRequestFields["output_config"] = config
			delete(config, "format")
			if len(config) == 0 {
				delete(addRequestFields, "output_config")
			}
		}
	}

	// Merge in additionalTools (Anthropic provider-tool tool_choice).
	for k, v := range pt.additionalTools {
		addRequestFields[k] = v
	}

	// Anthropic beta propagation.
	if bo.AnthropicBeta != nil || len(pt.betas) > 0 {
		betas := append([]string{}, bo.AnthropicBeta...)
		betas = append(betas, pt.betaOrder...)
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
func buildInferenceConfig(modelID string, opts provider.CallOptions, isAnthropic bool, bo BedrockOptions) (*inferenceConfig, []provider.Warning) {
	var warnings []provider.Warning
	inf := &inferenceConfig{}
	rejectsSampling := isAnthropic && rejectsSamplingParameters(modelID)
	if rejectsSampling {
		for _, feature := range []struct {
			name    string
			present bool
		}{
			{"temperature", opts.Temperature != nil},
			{"topK", opts.TopK != nil},
			{"topP", opts.TopP != nil},
		} {
			if feature.present {
				warnings = append(warnings, provider.Warning{
					Type: provider.WarnUnsupported, Feature: feature.name,
					Details: fmt.Sprintf("%s is not supported by %s and will be ignored", feature.name, modelID),
				})
			}
		}
	}

	if opts.MaxOutputTokens != nil {
		v := *opts.MaxOutputTokens
		inf.MaxTokens = &v
	}
	if opts.Temperature != nil && !rejectsSampling {
		t := *opts.Temperature
		if t > 1 && (!isOpenAIModel(modelID) || isOpenAIGptOSSModel(modelID)) {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("%v exceeds bedrock maximum of 1.0. clamped to 1.0", t),
			})
			t = 1
		} else if t < 0 && (!isOpenAIModel(modelID) || isOpenAIGptOSSModel(modelID)) {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("%v is below bedrock minimum of 0. clamped to 0", t),
			})
			t = 0
		}
		inf.Temperature = &t
	}
	if opts.TopP != nil && !rejectsSampling {
		v := *opts.TopP
		inf.TopP = &v
	}
	if opts.TopK != nil && !rejectsSampling {
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
	if bo.ReasoningConfig != nil && (bo.ReasoningConfig.Type == "enabled" || bo.ReasoningConfig.Type == "adaptive" || bo.ReasoningConfig.Type == reasoningTypeBetweenTools) && isAnthropic {
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

	if isOpenAIModel(modelID) {
		for _, feature := range []struct {
			name    string
			present bool
			clear   func()
		}{
			{"temperature", !isOpenAIGptOSSModel(modelID) && inf.Temperature != nil, func() { inf.Temperature = nil }},
			{"topP", !isOpenAIGptOSSModel(modelID) && inf.TopP != nil, func() { inf.TopP = nil }},
			{"stopSequences", len(inf.StopSequences) > 0, func() { inf.StopSequences = nil }},
		} {
			if feature.present {
				feature.clear()
				warnings = append(warnings, provider.Warning{
					Type: provider.WarnUnsupported, Feature: feature.name,
					Details: fmt.Sprintf("%s is not supported by this OpenAI model on the Converse API", feature.name),
				})
			}
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
func applyAnthropicPassThroughs(addFields map[string]any, inf **inferenceConfig, isAnth bool, bo BedrockOptions) []provider.Warning {
	var warnings []provider.Warning

	if bo.ReasoningConfig != nil {
		rc := bo.ReasoningConfig
		if !isAnth {
			if rc.BudgetTokens != 0 || rc.budgetTokensPresent {
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
		} else if rc.Type == "enabled" && (rc.BudgetTokens != 0 || rc.budgetTokensPresent) {
			addFields["thinking"] = map[string]any{"type": "enabled", "budget_tokens": rc.BudgetTokens}
			// Increase maxTokens by budget so the model has room for thinking
			// plus the actual reply. Upstream does the same; the user-facing
			// `MaxOutputTokens` stays unchanged in their accounting.
			if *inf == nil {
				*inf = &inferenceConfig{}
			}
			if (*inf).MaxTokens == nil {
				def := rc.BudgetTokens + 4096
				(*inf).MaxTokens = &def
			} else {
				sum := *(*inf).MaxTokens + rc.BudgetTokens
				(*inf).MaxTokens = &sum
			}
		} else if rc.Type == "adaptive" {
			m := map[string]any{"type": "adaptive"}
			if rc.Display != "" {
				m["display"] = rc.Display
			}
			addFields["thinking"] = m
		} else if rc.Type == reasoningTypeBetweenTools {
			// between_tools accepts no display or budget, and only low,
			// medium, and high effort.
			addFields["thinking"] = map[string]any{"type": reasoningTypeBetweenTools}
			if rc.MaxReasoningEffort == "xhigh" || rc.MaxReasoningEffort == "max" {
				warnings = append(warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "providerOptions.amazonBedrock.reasoningConfig.maxReasoningEffort",
					Details: fmt.Sprintf("effort '%s' is not supported with 'between_tools' thinking. The effort has been lowered to 'high'.", rc.MaxReasoningEffort),
				})
				rc.MaxReasoningEffort = "high"
			}
		}

		if rc.MaxReasoningEffort != "" && isAnth {
			ensureMap(addFields, "output_config")["effort"] = rc.MaxReasoningEffort
		}
	}

	return warnings
}

func resolveReasoningConfig(modelID string, isAnthropic bool, reasoning provider.ReasoningEffort, explicit *ReasoningConfig, warnings *[]provider.Warning) *ReasoningConfig {
	if reasoning == provider.ReasoningProviderDefault {
		return explicit
	}

	var resolved *ReasoningConfig
	if reasoning == provider.ReasoningNone {
		if isAnthropic {
			resolved = &ReasoningConfig{Type: anthropicNoneReasoningType(modelID)}
		} else {
			resolved = cloneReasoningConfig(explicit)
		}
	} else {
		if !isAnthropic && !isOpenAIModel(modelID) && !strings.Contains(modelID, "amazon.nova-2-lite-v1:0") && explicit == nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoning",
				Details: "Portable reasoning is not supported for this model and will be ignored. If the model supports a provider-specific reasoning configuration, use providerOptions.amazonBedrock.reasoningConfig.",
			})
			return nil
		}
		resolved = mergeReasoningConfig(deriveReasoningConfig(modelID, isAnthropic, reasoning, warnings), explicit)
		if !isAnthropic && strings.Contains(modelID, "amazon.nova-2-lite-v1:0") && resolved != nil && resolved.Type == "" {
			resolved.Type = "enabled"
		}
	}
	if resolved != nil && resolved.Type == "disabled" {
		resolved.BudgetTokens = 0
		resolved.budgetTokensPresent = false
		resolved.MaxReasoningEffort = ""
	}
	return resolved
}

// reasoningTypeBetweenTools is the lowest thinking setting on models that
// reject disabled thinking; see supportsBetweenToolsThinking.
const reasoningTypeBetweenTools = "between_tools"

// anthropicNoneReasoningType returns the thinking type for reasoning "none".
// Upstream @ai-sdk/amazon-bedrock 5.0.99 uses "disabled", which omits the
// thinking field and runs claude-sonnet-5-5 at its default adaptive effort.
// This intentionally sends between_tools instead so "none" keeps up-front
// thinking off, as the Anthropic provider does.
func anthropicNoneReasoningType(modelID string) string {
	if supportsBetweenToolsThinking(modelID) {
		return reasoningTypeBetweenTools
	}
	return "disabled"
}

func cloneReasoningConfig(config *ReasoningConfig) *ReasoningConfig {
	if config == nil {
		return nil
	}
	cloned := *config
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
	if explicit.BudgetTokens != 0 || explicit.budgetTokensPresent {
		merged.BudgetTokens = explicit.BudgetTokens
		merged.budgetTokensPresent = explicit.budgetTokensPresent
	}
	if explicit.Display != "" {
		merged.Display = explicit.Display
	}
	if explicit.MaxReasoningEffort != "" {
		merged.MaxReasoningEffort = explicit.MaxReasoningEffort
	}
	return merged
}

func deriveReasoningConfig(modelID string, isAnthropic bool, reasoning provider.ReasoningEffort, warnings *[]provider.Warning) *ReasoningConfig {
	if reasoning == provider.ReasoningProviderDefault {
		return nil
	}
	if reasoning == provider.ReasoningNone {
		if isAnthropic {
			return &ReasoningConfig{Type: anthropicNoneReasoningType(modelID)}
		}
		return nil
	}
	if isAnthropic {
		if supportsAdaptiveThinking(modelID) {
			effort, ok := reasoningEffort(reasoning, warnings)
			if !ok {
				*warnings = append(*warnings, provider.Warning{
					Type:    provider.WarnUnsupported,
					Feature: "reasoning",
					Details: fmt.Sprintf("reasoning %q is not supported by this model.", reasoning),
				})
				return nil
			}
			return &ReasoningConfig{Type: "adaptive", MaxReasoningEffort: effort}
		}
		budget, ok := reasoningBudget(modelID, reasoning)
		if !ok {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoning",
				Details: fmt.Sprintf("reasoning %q is not supported by this model.", reasoning),
			})
			return nil
		}
		return &ReasoningConfig{Type: "enabled", BudgetTokens: budget}
	}
	effort, ok := reasoningEffort(reasoning, warnings)
	if !ok {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "reasoning",
			Details: fmt.Sprintf("reasoning %q is not supported by this model.", reasoning),
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
func applyNonAnthropicEffort(addFields map[string]any, modelID string, isAnthropic bool, bo BedrockOptions) {
	if bo.ReasoningConfig == nil || bo.ReasoningConfig.MaxReasoningEffort == "" {
		return
	}
	if isAnthropic {
		return
	}
	effort := bo.ReasoningConfig.MaxReasoningEffort
	if isOpenAIModel(modelID) {
		if isOpenAIGptOSSModel(modelID) {
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
	if bo.ReasoningConfig.Type == "enabled" && bo.ReasoningConfig.BudgetTokens > 0 {
		reasoningConfig["budgetTokens"] = bo.ReasoningConfig.BudgetTokens
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
