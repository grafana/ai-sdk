package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestReviewApprovalNativeReemitsHistoricalCall(t *testing.T) {
	h := newRuntimeHarness(t, testLimits())
	h.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		r := validGenerateResult()
		r.Content = []provider.GenerateContentPart{
			{Type: provider.ContentToolCall, ToolCallID: "prior-call", ToolName: "mcp.lookup", Input: json.RawMessage(`{}`), ProviderExecuted: true},
			{Type: provider.ContentToolResult, ToolCallID: "prior-call", ToolName: "mcp.lookup", Result: json.RawMessage(`{"output":"done"}`)},
		}
		return r, nil
	}
	r := h.serve(validRequest(`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"prior-call","toolName":"mcp.lookup","input":{},"providerExecuted":true,"providerOptions":{"openai":{"approvalRequestId":"a"}}}]},{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":true}]}]}`))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
}

func TestReviewApprovalThirdTurnAcceptsReemittedHistory(t *testing.T) {
	h := newRuntimeHarness(t, testLimits())
	r := h.serve(validRequest(`{"prompt":[{"role":"assistant","content":[{"type":"tool-call","toolCallId":"prior-call","toolName":"mcp.lookup","input":{},"providerExecuted":true,"providerOptions":{"openai":{"approvalRequestId":"a"}}}]},{"role":"tool","content":[{"type":"tool-approval-response","approvalId":"a","approved":true}]},{"role":"assistant","content":[{"type":"tool-call","toolCallId":"prior-call","toolName":"mcp.lookup","input":{},"providerExecuted":true},{"type":"tool-result","toolCallId":"prior-call","toolName":"mcp.lookup","output":{"type":"json","value":{"output":"done"}}}]},{"role":"user","content":[{"type":"text","text":"continue"}]}]}`))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
}
