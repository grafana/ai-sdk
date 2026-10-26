package bedrock

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bothNamespaceOptions(modern, legacy string) provider.ProviderOptions {
	opts := rawPartOptions("amazonBedrock", modern)
	opts["bedrock"] = provider.RawProviderOption{Key: "bedrock", Raw: json.RawMessage(legacy)}
	return opts
}

func cachedPart(part provider.ContentPart, ttl string) provider.ContentPart {
	return withOptions(part, rawPartOptions("bedrock", `{"cachePoint":{"type":"default","ttl":"`+ttl+`"}}`))
}

func TestConvertPrompt_PartCachePoints(t *testing.T) {
	textResult := func(id string) provider.ContentPart {
		return provider.ToolResultPart(id, "add", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})
	}
	call := providerExecutedCall("c1", `{}`)
	cases := []struct {
		name   string
		prompt []provider.Message
		want   string
	}{
		{
			name:   "user content part",
			prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("Hello"), cachedPart(provider.TextPart("cached"), "5m"), provider.TextPart("World"))},
			want:   `[{"role":"user","content":[{"text":"Hello"},{"text":"cached"},{"cachePoint":{"type":"default","ttl":"5m"}},{"text":"World"}]}]`,
		},
		{
			name:   "assistant content part",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.TextPart("Hello"), cachedPart(provider.TextPart("cached"), "1h"), provider.TextPart("World"))},
			want:   `[{"role":"assistant","content":[{"text":"Hello"},{"text":"cached"},{"cachePoint":{"type":"default","ttl":"1h"}},{"text":"World"}]}]`,
		},
		{
			name:   "tool result part",
			prompt: []provider.Message{provider.NewToolMessage(cachedPart(textResult("c1"), "5m"), textResult("c2"))},
			want:   `[{"role":"user","content":[{"toolResult":{"toolUseId":"c1","content":[{"text":"ok"}]}},{"cachePoint":{"type":"default","ttl":"5m"}},{"toolResult":{"toolUseId":"c2","content":[{"text":"ok"}]}}]}]`,
		},
		{
			name:   "assistant tool result part",
			prompt: []provider.Message{provider.NewAssistantMessage(call, cachedPart(textResult("c1"), "5m"), provider.TextPart("done")), provider.UserText("next")},
			want:   `[{"role":"assistant","content":[{"toolUse":{"toolUseId":"c1","name":"add","input":{}}}]},{"role":"user","content":[{"toolResult":{"toolUseId":"c1","content":[{"text":"ok"}]}},{"cachePoint":{"type":"default","ttl":"5m"}}]},{"role":"assistant","content":[{"text":"done"}]},{"role":"user","content":[{"text":"next"}]}]`,
		},
		{
			name:   "part and message cache points",
			prompt: []provider.Message{cachedMessage(provider.RoleUser, cachedPart(provider.TextPart("a"), "1h"))},
			want:   `[{"role":"user","content":[{"text":"a"},{"cachePoint":{"type":"default","ttl":"1h"}},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`,
		},
		{
			name:   "legacy namespace fallback",
			prompt: []provider.Message{provider.NewUserMessage(withOptions(provider.TextPart("a"), bothNamespaceOptions(`{"guardContent":false}`, `{"cachePoint":{"type":"default"}}`)))},
			want:   `[{"role":"user","content":[{"text":"a"},{"cachePoint":{"type":"default"}}]}]`,
		},
		{
			name:   "modern namespace wins",
			prompt: []provider.Message{provider.NewUserMessage(withOptions(provider.TextPart("a"), bothNamespaceOptions(`{"cachePoint":{"type":"default","ttl":"1h"}}`, `{"cachePoint":{"type":"default","ttl":"5m"}}`)))},
			want:   `[{"role":"user","content":[{"text":"a"},{"cachePoint":{"type":"default","ttl":"1h"}}]}]`,
		},
		{
			name:   "user file part",
			prompt: []provider.Message{provider.NewUserMessage(cachedPart(provider.FilePart("image/png", provider.DataContent{Base64: "aGVsbG8="}), "5m"))},
			want:   `[{"role":"user","content":[{"image":{"format":"png","source":{"bytes":"aGVsbG8="}}},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`,
		},
		{
			name:   "user file part without data emits no cache point",
			prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("a"), cachedPart(provider.ContentPart{Type: provider.ContentPartTypeFile, MediaType: "image/png"}, "5m"))},
			want:   `[{"role":"user","content":[{"text":"a"}]}]`,
		},
		{
			name:   "null modern cache point falls back to legacy namespace",
			prompt: []provider.Message{provider.NewUserMessage(withOptions(provider.TextPart("a"), bothNamespaceOptions(`{"cachePoint":null}`, `{"cachePoint":{"type":"default","ttl":"5m"}}`)))},
			want:   `[{"role":"user","content":[{"text":"a"},{"cachePoint":{"type":"default","ttl":"5m"}}]}]`,
		},
		{
			name:   "signed reasoning part",
			prompt: []provider.Message{provider.NewAssistantMessage(withOptions(provider.ReasoningPart("r"), rawPartOptions("bedrock", `{"signature":"sig","cachePoint":{"type":"default"}}`))), provider.UserText("next")},
			want:   `[{"role":"assistant","content":[{"reasoningContent":{"reasoningText":{"text":"r","signature":"sig"}}},{"cachePoint":{"type":"default"}}]},{"role":"user","content":[{"text":"next"}]}]`,
		},
		{
			name:   "non-emitting assistant part does not borrow previous content",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.TextPart("a"), cachedPart(provider.ReasoningPart("unsigned"), "5m")), provider.UserText("next")},
			want:   `[{"role":"assistant","content":[{"text":"a"}]},{"role":"user","content":[{"text":"next"}]}]`,
		},
		{
			name:   "empty assistant text part does not borrow previous content",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.TextPart("a"), cachedPart(provider.TextPart("  "), "5m")), provider.UserText("next")},
			want:   `[{"role":"assistant","content":[{"text":"a"}]},{"role":"user","content":[{"text":"next"}]}]`,
		},
		{
			name:   "approval response part is ignored",
			prompt: []provider.Message{provider.NewToolMessage(textResult("c1"), cachedPart(provider.ToolApprovalResponsePart("approval-1", true, ""), "5m"))},
			want:   `[{"role":"user","content":[{"toolResult":{"toolUseId":"c1","content":[{"text":"ok"}]}}]}]`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, warnings, err := convertPrompt(tc.prompt, false, true)
			require.NoError(t, err)
			assert.Empty(t, warnings)
			encoded, err := json.Marshal(got.Messages)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestConvertPrompt_PartCachePointValidation(t *testing.T) {
	malformed := rawPartOptions("bedrock", `{"cachePoint":false}`)
	t.Run("emitted part rejects malformed cache point", func(t *testing.T) {
		_, _, err := convertPrompt([]provider.Message{provider.NewUserMessage(withOptions(provider.TextPart("a"), malformed))}, false, true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cachePoint")
	})
	t.Run("non-emitting part options are not parsed", func(t *testing.T) {
		_, _, err := convertPrompt([]provider.Message{provider.NewAssistantMessage(provider.TextPart("a"), withOptions(provider.ReasoningPart("unsigned"), malformed))}, false, true)
		require.NoError(t, err)
	})
	t.Run("typed reasoning metadata is read without error", func(t *testing.T) {
		part := provider.ReasoningPart("r")
		part.ProviderOptions = provider.BuildProviderOptions(ReasoningMetadata{Signature: "sig"})
		got, _, err := convertPrompt([]provider.Message{provider.NewAssistantMessage(part)}, false, true)
		require.NoError(t, err)
		require.Len(t, got.Messages, 1)
		assert.Len(t, got.Messages[0].Content, 1)
	})
}
