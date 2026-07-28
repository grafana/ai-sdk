package agentobservability

import (
	"context"
	"encoding/json"
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
//     applyTransformedInput, preserving original reasoning-block signatures
//     where the assistant text matches; the inner model is then invoked
//     with the new params (bypassing the middleware-level DoGenerate/DoStream
//     closures, which captured the pre-transform params).
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

	if resp.TransformedInput != nil && (len(resp.TransformedInput.Messages) > 0 || resp.TransformedInput.SystemPrompt != "") {
		newPrompt := applyTransformedInput(params.Prompt, *resp.TransformedInput)
		if len(newPrompt) > 0 {
			span.SetAttributes(attribute.String(SpanAttrHooksResult, "transform"))
			newParams := params
			newParams.Prompt = newPrompt
			return &newParams, nil
		}
	}

	span.SetAttributes(attribute.String(SpanAttrHooksResult, "allow"))
	return nil, nil
}

// buildHookEvaluateRequest assembles the preflight hook evaluation payload.
// Media is excluded because hooks predate Agent Observability media recording,
// and sending inline data or signed URLs would widen the preflight disclosure boundary.
func buildHookEvaluateRequest(model provider.LanguageModel, params provider.CallOptions, ctxInfo ContextInfo) agento11y.HookEvaluateRequest {
	system, msgs := messagesToAgento11yWithMedia(params.Prompt, false)
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

// applyTransformedInput rebuilds an ai-sdk prompt from an agento11y.HookInput
// while preserving the cryptographic signatures attached to reasoning blocks
// in the original prompt's assistant messages and the system prompt that
// the hook did not explicitly modify.
//
// Algorithm (matching the legacy claude path's findMatchingOriginalWithThinking):
//
//  1. System messages: if transformed.SystemPrompt is non-empty, replace the
//     original system context with a single SystemMessage built from it;
//     otherwise carry the original system messages forward unchanged. This
//     mirrors how messagesToAgento11y folds system messages into a separate
//     HookInput.SystemPrompt field — a hook that doesn't touch the system
//     context returns it as "" (the zero value), and we MUST NOT silently
//     drop the originals in that case.
//  2. Non-system messages: index the original assistant-role messages that
//     carry a reasoning part by their concatenated text content (excluding
//     reasoning text). For each transformed message:
//     - If role is assistant AND the same concatenated text appears in the
//     index AND has not yet been consumed, keep the original message
//     verbatim (preserves the signature).
//     - Otherwise, rebuild the message from the transformed SDK parts.
//
// Reasoning signatures don't round-trip through the underlying SDK wire schema, so any
// assistant message whose text the hook changed loses its signature.
func applyTransformedInput(originalPrompt []provider.Message, transformed agento11y.HookInput) []provider.Message {
	originalsWithReasoning := indexAssistantReasoningMessages(originalPrompt)
	used := make(map[int]bool, len(originalsWithReasoning))

	systemMsgs := systemMessagesForTransform(originalPrompt, transformed)

	body := make([]provider.Message, 0, len(transformed.Messages))
	for _, agento11yMsg := range transformed.Messages {
		role, ok := agento11yRoleToProvider(agento11yMsg.Role)
		if !ok {
			continue
		}

		if role == provider.RoleAssistant {
			text := concatAssistantTextFromAgento11y(agento11yMsg)
			if text != "" {
				if idx, orig, found := findMatching(originalsWithReasoning, text, used); found {
					used[idx] = true
					body = append(body, orig)
					continue
				}
			}
		}

		parts := rebuildPartsFromAgento11y(agento11yMsg.Parts)
		if len(parts) == 0 {
			continue
		}
		body = append(body, provider.Message{Role: role, Content: parts})
	}

	if len(systemMsgs) == 0 && len(body) == 0 {
		return nil
	}

	out := make([]provider.Message, 0, len(systemMsgs)+len(body))
	out = append(out, systemMsgs...)
	out = append(out, body...)
	return out
}

// systemMessagesForTransform returns the system-role messages that should
// precede the rebuilt non-system messages after a hook transform.
//
// Behavior:
//   - transformed.SystemPrompt != "": the hook explicitly set a new system
//     prompt → emit a single SystemMessage carrying that text. Any original
//     system messages are replaced.
//   - transformed.SystemPrompt == "": the hook either didn't touch the
//     system context or explicitly cleared it. We treat the zero value as
//     "didn't touch" — preserve the originals — because messagesToAgento11y
//     also produces "" when the original prompt has no system messages,
//     so the two cases are indistinguishable on the underlying SDK wire.
//
// This mirrors how the legacy internal Agent Observability integration
// handled the same edge case: hooks transform what they touch and leave the
// rest alone.
func systemMessagesForTransform(originalPrompt []provider.Message, transformed agento11y.HookInput) []provider.Message {
	if text := strings.TrimSpace(transformed.SystemPrompt); text != "" {
		return []provider.Message{{
			Role:    provider.RoleSystem,
			Content: []provider.ContentPart{provider.TextPart(transformed.SystemPrompt)},
		}}
	}
	originals := make([]provider.Message, 0)
	for _, msg := range originalPrompt {
		if msg.Role == provider.RoleSystem {
			originals = append(originals, msg)
		}
	}
	return originals
}

type indexedMessage struct {
	idx     int
	message provider.Message
	textKey string
}

func indexAssistantReasoningMessages(prompt []provider.Message) []indexedMessage {
	var out []indexedMessage
	for i, msg := range prompt {
		if msg.Role != provider.RoleAssistant {
			continue
		}
		hasReasoning := false
		for _, part := range msg.Content {
			if part.Type == provider.ContentPartTypeReasoning {
				hasReasoning = true
				break
			}
		}
		if !hasReasoning {
			continue
		}
		out = append(out, indexedMessage{
			idx:     i,
			message: msg,
			textKey: concatAssistantText(msg),
		})
	}
	return out
}

func concatAssistantText(msg provider.Message) string {
	var parts []string
	for _, p := range msg.Content {
		if p.Type == provider.ContentPartTypeText && p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "")
}

func concatAssistantTextFromAgento11y(msg agento11y.Message) string {
	var parts []string
	for _, p := range msg.Parts {
		if p.Kind == agento11y.PartKindText && p.Text != "" {
			parts = append(parts, p.Text)
		}
	}
	return strings.Join(parts, "")
}

func findMatching(originals []indexedMessage, text string, used map[int]bool) (int, provider.Message, bool) {
	for _, m := range originals {
		if used[m.idx] {
			continue
		}
		if m.textKey == text {
			return m.idx, m.message, true
		}
	}
	return 0, provider.Message{}, false
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

// rebuildPartsFromAgento11y converts an agento11y message's parts back to ai-sdk
// ContentParts. Reasoning parts come back WITHOUT signatures (which don't
// round-trip through the underlying SDK wire); callers that need signature
// preservation match against the original message via the algorithm in
// applyTransformedInput rather than rebuilding from these parts.
func rebuildPartsFromAgento11y(parts []agento11y.Part) []provider.ContentPart {
	out := make([]provider.ContentPart, 0, len(parts))
	for _, p := range parts {
		switch p.Kind {
		case agento11y.PartKindText:
			if p.Text == "" {
				continue
			}
			out = append(out, provider.TextPart(p.Text))
		case agento11y.PartKindThinking:
			// Drop reasoning parts: signatures can't survive a round-trip, and
			// callers should match against the original message instead.
			continue
		case agento11y.PartKindToolCall:
			if p.ToolCall == nil {
				continue
			}
			out = append(out, provider.ToolCallPart(p.ToolCall.ID, p.ToolCall.Name, p.ToolCall.InputJSON))
		case agento11y.PartKindToolResult:
			if p.ToolResult == nil {
				continue
			}
			out = append(out, toolResultPartFromAgento11y(p.ToolResult))
		}
	}
	return out
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
