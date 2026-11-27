package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go/option"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/schema"
)

//go:embed fixtures/anthropic-synthetic-signed-continuation.json
var anthropicContinuationEvents []byte

type anthropicContinuationRequest struct {
	Method  string          `json:"method"`
	Target  string          `json:"target"`
	Version string          `json:"version"`
	Beta    string          `json:"beta"`
	Body    json.RawMessage `json:"body"`
}

type anthropicContinuationTransport struct {
	responses [][]json.RawMessage
	requests  []anthropicContinuationRequest
}

func (t *anthropicContinuationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(r.Body)
	closeErr := r.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("reading synthetic continuation request: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("closing synthetic continuation request: %w", closeErr)
	}
	i := len(t.requests)
	t.requests = append(t.requests, anthropicContinuationRequest{
		Method: r.Method, Target: r.URL.String(), Version: r.Header.Get("anthropic-version"),
		Beta: strings.Join(r.Header.Values("anthropic-beta"), ","), Body: body,
	})
	if i >= len(t.responses) {
		return nil, fmt.Errorf("unexpected synthetic continuation request %d", i+1)
	}
	var stream strings.Builder
	for _, event := range t.responses[i] {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(event, &header); err != nil || header.Type == "" {
			return nil, fmt.Errorf("invalid synthetic continuation event: %s", event)
		}
		fmt.Fprintf(&stream, "event: %s\ndata: %s\n\n", header.Type, event)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(stream.String())),
		Request:    r,
	}, nil
}

func init() {
	registerScenario("anthropic-signed-continuation", handleAnthropicSignedContinuation)
}

func handleAnthropicSignedContinuation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		History []provider.Message `json:"history"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var responses [][]json.RawMessage
	if err := json.Unmarshal(anthropicContinuationEvents, &responses); err != nil || len(responses) != 3 {
		http.Error(w, "expected three synthetic Anthropic responses", http.StatusInternalServerError)
		return
	}
	messages := []provider.Message{provider.UserText("Look up the shelf label for b7.")}
	firstTurn := input.History == nil
	if !firstTurn {
		messages = append(messages, input.History...)
		messages = append(messages, provider.UserText("Where is the atlas?"))
		responses = responses[2:]
	} else {
		responses = responses[:2]
	}

	transport := &anthropicContinuationTransport{responses: responses}
	model := anthropic.New("test-key", "claude-sonnet-4-6",
		anthropic.WithRequestOptions(
			option.WithBaseURL("https://anthropic.invalid"),
			option.WithHTTPClient(&http.Client{Transport: transport}),
			option.WithMaxRetries(0),
		),
	)
	thinking := &anthropic.ThinkingConfig{Type: anthropic.ThinkingAdaptive, Display: anthropic.ThinkingDisplaySummarized}
	type lookupExecution struct {
		ID   string `json:"id"`
		Slot string `json:"slot"`
	}
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"slot":{"type":"string"}},"required":["slot"]}`))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type lookupResult struct {
		Label string `json:"label"`
		Slot  string `json:"slot"`
	}
	executions := []lookupExecution{}
	options := []aisdk.StreamOption{
		aisdk.WithModelMessages(messages...),
		aisdk.WithMaxRetries(0),
		aisdk.WithMaxOutputTokens(1024),
		aisdk.WithProviderOptions(anthropic.AnthropicOptions{Thinking: thinking}),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
		aisdk.WithTools(aisdk.ToolSet{
			"lookup": {
				Description: "Look up a shelf label.",
				InputSchema: inputSchema,
				Execute: func(_ context.Context, input json.RawMessage, options aisdk.ToolExecutionOptions) (json.RawMessage, error) {
					var args struct {
						Slot string `json:"slot"`
					}
					if err := json.Unmarshal(input, &args); err != nil {
						return nil, fmt.Errorf("decoding lookup input: %w", err)
					}
					executions = append(executions, lookupExecution{ID: options.ToolCallID, Slot: args.Slot})
					return json.Marshal(lookupResult{Label: "atlas", Slot: args.Slot})
				},
				ToModelOutput: func(output aisdk.ToolOutputContext) (*provider.ToolResultOutput, error) {
					var result lookupResult
					if err := json.Unmarshal(output.Output, &result); err != nil {
						return nil, fmt.Errorf("decoding lookup result: %w", err)
					}
					return &provider.ToolResultOutput{
						Type: provider.ToolOutputContent,
						Content: []provider.ToolResultContentValue{
							{Type: provider.ToolContentText, Text: "slot=" + result.Slot},
							{Type: provider.ToolContentText, Text: "label=" + result.Label},
						},
					}, nil
				},
			},
		}),
	}
	if firstTurn {
		options = append(options, aisdk.WithPrepareStep(func(step aisdk.PrepareStepState) (*aisdk.PrepareStepResult, error) {
			if step.StepNumber != 1 {
				return nil, nil
			}
			return &aisdk.PrepareStepResult{
				ProviderOptions: provider.BuildProviderOptions(anthropic.AnthropicOptions{
					Thinking: thinking, Compaction: &anthropic.CompactionConfig{Type: anthropic.CompactionSummarize},
				}),
			}, nil
		}))
	}
	result := aisdk.StreamText(r.Context(), model, options...)
	for range result.FullStream() {
	}
	if err := result.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	report := struct {
		History    []provider.Message             `json:"history"`
		Requests   []anthropicContinuationRequest `json:"requests"`
		Executions []lookupExecution              `json:"executions"`
		Finishes   []provider.FinishReason        `json:"finishes"`
		Text       string                         `json:"text"`
	}{
		History: result.ResponseMessages(), Requests: transport.requests, Executions: executions, Text: result.Text(),
	}
	for _, step := range result.Steps() {
		report.Finishes = append(report.Finishes, step.FinishReason)
	}
	body, err := json.Marshal(report)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}
