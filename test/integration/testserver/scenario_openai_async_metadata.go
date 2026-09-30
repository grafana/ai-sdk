package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() { registerScenario("openai-async-metadata", handleOpenAIAsyncMetadata) }

type openAIAsyncMetadataModel struct{ calls int }

func (*openAIAsyncMetadataModel) SpecificationVersion() string               { return "v4" }
func (*openAIAsyncMetadataModel) Provider() string                           { return "openai" }
func (*openAIAsyncMetadataModel) ModelID() string                            { return "gpt-6" }
func (*openAIAsyncMetadataModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*openAIAsyncMetadataModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, fmt.Errorf("openai-async-metadata: generate not supported")
}
func (m *openAIAsyncMetadataModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	if m.calls == 2 {
		if len(opts.Prompt) != 3 || len(opts.Prompt[1].Content) != 3 || len(opts.Prompt[2].Content) != 3 {
			return nil, fmt.Errorf("openai-async-metadata: incomplete continuation")
		}
		for i, expected := range []string{`{"itemId":"fc_true","async":true,"caller":{"type":"program","callerId":"prog_1"}}`, `{"itemId":"fc_false","async":false}`, `{"itemId":"fc_absent"}`} {
			part := opts.Prompt[1].Content[i]
			if part.Type != provider.ContentPartTypeToolCall || part.ProviderOptions == nil {
				return nil, fmt.Errorf("openai-async-metadata: missing continued call %d", i)
			}
			option, ok := part.ProviderOptions["openai"].(provider.RawProviderOption)
			if !ok || !jsonEqual(option.Raw, json.RawMessage(expected)) {
				return nil, fmt.Errorf("openai-async-metadata: lost async metadata on call %d", i)
			}
		}
		stream := make(chan provider.StreamPart, 4)
		stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "answer"}
		stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "answer", Delta: "continued"}
		stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "answer"}
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}
	stream := make(chan provider.StreamPart, 4)
	for _, entry := range []struct{ id, name, metadata string }{
		{"true", "lookup_true", `{"itemId":"fc_true","async":true,"caller":{"type":"program","callerId":"prog_1"}}`},
		{"false", "lookup_false", `{"itemId":"fc_false","async":false}`},
		{"absent", "lookup_absent", `{"itemId":"fc_absent"}`},
	} {
		stream <- provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "call_" + entry.id, ToolName: entry.name, Input: `{}`, ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(entry.metadata)}}
	}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func jsonEqual(a, b json.RawMessage) bool {
	var left, right any
	return json.Unmarshal(a, &left) == nil && json.Unmarshal(b, &right) == nil && reflect.DeepEqual(left, right)
}

func handleOpenAIAsyncMetadata(w http.ResponseWriter, r *http.Request) {
	model := &openAIAsyncMetadataModel{}
	execute := func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	}
	result := aisdk.StreamText(r.Context(), model,
		aisdk.WithModelMessages(provider.UserText("exercise async metadata")),
		aisdk.WithTools(aisdk.ToolSet{"lookup_true": {Execute: execute}, "lookup_false": {Execute: execute}, "lookup_absent": {Execute: execute}}),
		aisdk.WithStopWhen(aisdk.StepCountIs(2)),
	)
	if err := aisdk.WriteUIMessageStream(w, result); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
