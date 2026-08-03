package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mergeTestProviderOption struct {
	Nested map[string]any `json:"nested"`
	Keep   string         `json:"keep"`
}

func (mergeTestProviderOption) ProviderKey() string { return "test" }

func TestMergeStepProviderOptions_TypedAndRaw(t *testing.T) {
	base := provider.BuildProviderOptions(mergeTestProviderOption{
		Nested: map[string]any{"left": true, "replace": []string{"base"}},
		Keep:   "value",
	})
	override := provider.ProviderOptions{
		"test":  provider.RawProviderOption{Key: "test", Raw: json.RawMessage(`{"nested":{"right":true,"replace":[]}}`)},
		"other": provider.RawProviderOption{Key: "other", Raw: json.RawMessage(`{"enabled":true}`)},
	}

	merged, err := mergeStepProviderOptions(base, override)
	require.NoError(t, err)
	data, err := json.Marshal(merged)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"test":{"nested":{"left":true,"right":true,"replace":[]},"keep":"value"},
		"other":{"enabled":true}
	}`, string(data))

	baseData, err := json.Marshal(base)
	require.NoError(t, err)
	assert.JSONEq(t, `{"test":{"nested":{"left":true,"replace":["base"]},"keep":"value"}}`, string(baseData))
}
