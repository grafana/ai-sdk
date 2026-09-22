package main

import (
	"context"
	"net/http"
	"regexp"
	"sync"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

// The concurrent-tools scenario runs a fast and a slow tool in one step. The
// slow tool blocks until the client opens its gate through
// concurrent-tools-release, so a client can check what it has received while
// the slow tool is still running without depending on timing.
func init() {
	registerScenario("concurrent-tools", handleConcurrentTools)
	registerScenario("concurrent-tools-release", handleConcurrentToolsRelease)
}

var concurrentToolGates sync.Map // gate ID -> chan struct{}

type concurrentToolsModel struct {
	calls int
}

func (*concurrentToolsModel) SpecificationVersion() string               { return "v4" }
func (*concurrentToolsModel) Provider() string                           { return "test" }
func (*concurrentToolsModel) ModelID() string                            { return "test-concurrent-tools" }
func (*concurrentToolsModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*concurrentToolsModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *concurrentToolsModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	stream := make(chan provider.StreamPart, 4)
	if m.calls == 1 {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "slow-1", ToolName: "slow", Input: `{}`}
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "fast-1", ToolName: "fast", Input: `{}`}
		stream <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
			Usage:        &provider.Usage{},
		}
	} else {
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "Both tools finished."}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"}
		stream <- provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        &provider.Usage{},
		}
	}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

type concurrentToolInput struct{}

type concurrentToolOutput struct {
	Tool string `json:"tool"`
}

func handleConcurrentTools(w http.ResponseWriter, r *http.Request) {
	gateID := r.URL.Query().Get("gate")
	if gateID == "" {
		http.Error(w, "gate is required", http.StatusBadRequest)
		return
	}
	gate := make(chan struct{})
	if _, exists := concurrentToolGates.LoadOrStore(gateID, gate); exists {
		http.Error(w, "gate already in use", http.StatusConflict)
		return
	}
	defer concurrentToolGates.Delete(gateID)

	fast, err := aisdk.TypedTool(aisdk.TypedToolDef[concurrentToolInput, concurrentToolOutput]{
		Name:        "fast",
		Description: "Returns at once.",
		Execute: func(context.Context, concurrentToolInput, aisdk.ToolExecutionOptions) (concurrentToolOutput, error) {
			return concurrentToolOutput{Tool: "fast"}, nil
		},
	})
	if err != nil {
		http.Error(w, "creating fast tool", http.StatusInternalServerError)
		return
	}
	slow, err := aisdk.TypedTool(aisdk.TypedToolDef[concurrentToolInput, concurrentToolOutput]{
		Name:        "slow",
		Description: "Returns when the client opens its gate.",
		Execute: func(ctx context.Context, _ concurrentToolInput, _ aisdk.ToolExecutionOptions) (concurrentToolOutput, error) {
			select {
			case <-gate:
				return concurrentToolOutput{Tool: "slow"}, nil
			case <-ctx.Done():
				return concurrentToolOutput{}, ctx.Err()
			}
		},
	})
	if err != nil {
		http.Error(w, "creating slow tool", http.StatusInternalServerError)
		return
	}

	result := aisdk.StreamText(r.Context(), &concurrentToolsModel{},
		aisdk.WithModelMessages(provider.UserText("Run both tools.")),
		aisdk.WithTools(aisdk.ToolSet{"fast": fast, "slow": slow}),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleConcurrentToolsRelease(w http.ResponseWriter, r *http.Request) {
	gate, ok := concurrentToolGates.LoadAndDelete(r.URL.Query().Get("gate"))
	if !ok {
		http.Error(w, "unknown gate", http.StatusNotFound)
		return
	}
	close(gate.(chan struct{}))
	w.WriteHeader(http.StatusNoContent)
}
