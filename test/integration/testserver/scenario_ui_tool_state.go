package main

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
)

func init() {
	registerScenario("ui-tool-state-persistence", handleUIToolStatePersistence)
	registerScenario("ui-tool-state-chunks", handleUIToolStateChunks)
	registerScenario("ui-tool-state-agent", handleUIToolStateAgent)
	registerScenario("ui-tool-state-validation", handleUIToolStateAgent)
}

type uiToolStateRequest struct {
	Messages         []aisdk.UIMessage      `json:"messages"`
	Chunks           []aisdk.UIMessageChunk `json:"chunks"`
	InitialMessage   *aisdk.UIMessage       `json:"initialMessage"`
	IgnoreIncomplete bool                   `json:"ignoreIncomplete"`
	ConvertData      bool                   `json:"convertData"`
	ConvertOutput    bool                   `json:"convertOutput"`
}

func uiToolStateError(w http.ResponseWriter, err error, calls int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "providerCalls": calls})
}

func uiToolStateChunkStream(input []aisdk.UIMessageChunk) <-chan aisdk.UIMessageChunk {
	stream := make(chan aisdk.UIMessageChunk, len(input))
	for _, chunk := range input {
		stream <- chunk
	}
	close(stream)
	return stream
}

func handleUIToolStateChunks(w http.ResponseWriter, r *http.Request) {
	var input uiToolStateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		uiToolStateError(w, err, 0)
		return
	}
	if err := aisdk.PipeUIMessageStreamToResponse(w, uiToolStateChunkStream(input.Chunks)); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleUIToolStatePersistence(w http.ResponseWriter, r *http.Request) {
	var input uiToolStateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		uiToolStateError(w, err, 0)
		return
	}
	snapshots := []aisdk.UIMessage{}
	if input.Chunks != nil {
		var options []aisdk.UIMessageReaderOption
		if input.InitialMessage != nil {
			options = append(options, aisdk.WithUIMessageReaderInitialMessage(*input.InitialMessage))
		}
		for message := range aisdk.StreamUIMessage(uiToolStateChunkStream(input.Chunks), options...) {
			snapshots = append(snapshots, message)
		}
		message, err := aisdk.AssembleUIMessage(uiToolStateChunkStream(input.Chunks), options...)
		if err != nil {
			uiToolStateError(w, err, 0)
			return
		}
		input.Messages = []aisdk.UIMessage{message}
	}
	var options []aisdk.ConvertOption
	if input.IgnoreIncomplete {
		options = append(options, aisdk.WithIgnoreIncompleteToolCalls())
	}
	if input.ConvertData {
		options = append(options, aisdk.WithConvertDataPart(func(part aisdk.DataPart) (*provider.ContentPart, error) {
			if part.DataName == "skip" {
				return nil, nil
			}
			if part.DataName == "empty" {
				return &provider.ContentPart{Type: provider.ContentPartTypeText}, nil
			}
			return &provider.ContentPart{Type: provider.ContentPartTypeText, Text: string(part.Data)}, nil
		}))
	}
	if input.ConvertOutput {
		options = append(options, aisdk.WithTools(aisdk.ToolSet{"lookup": {ToModelOutput: func(ctx aisdk.ToolOutputContext) (*provider.ToolResultOutput, error) {
			return &provider.ToolResultOutput{Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{Type: provider.ToolContentText, Text: "converted:" + string(ctx.Output)}}}, nil
		}}}))
	}
	converted, err := aisdk.ConvertToModelMessages(input.Messages, options...)
	if err != nil {
		uiToolStateError(w, err, 0)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"uiMessages": input.Messages, "modelMessages": converted, "snapshots": snapshots})
}

type uiToolStateModel struct {
	metadata json.RawMessage
	calls    int
}

func (*uiToolStateModel) SpecificationVersion() string               { return "v4" }
func (*uiToolStateModel) Provider() string                           { return "test" }
func (*uiToolStateModel) ModelID() string                            { return "ui-tool-state" }
func (*uiToolStateModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (*uiToolStateModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *uiToolStateModel) DoStream(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	metadata, err := json.Marshal(map[string]any{"modelMessages": options.Prompt, "providerCalls": m.calls})
	if err != nil {
		return nil, err
	}
	m.metadata = metadata
	stream := make(chan provider.StreamPart, 5)
	finalized := false
	for _, message := range options.Prompt {
		for _, part := range message.Content {
			if !finalized && part.Type == provider.ContentPartTypeToolCall && part.ToolCallID == "pre" {
				stream <- provider.StreamPart{Type: provider.PartToolResult, ToolCallID: "pre", ToolName: "lookup", Result: json.RawMessage(`"final"`)}
				finalized = true
			}
		}
	}
	stream <- provider.StreamPart{Type: provider.PartTextStart, ID: "resumed"}
	stream <- provider.StreamPart{Type: provider.PartTextDelta, ID: "resumed", Delta: "resumed"}
	stream <- provider.StreamPart{Type: provider.PartTextEnd, ID: "resumed"}
	stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
	close(stream)
	return &provider.StreamResult{Stream: stream}, nil
}

func handleUIToolStateAgent(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Messages []aisdk.UIMessage `json:"messages"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		uiToolStateError(w, err, 0)
		return
	}
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`))
	if err != nil {
		uiToolStateError(w, err, 0)
		return
	}
	outputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"string"}`))
	if err != nil {
		uiToolStateError(w, err, 0)
		return
	}
	model := &uiToolStateModel{}
	agent := aisdk.NewToolLoopAgent(model, aisdk.WithToolLoopAgentOptions(aisdk.WithTools(aisdk.ToolSet{"lookup": {InputSchema: inputSchema, OutputSchema: outputSchema, Execute: func(context.Context, json.RawMessage, aisdk.ToolExecutionOptions) (json.RawMessage, error) {
		return json.RawMessage(`"approved"`), nil
	}}})))
	var finished aisdk.UIMessageStreamOnFinishState
	stream, err := aisdk.CreateAgentUIStream(r.Context(), agent, input.Messages, aisdk.OnUIMessageStreamFinish(func(state aisdk.UIMessageStreamOnFinishState) { finished = state }), aisdk.WithUIMessageStreamMessageMetadata(func(part aisdk.TextStreamPart) json.RawMessage {
		if _, ok := part.(aisdk.StreamFinish); !ok {
			return nil
		}
		return model.metadata
	}))
	if err != nil {
		uiToolStateError(w, err, model.calls)
		return
	}
	if r.PathValue("name") == "ui-tool-state-validation" {
		for range stream {
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": finished.ResponseMessage, "metadata": model.metadata})
		return
	}
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil && r.Context().Err() == nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
