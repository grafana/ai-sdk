package chatcompletions

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

// Path is the OpenAI Chat Completions adapter endpoint; authentication is supplied by the host.
const Path = "/v1/chat/completions"

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.URL.Path != Path {
		h.serveStored(w, req)
		return
	}
	if req.URL.Path != Path || req.URL.RawPath != "" || req.URL.RawQuery != "" {
		WriteError(w, 404)
		return
	}
	if req.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		WriteError(w, 405)
		return
	}
	media, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		WriteError(w, 415)
		return
	}
	if req.Header.Get("Content-Encoding") != "" {
		WriteError(w, 400)
		return
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		WriteError(w, 400)
		return
	}
	if !utf8.Valid(body) {
		WriteError(w, 400)
		return
	}
	r, err := mapRequest(body)
	if err != nil {
		WriteError(w, 400)
		return
	}
	var storageScope StorageScope
	if boolValue(r.Store) {
		var ok bool
		storageScope, ok = h.storageScope(req.Context())
		if !ok {
			WriteError(w, 400)
			return
		}
	}
	if boolValue(r.Store) {
		// This is a storage quota, not a generation/input-context limit.
		inputRecord, _ := json.Marshal(StoredCompletion{Messages: r.originalMessages, Metadata: r.Metadata})
		if len(inputRecord) > MaxStoredCompletionBytes {
			WriteError(w, http.StatusRequestEntityTooLarge)
			return
		}
		if err := h.storage.Store.Ready(req.Context(), storageScope); err != nil {
			writeStoredError(w, err)
			return
		}
	}
	call, err := h.runtime.Call(req.Context(), r.Model, r.options, execution.WithTranslator(chatTranslator(r)))
	if err != nil {
		if req.Context().Err() == context.Canceled {
			return
		}
		category, _ := execution.ClassifySelection(err)
		WriteError(w, categoryStatus(category))
		return
	}
	defer call.Close()
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		WriteError(w, 500)
		return
	}
	id := "chatcmpl-" + hex.EncodeToString(random)
	created := time.Now().Unix()
	if boolValue(r.Stream) {
		if boolValue(r.Store) {
			suppressUsage := r.StreamOptions == nil || !boolValue(r.StreamOptions.IncludeUsage)
			w = newStoredStreamWriter(w, func(raw []byte) error { return h.persistCompletion(req.Context(), r, storageScope, raw) }, suppressUsage)
			if r.StreamOptions == nil {
				r.StreamOptions = &struct {
					IncludeUsage       *bool `json:"include_usage"`
					IncludeObfuscation *bool `json:"include_obfuscation"`
				}{}
			}
			r.StreamOptions.IncludeUsage = ptr(true)
		}
		h.serveStream(req.Context(), w, call, r, id, call.ModelID(), created)
		return
	}
	result, err := call.Generate()
	if err != nil {
		if req.Context().Err() == context.Canceled {
			return
		}
		writeError(w, categoryStatus(execution.ClassifyProvider(err)), executionDiagnostics(nil, call.View(err), nil))
		return
	}
	view := call.View(nil)
	out, err := mapGenerate(result, r, id, call.ModelID(), created)
	if err != nil {
		writeError(w, 502, executionDiagnostics(nil, view, nil))
		return
	}
	if boolValue(r.Store) {
		out.Metadata = r.Metadata
		if out.Metadata == nil {
			out.Metadata = map[string]string{}
		}
	}
	out.Grafana = executionDiagnostics(out.Grafana, view, result.ProviderMetadata)
	encoded, err := json.Marshal(out)
	if err != nil {
		writeError(w, 502, executionDiagnostics(nil, view, nil))
		return
	}
	if boolValue(r.Store) {
		if err := h.persistCompletion(req.Context(), r, storageScope, encoded); err != nil {
			writeStoredError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}
