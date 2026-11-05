package nativeoptions

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	compatible "github.com/grafana/ai-sdk/providers/openai-compatible"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCompatibleTokenLimitGuard(t *testing.T) {
	options := provider.CallOptions{ProviderOptions: provider.BuildProviderOptions(compatible.OpenAIOptions{MaxTokensField: compatible.MaxTokensFieldCompletion})}
	require.NoError(t, Compatible(options, "vendor.chat"))
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		options.ProviderOptions = provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(`{"maxTokensField":"max_completion_tokens","` + field + `":1000000}`)}}
		require.Error(t, Compatible(options, "vendor.chat"))
	}
	options.ProviderOptions = provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(`{"Max_tokens":1,"MAX_COMPLETION_TOKENS":2}`)}}
	require.NoError(t, Compatible(options, "vendor.chat"))
}
