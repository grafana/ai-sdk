package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/fallback"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unaryCall(id, name string, providerExecuted bool) provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolCall, ToolCallID: id, ToolName: name, Input: json.RawMessage(`{}`), ProviderExecuted: providerExecuted}
}

func unaryResult(id, name, result string) provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolResult, ToolCallID: id, ToolName: name, Result: json.RawMessage(result)}
}

func unaryPreview(id, name string) provider.GenerateContentPart {
	part := unaryResult(id, name, `{"partial":true}`)
	part.Preliminary = new(true)
	return part
}

func unaryApproval(callID, approvalID string) provider.GenerateContentPart {
	return provider.GenerateContentPart{Type: provider.ContentToolApprovalRequest, ToolCallID: callID, ApprovalID: approvalID}
}

func withContent(part provider.GenerateContentPart, change func(*provider.GenerateContentPart)) provider.GenerateContentPart {
	change(&part)
	return part
}

type unaryRuleCase struct {
	name    string
	history ToolHistory
	content []provider.GenerateContentPart
	reason  *provider.UnifiedFinishReason
	invalid bool
}

func unaryRuleCases() []unaryRuleCase {
	pending := ToolHistory{"hosted": "search"}
	text := provider.GenerateContentPart{Type: provider.ContentText, Text: "ok"}
	return []unaryRuleCase{
		{name: "text", content: []provider.GenerateContentPart{text}},
		{name: "every content kind", content: []provider.GenerateContentPart{text, {Type: provider.ContentSource}, {Type: provider.ContentCustom}, {Type: provider.ContentReasoning}, {Type: provider.ContentReasoningFile}, {Type: provider.ContentFile}}},
		{name: "unknown content type", content: []provider.GenerateContentPart{{Type: "future"}}, invalid: true},
		{name: "provider-executed text", content: []provider.GenerateContentPart{withContent(text, func(p *provider.GenerateContentPart) { p.ProviderExecuted = true })}, invalid: true},
		{name: "dynamic custom part", content: []provider.GenerateContentPart{{Type: provider.ContentCustom, Dynamic: new(true)}}, invalid: true},
		{name: "preliminary reasoning", content: []provider.GenerateContentPart{{Type: provider.ContentReasoning, Preliminary: new(true)}}, invalid: true},
		{name: "false flags on a non-tool part", content: []provider.GenerateContentPart{withContent(text, func(p *provider.GenerateContentPart) { p.Dynamic, p.Preliminary = new(false), new(false) })}},
		{name: "unregistered finish reason", content: []provider.GenerateContentPart{text}, reason: new(provider.UnifiedFinishReason("future")), invalid: true},
		{name: "empty finish reason", content: []provider.GenerateContentPart{text}, reason: new(provider.UnifiedFinishReason("")), invalid: true},

		{name: "call and result", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryResult("c", "f", `{}`)}},
		{name: "call without ID", content: []provider.GenerateContentPart{unaryCall("", "f", false)}, invalid: true},
		{name: "call without name", content: []provider.GenerateContentPart{unaryCall("c", "", false)}, invalid: true},
		{name: "preliminary call", content: []provider.GenerateContentPart{withContent(unaryCall("c", "f", false), func(p *provider.GenerateContentPart) { p.Preliminary = new(true) })}, invalid: true},
		{name: "repeated call", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryCall("c", "f", true)}, invalid: true},
		{name: "result for an unknown call", content: []provider.GenerateContentPart{unaryResult("c", "f", `{}`)}, invalid: true},
		{name: "result with another name", content: []provider.GenerateContentPart{unaryCall("c", "f", false), unaryResult("c", "g", `{}`)}, invalid: true},
		{name: "repeated result", content: []provider.GenerateContentPart{unaryCall("c", "f", false), unaryResult("c", "f", `{}`), unaryResult("c", "f", `{}`)}, invalid: true},
		{name: "empty result", content: []provider.GenerateContentPart{unaryCall("c", "f", false), unaryResult("c", "f", ``)}, invalid: true},
		{name: "invalid result", content: []provider.GenerateContentPart{unaryCall("c", "f", false), unaryResult("c", "f", `{`)}, invalid: true},
		{name: "null result", content: []provider.GenerateContentPart{unaryCall("c", "f", false), unaryResult("c", "f", `null`)}, invalid: true},
		{name: "preview then final", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryPreview("c", "f"), unaryResult("c", "f", `{}`)}},
		{name: "preview without final", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryPreview("c", "f")}, invalid: true},

		{name: "result for a history call", history: pending, content: []provider.GenerateContentPart{unaryResult("hosted", "search", `{}`)}},
		{name: "history result with another name", history: pending, content: []provider.GenerateContentPart{unaryResult("hosted", "other", `{}`)}, invalid: true},
		{name: "history re-emission", history: pending, content: []provider.GenerateContentPart{unaryCall("hosted", "search", true), unaryResult("hosted", "search", `{}`)}},
		{name: "history re-emission not provider-executed", history: pending, content: []provider.GenerateContentPart{unaryCall("hosted", "search", false)}, invalid: true},
		{name: "history re-emission with another name", history: pending, content: []provider.GenerateContentPart{unaryCall("hosted", "other", true)}, invalid: true},
		{name: "second history re-emission", history: pending, content: []provider.GenerateContentPart{unaryCall("hosted", "search", true), unaryCall("hosted", "search", true)}, invalid: true},
		{name: "history re-emission after a preview", history: pending, content: []provider.GenerateContentPart{unaryPreview("hosted", "search"), unaryCall("hosted", "search", true)}, invalid: true},
		{name: "history re-emission after a final result", history: pending, content: []provider.GenerateContentPart{unaryResult("hosted", "search", `{}`), unaryCall("hosted", "search", true)}, invalid: true},
		{name: "pending history preview", history: pending, content: []provider.GenerateContentPart{unaryPreview("hosted", "search")}, invalid: true},
		{name: "approval on a history call", history: pending, content: []provider.GenerateContentPart{unaryApproval("hosted", "a")}},
		{name: "re-emission keeps the history approval", history: pending, content: []provider.GenerateContentPart{unaryApproval("hosted", "a"), unaryCall("hosted", "search", true), unaryApproval("hosted", "b")}, invalid: true},

		{name: "approval of a provider-executed call", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryApproval("c", "a"), unaryResult("c", "f", `{}`)}},
		{name: "approval of an unknown call", content: []provider.GenerateContentPart{unaryApproval("c", "a")}, invalid: true},
		{name: "approval of a client call", content: []provider.GenerateContentPart{unaryCall("c", "f", false), unaryApproval("c", "a")}, invalid: true},
		{name: "approval after a preview", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryPreview("c", "f"), unaryApproval("c", "a")}, invalid: true},
		{name: "approval after a final result", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryResult("c", "f", `{}`), unaryApproval("c", "a")}, invalid: true},
		{name: "repeated approval of one call", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryApproval("c", "a"), unaryApproval("c", "b")}, invalid: true},
		{name: "reused approval ID", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryCall("d", "f", true), unaryApproval("c", "a"), unaryApproval("d", "a")}, invalid: true},
		{name: "empty approval ID", content: []provider.GenerateContentPart{unaryCall("c", "f", true), unaryApproval("c", "")}, invalid: true},
		{name: "provider-executed approval", content: []provider.GenerateContentPart{unaryCall("c", "f", true), withContent(unaryApproval("c", "a"), func(p *provider.GenerateContentPart) { p.ProviderExecuted = true })}, invalid: true},
	}
}

