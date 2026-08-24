package main

import (
	"encoding/json"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
)

func init() {
	registerScenario("reset-step", handleResetStep)
}

func handleResetStep(w http.ResponseWriter, _ *http.Request) {
	stream := make(chan aisdk.UIMessageChunk, 14)
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkStart, MessageID: "message-reset"}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkStartStep}
	stream <- aisdk.TextStartChunk("completed")
	stream <- aisdk.TextDeltaChunk("completed", "Completed step")
	stream <- aisdk.TextEndChunk("completed")
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkFinishStep}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkStartStep}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkToolInputStart, ToolCallID: "stale-tool", ToolName: "deleteFile"}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkToolInputDelta, ToolCallID: "stale-tool", InputTextDelta: `{"path":"partial`}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkResetStep}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkToolInputStart, ToolCallID: "retried-tool", ToolName: "deleteFile"}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkToolInputAvailable, ToolCallID: "retried-tool", ToolName: "deleteFile", Input: json.RawMessage(`{"path":"target"}`)}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkFinishStep}
	stream <- aisdk.UIMessageChunk{Type: aisdk.ChunkFinish, FinishReason: "tool-calls"}
	close(stream)
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
