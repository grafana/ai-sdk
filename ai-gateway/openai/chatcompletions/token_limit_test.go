package chatcompletions

import (
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChatTokenLimitNativeField(t *testing.T) {
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		r := requestWith(t, `,"`+field+`":27`)
		target := execution.Target{Type: execution.TargetOpenAICompatible, ProviderName: "vendor.chat"}
		options, err := chatTranslator(r)(target, r.options)
		require.NoError(t, err)
		for _, streaming := range []bool{false, true} {
			body := captureMediaRequest(t, target, options, streaming)
			// The capture helper lowers the common budget to 32; the selector must not
			// carry an independent value that could override that effective budget.
			assert.JSONEq(t, "32", string(body[field]))
			other := "max_tokens"
			if field == other {
				other = "max_completion_tokens"
			}
			assert.NotContains(t, body, other)
			assert.NotContains(t, body, "maxTokensField")
		}
		assert.Equal(t, 27, *r.options.MaxOutputTokens)
	}
}
