package openaicompatible

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCacheTextBoundaries(t *testing.T) {
	text := "ab"
	a := "a"
	b := "b"
	parts := []ChatCacheTextPart{{Type: "text", Text: &a, Breakpoint: &ChatCacheBreakpoint{Mode: "explicit"}}, {Type: "text", Text: &b}}
	for _, role := range []provider.Role{provider.RoleSystem, provider.RoleAssistant, provider.RoleTool} {
		content := []provider.ContentPart{provider.TextPart(text)}
		if role == provider.RoleTool {
			content = []provider.ContentPart{provider.ToolResultPart("call", "f", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: text})}
		}
		prompt := []provider.Message{{Role: role, Content: content, ProviderOptions: provider.ProviderOptions{"openaiCompatible": ChatCompletionMessageOptions{ChatTextParts: parts}}}}
		before, err := json.Marshal(prompt)
		require.NoError(t, err)
		got, err := convertPrompt(prompt, "custom")
		require.NoError(t, err)
		raw, err := json.Marshal(got[0])
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"prompt_cache_breakpoint":{"mode":"explicit"}`)
		assert.NotContains(t, string(raw), "chatTextParts")
		after, err := json.Marshal(prompt)
		require.NoError(t, err)
		assert.Equal(t, string(before), string(after))
	}
	for _, raw := range []string{`[{"type":"text","text":"changed","prompt_cache_breakpoint":{"mode":"explicit"}}]`, `[{"type":"text","text":"ab","role":"system","prompt_cache_breakpoint":{"mode":"explicit"}}]`, `[{"type":"text","text":"ab","tool_call_id":"fake","prompt_cache_breakpoint":{"mode":"explicit"}}]`} {
		prompt := []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{provider.TextPart(text)}, ProviderOptions: provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"chatTextParts":` + raw + `}`)}}}}
		_, err := convertPrompt(prompt, "custom")
		require.Error(t, err)
	}
}
func TestCacheSingletonUserAndOptions(t *testing.T) {
	part := provider.TextPart("hello")
	part.ProviderOptions = provider.ProviderOptions{"openaiCompatible": ChatCompletionMessageOptions{Breakpoint: &ChatCacheBreakpoint{Mode: "explicit", TTL: "12h"}}}
	model := &model{providerName: "custom.chat", modelID: "native"}
	body, _, err := model.buildRequest(provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{part}}}, ProviderOptions: provider.ProviderOptions{"openaiCompatible": OpenAIOptions{PromptCacheOptions: &ChatPromptCacheOptions{Mode: "explicit", TTL: "30m"}}}}, false)
	require.NoError(t, err)
	raw, err := json.Marshal(body["messages"])
	require.NoError(t, err)
	assert.JSONEq(t, `[{"role":"user","content":[{"type":"text","text":"hello","prompt_cache_breakpoint":{"mode":"explicit","ttl":"12h"}}]}]`, string(raw))
	raw, err = json.Marshal(body["prompt_cache_options"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"mode":"explicit","ttl":"30m"}`, string(raw))
	require.Error(t, (&ChatPromptCacheOptions{TTL: "12h"}).Validate())
}
