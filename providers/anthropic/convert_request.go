package anthropic

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/grafana/ai-sdk/provider"
)

const (
	jsonResponseToolName           = "json"
	midConversationSystemBeta      = anthropic.AnthropicBeta("mid-conversation-system-2026-04-07")
	midConversationToolChangesBeta = anthropic.AnthropicBeta("mid-conversation-tool-changes-2026-07-01")
	serverSideFallbackDefaultBeta  = anthropic.AnthropicBeta("server-side-fallback-2026-07-01")
	serverSideFallbackExplicitBeta = anthropic.AnthropicBeta("server-side-fallback-2026-06-01")
)

func trimECMAScriptWhitespace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		switch r {
		case '\t', '\v', '\f', '\n', '\r', '\u2028', '\u2029', '\ufeff':
			return true
		default:
			return unicode.Is(unicode.Zs, r)
		}
	})
}

type buildResult struct {
	usesJsonResponseTool bool
	// markCodeExecutionDynamic mirrors upstream
	// hasWebTool20260209WithoutCodeExecution
	// (anthropic-language-model.ts:2639): when a web_fetch_20260209 or
	// web_search_20260209 provider tool is configured without an explicit
	// code_execution tool, downstream code_execution server_tool_use blocks
	// must be flagged with dynamic: true so they bypass the strict tool
	// validation layer.
	markCodeExecutionDynamic bool
	warnings                 []provider.Warning
	requestOptions           []option.RequestOption
}

func applyResponseFormat(p *anthropic.BetaMessageNewParams, rf *provider.ResponseFormat, caps modelCapabilities, defaultEagerInputStreaming bool, opts AnthropicOptions) buildResult {
	if rf.Type == provider.ResponseFormatText {
		return buildResult{}
	}

	if rf.Type == provider.ResponseFormatJSON && len(rf.Schema) == 0 {
		return buildResult{
			warnings: []provider.Warning{{
				Type:    provider.WarnUnsupported,
				Feature: "responseFormat",
				Details: "Anthropic does not support schemaless JSON mode; provide a schema via output.Object, output.Array, or output.Choice",
			}},
		}
	}

	if rf.Type != provider.ResponseFormatJSON {
		return buildResult{
			warnings: []provider.Warning{{
				Type:    provider.WarnUnsupported,
				Feature: "responseFormat",
				Details: fmt.Sprintf("unsupported response format type %q", rf.Type),
			}},
		}
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(rf.Schema, &schemaMap); err != nil {
		return buildResult{
			warnings: []provider.Warning{{
				Type:    provider.WarnUnsupported,
				Feature: "responseFormat",
				Details: fmt.Sprintf("failed to parse response format schema: %v", err),
			}},
		}
	}

	useStructuredOutput := caps.supportsStructuredOutput
	switch opts.StructuredOutputMode {
	case StructuredOutputFormat:
		useStructuredOutput = true
	case StructuredOutputJSONTool:
		useStructuredOutput = false
	}

	if useStructuredOutput {
		// Construct BetaJSONOutputFormatParam directly instead of calling
		// anthropic.BetaJSONSchemaOutputFormat. The SDK helper wraps a second
		// schema transform (anthropic-sdk-go's transformSchema) that drops
		// upstream-preserved keywords like definitions, allOf, $schema, $id,
		// enum, const, and default by stuffing them into description text as
		// Go map literals, and returns nil for root schemas that only have
		// allOf. Upstream's sanitize-json-schema is the single intended
		// transform on the wire payload.
		p.OutputConfig.Format = anthropic.BetaJSONOutputFormatParam{
			Schema: sanitizeJSONSchema(schemaMap),
		}
		return buildResult{}
	}

	jsonToolParam := &anthropic.BetaToolParam{
		Name:        jsonResponseToolName,
		Description: anthropic.String("Respond with a JSON object."),
		InputSchema: rawToolInputSchema(schemaMap),
	}
	// Mirror upstream behavior: when the JSON fallback tool is added on a
	// streaming request, it participates in the model-level
	// `eager_input_streaming` default just like any other function tool
	// (anthropic-language-model.ts:519-545, prepareTools is called once with
	// `[...tools, jsonResponseTool]`).
	if defaultEagerInputStreaming {
		jsonToolParam.EagerInputStreaming = anthropic.Bool(true)
	}
	p.Tools = append(p.Tools, anthropic.BetaToolUnionParam{OfTool: jsonToolParam})
	// Override any user-set tool choice to required (OfAny). The json tool must
	// be callable, and DisableParallelToolUse prevents multi-tool turns that
	// would complicate response remapping. This matches upstream behavior.
	p.ToolChoice = anthropic.BetaToolChoiceUnionParam{
		OfAny: &anthropic.BetaToolChoiceAnyParam{
			DisableParallelToolUse: anthropic.Bool(true),
		},
	}
	warnings := []provider.Warning(nil)
	if opts.DisableParallelToolUse != nil && !*opts.DisableParallelToolUse {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "providerOptions.anthropic.disableParallelToolUse",
			Details: "`disableParallelToolUse: false` is ignored when using the JSON response tool. Parallel tool use is disabled to ensure a single coherent JSON tool call.",
		})
	}
	return buildResult{usesJsonResponseTool: true, warnings: warnings}
}

func rawToolInputSchema(schema map[string]any) anthropic.BetaToolInputSchemaParam {
	if schema == nil {
		return anthropic.BetaToolInputSchemaParam{}
	}
	return anthropic.BetaToolInputSchemaParam{ExtraFields: schema}
}

type providerCapabilities struct {
	supportsNativeStructuredOutput bool
	supportsStrictTools            bool
	supportsDirectBetaFeatures     bool
}

var (
	directProviderCapabilities = providerCapabilities{
		supportsNativeStructuredOutput: true,
		supportsStrictTools:            true,
		supportsDirectBetaFeatures:     true,
	}
	vertexProviderCapabilities = providerCapabilities{}
)

func buildParams(modelID string, opts provider.CallOptions, stream bool) (anthropic.BetaMessageNewParams, toolNameMapping, []provider.Warning, buildResult, error) {
	return buildParamsWithCapabilities(modelID, opts, stream, directProviderCapabilities)
}

