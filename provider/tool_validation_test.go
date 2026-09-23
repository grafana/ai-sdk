package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTools(t *testing.T) {
	for _, tc := range []struct {
		name      string
		tool      Tool
		wantError bool
	}{
		{name: "nil provider args default to empty", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search"}},
		{name: "selected empty provider args", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{}}},
		{name: "nested provider args", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{"filters": json.RawMessage(`{"names":[null,0,false]}`)}}},
		{name: "malformed provider args", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{"maxUses": json.RawMessage(`{`)}}, wantError: true},
		{name: "empty provider arg value", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Args: map[string]json.RawMessage{"maxUses": nil}}, wantError: true},
		{name: "provider description", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Description: "forbidden"}, wantError: true},
		{name: "provider input schema", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", InputSchema: json.RawMessage(`{}`)}, wantError: true},
		{name: "provider examples", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", InputExamples: []InputExample{}}, wantError: true},
		{name: "provider strict false", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", Strict: boolPtr(false)}, wantError: true},
		{name: "provider options even if empty", tool: Tool{Type: ToolTypeProvider, ID: "anthropic.web_search", Name: "search", ProviderOptions: ProviderOptions{}}, wantError: true},
		{name: "function id", tool: Tool{Type: ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), ID: "anthropic.web_search"}, wantError: true},
		{name: "function args", tool: Tool{Type: ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), Args: map[string]json.RawMessage{}}, wantError: true},
		{name: "function strict false and options", tool: Tool{Type: ToolTypeFunction, Name: "search", InputSchema: json.RawMessage(`{}`), Strict: boolPtr(false), ProviderOptions: ProviderOptions{"anthropic": RawProviderOption{Key: "anthropic", Raw: json.RawMessage(`{}`)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTools([]Tool{tc.tool})
			if tc.wantError {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