func TestValidateGenerateResult(t *testing.T) {
	for _, tc := range unaryRuleCases() {
		t.Run(tc.name, func(t *testing.T) {
			reason := provider.FinishReasonStop
			if tc.reason != nil {
				reason = *tc.reason
			}
			err := ValidateGenerateResult(&provider.GenerateResult{Content: tc.content, FinishReason: provider.FinishReason{Unified: reason}}, tc.history)
			if tc.invalid {
				assert.ErrorIs(t, err, ErrInvalidOutput)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	t.Run("nil result", func(t *testing.T) {
		assert.ErrorIs(t, ValidateGenerateResult(nil, nil), ErrInvalidOutput)
	})

	t.Run("history is not modified", func(t *testing.T) {
		history := ToolHistory{"hosted": "search"}
		result := &provider.GenerateResult{Content: []provider.GenerateContentPart{unaryResult("hosted", "search", `{}`)}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}
		assert.NoError(t, ValidateGenerateResult(result, history))
		assert.NoError(t, ValidateGenerateResult(result, history))
		assert.Equal(t, ToolHistory{"hosted": "search"}, history)
	})
}

func TestCallGenerate_InvalidOutput(t *testing.T) {
	unregistered := func(context.Context) (*provider.GenerateResult, error) {
		return &provider.GenerateResult{FinishReason: provider.FinishReason{Unified: "future"}}, nil
	}

	t.Run("violation fails with the success view", func(t *testing.T) {
		model := &generateModel{modelID: "A", generate: unregistered}
		resolved := catalog.ResolvedModel{ID: "canonical", Model: model, Candidates: []catalog.ConfiguredCandidate{{Provider: "native", ModelID: "A"}}}
		call, err := newRuntime(t, CatalogSelector(staticResolver{resolved: resolved}), testLimits()).Call(t.Context(), "alias", provider.CallOptions{})
		require.NoError(t, err)
		t.Cleanup(call.Close)
		result, err := call.Generate()
		assert.Nil(t, result)
		require.ErrorIs(t, err, ErrInvalidOutput)
		assert.Equal(t, CategoryInternal, ClassifyProvider(err))
		view := call.View(err)
		require.NotNil(t, view.Overview)
		require.Len(t, view.Overview.Attempts, 1)
		assert.Equal(t, fallback.AttemptSelected, view.Overview.Attempts[0].Outcome)
		assert.Nil(t, view.Overview.Attempts[0].Error)
	})

	t.Run("history seeds the result", func(t *testing.T) {
		model := &generateModel{generate: func(context.Context) (*provider.GenerateResult, error) {
			return &provider.GenerateResult{Content: []provider.GenerateContentPart{unaryResult("hosted", "search", `{}`)}, FinishReason: provider.FinishReason{Unified: provider.FinishReasonStop}}, nil
		}}
		prompt := []provider.Message{provider.NewAssistantMessage(historyCall("hosted", "search", true))}
		call, err := newRuntime(t, directSelector(model), testLimits()).Call(t.Context(), "id", provider.CallOptions{Prompt: prompt})
		require.NoError(t, err)
		t.Cleanup(call.Close)
		_, err = call.Generate()
		assert.NoError(t, err)
	})

	for _, ending := range endingContexts(t) {
		t.Run(ending.name+" wins over a violation", func(t *testing.T) {
			model := &generateModel{generate: func(ctx context.Context) (*provider.GenerateResult, error) {
				<-ctx.Done()
				return unregistered(ctx)
			}}
			_, err := newCall(t, ending.ctx(), model, testLimits()).Generate()
			assert.ErrorIs(t, err, ending.err)
			assert.NotErrorIs(t, err, ErrInvalidOutput)
		})
	}
}
