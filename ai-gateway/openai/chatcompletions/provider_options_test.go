package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderOptions_PreserveNamespacesAndDefaults(t *testing.T) {
	r := requestWith(t, `,"parallel_tool_calls":false,"providerOptions":{"gateway":{"byok":{"openai":[{"apiKey":"synthetic-key"}]}},"openai":{"user":"chat-user","store":true,"unknown":{"null":null,"zero":0,"false":false,"large":9007199254740993}},"anthropic":{"container":{"id":"container"}},"custom":{"model":"blocked-override"}}`)
	original, err := json.Marshal(r.options.ProviderOptions)
	require.NoError(t, err)
	for _, target := range []execution.Target{{Type: execution.TargetOpenAI}, {Type: execution.TargetAnthropic}, {Type: execution.TargetOpenAICompatible, ProviderName: "custom.chat"}} {
		t.Run(string(target.Type), func(t *testing.T) {
			options, err := chatTranslator(r)(target, r.options)
			require.NoError(t, err)
			data, err := json.Marshal(options.ProviderOptions)
			require.NoError(t, err)
			assert.Contains(t, string(data), `"user":"chat-user"`)
			assert.Contains(t, string(data), `"unknown":{"false":false,"large":9007199254740993,"null":null,"zero":0}`)
			assert.Contains(t, string(data), `"model":"blocked-override"`)
			if target.Type == execution.TargetOpenAI {
				assert.Contains(t, string(data), `"store":false`)
			}
			after, err := json.Marshal(r.options.ProviderOptions)
			require.NoError(t, err)
			assert.Equal(t, original, after)
		})
	}
}

func TestProviderOptions_RejectNonObjects(t *testing.T) {
	for _, value := range []string{`null`, `[]`, `1`, `"value"`, `{"openai":null}`, `{"openai":[]}`, `{"gateway":"secret"}`} {
		_, err := mapRequest([]byte(`{"model":"alias","messages":[{"role":"user","content":"hello"}],"providerOptions":` + value + `}`))
		assert.Error(t, err, value)
	}
}
