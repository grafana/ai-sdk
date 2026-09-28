package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AsyncToolCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		async       *bool
		wantWarning bool
	}{
		{name: "supported", model: "gpt-6-astra", async: boolPointer(true)},
		{name: "unsupported", model: "gpt-5.6", async: boolPointer(true), wantWarning: true},
		{name: "mantle requires verification", model: "openai.gpt-6-astra", async: boolPointer(true), wantWarning: true},
		{name: "explicit false", model: "gpt-5.6", async: boolPointer(false)},
		{name: "absent", model: "gpt-6-astra"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			customArgs := map[string]json.RawMessage{}
			if tc.async != nil {
				value, err := json.Marshal(tc.async)
				require.NoError(t, err)
				customArgs["async"] = value
			}
			body, warnings := buildBody(t, tc.model, provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
				Tools: []provider.Tool{
					{Type: provider.ToolTypeFunction, Name: "lookup", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{Async: tc.async, AllowedCallers: []OpenAIAllowedCaller{OpenAIAllowedCallerDirect}})},
					{Type: provider.ToolTypeFunction, Name: "search", ProviderOptions: provider.BuildProviderOptions(OpenAIToolOptions{Async: tc.async, Namespace: &OpenAIToolNamespaceOptions{Name: "utilities"}})},
					{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql", Args: customArgs},
				},
			})
			tools := toolsArray(t, body)
			require.Len(t, tools, 3)
			fn := tools[0]
			namespace := tools[1]["tools"].([]any)[0].(map[string]any)
			custom := tools[2]
			assert.Equal(t, []any{"direct"}, fn["allowed_callers"])
			for _, tool := range []map[string]any{fn, namespace, custom} {
				if tc.async != nil && (!*tc.async || !tc.wantWarning) {
					assert.Equal(t, *tc.async, tool["async"])
				} else {
					assert.NotContains(t, tool, "async")
				}
			}
			if tc.wantWarning {
				for _, name := range []string{"lookup", "search", "write_sql"} {
					assert.Contains(t, warningFeatures(warnings), `async tool calling for "`+name+`"`)
				}
			} else {
				assert.Empty(t, warnings)
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
