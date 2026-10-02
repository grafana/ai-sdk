package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIMessageMetadata_Presence(t *testing.T) {
	for _, chunkType := range []ChunkType{ChunkTextStart, ChunkTextDelta, ChunkTextEnd, ChunkReasoningStart, ChunkReasoningDelta, ChunkReasoningEnd, ChunkReasoningFile, ChunkToolInputStart, ChunkToolInputAvailable, ChunkToolInputError, ChunkToolOutputAvailable, ChunkToolOutputError, ChunkSourceURL, ChunkSourceDocument} {
		t.Run(string(chunkType), func(t *testing.T) {
			raw, err := json.Marshal(UIMessageChunk{Type: chunkType, ProviderMetadata: provider.ProviderMetadata{}})
			require.NoError(t, err)
			assert.Contains(t, string(raw), `"providerMetadata":{}`)
			raw, err = json.Marshal(UIMessageChunk{Type: chunkType})
			require.NoError(t, err)
			assert.NotContains(t, string(raw), `"providerMetadata"`)
		})
	}
	for _, part := range []Part{
		TextPart{Text: "answer", ProviderMetadata: provider.ProviderMetadata{}},
		ReasoningPart{Text: "thought", ProviderMetadata: provider.ProviderMetadata{}},
		ReasoningFilePart{MediaType: "image/png", URL: "data:image/png;base64,", ProviderMetadata: provider.ProviderMetadata{}},
		SourceURLPart{SourceID: "source", URL: "https://example.test", ProviderMetadata: provider.ProviderMetadata{}},
		SourceDocumentPart{SourceID: "document", MediaType: "text/plain", Title: "Document", ProviderMetadata: provider.ProviderMetadata{}},
		FilePart{MediaType: "image/png", URL: "data:image/png;base64,", ProviderMetadata: provider.ProviderMetadata{}},
		DynamicToolUIPart{ToolName: "weather", ToolCallID: "dynamic", CallProviderMetadata: provider.ProviderMetadata{}, ResultProviderMetadata: provider.ProviderMetadata{}},
		ToolInvocationPart{ToolName: "weather", ToolCallID: "call", CallProviderMetadata: provider.ProviderMetadata{}, ResultProviderMetadata: provider.ProviderMetadata{}},
	} {
		raw, err := json.Marshal(UIMessage{ID: "message", Role: RoleAssistant, Parts: []Part{part}})
		require.NoError(t, err)
		var persisted UIMessage
		require.NoError(t, json.Unmarshal(raw, &persisted))
		assert.Equal(t, part, persisted.Parts[0])
	}
}

func TestUIMessageMetadata_AssemblyAndModelHistory(t *testing.T) {
	old := provider.ProviderMetadata{"future": json.RawMessage(`{"old":true}`)}
	empty := provider.ProviderMetadata{}
	input := []UIMessageChunk{
		{Type: ChunkStart, MessageID: "metadata"},
		{Type: ChunkTextStart, ID: "text", ProviderMetadata: old},
		{Type: ChunkTextDelta, ID: "text", Delta: "answer"},
		{Type: ChunkTextDelta, ID: "text", ProviderMetadata: empty},
		{Type: ChunkTextEnd, ID: "text"},
		{Type: ChunkToolInputStart, ToolCallID: "call", ToolName: "weather", ProviderMetadata: old},
		{Type: ChunkToolInputAvailable, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`), ProviderMetadata: empty},
		{Type: ChunkToolOutputAvailable, ToolCallID: "call", Output: json.RawMessage(`"sunny"`), ProviderMetadata: old},
		{Type: ChunkToolOutputAvailable, ToolCallID: "call", Output: json.RawMessage(`"sunny"`), ProviderMetadata: empty},
		{Type: ChunkFinish},
	}
	for i, chunk := range input {
		raw, err := json.Marshal(chunk)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &input[i]))
	}
	message, err := AssembleUIMessage(chunks(input...))
	require.NoError(t, err)
	raw, err := json.Marshal(message)
	require.NoError(t, err)
	var persisted UIMessage
	require.NoError(t, json.Unmarshal(raw, &persisted))
	assert.Equal(t, message.Parts, persisted.Parts)
	history, err := ConvertToModelMessages([]UIMessage{persisted})
	require.NoError(t, err)
	require.Len(t, history, 2)
	require.Len(t, history[0].Content, 2)
	for _, part := range history[0].Content {
		assert.NotNil(t, part.ProviderOptions)
		assert.Empty(t, part.ProviderOptions)
	}
	require.Len(t, history[1].Content, 1)
	assert.NotNil(t, history[1].Content[0].ProviderOptions)
	assert.Empty(t, history[1].Content[0].ProviderOptions)
}

func TestProviderMetadataToOptions_Presence(t *testing.T) {
	assert.Nil(t, providerMetadataToOptions(nil))
	assert.NotNil(t, providerMetadataToOptions(provider.ProviderMetadata{}))
	assert.Nil(t, optionsToProviderMetadata(nil))
	assert.NotNil(t, optionsToProviderMetadata(provider.ProviderOptions{}))
	assert.Nil(t, optionsToProviderMetadata(provider.ProviderOptions{"future": nil}))
}
