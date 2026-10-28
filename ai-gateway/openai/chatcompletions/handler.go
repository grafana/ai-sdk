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
	if boolValue(r.Stream) {
		WriteError(w, 400)
		return
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
	result, err := call.Generate()
	if err != nil {
		if req.Context().Err() == context.Canceled {
			return
		}
		WriteError(w, categoryStatus(execution.ClassifyProvider(err)))
		return
	}
	out, err := mapGenerate(result, r, id, call.ModelID(), created)
	if err != nil {
		WriteError(w, 502)
		return
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		WriteError(w, 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}
