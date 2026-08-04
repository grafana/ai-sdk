package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

const approvalToolName = "confirm_action"

func init() {
	registerScenario("tool-approval", handleToolApproval)
}

type approvalToolInput struct {
	Action string `json:"action" jsonschema:"description=Action requiring approval"`
}

type approvalToolOutput struct {
	Action   string `json:"action"`
	Executed bool   `json:"executed"`
}

type approvalModel struct {
	responded bool
	approved  bool
}

func (*approvalModel) SpecificationVersion() string               { return "v4" }
func (*approvalModel) Provider() string                           { return "test" }
func (*approvalModel) ModelID() string                            { return "test-tool-approval" }
func (*approvalModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*approvalModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *approvalModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	stream := make(chan provider.StreamPart, 5)
	if !m.responded {
		stream <- provider.StreamPart{
			Type:       provider.PartToolCall,
			ToolCallID: "approval-action-1",
			ToolName:   approvalToolName,
			Input:      `{"action":"deploy"}`,
		}
		stream <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
			Usage:        &provider.Usage{},
		}
	} else {
		text := "The action was denied."
		if m.approved {
			text = "The approved action was executed."
		}
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "approval-result"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "approval-result", Delta: text}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "approval-result"}
		stream <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{},
		}
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleToolApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Messages []aisdk.UIMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	responded, approved := findApprovalResponse(body.Messages)
	tool, err := aisdk.TypedTool(aisdk.TypedToolDef[approvalToolInput, approvalToolOutput]{
		Name:        approvalToolName,
		Description: "Execute an action after user approval.",
		Execute: func(_ context.Context, input approvalToolInput, _ aisdk.ToolExecutionOptions) (approvalToolOutput, error) {
			return approvalToolOutput{Action: input.Action, Executed: true}, nil
		},
	})
	if err != nil {
		http.Error(w, "creating tool", http.StatusInternalServerError)
		return
	}
	tool.NeedsApproval = aisdk.ApprovalRequired()

	agent := aisdk.NewToolLoopAgent(&approvalModel{responded: responded, approved: approved},
		aisdk.WithToolLoopAgentOptions(
			aisdk.WithTools(aisdk.ToolSet{approvalToolName: tool}),
			aisdk.WithStopWhen(aisdk.StepCountIs(5)),
		),
	)
	if err := aisdk.WriteAgentUIStream(w, r.Context(), agent, body.Messages); err != nil && r.Context().Err() == nil {
		http.Error(w, "streaming response", http.StatusInternalServerError)
	}
}

func findApprovalResponse(messages []aisdk.UIMessage) (responded bool, approved bool) {
	for _, message := range messages {
		for _, part := range message.Parts {
			toolPart, ok := part.(aisdk.ToolInvocationPart)
			if !ok || toolPart.ToolName != approvalToolName || toolPart.Approval == nil || toolPart.Approval.Approved == nil {
				continue
			}
			return true, *toolPart.Approval.Approved
		}
	}
	return false, false
}
