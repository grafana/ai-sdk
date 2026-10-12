package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	aisdk "github.com/grafana/ai-sdk"
)

const objectPartialJSON = `{"name":"Alice",`

func init() {
	registerScenario("chat-regenerate", handleChatRegenerate)
	registerScenario("chat-data", handleChatData)
	registerScenario("completion-overlap", handleCompletionOverlap)
	registerScenario("object-body-error", handleObjectBodyError)
	registerScenario("object-abortable", handleObjectAbortable)
}

func writeHookUIChunks(w http.ResponseWriter, chunks ...aisdk.UIMessageChunk) error {
	stream := make(chan aisdk.UIMessageChunk, len(chunks))
	for _, chunk := range chunks {
		stream <- chunk
	}
	close(stream)
	return aisdk.PipeUIMessageStreamToResponse(w, stream)
}

func hookTextChunks(messageID, text string) []aisdk.UIMessageChunk {
	return []aisdk.UIMessageChunk{
		{Type: aisdk.ChunkStart, MessageID: messageID},
		{Type: aisdk.ChunkStartStep},
		{Type: aisdk.ChunkTextStart, ID: "text-1"},
		{Type: aisdk.ChunkTextDelta, ID: "text-1", Delta: text},
		{Type: aisdk.ChunkTextEnd, ID: "text-1"},
		{Type: aisdk.ChunkFinishStep},
		{Type: aisdk.ChunkFinish, FinishReason: "stop"},
	}
}

func handleChatRegenerate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Trigger  string `json:"trigger"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 1 || request.Messages[0].Role != "user" {
		http.Error(w, "expected one user message", http.StatusBadRequest)
		return
	}
	messageID, text := "assistant-first", "First response"
	switch request.Trigger {
	case "submit-message":
	case "regenerate-message":
		messageID, text = "assistant-second", "Second response"
	default:
		http.Error(w, "unexpected chat trigger", http.StatusBadRequest)
		return
	}
	if err := writeHookUIChunks(w, hookTextChunks(messageID, text)...); err != nil && r.Context().Err() == nil {
		log.Printf("chat regenerate stream: %v", err)
	}
}

func handleChatData(w http.ResponseWriter, r *http.Request) {
	chunks := []aisdk.UIMessageChunk{
		{Type: aisdk.ChunkStart, MessageID: "assistant-data", MessageMetadata: json.RawMessage(`{"phase":"initial"}`)},
		{Type: aisdk.ChunkStartStep},
		{Type: aisdk.ChunkTextStart, ID: "text-1"},
		{Type: aisdk.ChunkTextDelta, ID: "text-1", Delta: "Data"},
		{Type: aisdk.ChunkMessageMetadata, MessageMetadata: json.RawMessage(`{"phase":"updated","source":"go"}`)},
		aisdk.DataChunk("weather", json.RawMessage(`{"temp":70}`), false),
		aisdk.DataChunk("notice", json.RawMessage(`{"status":"sent"}`), true),
		{Type: aisdk.ChunkTextDelta, ID: "text-1", Delta: " received"},
		{Type: aisdk.ChunkTextEnd, ID: "text-1"},
		{Type: aisdk.ChunkFinishStep},
		{Type: aisdk.ChunkFinish, FinishReason: "stop"},
	}
	stream := make(chan aisdk.UIMessageChunk, 1)
	go func() {
		defer close(stream)
		for _, chunk := range chunks {
			if chunk.Type == aisdk.ChunkMessageMetadata && !waitForContext(r.Context(), controlledStreamStartDelay) {
				return
			}
			select {
			case stream <- chunk:
			case <-r.Context().Done():
				return
			}
		}
	}()
	if err := aisdk.PipeUIMessageStreamToResponse(w, stream); err != nil && r.Context().Err() == nil {
		log.Printf("chat data stream: %v", err)
	}
}

type hookHeaderFlushingWriter struct{ http.ResponseWriter }

func (w hookHeaderFlushingWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
	flushHookResponse(w.ResponseWriter)
}

func (w hookHeaderFlushingWriter) Flush() { flushHookResponse(w.ResponseWriter) }

func handleHookReconnect(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") != "chat-interop" {
		http.NotFound(w, r)
		return
	}
	switch r.PathValue("name") {
	case "reconnect-stream":
		stream := make(chan aisdk.UIMessageChunk, 1)
		go func() {
			defer close(stream)
			if !waitForContext(r.Context(), controlledStreamStartDelay) {
				return
			}
			for _, chunk := range hookTextChunks("assistant-reconnected", "Reconnected response") {
				if chunk.Type == aisdk.ChunkTextEnd && !waitForContext(r.Context(), controlledStreamStartDelay) {
					return
				}
				select {
				case stream <- chunk:
				case <-r.Context().Done():
					return
				}
			}
		}()
		if err := aisdk.PipeUIMessageStreamToResponse(hookHeaderFlushingWriter{w}, stream); err != nil && r.Context().Err() == nil {
			log.Printf("chat reconnect stream: %v", err)
		}
	case "reconnect-empty":
		w.WriteHeader(http.StatusNoContent)
	case "reconnect-error":
		handleHTTPError(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleCompletionOverlap(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid completion request", http.StatusBadRequest)
		return
	}
	if request.Prompt == "first-error" {
		if !waitForContext(r.Context(), 2*controlledStreamHold) {
			return
		}
		handleHTTPError(w, r)
		return
	}
	if request.Prompt != "first" && request.Prompt != "second" {
		http.Error(w, "unexpected completion prompt", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, request.Prompt+" partial")
	flushHookResponse(w)
	if request.Prompt == "first" {
		if !waitForContext(r.Context(), 2*controlledStreamHold) {
			return
		}
		_, _ = fmt.Fprint(w, " finished")
		return
	}
	if !waitForContext(r.Context(), 4*controlledStreamHold) {
		return
	}
	_, _ = fmt.Fprint(w, " finished")
}

func flushHookResponse(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func handleObjectAbortable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprint(w, objectPartialJSON)
	flushHookResponse(w)
	<-r.Context().Done()
}

func handleObjectBodyError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(objectPartialJSON)+1))
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, objectPartialJSON)
	flushHookResponse(w)
	if !waitForContext(r.Context(), controlledStreamHold) {
		return
	}
}
