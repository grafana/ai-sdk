package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPartRoundTrip(t *testing.T) {
	t.Run("text part", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				TextPart{Text: "hello", State: "done"},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)

		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, "msg-1", got.ID)
		assert.Equal(t, RoleAssistant, got.Role)

		tp, ok := got.Parts[0].(TextPart)
		require.True(t, ok, "expected TextPart, got %T", got.Parts[0])
		assert.Equal(t, "hello", tp.Text)
		assert.Equal(t, "done", tp.State)
	})

	t.Run("reasoning part preserves provider metadata", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				ReasoningPart{
					Text:             "thinking...",
					State:            "done",
					ProviderMetadata: provider.ProviderMetadata{"anthropic": json.RawMessage(`{"cacheTokens":100}`)},
				},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)

		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		rp, ok := got.Parts[0].(ReasoningPart)
		require.True(t, ok, "expected ReasoningPart, got %T", got.Parts[0])
		assert.Equal(t, "thinking...", rp.Text)
		assert.Equal(t, "done", rp.State)
		assert.NotNil(t, rp.ProviderMetadata["anthropic"])
	})

	t.Run("empty assistant message survives round-trip", func(t *testing.T) {
		msg := UIMessage{ID: "msg-empty", Role: RoleAssistant, Parts: []Part{}}
		encoded, err := json.Marshal(msg)
		require.NoError(t, err)
		assert.JSONEq(t, `{"id":"msg-empty","role":"assistant","parts":[]}`, string(encoded))

		var got UIMessage
		require.NoError(t, json.Unmarshal(encoded, &got))
		assert.Equal(t, RoleAssistant, got.Role)
		assert.Empty(t, got.Parts)
	})

	t.Run("unknown type survives round-trip", func(t *testing.T) {
		data := `{"id":"msg-1","role":"assistant","parts":[{"type":"future-type","foo":"bar"}]}`
		var got UIMessage
		require.NoError(t, json.Unmarshal([]byte(data), &got))
		assert.Equal(t, "future-type", got.Parts[0].PartType())

		b, err := json.Marshal(got)
		require.NoError(t, err)

		var roundTripped UIMessage
		require.NoError(t, json.Unmarshal(b, &roundTripped))
		assert.Equal(t, "future-type", roundTripped.Parts[0].PartType())
	})
}

func TestPartTypePrefix(t *testing.T) {
	t.Run("tool invocation uses tool-<name> prefix", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				ToolInvocationPart{
					ToolCallID: "call-1",
					ToolName:   "weather",
					State:      ToolStateOutputAvailable,
					Input:      json.RawMessage(`{"city":"NYC"}`),
					Output:     json.RawMessage(`{"temp":72}`),
				},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)

		typStr := extractPartType(t, b)
		assert.Equal(t, "tool-weather", typStr)

		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		tip, ok := got.Parts[0].(ToolInvocationPart)
		require.True(t, ok, "expected ToolInvocationPart, got %T", got.Parts[0])
		assert.Equal(t, "weather", tip.ToolName)
		assert.Equal(t, ToolStateOutputAvailable, tip.State)
	})

	t.Run("data part uses data-<name> prefix", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				DataPart{DataName: "status", ID: "d1", Data: json.RawMessage(`{"step":"done"}`)},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)

		typStr := extractPartType(t, b)
		assert.Equal(t, "data-status", typStr)

		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		dp, ok := got.Parts[0].(DataPart)
		require.True(t, ok, "expected DataPart, got %T", got.Parts[0])
		assert.Equal(t, "status", dp.DataName)
		assert.Equal(t, "d1", dp.ID)
	})

	t.Run("dynamic tool part uses dynamic-tool type", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				DynamicToolUIPart{
					ToolCallID: "call-1",
					ToolName:   "mcp_search",
					State:      ToolStateInputAvailable,
					Input:      json.RawMessage(`{"q":"test"}`),
				},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)

		typStr := extractPartType(t, b)
		assert.Equal(t, "dynamic-tool", typStr)

		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		dtp, ok := got.Parts[0].(DynamicToolUIPart)
		require.True(t, ok, "expected DynamicToolUIPart, got %T", got.Parts[0])
		assert.Equal(t, "mcp_search", dtp.ToolName)
	})
}

