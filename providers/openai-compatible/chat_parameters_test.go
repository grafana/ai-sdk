package openaicompatible

import (
	"encoding/json"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNativePredictionAndSearch(t *testing.T) {
	m := &model{modelID: "native", providerName: "custom.chat"}
	for _, content := range []string{`"known text"`, `[{"type":"text","text":"known","prompt_cache_breakpoint":{"mode":"explicit","ttl":"12h"}}]`} {
		raw := json.RawMessage(`{"prediction":{"type":"content","content":` + content + `},"web_search_options":{"search_context_size":"high","user_location":{"type":"approximate","approximate":{"city":"Recife","country":"BR","timezone":"America/Recife"}}}}`)
		body, _, err := m.buildRequest(provider.CallOptions{ProviderOptions: provider.ProviderOptions{"custom": provider.RawProviderOption{Key: "custom", Raw: raw}}}, false)
		require.NoError(t, err)
		encoded, err := json.Marshal(body["prediction"])
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"content","content":`+content+`}`, string(encoded))
		require.IsType(t, &ChatWebSearchOptions{}, body["web_search_options"])
	}
	for _, content := range []string{`null`, " \n null\t", `[]`, `[{"type":"image_url","text":"x"}]`, `[{"type":"text","text":"x","unknown":"x"}]`, `[{"type":"text","text":"x","prompt_cache_breakpoint":{"mode":"implicit"}}]`} {
		require.Error(t, (&ChatPrediction{Type: "content", Content: json.RawMessage(content)}).Validate())
	}
	require.Error(t, (&ChatWebSearchOptions{SearchContextSize: "huge"}).Validate())
	require.Error(t, (&ChatWebSearchOptions{UserLocation: &ChatSearchLocation{Type: "exact"}}).Validate())
}
