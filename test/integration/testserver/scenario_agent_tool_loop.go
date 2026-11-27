package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

const (
	agentLoopPrompt       = "Check inventory and recover any missing record."
	agentLoopInstructions = "Use actual inventory results."
	agentLoopFailure      = "inventory unavailable"
)

func init() {
	registerScenario("agent-tool-loop", handleAgentToolLoop)
}

type agentLoopEvent []any

type agentLoopRequest struct {
	Prompt      []provider.Message `json:"prompt"`
	AgentMarker bool               `json:"agentMarker"`
}

type agentLoopStep struct {
	Number        int              `json:"number"`
	Finish        string           `json:"finish"`
	Content       []agentLoopEvent `json:"content"`
	ToolResultIDs []string         `json:"toolResultIds"`
}

type agentLoopTrace struct {
	Requests []agentLoopRequest     `json:"requests"`
	Events   []agentLoopEvent       `json:"events"`
	Chunks   []aisdk.UIMessageChunk `json:"chunks"`
	Steps    []agentLoopStep        `json:"steps"`
	Executed []agentLoopEvent       `json:"executed"`
	Text     string                 `json:"text"`
}

type agentLoopModel struct {
	requests []agentLoopRequest
	mutate   func(int, []provider.Message) []provider.Message
}

func (*agentLoopModel) SpecificationVersion() string               { return "v4" }
func (*agentLoopModel) Provider() string                           { return "test" }
func (*agentLoopModel) ModelID() string                            { return "agent-tool-loop" }
func (*agentLoopModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*agentLoopModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, errors.New("agent tool loop: unexpected unary call")
}

func (m *agentLoopModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	turn := len(m.requests) + 1
	prompt := opts.Prompt
	if m.mutate != nil {
		prompt = m.mutate(turn, prompt)
	}
	marker := false
	for name, value := range opts.Headers {
		if strings.EqualFold(name, "user-agent") {
			marker = slices.Contains(strings.Fields(value), "ai-sdk-agent/tool-loop")
		}
	}
	m.requests = append(m.requests, agentLoopRequest{Prompt: prompt, AgentMarker: marker})
	parts, err := agentLoopReply(turn, prompt)
	if err != nil {
		return nil, err
	}
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func agentLoopReply(turn int, prompt []provider.Message) ([]provider.StreamPart, error) {
	call := func(id, name, input string) provider.StreamPart {
		return provider.StreamPart{Type: provider.PartToolCall, ToolCallID: id, ToolName: name, Input: input}
	}
	finish := func(reason provider.UnifiedFinishReason) provider.StreamPart {
		return provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: reason}, Usage: &provider.Usage{}}
	}
	want := []provider.Message{provider.NewSystemMessage(agentLoopInstructions), provider.UserText(agentLoopPrompt)}
	if turn >= 2 {
		want = append(want,
			provider.Message{Role: provider.RoleAssistant, Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeToolCall, ToolCallID: "stock-1", ToolName: "lookup", Input: json.RawMessage(`{"sku":"A"}`)},
				{Type: provider.ContentPartTypeToolCall, ToolCallID: "stock-2", ToolName: "lookup", Input: json.RawMessage(`{"sku":"B"}`)},
			}},
			provider.Message{Role: provider.RoleTool, Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeToolResult, ToolCallID: "stock-1", ToolName: "lookup", Output: &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"count":7}`)}},
				{Type: provider.ContentPartTypeToolResult, ToolCallID: "stock-2", ToolName: "lookup", Output: &provider.ToolResultOutput{Type: provider.ToolOutputErrorText, Text: agentLoopFailure}},
			}},
		)
	}
	if turn == 3 {
		want = append(want,
			provider.Message{Role: provider.RoleAssistant, Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeToolCall, ToolCallID: "recover-1", ToolName: "recover", Input: json.RawMessage(`{"sku":"B"}`)},
			}},
			provider.Message{Role: provider.RoleTool, Content: []provider.ContentPart{
				{Type: provider.ContentPartTypeToolResult, ToolCallID: "recover-1", ToolName: "recover", Output: &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"count":3}`)}},
			}},
		)
	}
	got, err := agentLoopPromptJSON(prompt)
	if err != nil {
		return nil, err
	}
	expected, err := agentLoopPromptJSON(want)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(got, expected) {
		return nil, fmt.Errorf("agent tool loop: turn %d received wrong prompt: %v", turn, got)
	}
	switch turn {
	case 1:
		return []provider.StreamPart{call("stock-1", "lookup", `{"sku":"A"}`), call("stock-2", "lookup", `{"sku":"B"}`), finish(provider.FinishReasonToolCalls)}, nil
	case 2:
		return []provider.StreamPart{call("recover-1", "recover", string(prompt[2].Content[1].Input)), finish(provider.FinishReasonToolCalls)}, nil
	case 3:
		return []provider.StreamPart{
			{Type: provider.PartTextStart, ID: "answer"},
			{Type: provider.PartTextDelta, ID: "answer", Delta: fmt.Sprintf("A: %s; B recovered: %s", prompt[3].Content[0].Output.JSON, prompt[5].Content[0].Output.JSON)},
			{Type: provider.PartTextEnd, ID: "answer"}, finish(provider.FinishReasonStop),
		}, nil
	default:
		return nil, fmt.Errorf("agent tool loop: unexpected model turn %d", turn)
	}
}

func agentLoopPromptJSON(prompt []provider.Message) (any, error) {
	raw, err := json.Marshal(prompt)
	if err != nil {
		return nil, err
	}
	var value any
	err = json.Unmarshal(raw, &value)
	return value, err
}