func TestFilePart_ProviderReferenceJSONRoundTrip(t *testing.T) {
	msg := UIMessage{
		ID:   "msg-1",
		Role: RoleUser,
		Parts: []Part{FilePart{
			MediaType:         "application/pdf",
			URL:               "data:application/pdf;base64,abc",
			Filename:          "doc.pdf",
			ProviderReference: map[string]string{"openai": "file-abc123"},
		}},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"msg-1","role":"user","parts":[{"type":"file","mediaType":"application/pdf","url":"data:application/pdf;base64,abc","filename":"doc.pdf","providerReference":{"openai":"file-abc123"}}]}`, string(data))

	var got UIMessage
	require.NoError(t, json.Unmarshal(data, &got))
	require.Len(t, got.Parts, 1)
	part, ok := got.Parts[0].(FilePart)
	require.True(t, ok)
	assert.Equal(t, map[string]string{"openai": "file-abc123"}, part.ProviderReference)
}

func TestFilePart_EmptyProviderReferenceJSONRoundTrip(t *testing.T) {
	msg := UIMessage{
		ID:   "msg-1",
		Role: RoleUser,
		Parts: []Part{FilePart{
			MediaType:         "application/pdf",
			URL:               "https://example.com/doc.pdf",
			ProviderReference: map[string]string{},
		}},
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"msg-1","role":"user","parts":[{"type":"file","mediaType":"application/pdf","url":"https://example.com/doc.pdf","providerReference":{}}]}`, string(data))

	var got UIMessage
	require.NoError(t, json.Unmarshal(data, &got))
	part := got.Parts[0].(FilePart)
	assert.NotNil(t, part.ProviderReference)
	assert.Empty(t, part.ProviderReference)
}

func TestUnknownTypeFallback(t *testing.T) {
	data := `{"id":"msg-1","role":"assistant","parts":[{"type":"future-type","foo":"bar"}]}`
	var got UIMessage
	require.NoError(t, json.Unmarshal([]byte(data), &got))
	require.Len(t, got.Parts, 1)
	assert.Equal(t, "future-type", got.Parts[0].PartType())
}

func TestToolNameConflictReturnsError(t *testing.T) {
	data := `{"id":"msg-1","role":"assistant","parts":[{"type":"tool-foo","toolCallId":"c1","toolName":"bar","state":"input-available"}]}`
	var got UIMessage
	err := json.Unmarshal([]byte(data), &got)
	assert.Error(t, err, "expected error for conflicting tool type prefix and toolName field")
}

func TestAllPartTypes(t *testing.T) {
	msg := UIMessage{
		ID:   "msg-1",
		Role: RoleAssistant,
		Parts: []Part{
			TextPart{Text: "hi"},
			ReasoningPart{Text: "think"},
			ToolInvocationPart{ToolCallID: "c1", ToolName: "w", State: ToolStateInputAvailable},
			DynamicToolUIPart{ToolCallID: "c2", ToolName: "d", State: ToolStateInputAvailable},
			FilePart{MediaType: "image/png", URL: "https://example.com/img.png"},
			ReasoningFilePart{MediaType: "image/png", URL: "https://example.com/reasoning.png"},
			SourceURLPart{SourceID: "s1", URL: "https://example.com"},
			SourceDocumentPart{SourceID: "s2", MediaType: "application/pdf"},
			DataPart{DataName: "x", Data: json.RawMessage(`{}`)},
			StepStartPart{},
		},
	}
	b, err := json.Marshal(msg)
	require.NoError(t, err)

	var got UIMessage
	require.NoError(t, json.Unmarshal(b, &got))
	require.Len(t, got.Parts, 10)

	expectedTypes := []string{
		"text", "reasoning", "tool-w", "dynamic-tool",
		"file", "reasoning-file", "source-url", "source-document", "data-x", "step-start",
	}
	for i, p := range got.Parts {
		assert.Equal(t, expectedTypes[i], p.PartType(), "part %d", i)
	}
}

func TestPartTypeWireCompat(t *testing.T) {
	t.Run("DataPart returns data-<name>", func(t *testing.T) {
		assert.Equal(t, "data-usage", DataPart{DataName: "usage"}.PartType())
	})
	t.Run("DataPart with empty DataName", func(t *testing.T) {
		assert.Equal(t, "data-", DataPart{}.PartType())
	})
	t.Run("ToolInvocationPart returns tool-<name>", func(t *testing.T) {
		assert.Equal(t, "tool-search", ToolInvocationPart{ToolName: "search"}.PartType())
	})
	t.Run("ToolInvocationPart with empty ToolName", func(t *testing.T) {
		assert.Equal(t, "tool-", ToolInvocationPart{}.PartType())
	})
	t.Run("DataPart round-trip preserves PartType", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				DataPart{DataName: "usage", Data: json.RawMessage(`{"tokens":100}`)},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)
		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, "data-usage", got.Parts[0].PartType())
	})
	t.Run("ToolInvocationPart round-trip preserves PartType", func(t *testing.T) {
		msg := UIMessage{
			ID:   "msg-1",
			Role: RoleAssistant,
			Parts: []Part{
				ToolInvocationPart{ToolCallID: "c1", ToolName: "searchWeb", State: ToolStateInputAvailable},
			},
		}
		b, err := json.Marshal(msg)
		require.NoError(t, err)
		var got UIMessage
		require.NoError(t, json.Unmarshal(b, &got))
		assert.Equal(t, "tool-searchWeb", got.Parts[0].PartType())
	})
}

func extractPartType(t *testing.T, msgJSON []byte) string {
	t.Helper()
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(msgJSON, &raw))
	var parts []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw["parts"], &parts))
	var typStr string
	require.NoError(t, json.Unmarshal(parts[0]["type"], &typStr))
	return typStr
}
