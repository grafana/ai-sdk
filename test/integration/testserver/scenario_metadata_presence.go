package main

import (
	"encoding/json"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() { registerScenario("metadata-presence", handleMetadataPresence) }

func handleMetadataPresence(w http.ResponseWriter, r *http.Request) {
	old := provider.ProviderMetadata{"future": json.RawMessage(`{"old":true}`)}
	empty := provider.ProviderMetadata{}
	replacement := provider.ProviderMetadata{"future": json.RawMessage(`{"new":[null,false,0,"",[],{}]}`)}
	parts := []aisdk.UIMessageChunk{
		{Type: aisdk.ChunkStart, MessageID: "metadata"},
		{Type: aisdk.ChunkTextStart, ID: "text", ProviderMetadata: old},
		{Type: aisdk.ChunkTextDelta, ID: "text", Delta: "answer"},
		{Type: aisdk.ChunkTextDelta, ID: "text", Delta: "", ProviderMetadata: empty},
		{Type: aisdk.ChunkTextEnd, ID: "text"},
		{Type: aisdk.ChunkReasoningStart, ID: "reasoning", ProviderMetadata: old},
		{Type: aisdk.ChunkReasoningDelta, ID: "reasoning", Delta: "thought", ProviderMetadata: replacement},
		{Type: aisdk.ChunkReasoningEnd, ID: "reasoning"},
		{Type: aisdk.ChunkToolInputStart, ToolCallID: "call", ToolName: "weather", ProviderMetadata: old},
		{Type: aisdk.ChunkToolInputAvailable, ToolCallID: "call", ToolName: "weather", Input: json.RawMessage(`{}`), ProviderMetadata: empty},
		{Type: aisdk.ChunkToolOutputAvailable, ToolCallID: "call", Output: json.RawMessage(`"sunny"`), ProviderMetadata: replacement},
		{Type: aisdk.ChunkToolOutputAvailable, ToolCallID: "call", Output: json.RawMessage(`"sunny"`), ProviderMetadata: empty},
		{Type: aisdk.ChunkReasoningFile, MediaType: "image/png", URL: "data:image/png;base64,", ProviderMetadata: empty},
		{Type: aisdk.ChunkSourceURL, SourceID: "source", URL: "https://example.test", ProviderMetadata: empty},
		{Type: aisdk.ChunkFinish},
	}
	stream := make(chan aisdk.UIMessageChunk, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
