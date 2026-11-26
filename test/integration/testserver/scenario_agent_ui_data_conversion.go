package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
)

func init() {
	registerScenario("agent-ui-data-conversion", handleAgentUIDataConversion)
}

type agentUIDataModel struct {
	simpleTextModel
	calls    int
	metadata json.RawMessage
}

func (m *agentUIDataModel) DoStream(ctx context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.calls++
	metadata, err := json.Marshal(map[string]any{"prompt": opts.Prompt, "providerCalls": m.calls})
	if err != nil {
		return nil, err
	}
	m.metadata = metadata
	return m.simpleTextModel.DoStream(ctx, opts)
}

func convertAgentUIData(part aisdk.DataPart) (*provider.ContentPart, error) {
	switch part.DataName {
	case "error":
		return nil, errors.New("test: conversion failed")
	case "text", "file":
		var value string
		if err := json.Unmarshal(part.Data, &value); err != nil {
			return nil, fmt.Errorf("test: decoding data: %w", err)
		}
		if part.DataName == "text" {
			return new(provider.TextPart(value)), nil
		}
		file := provider.FilePart("text/plain", provider.Base64DataContent(value))
		file.Filename = new(part.ID + ".txt")
		return &file, nil
	default:
		return nil, nil
	}
}

func handleAgentUIDataConversion(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Messages []aisdk.UIMessage `json:"messages"`
		Write    bool              `json:"write"`
		Convert  bool              `json:"convert"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	model := &agentUIDataModel{}
	agent := aisdk.NewToolLoopAgent(model)
	opts := []aisdk.AgentUIStreamOption{
		aisdk.WithUIMessageStreamMessageMetadata(func(part aisdk.TextStreamPart) json.RawMessage {
			if _, ok := part.(aisdk.StreamFinish); ok {
				return model.metadata
			}
			return nil
		}),
	}
	if input.Convert {
		opts = append(opts, aisdk.WithAgentUIStreamConvertDataPart(convertAgentUIData))
	}
	if input.Write {
		if err := aisdk.WriteAgentUIStream(w, r.Context(), agent, input.Messages, opts...); err != nil && w.Header().Get("x-vercel-ai-ui-message-stream") == "" {
			writeAgentUIDataError(w, err, model.calls)
		}
		return
	}
	stream, err := aisdk.CreateAgentUIStream(r.Context(), agent, input.Messages, opts...)
	if err != nil {
		writeAgentUIDataError(w, err, model.calls)
		return
	}
	_ = aisdk.PipeAgentUIStreamToResponse(w, stream)
}

func writeAgentUIDataError(w http.ResponseWriter, err error, calls int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "providerCalls": calls})
}