func buildParamsWithCapabilities(modelID string, opts provider.CallOptions, stream bool, providerCaps providerCapabilities) (anthropic.BetaMessageNewParams, toolNameMapping, []provider.Warning, buildResult, error) {
	var warnings []provider.Warning
	anthropicOpts, hasAnthropicOpts, err := provider.ResolveOption[AnthropicOptions](opts.ProviderOptions, "anthropic")
	if err != nil {
		return anthropic.BetaMessageNewParams{}, toolNameMapping{}, nil, buildResult{}, fmt.Errorf("anthropic: invalid provider options: %w", err)
	}
	if err := validateFallbackConfig(anthropicOpts.Fallbacks); err != nil {
		return anthropic.BetaMessageNewParams{}, toolNameMapping{}, nil, buildResult{}, fmt.Errorf("anthropic: invalid provider options: %w", err)
	}
	v := &cacheControlValidator{}
	mapping := newToolNameMapping(opts.Tools)

	caps := getModelCapabilities(modelID)
	supportsNativeStructuredOutput := providerCaps.supportsNativeStructuredOutput && caps.supportsStructuredOutput
	supportsStrictTools := providerCaps.supportsStrictTools && caps.supportsStructuredOutput
	userSetMaxOutput := opts.MaxOutputTokens != nil
	maxTok := int64(caps.maxOutputTokens)
	if userSetMaxOutput {
		maxTok = int64(*opts.MaxOutputTokens)
	} else if !caps.isKnownModel {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnCompatibility,
			Feature: "maxOutputTokens",
			Details: fmt.Sprintf("The model %q is unknown. The max output tokens have been limited to %d. Set maxOutputTokens explicitly to override this limit.", modelID, caps.maxOutputTokens),
		})
	}

	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(modelID),
		MaxTokens: maxTok,
	}

	// Default eager_input_streaming on function tools when the request is
	// streaming and the caller has not disabled tool streaming. Mirrors
	// upstream's `defaultEagerInputStreaming = stream && (toolStreaming ?? true)`.
	defaultEagerInputStreaming := stream && resolveAnthropicToolStreaming(opts.ProviderOptions)

	var toolBetas []string
	if len(opts.Tools) > 0 {
		var toolWarnings []provider.Warning
		p.Tools, toolWarnings, toolBetas = convertToolsWithStrictTools(v, opts.Tools, defaultEagerInputStreaming, supportsStrictTools)
		warnings = append(warnings, toolWarnings...)
	}

	mcpToolUseIDs := make(map[string]bool)

	// Pre-pass: merge consecutive user/tool messages into one Anthropic user
	// block, consecutive assistant messages into one Anthropic assistant
	// block, and consecutive system messages into a single run of system
	// blocks. Mirrors upstream `groupIntoBlocks`
	// (`packages/anthropic/src/convert-to-anthropic-prompt.ts:1129`). Without
	// this pre-pass, an `assistant(tool_use) -> tool(tool_result) -> user(text)`
	// sequence would emit two adjacent `role: "user"` Anthropic messages and
	// the API would reject the request because the `tool_result` block is not
	// in the message immediately after the `tool_use`.
	blocks := groupIntoBlocks(opts.Prompt)
	systemSet := false
	for blockIndex, block := range blocks {
		switch block.kind {
		case promptBlockKindSystem:
			var systemContent []anthropic.BetaTextBlockParam
			var messageContent []anthropic.BetaContentBlockParamUnion
			toolChangeCount := 0
			for _, msg := range block.messages {
				systemOpts, err := resolveAnthropicSystemMessageOptions(msg.ProviderOptions)
				if err != nil {
					return anthropic.BetaMessageNewParams{}, mapping, warnings, buildResult{}, fmt.Errorf("anthropic: invalid system message provider options: %w", err)
				}
				toolChanges := systemOpts.ToolChanges
				hadToolChanges := len(toolChanges) > 0
				if hadToolChanges && !providerCaps.supportsDirectBetaFeatures {
					warnings = append(warnings, provider.Warning{
						Type:    provider.WarnUnsupported,
						Feature: "providerOptions.anthropic.toolChanges",
						Details: "mid-conversation tool changes are not supported by the Anthropic Vertex provider and were ignored",
					})
					toolChanges = nil
				}

				text := systemMessageText(msg)
				if text != "" || !hadToolChanges {
					textBlock := anthropic.BetaTextBlockParam{
						Text:         text,
						CacheControl: v.getCacheControl(msg.ProviderOptions, true),
					}
					systemContent = append(systemContent, textBlock)
					messageContent = append(messageContent, anthropic.BetaContentBlockParamUnion{OfText: &textBlock})
				}
				for _, change := range toolChanges {
					toolChangeCount++
					messageContent = append(messageContent, param.Override[anthropic.BetaContentBlockParamUnion](map[string]any{
						"type": change.Type,
						"tool": map[string]any{
							"type": "tool_reference",
							"name": mapping.toProviderToolName(change.ToolName),
						},
					}))
				}
			}

			if blockIndex == 0 || (!systemSet && toolChangeCount == 0) {
				if toolChangeCount > 0 {
					warnings = append(warnings, provider.Warning{
						Type:    provider.WarnOther,
						Feature: "providerOptions.anthropic.toolChanges",
						Message: "tool changes on the initial system message are not supported by Anthropic. Configure the initial tool set via the tools option instead. The tool changes have been ignored.",
					})
				}
				p.System = systemContent
				systemSet = true
				continue
			}

			if len(messageContent) == 0 {
				continue
			}
			p.Messages = append(p.Messages, anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRoleSystem,
				Content: messageContent,
			})
			p.Betas = appendBetaUnique(p.Betas, midConversationSystemBeta)
			if toolChangeCount > 0 {
				p.Betas = appendBetaUnique(p.Betas, midConversationToolChangesBeta)
			}
		case promptBlockKindUser:
			var content []anthropic.BetaContentBlockParamUnion
			for _, msg := range block.messages {
				switch msg.Role {
				case provider.RoleUser:
					content = append(content, convertUserContent(v, msg.Content, msg.ProviderOptions, &p.Betas, mcpToolUseIDs, &warnings)...)
				case provider.RoleTool:
					content = append(content, convertToolContent(v, msg.Content, msg.ProviderOptions, mcpToolUseIDs, &warnings)...)
				}
			}
			p.Messages = append(p.Messages, anthropic.BetaMessageParam{
				Role:    "user",
				Content: content,
			})
		case promptBlockKindAssistant:
			var content []anthropic.BetaContentBlockParamUnion
			for messageIndex, msg := range block.messages {
				converted := convertAssistantContent(v, mapping, msg.Content, msg.ProviderOptions, mcpToolUseIDs, &warnings)
				isFinalAssistantText := blockIndex == len(blocks)-1 &&
					messageIndex == len(block.messages)-1 &&
					len(msg.Content) > 0 &&
					msg.Content[len(msg.Content)-1].Type == provider.ContentPartTypeText
				if isFinalAssistantText && len(converted) > 0 {
					finalPart := &converted[len(converted)-1]
					if finalPart.OfText != nil {
						finalPart.OfText.Text = trimECMAScriptWhitespace(finalPart.OfText.Text)
					}
				}
				content = append(content, converted...)
			}
			p.Messages = append(p.Messages, anthropic.BetaMessageParam{
				Role:    anthropic.BetaMessageParamRoleAssistant,
				Content: content,
			})
		}
	}

	warnings = append(warnings, v.warnings...)

	if opts.ToolChoice != nil {
		if opts.ToolChoice.Type == provider.ToolChoiceNone {
			p.Tools = nil
		} else {
			p.ToolChoice = convertToolChoice(*opts.ToolChoice, mapping)
		}
	}
	applyDisableParallelToolUse(&p, anthropicOpts.DisableParallelToolUse, len(p.Tools) > 0)

	if opts.Temperature != nil {
		temperature := *opts.Temperature
		if temperature > 1 {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("%v exceeds anthropic maximum of 1.0. clamped to 1.0", temperature),
			})
			temperature = 1
		} else if temperature < 0 {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("%v is below anthropic minimum of 0. clamped to 0", temperature),
			})
			temperature = 0
		}
		p.Temperature = anthropic.Float(temperature)
	}
	if opts.TopP != nil {
		p.TopP = anthropic.Float(*opts.TopP)
	}
	if opts.TopK != nil {
		p.TopK = anthropic.Int(int64(*opts.TopK))
	}
	if len(opts.StopSequences) > 0 {
		p.StopSequences = opts.StopSequences
	}

	if opts.PresencePenalty != nil {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "presencePenalty",
			Details: "Anthropic does not support presence penalty",
		})
	}
	if opts.FrequencyPenalty != nil {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "frequencyPenalty",
			Details: "Anthropic does not support frequency penalty",
		})
	}
	if opts.Seed != nil {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "seed",
			Details: "Anthropic does not support seed",
		})
	}

	var br buildResult
	if opts.ResponseFormat != nil {
		responseFormatCaps := caps
		responseFormatCaps.supportsStructuredOutput = supportsNativeStructuredOutput
		br = applyResponseFormat(&p, opts.ResponseFormat, responseFormatCaps, defaultEagerInputStreaming, anthropicOpts)
		warnings = append(warnings, br.warnings...)
	}
	br.markCodeExecutionDynamic = hasWebTool20260209WithoutCodeExecution(opts.Tools)

	// Map top-level Reasoning to Anthropic thinking/effort. Provider options
	// always take precedence: we only fall back to top-level mapping when
	// `effort` is unset on AnthropicOptions. If thinking is set on
	// AnthropicOptions but effort is not, we still derive effort from the
	// top-level reasoning hint. Mirrors upstream
	// anthropic-language-model.ts:390-413.
	if opts.Reasoning != nil && !hasProviderEffort(opts.ProviderOptions) {
		reasoning := *opts.Reasoning
		if reasoning != provider.ReasoningProviderDefault {
			rc := resolveReasoningConfig(reasoning, caps, &warnings)
			if rc != nil {
				providerThinking := providerThinkingType(opts.ProviderOptions)
				applyReasoningConfigWithProviderHints(&p, rc, providerThinking)
			}
		}
	}

	if caps.rejectsThinkingDisabledAboveHighEffort &&
		anthropicOpts.Thinking != nil &&
		anthropicOpts.Thinking.Type == ThinkingDisabled &&
		(anthropicOpts.Effort == "xhigh" || anthropicOpts.Effort == "max") {
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "providerOptions.anthropic.effort",
			Details: fmt.Sprintf("effort '%s' is not supported by %s when thinking is disabled. The effort has been lowered to 'high'.", anthropicOpts.Effort, modelID),
		})
		anthropicOpts.Effort = "high"
	}

	applyFallbacks(&p, anthropicOpts.Fallbacks, providerCaps, &br, &warnings)
	applyProviderOptions(&p, anthropicOpts, hasAnthropicOpts, &warnings)

	for _, b := range toolBetas {
		p.Betas = appendBetaUnique(p.Betas, anthropic.AnthropicBeta(b))
	}

	if supportsNativeStructuredOutput && !br.usesJsonResponseTool && hasFunctionTools(opts.Tools) {
		p.Betas = appendBetaUnique(p.Betas, "structured-outputs-2025-11-13")
	}

	if p.Thinking.OfEnabled != nil && p.Thinking.OfEnabled.BudgetTokens == 0 {
		p.Thinking.OfEnabled.BudgetTokens = 1024
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnCompatibility,
			Feature: "extended thinking",
			Details: "thinking budget is required when thinking is enabled. using default budget of 1024 tokens.",
		})
	}

	if caps.rejectsSamplingParams {
		if p.Temperature.Valid() {
			p.Temperature = param.Opt[float64]{}
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: fmt.Sprintf("temperature is not supported by %s and will be ignored", modelID),
			})
		}
		if p.TopK.Valid() {
			p.TopK = param.Opt[int64]{}
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "topK",
				Details: fmt.Sprintf("topK is not supported by %s and will be ignored", modelID),
			})
		}
		if p.TopP.Valid() {
			p.TopP = param.Opt[float64]{}
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "topP",
				Details: fmt.Sprintf("topP is not supported by %s and will be ignored", modelID),
			})
		}
	}

	// When thinking is active (enabled or adaptive), Anthropic rejects
	// requests that also carry temperature/topP/topK sampling params.
	// Mirror upstream anthropic-language-model.ts:608-633: drop the params
	// and emit unsupported warnings.
	if p.Thinking.OfEnabled != nil || p.Thinking.OfAdaptive != nil {
		if p.Temperature.Valid() {
			p.Temperature = param.Opt[float64]{}
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "temperature",
				Details: "temperature is not supported when thinking is enabled",
			})
		}
		if p.TopK.Valid() {
			p.TopK = param.Opt[int64]{}
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "topK",
				Details: "topK is not supported when thinking is enabled",
			})
		}
		if p.TopP.Valid() {
			p.TopP = param.Opt[float64]{}
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "topP",
				Details: "topP is not supported when thinking is enabled",
			})
		}
	}

	if p.Thinking.OfEnabled == nil && p.Thinking.OfAdaptive == nil && caps.isKnownModel && p.Temperature.Valid() && p.TopP.Valid() {
		p.TopP = param.Opt[float64]{}
		warnings = append(warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "topP",
			Details: "topP is not supported when temperature is set. topP is ignored.",
		})
	}

	if p.Thinking.OfEnabled != nil {
		p.MaxTokens += p.Thinking.OfEnabled.BudgetTokens
	}

	modelMax := int64(caps.maxOutputTokens)
	if caps.isKnownModel && p.MaxTokens > modelMax {
		if userSetMaxOutput {
			warnings = append(warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "maxOutputTokens",
				Details: fmt.Sprintf(
					"%d (maxOutputTokens + thinkingBudget) is greater than %s %d max output tokens. "+
						"The max output tokens have been limited to %d.",
					p.MaxTokens, modelID, caps.maxOutputTokens, caps.maxOutputTokens),
			})
		}
		p.MaxTokens = modelMax
	}

	return p, mapping, warnings, br, nil
}

