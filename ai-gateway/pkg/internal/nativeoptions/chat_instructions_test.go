package nativeoptions

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompatibleInstructionRole(t *testing.T) {
	for _, tc := range []struct {
		role    provider.Role
		native  string
		allowed bool
	}{
		{provider.RoleSystem, `{"role":"developer"}`, true},
		{provider.RoleSystem, `{"role":"tool"}`, false},
		{provider.RoleSystem, `{"role":"assistant"}`, false},
		{provider.RoleSystem, `{"role":null}`, false},
		{provider.RoleSystem, `{"role":"developer","content":"replacement"}`, false},
		{provider.RoleAssistant, `{"role":"developer"}`, false},
		{provider.RoleUser, `{"role":"developer"}`, false},
	} {
		t.Run(string(tc.role)+tc.native, func(t *testing.T) {
			opts := provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(tc.native)}}
			content := provider.TextPart("text")
			content.ProviderOptions = opts
			err := Compatible(provider.CallOptions{Prompt: []provider.Message{{Role: tc.role, Content: []provider.ContentPart{content}, ProviderOptions: opts}}}, "custom")
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, tc.native, string(opts["openaiCompatible"].(provider.RawProviderOption).Raw))
		})
	}
}
