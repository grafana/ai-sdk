package execution

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func historyCall(id, name string, providerExecuted bool) provider.ContentPart {
	part := provider.ToolCallPart(id, name, json.RawMessage(`{}`))
	part.ProviderExecuted = providerExecuted
	return part
}

func historyResult(id, name string) provider.ContentPart {
	return provider.ToolResultPart(id, name, &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "done"})
}

func assistant(parts ...provider.ContentPart) provider.Message {
	return provider.NewAssistantMessage(parts...)
}

func TestRebuildToolHistory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prompt  []provider.Message
		pending ToolHistory
		invalid bool
	}{
		{name: "empty prompt", pending: ToolHistory{}},
		{name: "deferred result", prompt: []provider.Message{assistant(historyCall("call", "code", true))}, pending: ToolHistory{"call": "code"}},
		{name: "completed provider call", prompt: []provider.Message{assistant(historyCall("call", "code", true), historyResult("call", "code"))}, pending: ToolHistory{}},
		{name: "client call is never pending", prompt: []provider.Message{assistant(historyCall("call", "f", false))}, pending: ToolHistory{}},
		{name: "orphan result", prompt: []provider.Message{provider.NewToolMessage(historyResult("prior", "f"))}, pending: ToolHistory{}},
		{name: "orphan result then a call with its ID", prompt: []provider.Message{assistant(historyResult("call", "f"), historyCall("call", "f", true))}, invalid: true},
		{name: "approval re-emission", prompt: []provider.Message{assistant(historyCall("call", "code", true)), assistant(historyCall("call", "code", true))}, pending: ToolHistory{"call": "code"}},
		{name: "approval re-emission then its result", prompt: []provider.Message{assistant(historyCall("call", "code", true)), assistant(historyCall("call", "code", true), historyResult("call", "code"))}, pending: ToolHistory{}},
		{name: "re-emission after the result", prompt: []provider.Message{assistant(historyCall("call", "code", true), historyResult("call", "code")), assistant(historyCall("call", "code", true))}, invalid: true},
		{name: "second re-emission", prompt: []provider.Message{assistant(historyCall("call", "code", true)), assistant(historyCall("call", "code", true)), assistant(historyCall("call", "code", true))}, invalid: true},
		{name: "re-emission with another name", prompt: []provider.Message{assistant(historyCall("call", "code", true)), assistant(historyCall("call", "other", true))}, invalid: true},
		{name: "first call not provider-executed", prompt: []provider.Message{assistant(historyCall("call", "f", false), historyCall("call", "f", true))}, invalid: true},
		{name: "re-emission not provider-executed", prompt: []provider.Message{assistant(historyCall("call", "f", true), historyCall("call", "f", false))}, invalid: true},
		{name: "mismatched result", prompt: []provider.Message{assistant(historyCall("call", "f", true), historyResult("call", "other"))}, invalid: true},
		{name: "repeated result", prompt: []provider.Message{assistant(historyCall("call", "f", true), historyResult("call", "f"), historyResult("call", "f"))}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			history, err := RebuildToolHistory(tc.prompt)
			if tc.invalid {
				require.ErrorIs(t, err, ErrInvalidToolHistory)
				assert.Nil(t, history)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.pending, history)
		})
	}
}

func TestToolHistory_Clone(t *testing.T) {
	history := ToolHistory{"call": "code"}
	copied := history.clone()
	copied["other"] = "f"
	assert.Equal(t, ToolHistory{"call": "code"}, history)
	assert.Equal(t, ToolHistory{}, ToolHistory(nil).clone())
}

func TestRuntimeCall_ToolHistory(t *testing.T) {
	inconsistent := []provider.Message{assistant(historyCall("call", "f", true), historyResult("call", "other"))}
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		options provider.CallOptions
	}{
		{name: "alone", ctx: t.Context(), options: provider.CallOptions{Prompt: inconsistent}},
		{name: "beside a protected header", ctx: t.Context(), options: provider.CallOptions{Prompt: inconsistent, Headers: map[string]string{"Authorization": "Bearer caller"}}},
		{name: "beside gateway controls", ctx: t.Context(), options: provider.CallOptions{Prompt: inconsistent, ProviderOptions: rawOptions(map[string]string{ControlNamespace: `{"byok":{}}`})}},
		{name: "with an ended context", ctx: ended, options: provider.CallOptions{Prompt: inconsistent}},
	} {
		t.Run("inconsistent history "+tc.name, func(t *testing.T) {
			selector := &recordingSelector{model: &generateModel{}}
			_, err := newRuntime(t, selector.selector(), testLimits()).Call(tc.ctx, "id", tc.options)
			require.ErrorIs(t, err, ErrInvalidToolHistory)
			category, reason := ClassifySelection(err)
			assert.Equal(t, CategoryInvalidRequest, category)
			assert.Equal(t, ReasonNone, reason)
			assert.Zero(t, selector.calls.Load())
			assert.Zero(t, selector.model.callCount())
		})
	}

	t.Run("valid history is exposed as a copy", func(t *testing.T) {
		selector := &recordingSelector{model: &generateModel{}}
		call, err := newRuntime(t, selector.selector(), testLimits()).Call(t.Context(), "id", provider.CallOptions{Prompt: []provider.Message{assistant(historyCall("call", "code", true))}})
		require.NoError(t, err)
		t.Cleanup(call.Close)
		history := call.ToolHistory()
		assert.Equal(t, ToolHistory{"call": "code"}, history)
		delete(history, "call")
		history["other"] = "f"
		assert.Equal(t, ToolHistory{"call": "code"}, call.ToolHistory())
		assert.Equal(t, int32(1), selector.calls.Load())
	})
}
