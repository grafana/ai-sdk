package grafana

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLanguageModel_RequestSelectorBounds(t *testing.T) {
	client, err := NewWithCloudCredentials(CloudCredentialsConfig{StackID: 123, CAPToken: "dummy-cap", BaseURL: "https://gateway.invalid/api/v1/aisdk"})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, id string
		valid    bool
	}{
		{"configured", "grafana/assistant", true},
		{"native suffix", "openai/native@revision+suffix/part", true},
		{"UTF8 suffix", "anthropic/モデル", true},
		{"exact byte limit", "openai/" + strings.Repeat("x", 2048-len("openai/")), true},
		{"over byte limit", "openai/" + strings.Repeat("x", 2049-len("openai/")), false},
		{"empty", "", false},
		{"whitespace", "openai/not valid", false},
		{"control", "openai/not\nvalid", false},
		{"invalid UTF8", string([]byte{255}), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, err := client.LanguageModel(tc.id)
			if !tc.valid {
				require.Error(t, err)
				assert.Nil(t, model)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.id, model.ModelID())
		})
	}
}
