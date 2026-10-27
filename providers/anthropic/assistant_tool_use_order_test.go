package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildParams_AssistantToolUseOrder(t *testing.T) {
	// Synthetic request snapshots mirroring upstream moveToolUseBlocksToEnd in
	// @ai-sdk/anthropic 4.0.71; no recorded provider traffic covers this shape.
	thinking := func(text, signature string) provider.ContentPart {
		part := provider.ReasoningPart(text)
		part.ProviderOptions = makeProviderOpts(`{"signature":"` + signature + `"}`)
		return part
	}
	redacted := func(data string) provider.ContentPart {
		part := provider.ReasoningPart("")
		part.ProviderOptions = makeProviderOpts(`{"redactedData":"` + data + `"}`)
		return part
	}
	fallback := func() provider.ContentPart {
		part := provider.CustomPart(anthropicFallbackKind)
		part.ProviderOptions = makeProviderOpts(`{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`)
		return part
	}
	call := func(id string) provider.ContentPart {
		return provider.ToolCallPart(id, "saveNote", json.RawMessage(`{"id":"`+id+`"}`))
	}
	assistant := func(parts ...provider.ContentPart) provider.Message {
		return provider.NewAssistantMessage(parts...)
	}

	const (
		textX       = `{"type":"text","text":"x"}`
		textY       = `{"type":"text","text":"y"}`
		toolUseA    = `{"type":"tool_use","id":"a","name":"saveNote","input":{"id":"a"}}`
		toolUseB    = `{"type":"tool_use","id":"b","name":"saveNote","input":{"id":"b"}}`
		toolUseC    = `{"type":"tool_use","id":"c","name":"saveNote","input":{"id":"c"}}`
		thinking1   = `{"type":"thinking","thinking":"t1","signature":"sig-1"}`
		thinking2   = `{"type":"thinking","thinking":"t2","signature":"sig-2"}`
		redactedR   = `{"type":"redacted_thinking","data":"r"}`
		fallbackHop = `{"type":"fallback","from":{"model":"primary"},"to":{"model":"secondary"}}`
	)

	tests := []struct {
		name     string
		prompt   func() []provider.Message
		expected []string
	}{
		{
			name: "fallback boundary",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(call("a"), provider.TextPart("x"), fallback(), call("b"), provider.TextPart("y"))}
			},
			expected: []string{textX, toolUseA, fallbackHop, textY, toolUseB},
		},
		{
			name: "grouped consecutive assistant messages normalize as one block",
			prompt: func() []provider.Message {
				return []provider.Message{
					assistant(call("a"), provider.TextPart("x")),
					assistant(call("b"), provider.TextPart("y")),
				}
			},
			expected: []string{textX, textY, toolUseA, toolUseB},
		},
		{
			name: "thinking boundaries keep calls in their segments",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(thinking("t1", "sig-1"), call("a"), thinking("t2", "sig-2"), call("b"))}
			},
			expected: []string{thinking1, toolUseA, thinking2, toolUseB},
		},
		{
			name: "leading consecutive and trailing boundaries",
			prompt: func() []provider.Message {
				return []provider.Message{
					assistant(thinking("t1", "sig-1"), redacted("r"), call("a"), provider.TextPart("x")),
					assistant(call("b"), fallback(), thinking("t2", "sig-2")),
				}
			},
			expected: []string{thinking1, redactedR, textX, toolUseA, toolUseB, fallbackHop, thinking2},
		},
		{
			name: "redacted thinking boundary",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(call("a"), redacted("r"), provider.TextPart("x"))}
			},
			expected: []string{toolUseA, redactedR, textX},
		},
		{
			name: "final assistant text is trimmed before moving calls",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(call("a"), provider.TextPart("x  \n"))}
			},
			expected: []string{textX, toolUseA},
		},
		{
			name: "stable order of multiple calls and non-call blocks",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(call("a"), call("b"), provider.TextPart("x"), call("c"), provider.TextPart("y"))}
			},
			expected: []string{textX, textY, toolUseA, toolUseB, toolUseC},
		},
		{
			name: "already normalized content is unchanged",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(provider.TextPart("x"), call("a"), call("b"))}
			},
			expected: []string{textX, toolUseA, toolUseB},
		},
		{
			name: "content without client calls is unchanged",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(thinking("t1", "sig-1"), provider.TextPart("x"), provider.TextPart("y"))}
			},
			expected: []string{thinking1, textX, textY},
		},
		{
			name: "server tool blocks stay in the non-client partition",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(
					provider.TextPart("x"),
					call("a"),
					provider.ContentPart{Type: provider.ContentPartTypeToolCall, ToolCallID: "srv", ToolName: "web_search", Input: json.RawMessage(`{"query":"news"}`), ProviderExecuted: true},
					provider.ContentPart{Type: provider.ContentPartTypeToolResult, ToolCallID: "srv", ToolName: "web_search", ProviderExecuted: true, Output: &provider.ToolResultOutput{
						Type: provider.ToolOutputJSON,
						JSON: json.RawMessage(`[{"type":"web_search_result","url":"https://example.com","title":"Example","encryptedContent":"enc","pageAge":"1 hour ago"}]`),
					}},
				)}
			},
			expected: []string{
				textX,
				`{"type":"server_tool_use","id":"srv","name":"web_search","input":{"query":"news"}}`,
				`{"type":"web_search_tool_result","tool_use_id":"srv","content":[{"type":"web_search_result","url":"https://example.com","title":"Example","encrypted_content":"enc","page_age":"1 hour ago"}]}`,
				toolUseA,
			},
		},
		{
			name: "mcp tool use stays in the non-client partition",
			prompt: func() []provider.Message {
				return []provider.Message{assistant(
					call("a"),
					provider.ContentPart{Type: provider.ContentPartTypeToolCall, ToolCallID: "mcp", ToolName: "echo", Input: json.RawMessage(`{}`), ProviderExecuted: true, ProviderOptions: makeProviderOpts(`{"type":"mcp-tool-use","serverName":"srv"}`)},
				)}
			},
			expected: []string{
				`{"type":"mcp_tool_use","id":"mcp","name":"echo","input":{},"server_name":"srv"}`,
				toolUseA,
			},
		},
		{
			name: "cache control moves with its block",
			prompt: func() []provider.Message {
				return []provider.Message{
					{Role: provider.RoleAssistant, Content: []provider.ContentPart{call("a")}, ProviderOptions: makeProviderOpts(`{"cacheControl":{"type":"ephemeral"}}`)},
					assistant(provider.TextPart("x")),
				}
			},
			expected: []string{textX, `{"type":"tool_use","id":"a","name":"saveNote","input":{"id":"a"},"cache_control":{"type":"ephemeral"}}`},
		},
	}

	for _, tc := range tests {
		for name, caps := range map[string]providerCapabilities{"direct": directProviderCapabilities, "vertex": vertexProviderCapabilities} {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%t", tc.name, name, stream), func(t *testing.T) {
					prompt := tc.prompt()
					p, _, warnings, _, err := buildParamsWithCapabilities("claude-sonnet-4-6", provider.CallOptions{Prompt: prompt}, stream, caps)
					require.NoError(t, err)
					assert.Empty(t, warnings)
					assert.Equal(t, tc.prompt(), prompt)
					require.Len(t, p.Messages, 1)
					body, err := json.Marshal(p.Messages[0].Content)
					require.NoError(t, err)
					assert.JSONEq(t, "["+strings.Join(tc.expected, ",")+"]", string(body))
				})
			}
		}
	}
}

func TestMoveToolUseBlocksToEnd(t *testing.T) {
	t.Run("empty content", func(t *testing.T) {
		assert.Empty(t, moveToolUseBlocksToEnd(nil))
	})

	t.Run("input is not mutated", func(t *testing.T) {
		toolUse := sdk.BetaContentBlockParamUnion{OfToolUse: &sdk.BetaToolUseBlockParam{ID: "a"}}
		text := sdk.BetaContentBlockParamUnion{OfText: &sdk.BetaTextBlockParam{Text: "x"}}
		content := []sdk.BetaContentBlockParamUnion{toolUse, text}

		result := moveToolUseBlocksToEnd(content)

		assert.Equal(t, []sdk.BetaContentBlockParamUnion{text, toolUse}, result)
		assert.Equal(t, []sdk.BetaContentBlockParamUnion{toolUse, text}, content)
	})
}
