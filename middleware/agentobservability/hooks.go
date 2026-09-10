package agentobservability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/grafana/agento11y/go/agento11y"
	"github.com/grafana/ai-sdk/middleware"
	"github.com/grafana/ai-sdk/provider"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// HooksMiddleware returns a middleware that evaluates Agent Observability
// preflight hooks before delegating to the inner model. Flow per call:
//
//  1. If opts.Enabled is non-nil and returns false for the request context,
//     the inner model is invoked unchanged with no Agent Observability contact.
//  2. opts.ClientResolver(ctx) is invoked. A nil client makes the middleware
//     a no-op for that request.
//  3. An agento11y.HookEvaluateRequest is constructed from params (phase =
//     preflight) and submitted to the resolved client.
//  4. opts.MaxLatency, when greater than zero, bounds the EvaluateHook call
//     via context.WithTimeout on a DERIVED context — the deadline does NOT
//     propagate to the inner model call.
//  5. On deny: a *HookDenialError is returned without invoking the inner
//     model.
//  6. On allow (no TransformedInput): the inner model is invoked unchanged.
//  7. On allow + TransformedInput: params.Prompt is rebuilt via
//     applyTransformedInput. A transform that cannot be applied without
//     losing request content fails closed; the inner model is otherwise
//     invoked with the new params.
func HooksMiddleware(opts HooksOptions) middleware.Middleware {
	return middleware.Middleware{
		WrapGenerate: func(ctx context.Context, p middleware.WrapGenerateParams) (*provider.GenerateResult, error) {
			newParams, err := evaluateHook(ctx, opts, p.Model, p.Params)
			if err != nil {
				return nil, err
			}
			if newParams != nil {
				return p.Model.DoGenerate(ctx, *newParams)
			}
			return p.DoGenerate(ctx)
		},
		WrapStream: func(ctx context.Context, p middleware.WrapStreamParams) (*provider.StreamResult, error) {
			newParams, err := evaluateHook(ctx, opts, p.Model, p.Params)
			if err != nil {
				return nil, err
			}
			if newParams != nil {
				return p.Model.DoStream(ctx, *newParams)
			}
			return p.DoStream(ctx)
		},
	}
}

