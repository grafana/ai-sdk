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

	"github.com/grafana/ai-sdk/provider"
)

// Path is the sole native endpoint; authentication is supplied by the host.
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
	body, err := io.ReadAll(io.LimitReader(req.Body, h.limits.RequestBytes+1))
	if err != nil {
		WriteError(w, 400)
		return
	}
	if int64(len(body)) > h.limits.RequestBytes {
		WriteError(w, 413)
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
	ctx, cancel := context.WithTimeout(req.Context(), h.limits.ModelDuration)
	defer cancel()
	model, err := h.resolver.ResolveModel(ctx, r.Model)
	if err != nil {
		WriteError(w, errorStatus(err))
		return
	}
	if model.Model == nil {
		WriteError(w, 500)
		return
	}
	if r.applyPolicy(h.policies[model.ID]) != nil {
		WriteError(w, 400)
		return
	}
	random := make([]byte, 16)
	if _, err = rand.Read(random); err != nil {
		WriteError(w, 500)
		return
	}
	id := "chatcmpl-" + hex.EncodeToString(random)
	created := time.Now().Unix()
	if boolValue(r.Stream) {
		h.serveStream(ctx, cancel, w, model.Model, r, id, model.ID, created)
		return
	}
	type result struct {
		value *provider.GenerateResult
		err   error
	}
	done := make(chan result, 1)
	go func() { v, e := model.Model.DoGenerate(ctx, r.options); done <- result{v, e} }()
	var v result
	select {
	case <-ctx.Done():
		if req.Context().Err() == nil {
			WriteError(w, 504)
		}
		return
	case v = <-done:
	}
	if ctx.Err() != nil {
		if req.Context().Err() == nil {
			WriteError(w, 504)
		}
		return
	}
	if v.err != nil {
		WriteError(w, errorStatus(v.err))
		return
	}
	out, err := mapGenerate(v.value, r, id, model.ID, created, h.limits.ResponseBytes)
	if err != nil {
		WriteError(w, 502)
		return
	}
	encoded, err := json.Marshal(out)
	if err != nil || int64(len(encoded)) > h.limits.ResponseBytes {
		WriteError(w, 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(encoded)
}

// drain transfers sole channel ownership to a bounded cleanup consumer after cancellation.
func drain(ch <-chan provider.StreamPart, duration time.Duration) {
	if ch == nil {
		return
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	deadline := time.Now().Add(duration)
	for {
		if !time.Now().Before(deadline) {
			return
		}
		select {
		case <-timer.C:
			return
		case _, ok := <-ch:
			if !ok {
				return
			}
		}
	}
}
