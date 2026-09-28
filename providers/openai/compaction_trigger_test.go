package openai

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_CompactionTriggerFollowsHistory(t *testing.T) {
	trigger := true
	encrypted := "encrypted"
	for _, store := range []bool{true, false} {
		t.Run(map[bool]string{true: "stored", false: "stateless"}[store], func(t *testing.T) {
			compaction := provider.CustomPart("openai.compaction")
			compaction.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "cmp_1", EncryptedContent: &encrypted})
			body, warnings := buildBody(t, "gpt-6", provider.CallOptions{
				Prompt: []provider.Message{provider.NewAssistantMessage(compaction), provider.UserText("continue")},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
					Store: &store, ReasoningEffortUpdate: OpenAIReasoningEffortUpdateHigh, CompactionTrigger: &trigger,
				}),
			})
			assert.Empty(t, warnings)
			input := body["input"].([]any)
			require.Len(t, input, 4)
			assert.Equal(t, "configuration_update", input[0].(map[string]any)["type"])
			if store {
				assert.Equal(t, "item_reference", input[1].(map[string]any)["type"])
				assert.Equal(t, "cmp_1", input[1].(map[string]any)["id"])
			} else {
				assert.Equal(t, "compaction", input[1].(map[string]any)["type"])
				assert.Equal(t, "cmp_1", input[1].(map[string]any)["id"])
				assert.Equal(t, encrypted, input[1].(map[string]any)["encrypted_content"])
			}
			assert.Equal(t, "continue", input[2].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
			assert.Equal(t, "compaction_trigger", input[3].(map[string]any)["type"])
		})
	}
}
