package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/providers/bedrock"
)

const (
	novaLoopModel = "us.amazon.nova-2-lite-v1:0"
	novaLoopBase  = "https://bedrock-runtime.us-east-1.invalid"
)

func init() {
	registerScenario("bedrock-nova-loop", handleBedrockNovaLoop)
}

type novaLoopRequest struct {
	Method string          `json:"method"`
	URL    string          `json:"url"`
	Body   json.RawMessage `json:"body"`
}

type novaLoopTransport struct {
	frames   [][]byte
	requests []novaLoopRequest
}

func (tr *novaLoopTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body == nil {
		return nil, errors.New("missing Nova request body")
	}
	body, readErr := io.ReadAll(r.Body)
	if err := errors.Join(readErr, r.Body.Close()); err != nil {
		return nil, fmt.Errorf("consume Nova request body: %w", err)
	}
	turn := len(tr.requests)
	if r.Method != http.MethodPost || r.URL.Host != "bedrock-runtime.us-east-1.invalid" ||
		!strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=nova-test/") || turn >= len(tr.frames) {
		return nil, fmt.Errorf("unexpected Nova transport request on turn %d: %s %s", turn+1, r.Method, r.URL)
	}
	tr.requests = append(tr.requests, novaLoopRequest{r.Method, r.URL.String(), body})
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/vnd.amazon.eventstream"}},
		Body:       io.NopCloser(bytes.NewReader(tr.frames[turn])),
		Request:    r,
	}, nil
}

func handleBedrockNovaLoop(w http.ResponseWriter, r *http.Request) {
	var frames [][]byte
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&frames); err != nil || len(frames) != 2 {
		http.Error(w, "expected two synthetic ConverseStream responses", http.StatusBadRequest)
		return
	}
	transport := &novaLoopTransport{frames: frames}
	model := bedrock.New(novaLoopModel, bedrock.WithRegion("us-east-1"), bedrock.WithBaseURL(novaLoopBase),
		bedrock.WithCredentials(credentials.NewStaticCredentialsProvider("nova-test", "nova-secret", "")),
		bedrock.WithHTTPClient(&http.Client{Transport: transport}),
	)

	type input struct {
		Station string `json:"station"`
	}
	var started atomic.Int32
	bothStarted := make(chan struct{})
	tools := make(aisdk.ToolSet, 2)
	for name, reading := range map[string]string{"temperature": "21 C", "humidity": "58%"} {
		tool, err := aisdk.TypedTool(aisdk.TypedToolDef[input, string]{
			Name: name,
			Execute: func(ctx context.Context, in input, _ aisdk.ToolExecutionOptions) (string, error) {
				if started.Add(1) == 2 {
					close(bothStarted)
				}
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				select {
				case <-bothStarted:
					return in.Station + ": " + reading, nil
				case <-ctx.Done():
					return "", fmt.Errorf("both Nova tools did not start: %w", ctx.Err())
				}
			},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tools[name] = tool
	}

	agent := aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(
		aisdk.WithTools(tools), aisdk.WithMaxRetries(0), aisdk.WithStopWhen(aisdk.StepCountIs(2)),
		aisdk.WithReasoning(provider.ReasoningHigh), aisdk.WithMaxOutputTokens(1024),
		aisdk.WithPrepareStep(func(state aisdk.PrepareStepState) (*aisdk.PrepareStepResult, error) {
			if state.StepNumber != 1 {
				return nil, nil
			}
			return &aisdk.PrepareStepResult{ProviderOptions: provider.ProviderOptions{
				"amazonBedrock": bedrock.BedrockOptions{ReasoningConfig: &bedrock.ReasoningConfig{MaxReasoningEffort: "medium"}},
			}}, nil
		}),
	))
	result := agent.Stream(r.Context(), aisdk.WithAgentPrompt("Read temperature and humidity."))
	for range result.FullStream() {
	}
	if err := result.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type stepEvidence struct {
		Warnings     []provider.Warning    `json:"warnings"`
		FinishReason provider.FinishReason `json:"finishReason"`
	}
	steps := make([]stepEvidence, 0, 2)
	for _, step := range result.Steps() {
		for _, toolResult := range step.ToolResults {
			if toolResult.Error != nil {
				http.Error(w, toolResult.Error.Error(), http.StatusInternalServerError)
				return
			}
		}
		steps = append(steps, stepEvidence{append([]provider.Warning{}, step.Warnings...), step.FinishReason})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Requests []novaLoopRequest `json:"requests"`
		Steps    []stepEvidence    `json:"steps"`
		Text     string            `json:"text"`
	}{transport.requests, steps, result.Text()})
}
