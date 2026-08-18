package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultInstructions(t *testing.T) {
	defaultMessage := provider.NewSystemMessage("default instructions")
	defaultMessage.ProviderOptions = provider.ProviderOptions{
		"anthropic": provider.RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`)},
	}

	t.Run("prepends defaults for generate and stream", func(t *testing.T) {
		var prompts [][]provider.Message
		model := &mockModel{
			doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
				prompts = append(prompts, params.Prompt)
				return &provider.GenerateResult{}, nil
			},
			doStream: func(_ context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
				prompts = append(prompts, params.Prompt)
				stream := make(chan provider.StreamPart)
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			},
		}
		wrapped := WrapLanguageModel(model, DefaultInstructions(defaultMessage))
		input := []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))}

		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{Prompt: input})
		require.NoError(t, err)
		_, err = wrapped.DoStream(context.Background(), provider.CallOptions{Prompt: input})
		require.NoError(t, err)

		require.Len(t, prompts, 2)
		for _, prompt := range prompts {
			require.Len(t, prompt, 2)
			assert.Equal(t, provider.RoleSystem, prompt[0].Role)
			assert.Equal(t, "default instructions", prompt[0].Content[0].Text)
			assert.Contains(t, prompt[0].ProviderOptions, "anthropic")
			assert.Equal(t, provider.RoleUser, prompt[1].Role)
		}
		assert.Equal(t, []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))}, input)
	})

	t.Run("existing system message takes precedence", func(t *testing.T) {
		var received provider.CallOptions
		model := &mockModel{doGenerate: func(_ context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
			received = params
			return &provider.GenerateResult{}, nil
		}}
		wrapped := WrapLanguageModel(model, DefaultInstructions(defaultMessage))
		prompt := []provider.Message{
			provider.NewUserMessage(provider.TextPart("hello")),
			provider.NewSystemMessage("trusted instructions"),
		}

		_, err := wrapped.DoGenerate(context.Background(), provider.CallOptions{Prompt: prompt})
		require.NoError(t, err)
		assert.Equal(t, prompt, received.Prompt)
	})

	t.Run("empty defaults preserve prompt", func(t *testing.T) {
		params := provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.TextPart("hello"))}}
		result, err := DefaultInstructions().TransformParams(context.Background(), TransformParamsInput{Params: params})
		require.NoError(t, err)
		assert.Equal(t, params, result)
	})
}
