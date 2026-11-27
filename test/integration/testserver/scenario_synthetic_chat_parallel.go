package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	openaicompatible "github.com/grafana/ai-sdk/providers/openai-compatible"
)

func init() { registerScenario("synthetic-chat-parallel", handleSyntheticChatParallel) }

type chatBadgeInput struct {
	Locker string `json:"locker"`
}

type chatWordInput struct {
	Shelf string `json:"shelf"`
}

type chatParallelTrace struct {
	provider.LanguageModel
	calls [][]provider.StreamPart
}

func (m *chatParallelTrace) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	res, err := m.LanguageModel.DoStream(ctx, opts)
	if err != nil {
		return nil, err
	}
	native := res.Stream
	out := make(chan provider.StreamPart)
	res.Stream = out
	go func() {
		defer close(out)
		var parts []provider.StreamPart
		for part := range native {
			parts = append(parts, part)
			select {
			case out <- part:
			case <-ctx.Done():
				return
			}
		}
		m.calls = append(m.calls, parts)
	}()
	return res, nil
}

func handleSyntheticChatParallel(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Port uint16 `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Port == 0 {
		http.Error(w, "missing local synthetic Chat port", http.StatusBadRequest)
		return
	}
	model := &chatParallelTrace{LanguageModel: openaicompatible.New("synthetic-chat",
		openaicompatible.WithBaseURL(fmt.Sprintf("http://127.0.0.1:%d/v1", request.Port)),
		openaicompatible.WithAPIKey("synthetic-key"),
		openaicompatible.WithIncludeUsage(true),
	)}
	badge, err := aisdk.TypedTool(aisdk.TypedToolDef[chatBadgeInput, map[string]string]{
		Name: "read_badge",
		Execute: func(_ context.Context, input chatBadgeInput, opts aisdk.ToolExecutionOptions) (map[string]string, error) {
			if opts.ToolCallID != "call-badge" {
				return nil, fmt.Errorf("unexpected badge tool ID: %s", opts.ToolCallID)
			}
			return map[string]string{"badge": "badge:" + input.Locker}, nil
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	word, err := aisdk.TypedTool(aisdk.TypedToolDef[chatWordInput, string]{
		Name: "read_word",
		Execute: func(_ context.Context, input chatWordInput, opts aisdk.ToolExecutionOptions) (string, error) {
			if opts.ToolCallID != "call-word" {
				return "", fmt.Errorf("unexpected word tool ID: %s", opts.ToolCallID)
			}
			return "", errors.New("word unavailable: " + input.Shelf)
		},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("Run both lookups once.")),
		aisdk.WithTools(aisdk.ToolSet{"read_badge": badge, "read_word": word}),
		aisdk.WithMaxRetries(0), aisdk.WithStopWhen(aisdk.StepCountIs(2)),
	)
	ui := httptest.NewRecorder()
	if err := aisdk.WriteUIMessageStream(ui, result, aisdk.WithUIMessageStreamRedactToolErrors(false)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := result.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	steps := result.Steps()
	usage := make([]provider.Usage, len(steps))
	for i, step := range steps {
		usage[i] = step.Usage
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"uiSSE": ui.Body.String(), "parts": model.calls, "usage": usage,
		"text": result.Text(), "finishReason": result.FinishReason().Unified,
	})
}
