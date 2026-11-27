package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/openai"
	"github.com/grafana/ai-sdk/schema"
	"github.com/openai/openai-go/v3/option"
)

const (
	hostedApprovalBaseURL = "https://responses.invalid/v1"
	hostedApprovalTool    = "mcp.create_short_url"
)

func init() { registerScenario("hosted-approval", handleHostedApproval) }

type hostedApprovalRequest struct {
	Input []map[string]any `json:"input"`
	Tools []map[string]any `json:"tools"`
}

type hostedApprovalReplay struct {
	streams  [2]string
	binding  string
	itemID   string
	requests []hostedApprovalRequest
}

func loadHostedApprovalReplay() (*hostedApprovalReplay, error) {
	replay := &hostedApprovalReplay{}
	for i, stage := range []string{"3", "4"} {
		path := filepath.Join("..", "..", "conformance", "openai", "upstream", "mcp-tool-approval-"+stage, "input.chunks.txt")
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		replay.streams[i] = "data: " + strings.Join(lines, "\n\ndata: ") + "\n\n"
		if i == 0 {
			continue
		}
		for _, line := range lines {
			var event struct {
				Type string
				Item struct {
					ID                string
					Type              string
					Status            string
					ApprovalRequestID string `json:"approval_request_id"`
				}
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				return nil, err
			}
			if event.Type == "response.output_item.done" && event.Item.Type == "mcp_call" && event.Item.Status == "completed" {
				if replay.binding != "" || event.Item.ApprovalRequestID == "" || event.Item.ID == "" {
					return nil, fmt.Errorf("expected exactly one completed mcp_call with an item ID and approval binding")
				}
				replay.binding, replay.itemID = event.Item.ApprovalRequestID, event.Item.ID
			}
		}
	}
	if replay.binding == "" {
		return nil, fmt.Errorf("missing completed mcp_call approval binding")
	}
	return replay, nil
}

func (p *hostedApprovalReplay) RoundTrip(r *http.Request) (*http.Response, error) {
	step := len(p.requests)
	if r.Method != http.MethodPost || r.URL.String() != hostedApprovalBaseURL+"/responses" || step >= len(p.streams) {
		return nil, fmt.Errorf("unexpected Responses request %d: %s %s", step+1, r.Method, r.URL)
	}
	var body struct {
		hostedApprovalRequest
		Store              bool            `json:"store"`
		Stream             bool            `json:"stream"`
		PreviousResponseID json.RawMessage `json:"previous_response_id"`
		Conversation       json.RawMessage `json:"conversation"`
	}
	decodeErr := json.NewDecoder(r.Body).Decode(&body)
	if err := errors.Join(decodeErr, r.Body.Close()); err != nil {
		return nil, fmt.Errorf("reading replay request: %w", err)
	}
	p.requests = append(p.requests, body.hostedApprovalRequest)
	if !body.Stream || !body.Store || body.PreviousResponseID != nil || body.Conversation != nil {
		return nil, fmt.Errorf("expected streaming stored-item replay without previous_response_id or conversation")
	}
	var decisions, references int
	for _, item := range body.Input {
		switch item["type"] {
		case "function_call_output":
			return nil, fmt.Errorf("unexpected client tool result in request %d", step+1)
		case "mcp_approval_response":
			decisions++
			if len(item) != 3 || item["approval_request_id"] != p.binding || item["approve"] != true {
				return nil, fmt.Errorf("request %d: wrong approval response binding: %v", step+1, item)
			}
		case "item_reference":
			if item["id"] == p.binding {
				references++
				if len(item) != 2 {
					return nil, fmt.Errorf("request %d: invalid approval item reference: %v", step+1, item)
				}
			}
		}
	}
	if decisions != step || references != step {
		return nil, fmt.Errorf("request %d: got %d approval decisions and %d approval item references", step+1, decisions, references)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(p.streams[step])),
		Request:    r,
	}, nil
}