// systemMessageText concatenates the text content of a system-role Message.
// Constructor [provider.NewSystemMessage] packs a single text part, but we
// support callers that build system messages with multiple text parts.
func systemMessageText(msg provider.Message) string {
	var sb strings.Builder
	for _, p := range msg.Content {
		if p.Type == provider.ContentPartTypeText {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

func resolveAnthropicSystemMessageOptions(opts provider.ProviderOptions) (AnthropicSystemMessageOptions, error) {
	value, ok := opts["anthropic"]
	if !ok {
		return AnthropicSystemMessageOptions{}, nil
	}

	var result AnthropicSystemMessageOptions
	switch value := value.(type) {
	case AnthropicSystemMessageOptions:
		result = value
	case AnthropicOptions, AnthropicToolOptions, AnthropicCacheControl:
		return AnthropicSystemMessageOptions{}, nil
	case provider.RawProviderOption:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(value.Raw, &fields); err != nil {
			return AnthropicSystemMessageOptions{}, err
		}
		if raw, exists := fields["toolChanges"]; exists {
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				return AnthropicSystemMessageOptions{}, fmt.Errorf("toolChanges must be an array")
			}
			var changes []struct {
				ToolName *string `json:"toolName"`
			}
			if err := json.Unmarshal(raw, &changes); err != nil {
				return AnthropicSystemMessageOptions{}, fmt.Errorf("decoding toolChanges: %w", err)
			}
			for i, change := range changes {
				if change.ToolName == nil {
					return AnthropicSystemMessageOptions{}, fmt.Errorf("toolChanges[%d].toolName is required", i)
				}
			}
		}
		var err error
		result, _, err = provider.ResolveOption[AnthropicSystemMessageOptions](opts, "anthropic")
		if err != nil {
			return AnthropicSystemMessageOptions{}, err
		}
	default:
		return AnthropicSystemMessageOptions{}, fmt.Errorf("unexpected anthropic option type %T", value)
	}

	for i, change := range result.ToolChanges {
		if change.Type != ToolAddition && change.Type != ToolRemoval {
			return AnthropicSystemMessageOptions{}, fmt.Errorf("toolChanges[%d].type %q is not supported", i, change.Type)
		}
	}
	return result, nil
}

// promptBlockKind identifies the kind of an [promptBlock] produced by
// [groupIntoBlocks]. Mirrors upstream's `SystemBlock | AssistantBlock |
// UserBlock` discriminated union (`convert-to-anthropic-prompt.ts:1116-1127`).
type promptBlockKind int

const (
	promptBlockKindSystem promptBlockKind = iota
	promptBlockKindUser
	promptBlockKindAssistant
)

// promptBlock is a contiguous run of [provider.Message] entries that all map
// onto the same Anthropic message kind. A `promptBlockKindUser` block holds
// a mix of `RoleUser` and `RoleTool` source messages; the other kinds hold a
// homogeneous run.
type promptBlock struct {
	kind     promptBlockKind
	messages []provider.Message
}

// groupIntoBlocks pre-groups a `[]provider.Message` prompt into Anthropic
// message blocks before per-block conversion. Mirrors upstream
// `groupIntoBlocks` (`convert-to-anthropic-prompt.ts:1129`):
//
//   - A `RoleUser` or `RoleTool` source message appends to the current user
//     block (or opens one). This merges consecutive user/tool runs into a
//     single Anthropic user message so a `tool_result` always lands in the
//     message immediately following the `tool_use`.
//   - A `RoleAssistant` source message appends to the current assistant block
//     (or opens one).
//   - A `RoleSystem` source message appends to the current system block (or
//     opens one).
//   - Any other role is ignored, matching the existing per-message switch's
//     fall-through behavior.
func groupIntoBlocks(prompt []provider.Message) []promptBlock {
	var blocks []promptBlock
	for _, msg := range prompt {
		var kind promptBlockKind
		switch msg.Role {
		case provider.RoleSystem:
			kind = promptBlockKindSystem
		case provider.RoleAssistant:
			kind = promptBlockKindAssistant
		case provider.RoleUser, provider.RoleTool:
			kind = promptBlockKindUser
		default:
			continue
		}
		if n := len(blocks); n > 0 && blocks[n-1].kind == kind {
			blocks[n-1].messages = append(blocks[n-1].messages, msg)
			continue
		}
		blocks = append(blocks, promptBlock{kind: kind, messages: []provider.Message{msg}})
	}
	return blocks
}

// convertUserContent converts a `RoleUser` provider message's content parts
// into Anthropic content blocks. Mirrors the `case 'user':` branch of
// upstream `convert-to-anthropic-prompt.ts` user-block handler (line 126):
// text + file parts become text/image/document blocks, tool-result parts
// route through [appendToolResultBlock] (so the same logic produces a
// `tool_result` block whether the part appears in a `RoleUser` or a
// `RoleTool` provider message), and tool-approval-response parts are skipped
// silently (`if (part.type === 'tool-approval-response') { continue; }`,
// upstream line 319).
func convertUserContent(
	v *cacheControlValidator,
	parts []provider.ContentPart,
	msgOpts provider.ProviderOptions,
	betas *[]anthropic.AnthropicBeta,
	mcpToolUseIDs map[string]bool,
	warnings *[]provider.Warning,
) []anthropic.BetaContentBlockParamUnion {
	var blocks []anthropic.BetaContentBlockParamUnion
	for i, p := range parts {
		isLast := i == len(parts)-1
		switch p.Type {
		case provider.ContentPartTypeText:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
				OfText: &anthropic.BetaTextBlockParam{Text: p.Text, CacheControl: cc},
			})
		case provider.ContentPartTypeFile:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			switch {
			case strings.HasPrefix(p.MediaType, "image/"):
				if b, ok := convertImageFileContentPart(p, cc); ok {
					blocks = append(blocks, b)
				}
			case p.MediaType == "application/pdf":
				if b, ok := convertPDFDocumentContentPart(p, cc); ok {
					*betas = appendBetaUnique(*betas, "pdfs-2024-09-25")
					blocks = append(blocks, b)
				}
			case p.MediaType == "text/plain":
				if b, ok := convertTextDocumentContentPart(p, cc); ok {
					blocks = append(blocks, b)
				}
			}
		case provider.ContentPartTypeToolResult:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			blocks = appendToolResultBlock(blocks, p, cc, mcpToolUseIDs, warnings)
		case provider.ContentPartTypeToolApprovalResponse:
			// Mirrors upstream user-block handler line 319: silently skip.
			// The `RoleTool` path keeps its existing warning behavior in
			// [convertToolContent].
		}
	}
	return blocks
}

func convertImageFileContentPart(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam) (anthropic.BetaContentBlockParamUnion, bool) {
	if p.Data == nil {
		return anthropic.BetaContentBlockParamUnion{}, false
	}
	mediaType := p.MediaType
	if mediaType == "image/*" {
		mediaType = "image/jpeg"
	}
	b64 := p.Data.Base64
	if b64 == "" && len(p.Data.Bytes) > 0 {
		b64 = base64.StdEncoding.EncodeToString(p.Data.Bytes)
	}
	if b64 != "" {
		return anthropic.BetaContentBlockParamUnion{
			OfImage: &anthropic.BetaImageBlockParam{
				Source: anthropic.BetaImageBlockParamSourceUnion{
					OfBase64: &anthropic.BetaBase64ImageSourceParam{
						Data:      b64,
						MediaType: anthropic.BetaBase64ImageSourceMediaType(mediaType),
					},
				},
				CacheControl: cc,
			},
		}, true
	}
	if p.Data.URL != "" {
		return anthropic.BetaContentBlockParamUnion{
			OfImage: &anthropic.BetaImageBlockParam{
				Source: anthropic.BetaImageBlockParamSourceUnion{
					OfURL: &anthropic.BetaURLImageSourceParam{
						URL: p.Data.URL,
					},
				},
				CacheControl: cc,
			},
		}, true
	}
	return anthropic.BetaContentBlockParamUnion{}, false
}

// documentMetadata pulls Anthropic-specific document metadata
// (title/context/citations) out of a file part's ProviderOptions. Mirrors the
// upstream `getDocumentMetadata` and `shouldEnableCitations` helpers in
// convert-to-anthropic-prompt.ts.
type documentMetadata struct {
	title    string
	context  string
	citation bool
}

