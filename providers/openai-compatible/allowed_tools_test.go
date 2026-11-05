package openaicompatible

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowedTools_NativeHTTP(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		options := provider.CallOptions{Prompt: []provider.Message{provider.UserText("test")}, Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "function", InputSchema: json.RawMessage(`{"type":"object"}`)}, {Type: provider.ToolTypeProvider, ID: "openai.custom", Name: "custom"}}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceRequired}, ProviderOptions: provider.BuildProviderOptions(OpenAIOptions{AllowedTools: &AllowedToolsOption{ToolNames: []string{"custom", "function"}, Mode: provider.ToolChoiceRequired}})}
		calls := 0
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			assert.NotContains(t, body, "allowedTools")
			assert.JSONEq(t, `{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"custom","custom":{"name":"custom"}},{"type":"function","function":{"name":"function"}}]}}`, string(body["tool_choice"]))
			var tools []any
			require.NoError(t, json.Unmarshal(body["tools"], &tools))
			assert.Len(t, tools, 2)
			return &http.Response{StatusCode: 418, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic capture"}}`)), Request: req}, nil
		})}
		model := New("native", WithHTTPClient(client))
		var err error
		if streaming {
			_, err = model.DoStream(context.Background(), options)
		} else {
			_, err = model.DoGenerate(context.Background(), options)
		}
		require.Error(t, err)
		assert.Equal(t, 1, calls)
	}
}
func TestAllowedTools_RejectsOverridesAndUnknownNames(t *testing.T) {
	for _, selection := range []AllowedToolsOption{{ToolNames: []string{"missing"}}, {ToolNames: []string{"f", "f"}}, {ToolNames: []string{"f"}, Mode: provider.ToolChoiceNone}, {ToolNames: []string{"f"}, Mode: provider.ToolChoiceRequired}, {Mode: provider.ToolChoiceRequired}} {
		_, _, err := (&model{modelID: "native", providerName: "vendor"}).buildRequest(provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "f", InputSchema: json.RawMessage(`{"type":"object"}`)}}, ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto}, ProviderOptions: provider.BuildProviderOptions(OpenAIOptions{AllowedTools: &selection})}, false)
		require.ErrorIs(t, err, provider.ErrUnsupportedFunctionality)
	}
}
