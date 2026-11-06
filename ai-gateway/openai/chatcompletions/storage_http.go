package chatcompletions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const MaxStoredCompletionBytes = 512 << 10

type Option func(*handler) error

func WithStorage(config StorageConfig) Option {
	return func(h *handler) error {
		if config.Store == nil || config.ResolveScope == nil || config.Retention < time.Second || config.Retention > 365*24*time.Hour {
			return errors.New("chat completions: invalid storage configuration")
		}
		h.storage = &config
		return nil
	}
}
func (h *handler) storageScope(ctx context.Context) (StorageScope, bool) {
	if h.storage == nil {
		return StorageScope{}, false
	}
	scope, ok := h.storage.ResolveScope(ctx)
	return scope, ok && scope.Namespace != "" && scope.Owner != ""
}
func writeStoredError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrStoredCompletionNotFound) {
		WriteError(w, 404)
	} else {
		WriteError(w, 503)
	}
}
func writeStoredJSON(w http.ResponseWriter, value any) {
	raw, err := json.Marshal(value)
	if err != nil {
		WriteError(w, 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(raw)
}
func validStoredMetadata(metadata map[string]string) bool {
	if len(metadata) > 16 {
		return false
	}
	for key, value := range metadata {
		if !utf8.ValidString(key) || !utf8.ValidString(value) || len([]rune(key)) > 64 || len([]rune(value)) > 512 {
			return false
		}
	}
	return true
}

// serveStored handles only lifecycle operations. Authentication remains the
// host's responsibility; scope is resolved again for every operation.
func (h *handler) serveStored(w http.ResponseWriter, req *http.Request) {
	scope, ok := h.storageScope(req.Context())
	if !ok {
		if h.storage == nil && req.URL.Path == Path {
			WriteError(w, 405)
			return
		}
		WriteError(w, 404)
		return
	}
	escaped := req.URL.EscapedPath()
	if escaped != Path && !strings.HasPrefix(escaped, Path+"/") {
		WriteError(w, 404)
		return
	}
	suffix := strings.TrimPrefix(escaped, Path)
	parts := strings.Split(strings.TrimPrefix(suffix, "/"), "/")
	id := ""
	messages := false
	if suffix != "" {
		if !strings.HasPrefix(suffix, "/") || len(parts) > 2 {
			WriteError(w, 404)
			return
		}
		var err error
		id, err = url.PathUnescape(parts[0])
		if err != nil || id == "" || len(id) > 256 || !utf8.ValidString(id) {
			WriteError(w, 404)
			return
		}
		if len(parts) == 2 {
			messages = parts[1] == "messages"
			if !messages {
				WriteError(w, 404)
				return
			}
		}
	}
	if id == "" && req.Method == http.MethodGet {
		h.listStored(w, req, scope)
		return
	}
	if id == "" {
		WriteError(w, 405)
		return
	}
	if messages {
		if req.Method != http.MethodGet {
			WriteError(w, 405)
			return
		}
		h.listStoredMessages(w, req, scope, id)
		return
	}
	if req.URL.RawQuery != "" {
		WriteError(w, 400)
		return
	}
	switch req.Method {
	case http.MethodGet:
		record, err := h.storage.Store.Get(req.Context(), scope, id)
		if err != nil {
			writeStoredError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(record.Completion)
	case http.MethodDelete:
		if err := h.storage.Store.Delete(req.Context(), scope, id); err != nil {
			writeStoredError(w, err)
			return
		}
		writeStoredJSON(w, map[string]any{"id": id, "object": "chat.completion.deleted", "deleted": true})
	case http.MethodPost:
		media, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			WriteError(w, 415)
			return
		}
		if req.Header.Get("Content-Encoding") != "" {
			WriteError(w, 400)
			return
		}
		raw, err := io.ReadAll(io.LimitReader(req.Body, (128<<10)+1))
		var body struct {
			Metadata map[string]string `json:"metadata"`
		}
		if err != nil || len(raw) > 128<<10 || !utf8.Valid(raw) || decodeStrict(raw, &body) != nil || body.Metadata == nil || !validStoredMetadata(body.Metadata) {
			WriteError(w, 400)
			return
		}
		record, err := h.storage.Store.UpdateMetadata(req.Context(), scope, id, body.Metadata)
		if err != nil {
			writeStoredError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(record.Completion)
	default:
		WriteError(w, 405)
	}
}

type storedQuery struct {
	limit               int
	after, order, model string
	metadata            map[string]string
}

func parseStoredQuery(req *http.Request, filter bool) (storedQuery, error) {
	q := storedQuery{limit: 20, order: "asc", metadata: map[string]string{}}
	values, err := url.ParseQuery(req.URL.RawQuery)
	if err != nil {
		return q, errRequest
	}
	for key, items := range values {
		if len(items) != 1 {
			return q, errRequest
		}
		value := items[0]
		switch key {
		case "limit":
			q.limit, err = strconv.Atoi(value)
			if err != nil || q.limit < 1 || q.limit > 100 {
				return q, errRequest
			}
		case "after":
			q.after = value
		case "order":
			if value != "asc" && value != "desc" {
				return q, errRequest
			}
			q.order = value
		case "model":
			if !filter {
				return q, errRequest
			}
			q.model = value
		default:
			if !filter || !strings.HasPrefix(key, "metadata[") || !strings.HasSuffix(key, "]") {
				return q, errRequest
			}
			q.metadata[key[9:len(key)-1]] = value
		}
	}
	if !validStoredMetadata(q.metadata) {
		return q, errRequest
	}
	return q, nil
}
func (h *handler) listStored(w http.ResponseWriter, req *http.Request, scope StorageScope) {
	q, err := parseStoredQuery(req, true)
	if err != nil {
		WriteError(w, 400)
		return
	}
	page, err := h.storage.Store.List(req.Context(), scope, StorageListOptions{Limit: q.limit, After: q.after, Descending: q.order == "desc", Model: q.model, Metadata: q.metadata})
	if err != nil {
		if q.after != "" && errors.Is(err, ErrStoredCompletionNotFound) {
			WriteError(w, 400)
		} else {
			writeStoredError(w, err)
		}
		return
	}
	data := []json.RawMessage{}
	first, last := "", ""
	for _, record := range page.Records {
		if first == "" {
			first = record.ID
		}
		last = record.ID
		data = append(data, record.Completion)
	}
	writeStoredJSON(w, map[string]any{"object": "list", "data": data, "first_id": first, "last_id": last, "has_more": page.HasMore})
}
func (h *handler) listStoredMessages(w http.ResponseWriter, req *http.Request, scope StorageScope, id string) {
	q, err := parseStoredQuery(req, false)
	if err != nil {
		WriteError(w, 400)
		return
	}
	record, err := h.storage.Store.Get(req.Context(), scope, id)
	if err != nil {
		writeStoredError(w, err)
		return
	}
	data := []map[string]any{}
	first, last := "", ""
	after := q.after == ""
	hasMore := false
	for step := 0; step < len(record.Messages); step++ {
		i := step
		if q.order == "desc" {
			i = len(record.Messages) - 1 - step
		}
		messageID := fmt.Sprintf("%s-message-%d", id, i)
		if !after {
			if messageID == q.after {
				after = true
			}
			continue
		}
		if len(data) == q.limit {
			hasMore = true
			break
		}
		var message map[string]any
		if json.Unmarshal(record.Messages[i], &message) != nil {
			WriteError(w, 503)
			return
		}
		message["id"] = messageID
		message["content_parts"] = nil
		if parts, ok := message["content"].([]any); ok {
			message["content_parts"] = parts
			message["content"] = nil
		}
		if _, ok := message["name"]; !ok {
			message["name"] = nil
		}
		if first == "" {
			first = messageID
		}
		last = messageID
		data = append(data, message)
	}
	if !after {
		WriteError(w, 400)
		return
	}
	writeStoredJSON(w, map[string]any{"object": "list", "data": data, "first_id": first, "last_id": last, "has_more": hasMore})
}
func (h *handler) persistCompletion(ctx context.Context, r mappedRequest, scope StorageScope, raw []byte) error {
	var out map[string]json.RawMessage
	if json.Unmarshal(raw, &out) != nil {
		return ErrCompletionStorageUnavailable
	}
	delete(out, "grafana")
	var choices []map[string]json.RawMessage
	if json.Unmarshal(out["choices"], &choices) != nil {
		return ErrCompletionStorageUnavailable
	}
	for _, choice := range choices {
		delete(choice, "grafana")
	}
	out["choices"], _ = json.Marshal(choices)
	metadata := r.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	out["metadata"], _ = json.Marshal(metadata)
	record := StoredCompletion{Metadata: metadata, ExpiresAt: time.Now().Add(h.storage.Retention).Unix()}
	if json.Unmarshal(out["id"], &record.ID) != nil || json.Unmarshal(out["created"], &record.Created) != nil || json.Unmarshal(out["model"], &record.Model) != nil {
		return ErrCompletionStorageUnavailable
	}
	record.Completion, _ = json.Marshal(out)
	record.Messages = r.originalMessages
	encoded, err := json.Marshal(record)
	if err != nil || len(encoded) > MaxStoredCompletionBytes {
		return ErrCompletionStorageUnavailable
	}
	return h.storage.Store.Create(ctx, scope, record)
}
