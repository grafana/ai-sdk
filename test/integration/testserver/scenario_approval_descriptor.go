package main

import (
	"encoding/json"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
)

func init() {
	registerScenario("approval-descriptor", handleApprovalDescriptor)
}

func handleApprovalDescriptor(w http.ResponseWriter, _ *http.Request) {
	stream := make(chan aisdk.UIMessageChunk, 3)
	stream <- aisdk.UIMessageChunk{
		Type:       aisdk.ChunkToolInputAvailable,
		ToolCallID: "call-1",
		ToolName:   "deleteAccount",
		Input:      json.RawMessage(`{"userId":"user-123"}`),
	}
	stream <- aisdk.UIMessageChunk{
		Type:               aisdk.ChunkToolApprovalRequest,
		ApprovalID:         "approval-1",
		ToolCallID:         "call-1",
		ApprovalDescriptor: json.RawMessage(`{"action":"deleteAccount","permissions":["account:delete"],"risk":"high"}`),
	}
	stream <- aisdk.UIMessageChunk{
		Type:       aisdk.ChunkToolApprovalResponse,
		ApprovalID: "approval-1",
		Approved:   true,
	}
	close(stream)

	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil {
		http.Error(w, "streaming response", http.StatusInternalServerError)
	}
}