func extractDocumentMetadata(opts provider.ProviderOptions) documentMetadata {
	raw := extractRawJSON(opts)
	if raw == nil {
		return documentMetadata{}
	}
	var data struct {
		Title     *string `json:"title"`
		Context   *string `json:"context"`
		Citations *struct {
			Enabled bool `json:"enabled"`
		} `json:"citations"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return documentMetadata{}
	}
	m := documentMetadata{}
	if data.Title != nil {
		m.title = *data.Title
	}
	if data.Context != nil {
		m.context = *data.Context
	}
	if data.Citations != nil {
		m.citation = data.Citations.Enabled
	}
	return m
}

// applyDocumentMetadata sets title/context/citations on a document block.
// Title falls back to the file part's filename per upstream
// convert-to-anthropic-prompt.ts:249,277.
func applyDocumentMetadata(doc *anthropic.BetaRequestDocumentBlockParam, p provider.ContentPart) {
	meta := extractDocumentMetadata(p.ProviderOptions)
	title := meta.title
	if title == "" {
		title = p.Filename
	}
	if title != "" {
		doc.Title = anthropic.String(title)
	}
	if meta.context != "" {
		doc.Context = anthropic.String(meta.context)
	}
	if meta.citation {
		doc.Citations = anthropic.BetaCitationsConfigParam{
			Enabled: anthropic.Bool(true),
		}
	}
}

// convertPDFDocumentContentPart maps an application/pdf file part to a
// document block. URL data is sent as the upstream `url` source; binary or
// base64 data uses the base64 source variant. Title/context/citations are
// pulled from the file part's anthropic ProviderOptions.
func convertPDFDocumentContentPart(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam) (anthropic.BetaContentBlockParamUnion, bool) {
	if p.Data == nil {
		return anthropic.BetaContentBlockParamUnion{}, false
	}
	doc := anthropic.BetaRequestDocumentBlockParam{CacheControl: cc}
	switch {
	case p.Data.URL != "":
		doc.Source = anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfURL: &anthropic.BetaURLPDFSourceParam{URL: p.Data.URL},
		}
	case p.Data.Base64 != "":
		doc.Source = anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfBase64: &anthropic.BetaBase64PDFSourceParam{Data: p.Data.Base64},
		}
	case len(p.Data.Bytes) > 0:
		doc.Source = anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfBase64: &anthropic.BetaBase64PDFSourceParam{
				Data: base64.StdEncoding.EncodeToString(p.Data.Bytes),
			},
		}
	default:
		return anthropic.BetaContentBlockParamUnion{}, false
	}
	applyDocumentMetadata(&doc, p)
	return anthropic.BetaContentBlockParamUnion{OfDocument: &doc}, true
}

// convertTextDocumentContentPart maps a text/plain file part to a document
// block. Mirrors upstream convert-to-anthropic-prompt.ts:256-283: URL data
// uses the url source, byte/base64/inline-text data uses the
// `media_type: "text/plain"` plain-text source.
func convertTextDocumentContentPart(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam) (anthropic.BetaContentBlockParamUnion, bool) {
	if p.Data == nil {
		return anthropic.BetaContentBlockParamUnion{}, false
	}
	doc := anthropic.BetaRequestDocumentBlockParam{CacheControl: cc}
	switch {
	case p.Data.URL != "":
		doc.Source = anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfURL: &anthropic.BetaURLPDFSourceParam{URL: p.Data.URL},
		}
	case len(p.Data.Bytes) > 0:
		doc.Source = anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfText: &anthropic.BetaPlainTextSourceParam{Data: string(p.Data.Bytes)},
		}
	case p.Data.Base64 != "":
		decoded, err := base64.StdEncoding.DecodeString(p.Data.Base64)
		if err != nil {
			return anthropic.BetaContentBlockParamUnion{}, false
		}
		doc.Source = anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfText: &anthropic.BetaPlainTextSourceParam{Data: string(decoded)},
		}
	default:
		return anthropic.BetaContentBlockParamUnion{}, false
	}
	applyDocumentMetadata(&doc, p)
	return anthropic.BetaContentBlockParamUnion{OfDocument: &doc}, true
}

func convertAssistantContent(v *cacheControlValidator, mapping toolNameMapping, parts []provider.ContentPart, msgOpts provider.ProviderOptions, mcpToolUseIDs map[string]bool, warnings *[]provider.Warning) []anthropic.BetaContentBlockParamUnion {
	var blocks []anthropic.BetaContentBlockParamUnion
	for i, p := range parts {
		isLast := i == len(parts)-1
		switch p.Type {
		case provider.ContentPartTypeText:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			if isCompaction(p.ProviderOptions) {
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfCompaction: &anthropic.BetaCompactionBlockParam{
						Content:      anthropic.String(p.Text),
						CacheControl: cc,
					},
				})
			} else {
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfText: &anthropic.BetaTextBlockParam{
						Text:         p.Text,
						Citations:    extractCitations(p.ProviderOptions),
						CacheControl: cc,
					},
				})
			}
		case provider.ContentPartTypeReasoning:
			v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, false)
			// Mirror upstream convert-to-anthropic-prompt.ts:512-545:
			// signature -> thinking block, otherwise redactedData ->
			// redacted_thinking block. Bare text reasoning still goes through
			// as a thinking block for backwards compatibility with providers
			// that don't surface a signature.
			sig := extractSignature(p.ProviderOptions)
			redacted := extractRedactedData(p.ProviderOptions)
			switch {
			case sig != "":
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfThinking: &anthropic.BetaThinkingBlockParam{
						Thinking:  p.Text,
						Signature: sig,
					},
				})
			case redacted != "":
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfRedactedThinking: &anthropic.BetaRedactedThinkingBlockParam{
						Data: redacted,
					},
				})
			case p.Text != "":
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfThinking: &anthropic.BetaThinkingBlockParam{
						Thinking: p.Text,
					},
				})
			}
		case provider.ContentPartTypeToolCall:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			if isMCPToolUse(p.ProviderOptions) {
				serverName, ok := extractMCPServerName(p.ProviderOptions)
				if !ok {
					*warnings = append(*warnings, provider.Warning{
						Type:    provider.WarnOther,
						Feature: "mcp-tool-use",
						Message: "mcp tool use server name is required and must be a string",
					})
					break
				}
				mcpToolUseIDs[p.ToolCallID] = true
				var input any
				if len(p.Input) > 0 {
					if err := json.Unmarshal(p.Input, &input); err != nil {
						v.warnings = append(v.warnings, provider.Warning{
							Type:    provider.WarnOther,
							Feature: "mcp-tool-use-input",
							Message: fmt.Sprintf("failed to parse MCP tool use input for %s: %v", p.ToolName, err),
						})
					}
				}
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{
					OfMCPToolUse: &anthropic.BetaMCPToolUseBlockParam{
						ID:           p.ToolCallID,
						Name:         p.ToolName,
						Input:        input,
						ServerName:   serverName,
						CacheControl: cc,
					},
				})
			} else if p.ProviderExecuted {
				block := convertProviderExecutedToolCall(p, mapping, cc, warnings)
				if block != nil {
					blocks = append(blocks, *block)
				}
			} else {
				block := anthropic.BetaToolUseBlockParam{
					ID:           p.ToolCallID,
					Name:         mapping.toProviderToolName(p.ToolName),
					Input:        toAnthropicToolInput(p.Input),
					CacheControl: cc,
				}
				if caller, ok := extractCallerMetadata(p.ProviderOptions); ok {
					block.Caller = caller
				}
				blocks = append(blocks, anthropic.BetaContentBlockParamUnion{OfToolUse: &block})
			}
		case provider.ContentPartTypeToolResult:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			block := convertProviderExecutedToolResult(p, mapping, cc, mcpToolUseIDs, warnings)
			if block != nil {
				blocks = append(blocks, *block)
			}
		case provider.ContentPartTypeToolApprovalRequest:
			// Approval requests are local conversation bookkeeping. Anthropic's
			// prompt format only accepts approval responses for provider-executed tools.
			continue
		case provider.ContentPartTypeCustom:
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "customContent",
				Details: fmt.Sprintf("Anthropic does not support custom content part kind %q", p.Kind),
			})
		case provider.ContentPartTypeReasoningFile:
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnUnsupported,
				Feature: "reasoningFile",
				Details: "Anthropic does not support reasoning file content parts",
			})
		}
	}
	return blocks
}

var serverToolNames = map[string]anthropic.BetaServerToolUseBlockParamName{
	"code_execution":         anthropic.BetaServerToolUseBlockParamNameCodeExecution,
	"web_search":             anthropic.BetaServerToolUseBlockParamNameWebSearch,
	"web_fetch":              anthropic.BetaServerToolUseBlockParamNameWebFetch,
	"tool_search_tool_regex": anthropic.BetaServerToolUseBlockParamNameToolSearchToolRegex,
	"tool_search_tool_bm25":  anthropic.BetaServerToolUseBlockParamNameToolSearchToolBm25,
}

func toAnthropicToolInput(input json.RawMessage) any {
	if len(input) == 0 {
		return json.RawMessage(`{}`)
	}

	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return map[string]any{"rawInvalidInput": string(input)}
	}
	if _, ok := value.(map[string]any); ok {
		return input
	}
	return map[string]any{"rawInvalidInput": value}
}

func convertProviderExecutedToolCall(p provider.ContentPart, mapping toolNameMapping, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	providerToolName := mapping.toProviderToolName(p.ToolName)

	var input any
	if len(p.Input) > 0 {
		if err := json.Unmarshal(p.Input, &input); err != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "providerExecutedToolCall",
				Message: fmt.Sprintf("failed to unmarshal tool call input for %s: %v", p.ToolName, err),
			})
			return nil
		}
	}

	if providerToolName == "code_execution" {
		inputMap, ok := input.(map[string]any)
		if ok {
			if typeVal, hasType := inputMap["type"]; hasType {
				typeStr, _ := typeVal.(string)
				switch typeStr {
				case "bash_code_execution", "text_editor_code_execution":
					block := anthropic.BetaContentBlockParamUnion{
						OfServerToolUse: &anthropic.BetaServerToolUseBlockParam{
							ID:           p.ToolCallID,
							Name:         anthropic.BetaServerToolUseBlockParamName(typeStr),
							Input:        input,
							CacheControl: cc,
						},
					}
					return &block
				case "programmatic-tool-call":
					delete(inputMap, "type")
					block := anthropic.BetaContentBlockParamUnion{
						OfServerToolUse: &anthropic.BetaServerToolUseBlockParam{
							ID:           p.ToolCallID,
							Name:         anthropic.BetaServerToolUseBlockParamNameCodeExecution,
							Input:        inputMap,
							CacheControl: cc,
						},
					}
					return &block
				}
			}
		}
	}

	name, ok := serverToolNames[providerToolName]
	if !ok {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolCall",
			Message: fmt.Sprintf("provider executed tool call for tool %s is not supported", p.ToolName),
		})
		return nil
	}

	block := anthropic.BetaContentBlockParamUnion{
		OfServerToolUse: &anthropic.BetaServerToolUseBlockParam{
			ID:           p.ToolCallID,
			Name:         name,
			Input:        input,
			CacheControl: cc,
		},
	}
	return &block
}

func convertProviderExecutedToolResult(p provider.ContentPart, mapping toolNameMapping, cc anthropic.BetaCacheControlEphemeralParam, mcpToolUseIDs map[string]bool, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	if p.Output == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result for tool %s has no output", p.ToolName),
		})
		return nil
	}
	if mcpToolUseIDs[p.ToolCallID] {
		mcpContent := serializeMCPToolResultContent(p.Output)
		isError := p.Output.Type == provider.ToolOutputErrorJSON || p.Output.Type == provider.ToolOutputErrorText
		block := anthropic.BetaContentBlockParamUnion{
			OfMCPToolResult: &anthropic.BetaRequestMCPToolResultBlockParam{
				ToolUseID:    p.ToolCallID,
				IsError:      anthropic.Bool(isError),
				Content:      mcpContent,
				CacheControl: cc,
			},
		}
		return &block
	}

	providerToolName := mapping.toProviderToolName(p.ToolName)

	switch providerToolName {
	case "web_search":
		return convertInlineWebSearchResult(p, cc, warnings)
	case "web_fetch":
		return convertInlineWebFetchResult(p, cc, warnings)
	case "code_execution":
		return convertInlineCodeExecutionResult(p, cc, warnings)
	case "tool_search_tool_regex", "tool_search_tool_bm25":
		return convertInlineToolSearchResult(p, cc, warnings)
	default:
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result for tool %s is not supported", p.ToolName),
		})
		return nil
	}
}

func convertInlineWebSearchResult(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	outputJSON := extractOutputJSON(p.Output)
	if outputJSON == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result output type %s for web_search is not supported", toolOutputTypeLabel(p.Output)),
		})
		return nil
	}

	var camelResults []webSearchResult
	if err := json.Unmarshal(outputJSON, &camelResults); err != nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("failed to unmarshal web_search result: %v", err),
		})
		return nil
	}

	sdkResults := make([]anthropic.BetaWebSearchResultBlockParam, len(camelResults))
	for i, r := range camelResults {
		sdkResults[i] = anthropic.BetaWebSearchResultBlockParam{
			URL:              r.URL,
			Title:            r.Title,
			EncryptedContent: r.EncryptedContent,
		}
		if r.PageAge != nil {
			sdkResults[i].PageAge = anthropic.String(*r.PageAge)
		}
	}

	block := anthropic.BetaContentBlockParamUnion{
		OfWebSearchToolResult: &anthropic.BetaWebSearchToolResultBlockParam{
			ToolUseID: p.ToolCallID,
			Content: anthropic.BetaWebSearchToolResultBlockParamContentUnion{
				OfResultBlock: sdkResults,
			},
			CacheControl: cc,
		},
	}
	return &block
}

func convertInlineWebFetchResult(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	if p.Output == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: "provider executed tool result output type <nil> for web_fetch is not supported",
		})
		return nil
	}
	if p.Output.Type == provider.ToolOutputErrorJSON || p.Output.Type == provider.ToolOutputErrorText {
		var errorInfo struct {
			ErrorCode string `json:"errorCode"`
		}
		outputJSON := extractOutputJSON(p.Output)
		if outputJSON != nil {
			if err := json.Unmarshal(outputJSON, &errorInfo); err != nil {
				*warnings = append(*warnings, provider.Warning{
					Type:    provider.WarnOther,
					Feature: "providerExecutedToolResult",
					Message: fmt.Sprintf("failed to unmarshal web_fetch error info: %v", err),
				})
			}
		}
		if errorInfo.ErrorCode == "" {
			errorInfo.ErrorCode = "unavailable"
		}
		block := anthropic.BetaContentBlockParamUnion{
			OfWebFetchToolResult: &anthropic.BetaWebFetchToolResultBlockParam{
				ToolUseID: p.ToolCallID,
				Content: anthropic.BetaWebFetchToolResultBlockParamContentUnion{
					OfRequestWebFetchToolResultError: &anthropic.BetaWebFetchToolResultErrorBlockParam{
						ErrorCode: anthropic.BetaWebFetchToolResultErrorCode(errorInfo.ErrorCode),
					},
				},
				CacheControl: cc,
			},
		}
		return &block
	}

	outputJSON := extractOutputJSON(p.Output)
	if outputJSON == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result output type %s for web_fetch is not supported", toolOutputTypeLabel(p.Output)),
		})
		return nil
	}

	var camel webFetchResult
	if err := json.Unmarshal(outputJSON, &camel); err != nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("failed to unmarshal web_fetch result: %v", err),
		})
		return nil
	}

	var source anthropic.BetaRequestDocumentBlockSourceUnionParam
	switch camel.Content.Source.MediaType {
	case "application/pdf":
		source.OfBase64 = &anthropic.BetaBase64PDFSourceParam{
			Data: camel.Content.Source.Data,
		}
	default:
		source.OfText = &anthropic.BetaPlainTextSourceParam{
			Data: camel.Content.Source.Data,
		}
	}

	doc := anthropic.BetaRequestDocumentBlockParam{
		Source: source,
	}
	if camel.Content.Title != nil {
		doc.Title = anthropic.String(*camel.Content.Title)
	}
	if camel.Content.Citations != nil {
		doc.Citations = anthropic.BetaCitationsConfigParam{
			Enabled: anthropic.Bool(camel.Content.Citations.Enabled),
		}
	}

	fetchParam := anthropic.BetaWebFetchBlockParam{
		URL:     camel.URL,
		Content: doc,
	}
	if camel.RetrievedAt != nil {
		fetchParam.RetrievedAt = anthropic.String(*camel.RetrievedAt)
	}

	block := anthropic.BetaContentBlockParamUnion{
		OfWebFetchToolResult: &anthropic.BetaWebFetchToolResultBlockParam{
			ToolUseID: p.ToolCallID,
			Content: anthropic.BetaWebFetchToolResultBlockParamContentUnion{
				OfRequestWebFetchResultBlock: &fetchParam,
			},
			CacheControl: cc,
		},
	}
	return &block
}

func convertInlineCodeExecutionResult(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	if p.Output == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: "provider executed tool result output type <nil> for code_execution is not supported",
		})
		return nil
	}
	if p.Output.Type == provider.ToolOutputErrorJSON || p.Output.Type == provider.ToolOutputErrorText {
		return convertCodeExecutionErrorResult(p, cc, warnings)
	}

	outputJSON := extractOutputJSON(p.Output)
	if outputJSON == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result output type %s for code_execution is not supported", toolOutputTypeLabel(p.Output)),
		})
		return nil
	}

	var typeCheck struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(outputJSON, &typeCheck); err != nil || typeCheck.Type == "" {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result output value is not a valid code execution result for tool %s", p.ToolName),
		})
		return nil
	}

	switch typeCheck.Type {
	case "code_execution_result":
		var result anthropic.BetaCodeExecutionResultBlockParam
		if err := json.Unmarshal(outputJSON, &result); err != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "providerExecutedToolResult",
				Message: fmt.Sprintf("failed to unmarshal code_execution_result: %v", err),
			})
			return nil
		}
		block := anthropic.BetaContentBlockParamUnion{
			OfCodeExecutionToolResult: &anthropic.BetaCodeExecutionToolResultBlockParam{
				ToolUseID: p.ToolCallID,
				Content: anthropic.BetaCodeExecutionToolResultBlockParamContentUnion{
					OfResultBlock: &result,
				},
				CacheControl: cc,
			},
		}
		return &block

	case "encrypted_code_execution_result":
		var result anthropic.BetaEncryptedCodeExecutionResultBlockParam
		if err := json.Unmarshal(outputJSON, &result); err != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "providerExecutedToolResult",
				Message: fmt.Sprintf("failed to unmarshal encrypted_code_execution_result: %v", err),
			})
			return nil
		}
		block := anthropic.BetaContentBlockParamUnion{
			OfCodeExecutionToolResult: &anthropic.BetaCodeExecutionToolResultBlockParam{
				ToolUseID: p.ToolCallID,
				Content: anthropic.BetaCodeExecutionToolResultBlockParamContentUnion{
					OfRequestEncryptedCodeExecutionResultBlock: &result,
				},
				CacheControl: cc,
			},
		}
		return &block

	case "bash_code_execution_result":
		var result anthropic.BetaBashCodeExecutionResultBlockParam
		if err := json.Unmarshal(outputJSON, &result); err != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "providerExecutedToolResult",
				Message: fmt.Sprintf("failed to unmarshal %s: %v", typeCheck.Type, err),
			})
			return nil
		}
		block := anthropic.BetaContentBlockParamUnion{
			OfBashCodeExecutionToolResult: &anthropic.BetaBashCodeExecutionToolResultBlockParam{
				ToolUseID: p.ToolCallID,
				Content: anthropic.BetaBashCodeExecutionToolResultBlockParamContentUnion{
					OfRequestBashCodeExecutionResultBlock: &result,
				},
				CacheControl: cc,
			},
		}
		return &block

	case "bash_code_execution_tool_result_error":
		var result anthropic.BetaBashCodeExecutionToolResultErrorParam
		if err := json.Unmarshal(outputJSON, &result); err != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "providerExecutedToolResult",
				Message: fmt.Sprintf("failed to unmarshal %s: %v", typeCheck.Type, err),
			})
			return nil
		}
		block := anthropic.BetaContentBlockParamUnion{
			OfBashCodeExecutionToolResult: &anthropic.BetaBashCodeExecutionToolResultBlockParam{
				ToolUseID: p.ToolCallID,
				Content: anthropic.BetaBashCodeExecutionToolResultBlockParamContentUnion{
					OfRequestBashCodeExecutionToolResultError: &result,
				},
				CacheControl: cc,
			},
		}
		return &block

	default:
		return convertTextEditorCodeExecutionResult(p, outputJSON, cc, warnings)
	}
}

func convertCodeExecutionErrorResult(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	outputJSON := extractOutputJSON(p.Output)
	var errorInfo struct {
		Type      string `json:"type"`
		ErrorCode string `json:"errorCode"`
	}
	if outputJSON != nil {
		if err := json.Unmarshal(outputJSON, &errorInfo); err != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "providerExecutedToolResult",
				Message: fmt.Sprintf("failed to unmarshal code_execution error info: %v", err),
			})
		}
	}

	if errorInfo.Type == "code_execution_tool_result_error" {
		block := anthropic.BetaContentBlockParamUnion{
			OfCodeExecutionToolResult: &anthropic.BetaCodeExecutionToolResultBlockParam{
				ToolUseID: p.ToolCallID,
				Content: anthropic.BetaCodeExecutionToolResultBlockParamContentUnion{
					OfError: &anthropic.BetaCodeExecutionToolResultErrorParam{
						ErrorCode: anthropic.BetaCodeExecutionToolResultErrorCode(errorInfo.ErrorCode),
					},
				},
				CacheControl: cc,
			},
		}
		return &block
	}

	errorCode := errorInfo.ErrorCode
	if errorCode == "" {
		errorCode = "unknown"
	}
	block := anthropic.BetaContentBlockParamUnion{
		OfBashCodeExecutionToolResult: &anthropic.BetaBashCodeExecutionToolResultBlockParam{
			ToolUseID: p.ToolCallID,
			Content: anthropic.BetaBashCodeExecutionToolResultBlockParamContentUnion{
				OfRequestBashCodeExecutionToolResultError: &anthropic.BetaBashCodeExecutionToolResultErrorParam{
					ErrorCode: anthropic.BetaBashCodeExecutionToolResultErrorParamErrorCode(errorCode),
				},
			},
			CacheControl: cc,
		},
	}
	return &block
}

func convertTextEditorCodeExecutionResult(p provider.ContentPart, outputJSON json.RawMessage, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	var content anthropic.BetaTextEditorCodeExecutionToolResultBlockParamContentUnion
	if err := json.Unmarshal(outputJSON, &content); err != nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("failed to unmarshal text_editor code execution result: %v", err),
		})
		return nil
	}
	block := anthropic.BetaContentBlockParamUnion{
		OfTextEditorCodeExecutionToolResult: &anthropic.BetaTextEditorCodeExecutionToolResultBlockParam{
			ToolUseID:    p.ToolCallID,
			Content:      content,
			CacheControl: cc,
		},
	}
	return &block
}

func convertInlineToolSearchResult(p provider.ContentPart, cc anthropic.BetaCacheControlEphemeralParam, warnings *[]provider.Warning) *anthropic.BetaContentBlockParamUnion {
	outputJSON := extractOutputJSON(p.Output)
	if outputJSON == nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("provider executed tool result output type %s for tool_search is not supported", toolOutputTypeLabel(p.Output)),
		})
		return nil
	}

	var refs []struct {
		ToolName string `json:"toolName"`
	}
	if err := json.Unmarshal(outputJSON, &refs); err != nil {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "providerExecutedToolResult",
			Message: fmt.Sprintf("failed to unmarshal tool_search result: %v", err),
		})
		return nil
	}

	var toolRefs []anthropic.BetaToolReferenceBlockParam
	for _, ref := range refs {
		toolRefs = append(toolRefs, anthropic.BetaToolReferenceBlockParam{
			ToolName: ref.ToolName,
		})
	}

	block := anthropic.BetaContentBlockParamUnion{
		OfToolSearchToolResult: &anthropic.BetaToolSearchToolResultBlockParam{
			ToolUseID: p.ToolCallID,
			Content: anthropic.BetaToolSearchToolResultBlockParamContentUnion{
				OfRequestToolSearchToolSearchResultBlock: &anthropic.BetaToolSearchToolSearchResultBlockParam{
					ToolReferences: toolRefs,
				},
			},
			CacheControl: cc,
		},
	}
	return &block
}

func extractOutputJSON(output *provider.ToolResultOutput) json.RawMessage {
	if output == nil {
		return nil
	}
	switch output.Type {
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		return output.JSON
	default:
		return nil
	}
}

func toolOutputTypeLabel(output *provider.ToolResultOutput) string {
	if output == nil {
		return "<nil>"
	}
	return string(output.Type)
}

func convertToolContent(v *cacheControlValidator, parts []provider.ContentPart, msgOpts provider.ProviderOptions, mcpToolUseIDs map[string]bool, warnings *[]provider.Warning) []anthropic.BetaContentBlockParamUnion {
	var blocks []anthropic.BetaContentBlockParamUnion
	for i, p := range parts {
		isLast := i == len(parts)-1
		switch p.Type {
		case provider.ContentPartTypeToolResult:
			cc := v.resolveCacheControl(p.ProviderOptions, msgOpts, isLast, true)
			blocks = appendToolResultBlock(blocks, p, cc, mcpToolUseIDs, warnings)
		case provider.ContentPartTypeToolApprovalResponse:
			continue
		}
	}
	return blocks
}

// appendToolResultBlock converts a [provider.ContentPart] of type
// [provider.ContentPartTypeToolResult] into the matching Anthropic content
// block and appends it to blocks. Mirrors the user-block tool-result branch in
// upstream `convert-to-anthropic-prompt.ts:743` (case `'tool-result'`).
//
// Whether the block becomes an `OfMCPToolResult` or a plain `OfToolResult`
// depends on whether the tool-call id was emitted by a prior `mcp_tool_use`
// (tracked across the request via mcpToolUseIDs).
//
// Cache-control is resolved by the caller because the cascade rule depends on
// whether the part is the last part of its *source* [provider.Message]
// (mirrors upstream `validator.getCacheControl(message.providerOptions, ...)`
// keyed off the source-message last-part flag, not the merged-block last-part
// flag).
func appendToolResultBlock(
	blocks []anthropic.BetaContentBlockParamUnion,
	p provider.ContentPart,
	cc anthropic.BetaCacheControlEphemeralParam,
	mcpToolUseIDs map[string]bool,
	warnings *[]provider.Warning,
) []anthropic.BetaContentBlockParamUnion {
	if mcpToolUseIDs[p.ToolCallID] {
		mcpContent := serializeMCPToolResultContent(p.Output)
		isError := p.Output != nil && (p.Output.Type == provider.ToolOutputErrorJSON || p.Output.Type == provider.ToolOutputErrorText)
		return append(blocks, anthropic.BetaContentBlockParamUnion{
			OfMCPToolResult: &anthropic.BetaRequestMCPToolResultBlockParam{
				ToolUseID:    p.ToolCallID,
				IsError:      anthropic.Bool(isError),
				Content:      mcpContent,
				CacheControl: cc,
			},
		})
	}
	content := serializeToolOutput(p.Output, warnings)
	result := &anthropic.BetaToolResultBlockParam{
		ToolUseID:    p.ToolCallID,
		Content:      content,
		CacheControl: cc,
	}
	// Mirrors upstream `convert-to-anthropic-prompt.ts:471-474`: set
	// is_error: true when the tool output is an error variant; otherwise
	// leave the field unset (omitzero) to match upstream `undefined`.
	if p.Output != nil && (p.Output.Type == provider.ToolOutputErrorJSON || p.Output.Type == provider.ToolOutputErrorText) {
		result.IsError = anthropic.Bool(true)
	}
	return append(blocks, anthropic.BetaContentBlockParamUnion{
		OfToolResult: result,
	})
}

func serializeToolOutput(output *provider.ToolResultOutput, warnings *[]provider.Warning) []anthropic.BetaToolResultBlockParamContentUnion {
	if output == nil {
		return []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: ""}},
		}
	}
	switch output.Type {
	case provider.ToolOutputText, provider.ToolOutputErrorText:
		return []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: output.Text}},
		}
	case provider.ToolOutputJSON, provider.ToolOutputErrorJSON:
		return []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: string(output.JSON)}},
		}
	case provider.ToolOutputExecutionDenied:
		reason := output.Reason
		if reason == "" {
			reason = "tool execution was denied"
		}
		return []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: reason}},
		}
	case provider.ToolOutputContent:
		var blocks []anthropic.BetaToolResultBlockParamContentUnion
		for _, v := range output.Content {
			switch v.Type {
			case provider.ToolContentText:
				blocks = append(blocks, anthropic.BetaToolResultBlockParamContentUnion{
					OfText: &anthropic.BetaTextBlockParam{Text: v.Text},
				})
			case provider.ToolContentFileData:
				if v.Data != "" && strings.HasPrefix(v.MediaType, "image/") {
					blocks = append(blocks, anthropic.BetaToolResultBlockParamContentUnion{
						OfImage: &anthropic.BetaImageBlockParam{
							Source: anthropic.BetaImageBlockParamSourceUnion{
								OfBase64: &anthropic.BetaBase64ImageSourceParam{
									Data:      v.Data,
									MediaType: anthropic.BetaBase64ImageSourceMediaType(v.MediaType),
								},
							},
						},
					})
				}
			}
		}
		if len(blocks) == 0 {
			return []anthropic.BetaToolResultBlockParamContentUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: ""}},
			}
		}
		return blocks
	default:
		if warnings != nil {
			*warnings = append(*warnings, provider.Warning{
				Type:    provider.WarnOther,
				Feature: "toolResultOutput",
				Message: fmt.Sprintf("tool result output type %s is not supported", output.Type),
			})
		}
		return []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &anthropic.BetaTextBlockParam{Text: ""}},
		}
	}
}

func convertTools(v *cacheControlValidator, tools []provider.Tool, defaultEagerInputStreaming bool) ([]anthropic.BetaToolUnionParam, []provider.Warning, []string) {
	return convertToolsWithStrictTools(v, tools, defaultEagerInputStreaming, true)
}

func convertToolsWithStrictTools(v *cacheControlValidator, tools []provider.Tool, defaultEagerInputStreaming, supportsStrictTools bool) ([]anthropic.BetaToolUnionParam, []provider.Warning, []string) {
	var result []anthropic.BetaToolUnionParam
	var warnings []provider.Warning
	betaSet := map[string]struct{}{}
	for _, t := range tools {
		switch t.Type {
		case provider.ToolTypeProvider:
			param, betas, warning := convertProviderTool(t)
			if warning != nil {
				warnings = append(warnings, *warning)
				continue
			}
			result = append(result, param)
			for _, b := range betas {
				betaSet[b] = struct{}{}
			}
		case provider.ToolTypeFunction:
			cc := v.getCacheControl(t.ProviderOptions, true)

			var schema anthropic.BetaToolInputSchemaParam
			if len(t.InputSchema) > 0 {
				var raw map[string]any
				if err := json.Unmarshal(t.InputSchema, &raw); err == nil {
					schema = rawToolInputSchema(raw)
				}
			}

			tp := &anthropic.BetaToolParam{
				Name:         t.Name,
				Description:  anthropic.String(t.Description),
				InputSchema:  schema,
				CacheControl: cc,
			}
			if t.Strict != nil {
				if supportsStrictTools {
					tp.Strict = anthropic.Bool(*t.Strict)
				} else {
					warnings = append(warnings, provider.Warning{
						Type:    provider.WarnUnsupported,
						Feature: "strict",
						Details: fmt.Sprintf("Tool '%s' has strict: %t, but strict mode is not supported by this provider. The strict property will be ignored.", t.Name, *t.Strict),
					})
				}
			}

			toolOpts, _, err := provider.ResolveOption[AnthropicToolOptions](t.ProviderOptions, "anthropic")
			if err != nil {
				if _, ok := t.ProviderOptions["anthropic"].(AnthropicCacheControl); !ok {
					warnings = append(warnings, provider.Warning{
						Type:    provider.WarnOther,
						Feature: fmt.Sprintf("tool %s provider options", t.Name),
						Message: fmt.Sprintf("Failed to parse Anthropic provider options for tool %s: %v", t.Name, err),
					})
				}
			}
			if toolOpts.DeferLoading != nil {
				tp.DeferLoading = anthropic.Bool(*toolOpts.DeferLoading)
			}
			if len(toolOpts.AllowedCallers) > 0 {
				tp.AllowedCallers = toolOpts.AllowedCallers
				betaSet["advanced-tool-use-2025-11-20"] = struct{}{}
			}
			// Resolve the effective eager_input_streaming value: per-tool
			// explicit value (true or false) wins over the model-level default,
			// otherwise fall back to the streaming-context default. Only emit
			// the SDK field when the resolved value is true, matching upstream
			// `...(eagerInputStreaming ? { eager_input_streaming: true } : {})`
			// (anthropic-prepare-tools.ts:105). An explicit false therefore
			// suppresses the default without sending the field on the wire.
			eagerInputStreaming := defaultEagerInputStreaming
			if toolOpts.EagerInputStreaming != nil {
				eagerInputStreaming = *toolOpts.EagerInputStreaming
			}
			if eagerInputStreaming {
				tp.EagerInputStreaming = anthropic.Bool(true)
			}

			if len(t.InputExamples) > 0 {
				var examples []map[string]any
				for _, ex := range t.InputExamples {
					var m map[string]any
					if json.Unmarshal(ex.Input, &m) == nil {
						examples = append(examples, m)
					}
				}
				if len(examples) > 0 {
					tp.InputExamples = examples
					betaSet["advanced-tool-use-2025-11-20"] = struct{}{}
				}
			}

			result = append(result, anthropic.BetaToolUnionParam{OfTool: tp})
		}
	}

	var betas []string
	for b := range betaSet {
		betas = append(betas, b)
	}
	return result, warnings, betas
}

func convertProviderTool(t provider.Tool) (anthropic.BetaToolUnionParam, []string, *provider.Warning) {
	switch t.ID {
	case "anthropic.web_search_20250305":
		param := &anthropic.BetaWebSearchTool20250305Param{}
		if raw, ok := t.Args["maxUses"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) == nil {
				param.MaxUses = anthropic.Int(v)
			}
		}
		if raw, ok := t.Args["allowedDomains"]; ok {
			var v []string
			if json.Unmarshal(raw, &v) == nil {
				param.AllowedDomains = v
			}
		}
		if raw, ok := t.Args["blockedDomains"]; ok {
			var v []string
			if json.Unmarshal(raw, &v) == nil {
				param.BlockedDomains = v
			}
		}
		if raw, ok := t.Args["userLocation"]; ok {
			var loc anthropic.BetaUserLocationParam
			if json.Unmarshal(raw, &loc) == nil {
				param.UserLocation = loc
			}
		}
		return anthropic.BetaToolUnionParam{OfWebSearchTool20250305: param}, nil, nil

	case "anthropic.tool_search_bm25_20251119":
		return anthropic.BetaToolUnionParam{
			OfToolSearchToolBm25_20251119: &anthropic.BetaToolSearchToolBm25_20251119Param{
				Type: anthropic.BetaToolSearchToolBm25_20251119TypeToolSearchToolBm25_20251119,
			},
		}, nil, nil

	case "anthropic.tool_search_regex_20251119":
		return anthropic.BetaToolUnionParam{
			OfToolSearchToolRegex20251119: &anthropic.BetaToolSearchToolRegex20251119Param{
				Type: anthropic.BetaToolSearchToolRegex20251119TypeToolSearchToolRegex20251119,
			},
		}, nil, nil

	case "anthropic.code_execution_20250522":
		return anthropic.BetaToolUnionParam{
			OfCodeExecutionTool20250522: &anthropic.BetaCodeExecutionTool20250522Param{},
		}, []string{"code-execution-2025-05-22"}, nil

	case "anthropic.code_execution_20250825":
		return anthropic.BetaToolUnionParam{
			OfCodeExecutionTool20250825: &anthropic.BetaCodeExecutionTool20250825Param{},
		}, []string{"code-execution-2025-08-25"}, nil

	case "anthropic.code_execution_20260120":
		return anthropic.BetaToolUnionParam{
			OfCodeExecutionTool20260120: &anthropic.BetaCodeExecutionTool20260120Param{},
		}, nil, nil

	case "anthropic.computer_20241022":
		param := &anthropic.BetaToolComputerUse20241022Param{}
		extractDisplayDimensions(t.Args, &param.DisplayWidthPx, &param.DisplayHeightPx)
		if raw, ok := t.Args["displayNumber"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) == nil {
				param.DisplayNumber = anthropic.Int(v)
			}
		}
		return anthropic.BetaToolUnionParam{OfComputerUseTool20241022: param}, []string{"computer-use-2024-10-22"}, nil

	case "anthropic.computer_20250124":
		param := &anthropic.BetaToolComputerUse20250124Param{}
		extractDisplayDimensions(t.Args, &param.DisplayWidthPx, &param.DisplayHeightPx)
		if raw, ok := t.Args["displayNumber"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) == nil {
				param.DisplayNumber = anthropic.Int(v)
			}
		}
		return anthropic.BetaToolUnionParam{OfComputerUseTool20250124: param}, []string{"computer-use-2025-01-24"}, nil

	case "anthropic.computer_20251124":
		param := &anthropic.BetaToolComputerUse20251124Param{}
		extractDisplayDimensions(t.Args, &param.DisplayWidthPx, &param.DisplayHeightPx)
		if raw, ok := t.Args["displayNumber"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) == nil {
				param.DisplayNumber = anthropic.Int(v)
			}
		}
		if raw, ok := t.Args["enableZoom"]; ok {
			var v bool
			if json.Unmarshal(raw, &v) == nil {
				param.EnableZoom = anthropic.Bool(v)
			}
		}
		return anthropic.BetaToolUnionParam{OfComputerUseTool20251124: param}, []string{"computer-use-2025-11-24"}, nil

	case "anthropic.text_editor_20241022":
		return anthropic.BetaToolUnionParam{
			OfTextEditor20241022: &anthropic.BetaToolTextEditor20241022Param{},
		}, []string{"computer-use-2024-10-22"}, nil

	case "anthropic.text_editor_20250124":
		return anthropic.BetaToolUnionParam{
			OfTextEditor20250124: &anthropic.BetaToolTextEditor20250124Param{},
		}, []string{"computer-use-2025-01-24"}, nil

	case "anthropic.text_editor_20250429":
		return anthropic.BetaToolUnionParam{
			OfTextEditor20250429: &anthropic.BetaToolTextEditor20250429Param{},
		}, []string{"computer-use-2025-01-24"}, nil

	case "anthropic.text_editor_20250728":
		param := &anthropic.BetaToolTextEditor20250728Param{}
		if raw, ok := t.Args["maxCharacters"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) == nil {
				param.MaxCharacters = anthropic.Int(v)
			}
		}
		return anthropic.BetaToolUnionParam{OfTextEditor20250728: param}, nil, nil

	case "anthropic.bash_20241022":
		return anthropic.BetaToolUnionParam{
			OfBashTool20241022: &anthropic.BetaToolBash20241022Param{},
		}, []string{"computer-use-2024-10-22"}, nil

	case "anthropic.bash_20250124":
		return anthropic.BetaToolUnionParam{
			OfBashTool20250124: &anthropic.BetaToolBash20250124Param{},
		}, []string{"computer-use-2025-01-24"}, nil

	case "anthropic.memory_20250818":
		return anthropic.BetaToolUnionParam{
			OfMemoryTool20250818: &anthropic.BetaMemoryTool20250818Param{},
		}, []string{"context-management-2025-06-27"}, nil

	case "anthropic.web_fetch_20250910":
		a := extractWebFetchArgs(t.Args)
		param := &anthropic.BetaWebFetchTool20250910Param{
			AllowedDomains: a.AllowedDomains,
			BlockedDomains: a.BlockedDomains,
			Citations:      a.Citations,
		}
		if a.HasMaxUses {
			param.MaxUses = anthropic.Opt(a.MaxUses)
		}
		if a.HasMaxContent {
			param.MaxContentTokens = anthropic.Opt(a.MaxContentTokens)
		}
		return anthropic.BetaToolUnionParam{OfWebFetchTool20250910: param}, []string{"web-fetch-2025-09-10"}, nil

	case "anthropic.web_fetch_20260209":
		a := extractWebFetchArgs(t.Args)
		param := &anthropic.BetaWebFetchTool20260209Param{
			AllowedDomains: a.AllowedDomains,
			BlockedDomains: a.BlockedDomains,
			Citations:      a.Citations,
		}
		if a.HasMaxUses {
			param.MaxUses = anthropic.Opt(a.MaxUses)
		}
		if a.HasMaxContent {
			param.MaxContentTokens = anthropic.Opt(a.MaxContentTokens)
		}
		return anthropic.BetaToolUnionParam{OfWebFetchTool20260209: param}, []string{"code-execution-web-tools-2026-02-09"}, nil

	case "anthropic.web_search_20260209":
		param := &anthropic.BetaWebSearchTool20260209Param{}
		if raw, ok := t.Args["maxUses"]; ok {
			var v int64
			if json.Unmarshal(raw, &v) == nil {
				param.MaxUses = anthropic.Opt(v)
			}
		}
		if raw, ok := t.Args["allowedDomains"]; ok {
			var v []string
			if json.Unmarshal(raw, &v) == nil {
				param.AllowedDomains = v
			}
		}
		if raw, ok := t.Args["blockedDomains"]; ok {
			var v []string
			if json.Unmarshal(raw, &v) == nil {
				param.BlockedDomains = v
			}
		}
		if raw, ok := t.Args["userLocation"]; ok {
			var loc anthropic.BetaUserLocationParam
			if json.Unmarshal(raw, &loc) == nil {
				param.UserLocation = loc
			}
		}
		return anthropic.BetaToolUnionParam{OfWebSearchTool20260209: param}, []string{"code-execution-web-tools-2026-02-09"}, nil

	default:
		return anthropic.BetaToolUnionParam{}, nil, &provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: fmt.Sprintf("provider tool %s", t.ID),
			Message: fmt.Sprintf("Anthropic provider does not support provider tool: %s", t.ID),
		}
	}
}

func extractDisplayDimensions(args map[string]json.RawMessage, width, height *int64) {
	if raw, ok := args["displayWidthPx"]; ok {
		var v int64
		if json.Unmarshal(raw, &v) == nil {
			*width = v
		}
	}
	if raw, ok := args["displayHeightPx"]; ok {
		var v int64
		if json.Unmarshal(raw, &v) == nil {
			*height = v
		}
	}
}

type webFetchArgs struct {
	MaxUses          int64
	AllowedDomains   []string
	BlockedDomains   []string
	Citations        anthropic.BetaCitationsConfigParam
	MaxContentTokens int64
	HasMaxUses       bool
	HasMaxContent    bool
}

func extractWebFetchArgs(args map[string]json.RawMessage) webFetchArgs {
	var a webFetchArgs
	if raw, ok := args["maxUses"]; ok {
		if json.Unmarshal(raw, &a.MaxUses) == nil {
			a.HasMaxUses = true
		}
	}
	if raw, ok := args["allowedDomains"]; ok {
		json.Unmarshal(raw, &a.AllowedDomains) //nolint:errcheck
	}
	if raw, ok := args["blockedDomains"]; ok {
		json.Unmarshal(raw, &a.BlockedDomains) //nolint:errcheck
	}
	if raw, ok := args["citations"]; ok {
		json.Unmarshal(raw, &a.Citations) //nolint:errcheck
	}
	if raw, ok := args["maxContentTokens"]; ok {
		if json.Unmarshal(raw, &a.MaxContentTokens) == nil {
			a.HasMaxContent = true
		}
	}
	return a
}

func convertToolChoice(tc provider.ToolChoice, mapping toolNameMapping) anthropic.BetaToolChoiceUnionParam {
	switch tc.Type {
	case provider.ToolChoiceAuto:
		return anthropic.BetaToolChoiceUnionParam{OfAuto: &anthropic.BetaToolChoiceAutoParam{}}
	case provider.ToolChoiceNone:
		return anthropic.BetaToolChoiceUnionParam{OfNone: &anthropic.BetaToolChoiceNoneParam{}}
	case provider.ToolChoiceRequired:
		return anthropic.BetaToolChoiceUnionParam{OfAny: &anthropic.BetaToolChoiceAnyParam{}}
	case provider.ToolChoiceTool:
		return anthropic.BetaToolChoiceUnionParam{OfTool: &anthropic.BetaToolChoiceToolParam{Name: mapping.toProviderToolName(tc.ToolName)}}
	default:
		return anthropic.BetaToolChoiceUnionParam{OfAuto: &anthropic.BetaToolChoiceAutoParam{}}
	}
}

func applyDisableParallelToolUse(p *anthropic.BetaMessageNewParams, value *bool, hasTools bool) {
	if value == nil {
		return
	}
	if p.ToolChoice.OfAuto != nil {
		p.ToolChoice.OfAuto.DisableParallelToolUse = anthropic.Bool(*value)
		return
	}
	if p.ToolChoice.OfAny != nil {
		p.ToolChoice.OfAny.DisableParallelToolUse = anthropic.Bool(*value)
		return
	}
	if p.ToolChoice.OfTool != nil {
		p.ToolChoice.OfTool.DisableParallelToolUse = anthropic.Bool(*value)
		return
	}
	if hasTools && *value {
		p.ToolChoice = anthropic.BetaToolChoiceUnionParam{OfAuto: &anthropic.BetaToolChoiceAutoParam{DisableParallelToolUse: anthropic.Bool(true)}}
	}
}

func applyFallbacks(p *anthropic.BetaMessageNewParams, fallbacks *FallbackConfig, caps providerCapabilities, br *buildResult, warnings *[]provider.Warning) {
	if fallbacks == nil || (!fallbacks.Default && len(fallbacks.Chain) == 0) {
		return
	}
	if !caps.supportsDirectBetaFeatures {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnUnsupported,
			Feature: "providerOptions.anthropic.fallbacks",
			Details: "server-side fallbacks are not supported by the Anthropic Vertex provider and were ignored",
		})
		return
	}
	if fallbacks.Default {
		br.requestOptions = append(br.requestOptions, option.WithJSONSet("fallbacks", "default"))
		p.Betas = appendBetaUnique(p.Betas, serverSideFallbackDefaultBeta)
		return
	}

	p.Fallbacks = make([]anthropic.BetaFallbackParam, len(fallbacks.Chain))
	for i, fallback := range fallbacks.Chain {
		converted := anthropic.BetaFallbackParam{
			Model: anthropic.Model(fallback.Model),
			Speed: anthropic.BetaFallbackParamSpeed(fallback.Speed),
		}
		if fallback.MaxTokens != nil {
			converted.MaxTokens = anthropic.Int(int64(*fallback.MaxTokens))
		}
		if len(fallback.Thinking) > 0 {
			converted.Thinking = param.Override[anthropic.BetaFallbackParamThinkingUnion](fallback.Thinking)
		}
		if len(fallback.OutputConfig) > 0 {
			converted.OutputConfig = param.Override[anthropic.BetaOutputConfigParam](fallback.OutputConfig)
		}
		p.Fallbacks[i] = converted
	}
	p.Betas = appendBetaUnique(p.Betas, serverSideFallbackExplicitBeta)
}

func applyProviderOptions(p *anthropic.BetaMessageNewParams, ao AnthropicOptions, ok bool, warnings *[]provider.Warning) {
	if !ok {
		return
	}

	if ao.Thinking != nil {
		switch ao.Thinking.Type {
		case ThinkingEnabled:
			p.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfEnabled: &anthropic.BetaThinkingConfigEnabledParam{
					BudgetTokens: int64(ao.Thinking.BudgetTokens),
				},
			}
		case ThinkingDisabled:
			p.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfDisabled: &anthropic.BetaThinkingConfigDisabledParam{},
			}
		case ThinkingAdaptive:
			adaptive := &anthropic.BetaThinkingConfigAdaptiveParam{}
			if ao.Thinking.Display != "" {
				adaptive.Display = anthropic.BetaThinkingConfigAdaptiveDisplay(ao.Thinking.Display)
			}
			p.Thinking = anthropic.BetaThinkingConfigParamUnion{
				OfAdaptive: adaptive,
			}
		}
	}

	if ao.Effort != "" {
		p.OutputConfig.Effort = anthropic.BetaOutputConfigEffort(ao.Effort)
	}

	if ao.TaskBudget != nil {
		if validateTaskBudget(*ao.TaskBudget, warnings) {
			// The Anthropic API currently only supports the "tokens" budget
			// type. The SDK marshals Type as the literal "tokens" automatically.
			tb := anthropic.BetaTokenTaskBudgetParam{
				Total: ao.TaskBudget.Total,
			}
			if ao.TaskBudget.Remaining != nil {
				tb.Remaining = anthropic.Int(*ao.TaskBudget.Remaining)
			}
			p.OutputConfig.TaskBudget = tb
			p.Betas = appendBetaUnique(p.Betas, "task-budgets-2026-03-13")
		}
	}

	if len(ao.MCPServers) > 0 {
		servers := make([]anthropic.BetaRequestMCPServerURLDefinitionParam, len(ao.MCPServers))
		for i, s := range ao.MCPServers {
			srv := anthropic.BetaRequestMCPServerURLDefinitionParam{
				Name: s.Name,
				URL:  s.URL,
			}
			if s.AuthorizationToken != "" {
				srv.AuthorizationToken = anthropic.String(s.AuthorizationToken)
			}
			if s.ToolConfiguration != nil {
				srv.ToolConfiguration = anthropic.BetaRequestMCPServerToolConfigurationParam{
					Enabled:      anthropic.Bool(s.ToolConfiguration.Enabled),
					AllowedTools: s.ToolConfiguration.AllowedTools,
				}
			}
			servers[i] = srv
		}
		p.MCPServers = servers
		p.Betas = appendBetaUnique(p.Betas, "mcp-client-2025-04-04")
	}

	if ao.Container != nil {
		container := anthropic.BetaContainerParams{}
		if ao.Container.ID != "" {
			container.ID = anthropic.String(ao.Container.ID)
		}
		for _, skill := range ao.Container.Skills {
			sp := anthropic.BetaSkillParams{
				SkillID: skill.SkillID,
				Type:    anthropic.BetaSkillParamsType(skill.Type),
			}
			if skill.Version != "" {
				sp.Version = anthropic.String(skill.Version)
			}
			container.Skills = append(container.Skills, sp)
		}
		p.Container = anthropic.BetaMessageNewParamsContainerUnion{OfContainers: &container}
		if len(container.Skills) > 0 {
			p.Betas = appendBetaUnique(p.Betas, "skills-2025-10-02")
			p.Betas = appendBetaUnique(p.Betas, "files-api-2025-04-14")
		}
	}

	for _, beta := range ao.Betas {
		p.Betas = appendBetaUnique(p.Betas, anthropic.AnthropicBeta(beta))
	}

}

// taskBudgetMinTotal mirrors upstream's `z.number().int().min(20000)`
// validator on `taskBudget.total`.
const taskBudgetMinTotal int64 = 20000

// validateTaskBudget checks that a caller-supplied TaskBudgetConfig matches
// the upstream Zod schema constraints. Invalid budgets are skipped with an
// "other" warning instead of being silently corrected, mirroring upstream's
// fail-loud behavior while keeping the Go callsite ergonomic. Returns true
// when the budget is valid and should be applied.
func validateTaskBudget(tb TaskBudgetConfig, warnings *[]provider.Warning) bool {
	// Type is optional in Go (zero value = "tokens"); but if set, only
	// "tokens" is accepted (upstream uses z.literal('tokens')).
	if tb.Type != "" && tb.Type != TaskBudgetTokens {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "taskBudget",
			Message: fmt.Sprintf("taskBudget.type %q is not supported; only %q is accepted. task budget will be ignored.", tb.Type, TaskBudgetTokens),
		})
		return false
	}
	if tb.Total < taskBudgetMinTotal {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "taskBudget",
			Message: fmt.Sprintf("taskBudget.total must be at least %d (got %d). task budget will be ignored.", taskBudgetMinTotal, tb.Total),
		})
		return false
	}
	if tb.Remaining != nil && *tb.Remaining < 0 {
		*warnings = append(*warnings, provider.Warning{
			Type:    provider.WarnOther,
			Feature: "taskBudget",
			Message: fmt.Sprintf("taskBudget.remaining must be >= 0 (got %d). task budget will be ignored.", *tb.Remaining),
		})
		return false
	}
	return true
}

func extractRawJSON(opts provider.ProviderOptions) json.RawMessage {
	if opts == nil {
		return nil
	}
	opt, ok := opts["anthropic"]
	if !ok {
		return nil
	}
	if raw, ok := opt.(provider.RawProviderOption); ok {
		return raw.Raw
	}
	return nil
}

func extractSignature(opts provider.ProviderOptions) string {
	raw := extractRawJSON(opts)
	if raw == nil {
		return ""
	}
	var data struct {
		Signature string `json:"signature"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	return data.Signature
}

// extractRedactedData reads the anthropic-namespaced `redactedData` value off
// a reasoning ContentPart's ProviderOptions; used to round-trip Anthropic's
// `redacted_thinking` blocks across multi-turn requests.
func extractRedactedData(opts provider.ProviderOptions) string {
	raw := extractRawJSON(opts)
	if raw == nil {
		return ""
	}
	var data struct {
		RedactedData string `json:"redactedData"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	return data.RedactedData
}

func extractAnthropicType(opts provider.ProviderOptions) string {
	raw := extractRawJSON(opts)
	if raw == nil {
		return ""
	}
	var data struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	return data.Type
}

func isCompaction(opts provider.ProviderOptions) bool {
	return extractAnthropicType(opts) == "compaction"
}

func isMCPToolUse(opts provider.ProviderOptions) bool {
	return extractAnthropicType(opts) == "mcp-tool-use"
}

func extractMCPServerName(opts provider.ProviderOptions) (string, bool) {
	raw := extractRawJSON(opts)
	if raw == nil {
		return "", false
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil {
		return "", false
	}
	serverNameRaw, ok := data["serverName"]
	if !ok {
		return "", false
	}
	var serverName string
	if json.Unmarshal(serverNameRaw, &serverName) != nil || serverName == "" {
		return "", false
	}
	return serverName, true
}

func extractCallerMetadata(opts provider.ProviderOptions) (anthropic.BetaToolUseBlockParamCallerUnion, bool) {
	raw := extractRawJSON(opts)
	if raw == nil {
		return anthropic.BetaToolUseBlockParamCallerUnion{}, false
	}
	var data struct {
		Caller *struct {
			Type   string `json:"type"`
			ToolID string `json:"toolId"`
		} `json:"caller"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Caller == nil {
		return anthropic.BetaToolUseBlockParamCallerUnion{}, false
	}
	switch data.Caller.Type {
	case "direct":
		return anthropic.BetaToolUseBlockParamCallerUnion{OfDirect: &anthropic.BetaDirectCallerParam{}}, true
	case "code_execution_20250825":
		if data.Caller.ToolID == "" {
			return anthropic.BetaToolUseBlockParamCallerUnion{}, false
		}
		return anthropic.BetaToolUseBlockParamCallerUnion{OfCodeExecution20250825: &anthropic.BetaServerToolCallerParam{ToolID: data.Caller.ToolID}}, true
	case "code_execution_20260120":
		if data.Caller.ToolID == "" {
			return anthropic.BetaToolUseBlockParamCallerUnion{}, false
		}
		return anthropic.BetaToolUseBlockParamCallerUnion{OfCodeExecution20260120: &anthropic.BetaServerToolCaller20260120Param{ToolID: data.Caller.ToolID}}, true
	default:
		return anthropic.BetaToolUseBlockParamCallerUnion{}, false
	}
}

func serializeMCPToolResultContent(output *provider.ToolResultOutput) anthropic.BetaRequestMCPToolResultBlockParamContentUnion {
	if output == nil {
		return anthropic.BetaRequestMCPToolResultBlockParamContentUnion{
			OfString: anthropic.String(""),
		}
	}
	raw := output.JSON
	if output.Type == provider.ToolOutputText || output.Type == provider.ToolOutputErrorText {
		return anthropic.BetaRequestMCPToolResultBlockParamContentUnion{
			OfString: anthropic.String(output.Text),
		}
	}
	if len(raw) == 0 {
		return anthropic.BetaRequestMCPToolResultBlockParamContentUnion{
			OfString: anthropic.String(""),
		}
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return anthropic.BetaRequestMCPToolResultBlockParamContentUnion{
			OfString: anthropic.String(s),
		}
	}
	var blocks []anthropic.BetaTextBlockParam
	if json.Unmarshal(raw, &blocks) == nil && len(blocks) > 0 {
		return anthropic.BetaRequestMCPToolResultBlockParamContentUnion{
			OfBetaMCPToolResultBlockContent: blocks,
		}
	}
	return anthropic.BetaRequestMCPToolResultBlockParamContentUnion{
		OfString: anthropic.String(string(raw)),
	}
}

func hasFunctionTools(tools []provider.Tool) bool {
	for _, t := range tools {
		if t.Type == provider.ToolTypeFunction {
			return true
		}
	}
	return false
}

// hasWebTool20260209WithoutCodeExecution reports whether the request
// configures a web_fetch_20260209 or web_search_20260209 provider tool
// without also configuring any code_execution provider tool. Mirrors
// upstream anthropic-language-model.ts:2639. When true, callers must mark
// implicit `code_execution` server_tool_use blocks as `dynamic: true` so the
// strict tool-validation layer accepts them — the 20260209 web tools
// internally trigger `code_execution` server-side, and would otherwise be
// rejected because the caller never declared a `code_execution` tool.
func hasWebTool20260209WithoutCodeExecution(tools []provider.Tool) bool {
	var hasWebTool20260209, hasCodeExecutionTool bool
	for _, t := range tools {
		if t.Type != provider.ToolTypeProvider {
			continue
		}
		switch t.ID {
		case "anthropic.web_fetch_20260209", "anthropic.web_search_20260209":
			hasWebTool20260209 = true
		case "anthropic.code_execution_20250522",
			"anthropic.code_execution_20250825",
			"anthropic.code_execution_20260120":
			hasCodeExecutionTool = true
		}
	}
	return hasWebTool20260209 && !hasCodeExecutionTool
}

// resolveAnthropicToolStreaming reads AnthropicOptions.ToolStreaming and
// returns its effective boolean value. Missing AnthropicOptions, malformed
// options, or a nil ToolStreaming pointer all resolve to true (matching
// upstream's `?? true` semantics).
func resolveAnthropicToolStreaming(opts provider.ProviderOptions) bool {
	ao, ok, err := provider.ResolveOption[AnthropicOptions](opts, "anthropic")
	if err != nil || !ok || ao.ToolStreaming == nil {
		return true
	}
	return *ao.ToolStreaming
}

func appendBetaUnique(betas []anthropic.AnthropicBeta, beta anthropic.AnthropicBeta) []anthropic.AnthropicBeta {
	for _, b := range betas {
		if b == beta {
			return betas
		}
	}
	return append(betas, beta)
}