// evaluateHook performs the preflight evaluation. It returns:
//   - (nil, nil) when the inner model should be invoked with the original
//     params (passthrough / allow without transform).
//   - (*CallOptions, nil) when the inner model should be invoked with a
//     transformed prompt.
//   - (nil, error) when the request was denied or the hook call failed.
func evaluateHook(ctx context.Context, opts HooksOptions, model provider.LanguageModel, params provider.CallOptions) (*provider.CallOptions, error) {
	tracer := otel.Tracer(tracerName)

	if opts.Enabled != nil && !opts.Enabled(ctx) {
		return nil, nil
	}
	client := resolveClient(ctx, opts.ClientResolver)
	if client == nil {
		return nil, nil
	}

	ctxInfo := resolveContextInfo(ctx, opts.ContextProvider)

	ctx, span := tracer.Start(ctx, SpanNameHooksPreflight, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()
	if model != nil {
		span.SetAttributes(
			attribute.String("gen_ai.provider.name", model.Provider()),
			attribute.String("gen_ai.request.model", model.ModelID()),
		)
	}

	req := buildHookEvaluateRequest(model, params, ctxInfo)

	hookCtx := ctx
	if opts.MaxLatency > 0 {
		var cancel context.CancelFunc
		hookCtx, cancel = context.WithTimeout(ctx, opts.MaxLatency)
		defer cancel()
	}

	resp, err := client.EvaluateHook(hookCtx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	if resp == nil {
		return nil, nil
	}

	span.SetAttributes(attribute.String(SpanAttrHooksAction, string(resp.Action)))

	if resp.Action == agento11y.HookActionDeny {
		denialErr := &HookDenialError{
			Reason: resp.Reason,
			RuleID: resp.RuleID,
		}
		span.SetAttributes(attribute.String(SpanAttrHooksResult, "deny"))
		if resp.RuleID != "" {
			span.SetAttributes(attribute.String(SpanAttrHooksRuleID, resp.RuleID))
		}
		span.SetStatus(codes.Error, denialErr.Error())
		return nil, denialErr
	}

	if resp.TransformedInput != nil {
		newPrompt, err := applyTransformedInputWithTools(params.Prompt, params.Tools, *resp.TransformedInput)
		if err == nil {
			var tools []provider.Tool
			tools, err = applyTransformedTools(params.Tools, resp.TransformedInput.Tools)
			if err == nil {
				err = validateTransformedToolChoice(params.ToolChoice, tools)
			}
			if err == nil {
				span.SetAttributes(attribute.String(SpanAttrHooksResult, "transform"))
				newParams := params
				newParams.Prompt = newPrompt
				newParams.Tools = tools
				return &newParams, nil
			}
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(attribute.String(SpanAttrHooksResult, "allow"))
	return nil, nil
}

// buildHookEvaluateRequest assembles the preflight hook evaluation payload.
// Media is excluded because hooks predate Agent Observability media recording,
// and sending inline data or signed URLs would widen the preflight disclosure boundary.
func buildHookEvaluateRequest(model provider.LanguageModel, params provider.CallOptions, ctxInfo ContextInfo) agento11y.HookEvaluateRequest {
	system, msgs := messagesToAgento11yWithMediaAndTools(params.Prompt, false, params.Tools)
	tools := toolsToAgento11y(params.Tools)

	hookCtx := agento11y.HookContext{
		AgentName:    ctxInfo.AgentName,
		AgentVersion: ctxInfo.AgentVersion,
		Tags:         cloneStringMap(ctxInfo.Tags),
	}
	if model != nil {
		hookCtx.Model = &agento11y.HookModel{
			Provider: model.Provider(),
			Name:     model.ModelID(),
		}
	}

	return agento11y.HookEvaluateRequest{
		Phase:   agento11y.HookPhasePreflight,
		Context: hookCtx,
		Input: agento11y.HookInput{
			Messages:            msgs,
			SystemPrompt:        system,
			Tools:               tools,
			ConversationPreview: flattenMessagesForPreview(msgs),
		},
	}
}

// flattenMessagesForPreview produces a plain-text preview of the prompt for
// hook evaluators that don't yet support the structured Messages field.
// Matches the shape the legacy claude path emits today.
func flattenMessagesForPreview(messages []agento11y.Message) string {
	if len(messages) == 0 {
		return ""
	}
	parts := make([]string, 0, len(messages))
	for _, m := range messages {
		for _, p := range m.Parts {
			text := strings.TrimSpace(p.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// applyTransformedInput rebuilds a complete replacement prompt from a hook
// response. It preserves reasoning signatures only on unchanged reasoning
// parts and rejects transformations that cannot be represented faithfully.
func applyTransformedInput(originalPrompt []provider.Message, transformed agento11y.HookInput) ([]provider.Message, error) {
	return applyTransformedInputWithTools(originalPrompt, nil, transformed)
}

func applyTransformedInputWithTools(originalPrompt []provider.Message, tools []provider.Tool, transformed agento11y.HookInput) ([]provider.Message, error) {
	if err := validateTransformablePrompt(originalPrompt); err != nil {
		return nil, err
	}
	if len(transformed.Messages) == 0 && transformed.SystemPrompt == "" {
		return nil, fmt.Errorf("%w: transformed input is empty", ErrHookTransformFailed)
	}

	out := make([]provider.Message, 0, len(transformed.Messages)+1)
	if transformed.SystemPrompt != "" {
		out = append(out, provider.NewSystemMessage(transformed.SystemPrompt))
	}

	matcher := newTransformedPartMatcher(originalPrompt, tools)
	for i, msg := range transformed.Messages {
		role, ok := agento11yRoleToProvider(msg.Role)
		if !ok {
			return nil, fmt.Errorf("%w: message %d has unsupported role %q", ErrHookTransformFailed, i, msg.Role)
		}
		if err := validateTransformedMessage(msg); err != nil {
			return nil, fmt.Errorf("%w: message %d: %v", ErrHookTransformFailed, i, err)
		}

		parts, err := rebuildPartsFromAgento11y(msg.Parts, matcher)
		if err != nil {
			return nil, fmt.Errorf("%w: message %d: %v", ErrHookTransformFailed, i, err)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("%w: message %d has no supported parts", ErrHookTransformFailed, i)
		}
		out = append(out, provider.Message{Role: role, Content: parts})
	}
	return out, nil
}

func validateTransformablePrompt(prompt []provider.Message) error {
	for i, msg := range prompt {
		if len(msg.ProviderOptions) > 0 {
			return fmt.Errorf("%w: original message %d has undisclosed provider options", ErrHookTransformFailed, i)
		}
		switch msg.Role {
		case provider.RoleSystem:
			for _, part := range msg.Content {
				if len(part.ProviderOptions) > 0 {
					return fmt.Errorf("%w: original system message %d has undisclosed part provider options", ErrHookTransformFailed, i)
				}
				if part.Type != provider.ContentPartTypeText {
					return fmt.Errorf("%w: original system message %d contains undisclosed %q content", ErrHookTransformFailed, i, part.Type)
				}
			}
			continue
		case provider.RoleUser, provider.RoleAssistant, provider.RoleTool:
		default:
			return fmt.Errorf("%w: original message %d has unsupported role %q", ErrHookTransformFailed, i, msg.Role)
		}
		for _, part := range msg.Content {
			switch part.Type {
			case provider.ContentPartTypeText:
				if len(part.ProviderOptions) > 0 {
					return fmt.Errorf("%w: original message %d has undisclosed text provider options", ErrHookTransformFailed, i)
				}
			case provider.ContentPartTypeReasoning:
				if msg.Role != provider.RoleAssistant {
					return fmt.Errorf("%w: original message %d has reasoning under role %q", ErrHookTransformFailed, i, msg.Role)
				}
				if len(part.ProviderOptions) > 0 && !hasSupportedReasoningSignature(part.ProviderOptions) {
					return fmt.Errorf("%w: original message %d has unsupported reasoning metadata", ErrHookTransformFailed, i)
				}
				if part.Text == "" && len(part.ProviderOptions) > 0 {
					return fmt.Errorf("%w: original message %d has undisclosed empty reasoning metadata", ErrHookTransformFailed, i)
				}
			case provider.ContentPartTypeToolCall, provider.ContentPartTypeToolResult:
			default:
				return fmt.Errorf("%w: original message %d contains undisclosed %q content", ErrHookTransformFailed, i, part.Type)
			}
		}
	}
	return nil
}

func hasSupportedReasoningSignature(options provider.ProviderOptions) bool {
	if len(options) != 1 {
		return false
	}
	option, ok := options["anthropic"].(provider.RawProviderOption)
	if !ok || option.Key != "anthropic" {
		return false
	}
	value, ok := decodeJSONValue(option.Raw)
	if !ok {
		return false
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 1 {
		return false
	}
	signature, ok := object["signature"].(string)
	return ok && signature != ""
}

func applyTransformedTools(original []provider.Tool, transformed []agento11y.ToolDefinition) ([]provider.Tool, error) {
	if len(transformed) == 0 {
		return nil, nil
	}
	used := make([]bool, len(original))
	out := make([]provider.Tool, 0, len(transformed))
	for i, transformedTool := range transformed {
		if len(transformedTool.InputSchema) > 0 {
			if _, ok := decodeJSONValue(transformedTool.InputSchema); !ok {
				return nil, fmt.Errorf("%w: transformed tool %d has invalid input schema", ErrHookTransformFailed, i)
			}
		}
		match := -1
		for j, tool := range original {
			mapped := toolsToAgento11y([]provider.Tool{tool})
			if used[j] || len(mapped) != 1 || !toolDefinitionEqual(mapped[0], transformedTool) {
				continue
			}
			if match >= 0 {
				return nil, fmt.Errorf("%w: transformed tool %d is ambiguous", ErrHookTransformFailed, i)
			}
			match = j
		}
		if match < 0 {
			return nil, fmt.Errorf("%w: transformed tool %d cannot be reconstructed", ErrHookTransformFailed, i)
		}
		used[match] = true
		out = append(out, original[match])
	}
	return out, nil
}

func validateTransformedToolChoice(choice *provider.ToolChoice, tools []provider.Tool) error {
	if choice == nil {
		return nil
	}
	switch choice.Type {
	case provider.ToolChoiceAuto, provider.ToolChoiceNone:
		return nil
	case provider.ToolChoiceRequired:
		if len(tools) == 0 {
			return fmt.Errorf("%w: required tool choice has no transformed tools", ErrHookTransformFailed)
		}
		return nil
	case provider.ToolChoiceTool:
		for _, tool := range tools {
			if tool.Name == choice.ToolName {
				return nil
			}
		}
		return fmt.Errorf("%w: selected tool %q was removed by transform", ErrHookTransformFailed, choice.ToolName)
	default:
		return fmt.Errorf("%w: unsupported tool choice %q", ErrHookTransformFailed, choice.Type)
	}
}

func toolDefinitionEqual(a, b agento11y.ToolDefinition) bool {
	return a.Name == b.Name &&
		a.Description == b.Description &&
		a.Type == b.Type &&
		a.Deferred == b.Deferred &&
		jsonValueEqual(a.InputSchema, b.InputSchema)
}

func validateTransformedMessage(message agento11y.Message) error {
	if message.Name != "" {
		return fmt.Errorf("message name cannot be reconstructed")
	}
	generation := agento11y.Generation{
		Model: agento11y.ModelRef{Provider: "hook", Name: "transform"},
		Input: []agento11y.Message{message},
	}
	if err := generation.Validate(); err != nil {
		return err
	}
	for i, part := range message.Parts {
		switch part.Kind {
		case agento11y.PartKindText:
			if part.Metadata.ProviderType != "" {
				return fmt.Errorf("part %d has unsupported text provider type %q", i, part.Metadata.ProviderType)
			}
		case agento11y.PartKindThinking:
			if part.Metadata.ProviderType != "" && part.Metadata.ProviderType != "thinking" {
				return fmt.Errorf("part %d has unsupported thinking provider type %q", i, part.Metadata.ProviderType)
			}
		case agento11y.PartKindToolCall:
			if strings.TrimSpace(part.ToolCall.ID) == "" || strings.TrimSpace(part.ToolCall.Name) == "" {
				return fmt.Errorf("part %d tool call requires id and name", i)
			}
			if len(part.ToolCall.InputJSON) > 0 {
				if _, ok := decodeJSONValue(part.ToolCall.InputJSON); !ok {
					return fmt.Errorf("part %d has invalid tool call JSON", i)
				}
			}
		case agento11y.PartKindToolResult:
			if strings.TrimSpace(part.ToolResult.ToolCallID) == "" || strings.TrimSpace(part.ToolResult.Name) == "" {
				return fmt.Errorf("part %d tool result requires id and name", i)
			}
			hasText := part.ToolResult.Content != ""
			hasJSON := len(part.ToolResult.ContentJSON) > 0
			if hasText == hasJSON {
				return fmt.Errorf("part %d tool result requires exactly one content payload", i)
			}
			if hasJSON {
				if _, ok := decodeJSONValue(part.ToolResult.ContentJSON); !ok {
					return fmt.Errorf("part %d has invalid tool result JSON", i)
				}
			}
		}
	}
	return nil
}

func agento11yRoleToProvider(role agento11y.Role) (provider.Role, bool) {
	switch role {
	case agento11y.RoleUser:
		return provider.RoleUser, true
	case agento11y.RoleAssistant:
		return provider.RoleAssistant, true
	case agento11y.RoleTool:
		return provider.RoleTool, true
	default:
		return "", false
	}
}

type transformedPartCandidate struct {
	part provider.ContentPart
	used bool
}

type transformedPartMatcher struct {
	tools       []provider.Tool
	reasonings  []transformedPartCandidate
	toolCalls   []transformedPartCandidate
	toolResults []transformedPartCandidate
}

func newTransformedPartMatcher(prompt []provider.Message, tools []provider.Tool) *transformedPartMatcher {
	matcher := &transformedPartMatcher{tools: tools}
	for _, msg := range prompt {
		for _, part := range msg.Content {
			candidate := transformedPartCandidate{part: part}
			switch part.Type {
			case provider.ContentPartTypeReasoning:
				matcher.reasonings = append(matcher.reasonings, candidate)
			case provider.ContentPartTypeToolCall:
				matcher.toolCalls = append(matcher.toolCalls, candidate)
			case provider.ContentPartTypeToolResult:
				matcher.toolResults = append(matcher.toolResults, candidate)
			}
		}
	}
	return matcher
}

func rebuildPartsFromAgento11y(parts []agento11y.Part, matcher *transformedPartMatcher) ([]provider.ContentPart, error) {
	out := make([]provider.ContentPart, 0, len(parts))
	for i, p := range parts {
		var part provider.ContentPart
		var err error
		switch p.Kind {
		case agento11y.PartKindText:
			if p.Text == "" {
				return nil, fmt.Errorf("part %d has empty text", i)
			}
			part = provider.TextPart(p.Text)
		case agento11y.PartKindThinking:
			if p.Thinking == "" {
				return nil, fmt.Errorf("part %d has empty thinking", i)
			}
			part, err = matcher.matchReasoning(p.Thinking)
		case agento11y.PartKindToolCall:
			part, err = matcher.matchToolCall(p.ToolCall, p.Metadata.ProviderType)
		case agento11y.PartKindToolResult:
			part, err = matcher.matchToolResult(p.ToolResult, p.Metadata.ProviderType)
		default:
			return nil, fmt.Errorf("part %d has unsupported kind %q", i, p.Kind)
		}
		if err != nil {
			return nil, fmt.Errorf("part %d: %w", i, err)
		}
		out = append(out, part)
	}
	return out, nil
}

func (m *transformedPartMatcher) matchReasoning(text string) (provider.ContentPart, error) {
	match := -1
	for i := range m.reasonings {
		if m.reasonings[i].used || m.reasonings[i].part.Text != text {
			continue
		}
		if match >= 0 {
			return provider.ContentPart{}, fmt.Errorf("reasoning signature is ambiguous")
		}
		match = i
	}
	if match < 0 {
		for i := range m.reasonings {
			if len(m.reasonings[i].part.ProviderOptions) > 0 {
				return provider.ContentPart{}, fmt.Errorf("reasoning signature cannot be preserved")
			}
		}
		return provider.ReasoningPart(text), nil
	}
	m.reasonings[match].used = true
	return m.reasonings[match].part, nil
}

func (m *transformedPartMatcher) matchToolCall(call *agento11y.ToolCall, providerType string) (provider.ContentPart, error) {
	rebuilt := provider.ToolCallPart(call.ID, call.Name, call.InputJSON)
	match := -1
	providerSpecific := false
	for i := range m.toolCalls {
		candidate := m.toolCalls[i].part
		if candidate.ToolCallID != call.ID {
			continue
		}
		mapped, _, ok := contentPartToAgento11yWithTools(candidate, true, m.tools)
		candidateProviderType := mapped.Metadata.ProviderType
		providerSpecific = providerSpecific || candidate.ProviderExecuted || len(candidate.ProviderOptions) > 0 ||
			(ok && candidateProviderType != "" && candidateProviderType != "tool_use")
		if m.toolCalls[i].used {
			continue
		}
		if candidate.ToolName != call.Name || !jsonValueEqual(candidate.Input, call.InputJSON) {
			continue
		}
		if !ok || candidateProviderType != providerType {
			continue
		}
		if match >= 0 {
			return provider.ContentPart{}, fmt.Errorf("tool call %q is ambiguous", call.ID)
		}
		match = i
	}
	if match >= 0 {
		candidate := m.toolCalls[match].part
		m.toolCalls[match].used = true
		return candidate, nil
	}
	if providerSpecific || (providerType != "" && providerType != "tool_use") {
		return provider.ContentPart{}, fmt.Errorf("provider tool call %q cannot be reconstructed", call.ID)
	}
	return rebuilt, nil
}

func (m *transformedPartMatcher) matchToolResult(result *agento11y.ToolResult, providerType string) (provider.ContentPart, error) {
	rebuilt := toolResultPartFromAgento11y(result)
	match := -1
	providerSpecific := false
	for i := range m.toolResults {
		candidate := m.toolResults[i].part
		if candidate.ToolCallID != result.ToolCallID {
			continue
		}
		mapped, _, ok := contentPartToAgento11yWithTools(candidate, true, m.tools)
		candidateProviderType := mapped.Metadata.ProviderType
		providerSpecific = providerSpecific || candidate.ProviderExecuted || len(candidate.ProviderOptions) > 0 ||
			(ok && candidateProviderType != "" && candidateProviderType != "tool_result")
		if m.toolResults[i].used {
			continue
		}
		if !ok || !toolResultEqual(mapped.ToolResult, result) || candidateProviderType != providerType {
			continue
		}
		if match >= 0 {
			return provider.ContentPart{}, fmt.Errorf("tool result %q is ambiguous", result.ToolCallID)
		}
		match = i
	}
	if match >= 0 {
		candidate := m.toolResults[match].part
		m.toolResults[match].used = true
		return candidate, nil
	}
	if providerSpecific || (providerType != "" && providerType != "tool_result") {
		return provider.ContentPart{}, fmt.Errorf("provider tool result %q cannot be reconstructed", result.ToolCallID)
	}
	return rebuilt, nil
}

func toolResultEqual(a, b *agento11y.ToolResult) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ToolCallID == b.ToolCallID &&
		a.Name == b.Name &&
		a.IsError == b.IsError &&
		a.Content == b.Content &&
		jsonValueEqual(a.ContentJSON, b.ContentJSON)
}

func jsonValueEqual(a, b json.RawMessage) bool {
	a = bytes.TrimSpace(a)
	b = bytes.TrimSpace(b)
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	if !json.Valid(a) || !json.Valid(b) {
		return false
	}
	left, leftOK := decodeJSONValue(a)
	right, rightOK := decodeJSONValue(b)
	return leftOK && rightOK && reflect.DeepEqual(left, right)
}

func toolResultPartFromAgento11y(result *agento11y.ToolResult) provider.ContentPart {
	output := &provider.ToolResultOutput{}
	switch {
	case len(result.ContentJSON) > 0:
		output.Type = provider.ToolOutputJSON
		output.JSON = json.RawMessage(append([]byte(nil), result.ContentJSON...))
	case result.Content != "":
		output.Type = provider.ToolOutputText
		output.Text = result.Content
	}
	if result.IsError {
		if output.Type == provider.ToolOutputJSON {
			output.Type = provider.ToolOutputErrorJSON
		} else {
			output.Type = provider.ToolOutputErrorText
		}
	}
	return provider.ToolResultPart(result.ToolCallID, result.Name, output)
}
