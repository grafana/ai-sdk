package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeToolMetadata_OpaquePresence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata provider.ProviderMetadata
	}{
		{name: "omitted"},
		{name: "empty", metadata: provider.ProviderMetadata{}},
		{name: "inert semantic fields", metadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"type":"mcp-tool-use","serverName":"unconfigured","caller":null}`),
			"openai":    json.RawMessage(`{"itemId":42,"namespace":null,"caller":{"type":"future"}}`),
		}},
		{name: "extensions", metadata: provider.ProviderMetadata{
			"anthropic": json.RawMessage(`{"caller":{"type":"direct","extension":{"nested":[null,false,{}]}}}`),
			"future":    json.RawMessage(`{"credential":"opaque-not-a-routing-authority","nested":{"unicode":"☃<&>"}}`),
		}},
	} {
		for _, streaming := range []bool{false, true} {
			name := tc.name + "/unary"
			if streaming {
				name = tc.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
					result := validGenerateResult()
					result.Content = []provider.GenerateContentPart{
						{Type: provider.ContentToolCall, ToolCallID: "call", ToolName: "echo", Input: json.RawMessage(`{}`), ProviderExecuted: true, Dynamic: new(false), ProviderMetadata: tc.metadata},
						{Type: provider.ContentToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Dynamic: new(false), Preliminary: new(false), ProviderMetadata: tc.metadata},
					}
					return result, nil
				}
				harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
					return &provider.StreamResult{Stream: makeStream(
						provider.StreamPart{Type: provider.PartToolInputStart, ID: "call", ToolName: "echo", ProviderExecuted: true, Dynamic: new(false), ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolInputDelta, ID: "call", Delta: "{}", ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolInputEnd, ID: "call", ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call", ToolName: "echo", Input: "{}", ProviderExecuted: true, Dynamic: new(false), ProviderMetadata: tc.metadata},
						provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "call", ToolName: "echo", Result: json.RawMessage(`false`), Dynamic: new(false), Preliminary: new(false), ProviderMetadata: tc.metadata},
						finishPart(),
					)}, nil
				}
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request.Header.Set(HeaderStreaming, "true")
				}
				response := harness.serve(request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var parts []json.RawMessage
				if streaming {
					requireStreamBodyMatchesSchema(t, response.Body.String())
					for _, frame := range strings.Split(response.Body.String(), "\n\n") {
						if strings.Contains(frame, `"type":"tool-`) {
							parts = append(parts, json.RawMessage(strings.TrimPrefix(frame, "data: ")))
						}
					}
					require.Len(t, parts, 5)
				} else {
					var body struct {
						Content []json.RawMessage `json:"content"`
					}
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
					parts = body.Content
					require.Len(t, parts, 2)
				}
				for _, part := range parts {
					var fields map[string]json.RawMessage
					require.NoError(t, json.Unmarshal(part, &fields))
					metadata, exists := fields["providerMetadata"]
					assert.Equal(t, tc.metadata != nil, exists)
					if tc.metadata != nil {
						want, err := json.Marshal(tc.metadata)
						require.NoError(t, err)
						assert.JSONEq(t, string(want), string(metadata))
					}
					if string(fields["type"]) == `"tool-call"` || string(fields["type"]) == `"tool-result"` {
						assert.Equal(t, "false", string(fields["dynamic"]))
					}
					if string(fields["type"]) == `"tool-result"` {
						assert.Equal(t, "false", string(fields["preliminary"]))
					}
				}
			})
		}
	}
}