func runAgentToolLoop(ctx context.Context, model *agentLoopModel) (agentLoopTrace, error) {
	var trace agentLoopTrace
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"sku":{"type":"string"}},"required":["sku"],"additionalProperties":false}`))
	if err != nil {
		return trace, err
	}
	var mu sync.Mutex
	record := func(name string, input json.RawMessage, opts aisdk.ToolExecutionOptions) {
		mu.Lock()
		defer mu.Unlock()
		trace.Executed = append(trace.Executed, agentLoopEvent{opts.ToolCallID, name, input})
	}
	secondStarted, firstEmitted := make(chan struct{}), make(chan struct{})
	var startOnce, emitOnce sync.Once
	wait := func(ctx context.Context, ready <-chan struct{}) error {
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	tools := aisdk.ToolSet{
		"lookup": {InputSchema: inputSchema, Execute: func(ctx context.Context, input json.RawMessage, opts aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			record("lookup", input, opts)
			if opts.ToolCallID == "stock-2" {
				startOnce.Do(func() { close(secondStarted) })
				if err := wait(ctx, firstEmitted); err != nil {
					return nil, err
				}
				return nil, errors.New(agentLoopFailure)
			}
			if err := wait(ctx, secondStarted); err != nil {
				return nil, err
			}
			return json.RawMessage(`{"count":7}`), nil
		}},
		"recover": {InputSchema: inputSchema, Execute: func(_ context.Context, input json.RawMessage, opts aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			record("recover", input, opts)
			return json.RawMessage(`{"count":3}`), nil
		}},
		"unused": {InputSchema: inputSchema, Execute: func(_ context.Context, input json.RawMessage, opts aisdk.ToolExecutionOptions) (json.RawMessage, error) {
			record("unused", input, opts)
			return nil, errors.New("agent tool loop: unexpected tool execution")
		}},
	}
	agent := aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(aisdk.WithSystem(agentLoopInstructions), aisdk.WithTools(tools), aisdk.WithMaxRetries(0)))
	result := agent.Stream(ctx, aisdk.WithAgentPrompt(agentLoopPrompt))
	chunks := result.ToUIMessageStream(
		aisdk.WithUIMessageStreamGenerateID(func() string { return "agent-loop-message" }),
		aisdk.WithUIMessageStreamMessageMetadata(func(part aisdk.TextStreamPart) json.RawMessage {
			trace.Events = append(trace.Events, agentLoopStreamEvent(part))
			return nil
		}),
	)
	for chunk := range chunks {
		trace.Chunks = append(trace.Chunks, chunk)
		if chunk.Type == aisdk.ChunkToolOutputAvailable && chunk.ToolCallID == "stock-1" {
			emitOnce.Do(func() { close(firstEmitted) })
		}
	}
	result.Wait()
	if err := result.Err(); err != nil {
		return trace, err
	}
	trace.Requests = model.requests
	trace.Text = result.Text()
	for _, step := range result.Steps() {
		value := agentLoopStep{Number: step.StepNumber, Finish: string(step.FinishReason.Unified), Content: []agentLoopEvent{}, ToolResultIDs: []string{}}
		for _, part := range step.Content {
			switch p := part.(type) {
			case aisdk.ToolCallContent:
				value.Content = append(value.Content, agentLoopEvent{"tool-call", p.ToolCallID, p.ToolName, p.Input})
			case aisdk.ToolResultContent:
				value.Content = append(value.Content, agentLoopEvent{"tool-result", p.ToolCallID, p.ToolName, p.Output})
			case aisdk.ToolErrorContent:
				value.Content = append(value.Content, agentLoopEvent{"tool-error", p.ToolCallID, p.ToolName, fmt.Sprint(p.Error)})
			case aisdk.TextContent:
				value.Content = append(value.Content, agentLoopEvent{"text", p.Text})
			default:
				return trace, fmt.Errorf("agent tool loop: unexpected step content %T", part)
			}
		}
		for _, tr := range step.ToolResults {
			value.ToolResultIDs = append(value.ToolResultIDs, tr.ToolCallID)
		}
		trace.Steps = append(trace.Steps, value)
	}
	return trace, nil
}

func agentLoopStreamEvent(part aisdk.TextStreamPart) agentLoopEvent {
	switch p := part.(type) {
	case aisdk.StreamStart:
		return agentLoopEvent{"start"}
	case aisdk.StreamStartStep:
		return agentLoopEvent{"start-step"}
	case aisdk.StreamFinishStep:
		return agentLoopEvent{"finish-step", p.FinishReason.Unified}
	case aisdk.StreamFinish:
		return agentLoopEvent{"finish", p.FinishReason.Unified}
	case aisdk.StreamToolCall:
		return agentLoopEvent{"tool-call", p.ToolCallID, p.ToolName, p.Input}
	case aisdk.StreamToolResult:
		return agentLoopEvent{"tool-result", p.ToolCallID, p.ToolName, p.Output, p.Preliminary}
	case aisdk.StreamToolError:
		return agentLoopEvent{"tool-error", p.ToolCallID, p.ToolName, p.Error.Error()}
	case aisdk.StreamTextStart:
		return agentLoopEvent{"text-start", p.ID}
	case aisdk.StreamTextDelta:
		return agentLoopEvent{"text-delta", p.ID, p.Text}
	case aisdk.StreamTextEnd:
		return agentLoopEvent{"text-end", p.ID}
	default:
		return agentLoopEvent{fmt.Sprintf("unexpected: %T", part)}
	}
}

func handleAgentToolLoop(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	trace, err := runAgentToolLoop(ctx, &agentLoopModel{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(trace); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