func handleHostedApproval(w http.ResponseWriter, r *http.Request) {
	transcript, err := runHostedApproval(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(transcript)
}

func runHostedApproval(ctx context.Context) (map[string]any, error) {
	replay, err := loadHostedApprovalReplay()
	if err != nil {
		return nil, err
	}
	model := openai.NewResponses("offline-test-key", "gpt-5-mini", openai.WithRequestOptions(
		option.WithBaseURL(hostedApprovalBaseURL), option.WithHTTPClient(&http.Client{Transport: replay}), option.WithMaxRetries(0),
	))
	var localExecutions atomic.Int32
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object"}`))
	if err != nil {
		return nil, err
	}
	tools := aisdk.ToolSet{
		"MCP": {
			Type: aisdk.UserToolProvider,
			ID:   "openai.mcp",
			Args: map[string]json.RawMessage{
				"serverLabel": json.RawMessage(`"zip1"`), "serverUrl": json.RawMessage(`"https://mcp.invalid"`), "requireApproval": json.RawMessage(`"always"`),
			},
		},
		hostedApprovalTool: {
			InputSchema: inputSchema,
			Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
				localExecutions.Add(1)
				return nil, fmt.Errorf("hosted tool must not execute locally")
			},
		},
	}
	store := true
	run := func(history []provider.Message) (*aisdk.StreamTextResult, error) {
		result := aisdk.StreamText(ctx, model, aisdk.WithModelMessages(history...), aisdk.WithTools(tools),
			aisdk.WithActiveTools("MCP"), aisdk.WithMaxRetries(0), aisdk.WithStopWhen(aisdk.StepCountIs(3)),
			aisdk.WithProviderOptions(openai.OpenAIResponsesOptions{Store: &store}))
		var streamErr error
		for part := range result.FullStream() {
			switch part := part.(type) {
			case aisdk.StreamError:
				streamErr = part.Error
			case aisdk.StreamAbort:
				streamErr = fmt.Errorf("unexpected stream abort: %s", part.Reason)
			}
		}
		if streamErr != nil {
			return nil, streamErr
		}
		return result, result.Err()
	}

	history := []provider.Message{provider.UserText("shorten ai-sdk.dev")}
	first, err := run(history)
	if err != nil {
		return nil, fmt.Errorf("first SDK call: %w", err)
	}
	firstSteps := first.Steps()
	if len(replay.requests) != 1 || len(firstSteps) != 1 || len(firstSteps[0].ToolApprovalRequests) != 1 || len(firstSteps[0].ToolCalls) != 1 || len(first.ToolResults()) != 0 {
		return nil, fmt.Errorf("first SDK call did not stop on exactly one pending approval")
	}
	approval, firstCall := firstSteps[0].ToolApprovalRequests[0], firstSteps[0].ToolCalls[0]
	if approval.ApprovalID != replay.binding || firstCall.ToolCallID == "" || approval.ToolCallID != firstCall.ToolCallID || !firstCall.ProviderExecuted {
		return nil, fmt.Errorf("incorrect approval correlation: %+v", approval)
	}
	history = append(history, first.ResponseMessages()...)
	history = append(history, provider.NewToolMessage(provider.ProviderExecutedToolApprovalResponsePart(approval.ApprovalID, true, "")))
	second, err := run(history)
	if err != nil {
		return nil, fmt.Errorf("resumed SDK call: %w", err)
	}
	secondSteps := second.Steps()
	calls, results := second.ToolCalls(), second.ToolResults()
	if len(replay.requests) != 2 || len(secondSteps) != 1 || len(secondSteps[0].ToolApprovalRequests) != 0 || len(calls) != 1 || len(results) != 1 {
		return nil, fmt.Errorf("resumed SDK call did not produce exactly one hosted result")
	}
	if calls[0].ToolCallID != replay.itemID || results[0].ToolCallID != replay.itemID || !calls[0].ProviderExecuted || !results[0].ProviderExecuted || results[0].IsError || results[0].Preliminary {
		return nil, fmt.Errorf("expected final hosted result for native item %q, got call %q and result %q", replay.itemID, calls[0].ToolCallID, results[0].ToolCallID)
	}
	return map[string]any{
		"requests": replay.requests, "localExecutions": localExecutions.Load(), "approvalId": approval.ApprovalID,
		"first": map[string]any{"input": firstCall.Input, "toolName": firstCall.ToolName, "text": first.Text(), "finish": first.FinishReason().Unified},
		"second": map[string]any{"input": calls[0].Input, "toolName": calls[0].ToolName, "resultName": results[0].ToolName, "output": results[0].Output,
			"text": second.Text(), "finish": second.FinishReason().Unified, "toolCallId": results[0].ToolCallID, "metadata": results[0].ProviderMetadata},
	}, nil
}
