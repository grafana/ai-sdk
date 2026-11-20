package messages

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveForTarget(t *testing.T, target execution.Target, body string) provider.CallOptions {
	t.Helper()
	var seen provider.CallOptions
	model := &fakeModel{generate: func(_ context.Context, o provider.CallOptions) (*provider.GenerateResult, error) {
		seen = o
		return textResult(), nil
	}}
	entries, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: "canonical", Aliases: []string{"alias"}}, Model: execution.Translate(model, target)}})
	require.NoError(t, err)
	runtime, err := execution.New(execution.Config{Selector: execution.CatalogSelector(entries), Limits: execution.DefaultLimits})
	require.NoError(t, err)
	h, err := New(runtime)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", Path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	return seen
}

func rawOption(t *testing.T, options provider.CallOptions, namespace string) map[string]any {
	t.Helper()
	option, ok := options.ProviderOptions[namespace].(provider.RawProviderOption)
	require.True(t, ok, "no %s option in %v", namespace, options.ProviderOptions)
	var values map[string]any
	require.NoError(t, json.Unmarshal(option.Raw, &values))
	return values
}

func TestHandler_DisableParallelToolUse(t *testing.T) {
	const tools = `"tools":[{"name":"Read","input_schema":{"type":"object"}}]`
	body := func(choice string) string {
		return `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],` + tools + choice + `}`
	}
	for _, tc := range []struct {
		name      string
		target    execution.Target
		namespace string
		want      map[string]any
	}{
		{"anthropic", execution.Target{Type: execution.TargetAnthropic}, "anthropic", map[string]any{"disableParallelToolUse": true}},
		{"openai", execution.Target{Type: execution.TargetOpenAI}, "openai", map[string]any{"parallelToolCalls": false, "store": false}},
		{"compatible", execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"}, "vendor", map[string]any{"parallel_tool_calls": false, "store": false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := serveForTarget(t, tc.target, body(`,"tool_choice":{"type":"auto","disable_parallel_tool_use":true}`))
			assert.Equal(t, tc.want, rawOption(t, seen, tc.namespace))
			require.NotNil(t, seen.ToolChoice)
			assert.Equal(t, provider.ToolChoiceAuto, seen.ToolChoice.Type)
		})
	}

	t.Run("explicit false keeps parallel calls on", func(t *testing.T) {
		seen := serveForTarget(t, execution.Target{Type: execution.TargetOpenAI}, body(`,"tool_choice":{"type":"auto","disable_parallel_tool_use":false}`))
		assert.Equal(t, map[string]any{"parallelToolCalls": true, "store": false}, rawOption(t, seen, "openai"))
	})

	t.Run("unset sets no provider option", func(t *testing.T) {
		for _, choice := range []string{``, `,"tool_choice":{"type":"auto"}`} {
			seen := serveForTarget(t, execution.Target{Type: execution.TargetAnthropic}, body(choice))
			assert.Empty(t, seen.ProviderOptions)
		}
	})
}

// The request fixture keeps the structure of a Claude Code request (block
// types, cache-control placement, tool shapes, a system-role message, thinking
// history) with placeholder text. It is a client request fixture, not a
// recorded provider response.
func TestHandler_ClaudeCodeRequestFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/claude_code_request.json")
	require.NoError(t, err)
	require.NotContains(t, string(body), "You are Claude Code")

	var seen provider.CallOptions
	h := testHandler(t, &fakeModel{stream: func(_ context.Context, o provider.CallOptions) (*provider.StreamResult, error) {
		seen = o
		return streamOf(
			provider.StreamPart{Type: provider.PartTextStart, ID: "t"},
			provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "ok"},
			provider.StreamPart{Type: provider.PartTextEnd, ID: "t"},
			finish(provider.FinishReasonStop),
		), nil
	}})
	req := httptest.NewRequest("POST", Path+"?beta=true", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Beta", "claude-code-20250219,interleaved-thinking-2025-05-14")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "event: message_stop")

	// Each of the three system blocks and the in-conversation reminder keeps
	// its own message; thinking history is dropped.
	require.Len(t, seen.Prompt, 7)
	assert.Equal(t, []provider.Role{provider.RoleSystem, provider.RoleSystem, provider.RoleSystem, provider.RoleUser, provider.RoleSystem, provider.RoleAssistant, provider.RoleTool}, roles(seen.Prompt))
	require.Len(t, seen.Tools, 2)
	assert.Empty(t, seen.ProviderOptions)
}

func roles(messages []provider.Message) []provider.Role {
	out := make([]provider.Role, len(messages))
	for i, m := range messages {
		out[i] = m.Role
	}
	return out
}

func TestHandler_StorageDisabled(t *testing.T) {
	for _, tc := range []struct {
		name      string
		target    execution.Target
		namespace string
	}{
		{"openai", execution.Target{Type: execution.TargetOpenAI}, "openai"},
		{"compatible", execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"}, "vendor"},
		{"compatible default", execution.Target{Type: execution.TargetOpenAICompatible}, "openai-compatible"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := serveForTarget(t, tc.target, `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`)
			assert.Equal(t, map[string]any{"store": false}, rawOption(t, seen, tc.namespace))
		})
	}
}
