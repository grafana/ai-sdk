package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFallbackContent_Output(t *testing.T) {
	block := `{"type":"fallback","from":{"model":"primary","extra":true},"to":{"model":"secondary"},"trigger":{"type":"refusal"}}`
	metadata := `{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`
	t.Run("empty models", func(t *testing.T) {
		got, err := marshalFallbackMetadata(anthropic.BetaFallbackBlock{})
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"fallback","from":{"model":""},"to":{"model":""}}`, string(got["anthropic"]))
	})
	t.Run("unary", func(t *testing.T) {
		msg := unmarshalMessage(t, `{"id":"msg_1","type":"message","role":"assistant","model":"secondary","content":[{"type":"thinking","thinking":"before","signature":"sig-1"},`+block+`,{"type":"thinking","thinking":"after","signature":"sig-2"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
		result, err := convertResponse(msg, toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
		require.NoError(t, err)
		require.Len(t, result.Content, 3)
		assert.Equal(t, provider.ContentCustom, result.Content[1].Type)
		assert.Equal(t, "anthropic.fallback", result.Content[1].Kind)
		assert.JSONEq(t, metadata, string(result.Content[1].ProviderMetadata["anthropic"]))
		assert.Equal(t, "before", result.Content[0].Text)
		assert.Equal(t, "after", result.Content[2].Text)
	})
	t.Run("stream", func(t *testing.T) {
		parts := collectParts([]anthropic.BetaRawMessageStreamEventUnion{
			unmarshalEvent(t, `{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":0}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":1,"content_block":`+block+`}`),
			unmarshalEvent(t, `{"type":"content_block_stop","index":1}`),
			unmarshalEvent(t, `{"type":"content_block_start","index":2,"content_block":{"type":"thinking","thinking":"","signature":""}}`),
		})
		require.Len(t, parts, 4)
		assert.Equal(t, provider.PartReasoningEnd, parts[1].Type)
		assert.Equal(t, provider.PartCustom, parts[2].Type)
		assert.Equal(t, "anthropic.fallback", parts[2].Kind)
		assert.JSONEq(t, metadata, string(parts[2].ProviderMetadata["anthropic"]))
		assert.Equal(t, provider.PartReasoningStart, parts[3].Type)
	})
}

func TestFallbackContent_Continuation(t *testing.T) {
	for _, tc := range []struct {
		name, metadata, expected string
	}{
		{"valid", `{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"},"trigger":{},"cacheControl":{"type":"ephemeral"}}`, `{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`},
		{"empty models", `{"type":"fallback","from":{"model":""},"to":{"model":""}}`, `{"type":"fallback","from":{"model":""},"to":{"model":""}}`},
		{"missing destination", `{"type":"fallback","from":{"model":"primary"}}`, ""},
		{"missing type", `{"from":{"model":"primary"},"to":{"model":"secondary"}}`, ""},
		{"wrong type", `{"type":"text","from":{"model":"primary"},"to":{"model":"secondary"}}`, ""},
		{"null model", `{"type":"fallback","from":{"model":null},"to":{"model":"secondary"}}`, ""},
		{"nonstring model", `{"type":"fallback","from":{"model":42},"to":{"model":"secondary"}}`, ""},
		{"uppercase keys", `{"Type":"fallback","From":{"Model":"primary"},"To":{"Model":"secondary"}}`, ""},
		{"uppercase type", `{"Type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`, ""},
		{"uppercase from", `{"type":"fallback","From":{"model":"primary"},"to":{"model":"secondary"}}`, ""},
		{"uppercase to", `{"type":"fallback","from":{"model":"primary"},"To":{"model":"secondary"}}`, ""},
		{"uppercase source model", `{"type":"fallback","from":{"Model":"primary"},"to":{"model":"secondary"}}`, ""},
		{"uppercase destination model", `{"type":"fallback","from":{"model":"primary"},"to":{"Model":"secondary"}}`, ""},
		{"unrelated uppercase keys", `{"type":"fallback","from":{"model":"primary","Model":"ignored"},"to":{"model":"secondary"},"Type":"ignored"}`, `{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part := provider.CustomPart("anthropic.fallback")
			part.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(tc.metadata)}}
			before := provider.ReasoningPart("before")
			before.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"sig-1"}`)}}
			after := provider.ReasoningPart("after")
			after.ProviderOptions = provider.ProviderOptions{"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"signature":"sig-2"}`)}}
			p, _, warnings, _, err := buildParams("claude-sonnet-4-6", provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleAssistant, Content: []provider.ContentPart{before, part, after}}}}, false)
			require.NoError(t, err)
			require.Len(t, p.Messages, 1)
			content := p.Messages[0].Content
			if tc.expected != "" {
				require.Empty(t, warnings)
				require.Len(t, content, 3)
				require.NotNil(t, content[1].OfFallback)
				wire, err := json.Marshal(content[1])
				require.NoError(t, err)
				assert.JSONEq(t, tc.expected, string(wire))
				assert.NotContains(t, string(wire), "cache_control")
				assert.NotContains(t, string(wire), "trigger")
			} else {
				require.Len(t, content, 2)
				require.Len(t, warnings, 1)
				assert.Equal(t, provider.WarnOther, warnings[0].Type)
				assert.Equal(t, "anthropic fallback metadata must include from.model and to.model", warnings[0].Message)
			}
			assert.Equal(t, "sig-1", content[0].OfThinking.Signature)
			assert.Equal(t, "sig-2", content[len(content)-1].OfThinking.Signature)
		})
	}
}
