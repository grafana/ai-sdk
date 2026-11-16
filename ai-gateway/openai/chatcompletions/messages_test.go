package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatMessageVariants(t *testing.T) {
	raw := `{"model":"alias","messages":[{"role":"developer","content":"rules","name":"policy"},{"role":"user","content":"hello","name":"alice"},{"role":"system","content":"more rules"},{"role":"assistant","refusal":"no"},{"role":"assistant","content":[{"type":"refusal","refusal":"still no"}]},{"role":"assistant","tool_calls":[{"id":"c","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"c","content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}]}`
	r, err := mapRequest([]byte(raw))
	require.NoError(t, err)
	assert.Equal(t, provider.RoleSystem, r.options.Prompt[0].Role)
	assert.Equal(t, "no", r.options.Prompt[3].Content[0].Text)
	assert.Equal(t, "still no", r.options.Prompt[4].Content[0].Text)
	assert.Equal(t, "firstsecond", r.options.Prompt[6].Content[0].Output.Text)
	translated, err := chatTranslator(r)(execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor"}, r.options)
	require.NoError(t, err)
	b, err := json.Marshal(translated.Prompt[0].ProviderOptions)
	require.NoError(t, err)
	assert.JSONEq(t, `{"openaiCompatible":{"name":"policy","role":"developer"}}`, string(b))
	b, err = json.Marshal(translated.Prompt[3].ProviderOptions)
	require.NoError(t, err)
	assert.JSONEq(t, `{"openaiCompatible":{"refusal":"no"}}`, string(b))
	b, err = json.Marshal(translated.Prompt[4].ProviderOptions)
	require.NoError(t, err)
	assert.JSONEq(t, `{"openaiCompatible":{"refusal":"still no"}}`, string(b))
	assert.Empty(t, translated.Prompt[4].Content)
	assert.Equal(t, "still no", r.options.Prompt[4].Content[0].Text)
	assert.Nil(t, r.options.Prompt[0].ProviderOptions)
	for _, role := range []string{"user", "system", "tool"} {
		_, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"` + role + `","content":"x","refusal":"no"}]}`))
		require.Error(t, err)
	}
}
