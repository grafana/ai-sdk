package openai

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AssistantTextEncoding(t *testing.T) {
	for _, tc := range []struct {
		name  string
		store bool
		part  OpenAIPartOptions
		text  string
		want  map[string]any
	}{
		{name: "reconstructed", store: true, text: "prior answer", want: map[string]any{"role": "assistant", "content": "prior answer"}},
		{name: "unstored", text: "prior answer", want: map[string]any{"role": "assistant", "content": "prior answer"}},
		{name: "empty", want: map[string]any{"role": "assistant", "content": ""}},
		{name: "unstored item", part: OpenAIPartOptions{ItemID: "msg_1"}, text: "prior answer", want: map[string]any{"role": "assistant", "content": "prior answer"}},
		{name: "commentary", part: OpenAIPartOptions{ItemID: "msg_1", Phase: "commentary"}, text: "prior answer", want: map[string]any{"role": "assistant", "content": "prior answer", "phase": "commentary"}},
		{name: "final answer", part: OpenAIPartOptions{ItemID: "msg_1", Phase: "final_answer"}, text: "prior answer", want: map[string]any{"role": "assistant", "content": "prior answer", "phase": "final_answer"}},
		{name: "stored reference", store: true, part: OpenAIPartOptions{ItemID: "msg_1"}, text: "prior answer", want: map[string]any{"type": "item_reference", "id": "msg_1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{
					Type: provider.ContentPartTypeText, Text: tc.text,
					ProviderOptions: provider.BuildProviderOptions(tc.part),
				})},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &tc.store}),
			})
			input, ok := body["input"].([]any)
			require.True(t, ok)
			require.Len(t, input, 1)
			assert.Equal(t, tc.want, input[0])
		})
	}
}
