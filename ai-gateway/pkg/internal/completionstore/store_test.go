package completionstore

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	chat "github.com/grafana/ai-sdk/ai-gateway/openai/chatcompletions"
	stored "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/storedchatcompletion/v1alpha1"
	"github.com/grafana/grafana-app-sdk/k8s"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

func TestResourceStorageLifecycleIsolationAndExpiry(t *testing.T) {
	var mu sync.Mutex
	objects := map[string]*stored.StoredChatCompletion{}
	revision := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		namespace, name := "", ""
		if len(parts) >= 6 && parts[3] == "namespaces" {
			namespace = parts[4]
			if len(parts) == 7 {
				name = parts[6]
			}
		}
		key := namespace + "/" + name
		fail := func(code int, reason string) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]any{"kind": "Status", "apiVersion": "v1", "status": "Failure", "reason": reason, "code": code})
		}
		switch r.Method {
		case http.MethodGet:
			if name != "" {
				if object := objects[key]; object != nil {
					_ = json.NewEncoder(w).Encode(object)
				} else {
					fail(404, "NotFound")
				}
				return
			}
			items := []*stored.StoredChatCompletion{}
			for _, object := range objects {
				if namespace != "" && object.Namespace != namespace {
					continue
				}
				filter := r.URL.Query().Get("labelSelector")
				if filter != "" && filter != ownerLabel+"="+object.Spec.Owner {
					continue
				}
				items = append(items, object)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "aigateway.ext.grafana.app/v1alpha1", "kind": "StoredChatCompletionList", "metadata": map[string]any{}, "items": items})
		case http.MethodPost, http.MethodPut:
			var object stored.StoredChatCompletion
			require.NoError(t, json.NewDecoder(r.Body).Decode(&object))
			key = namespace + "/" + object.Name
			if r.Method == http.MethodPost && objects[key] != nil {
				fail(409, "AlreadyExists")
				return
			}
			if r.Method == http.MethodPut && (objects[key] == nil || objects[key].ResourceVersion != object.ResourceVersion) {
				fail(409, "Conflict")
				return
			}
			revision++
			object.ResourceVersion = fmt.Sprint(revision)
			objects[key] = &object
			_ = json.NewEncoder(w).Encode(object)
		case http.MethodDelete:
			if objects[key] == nil {
				fail(404, "NotFound")
				return
			}
			delete(objects, key)
			_ = json.NewEncoder(w).Encode(map[string]any{"kind": "Status", "apiVersion": "v1", "status": "Success", "code": 200})
		}
	}))
	defer server.Close()
	registry := k8s.NewClientRegistry(rest.Config{Host: server.URL}, k8s.DefaultClientConfig())
	client, err := registry.ClientFor(stored.Kind())
	require.NoError(t, err)
	now := time.Unix(1000, 0)
	store := New()
	store.now = func() time.Time { return now }
	store.Bind(client)
	alice := chat.StorageScope{Namespace: "tenant-a", Owner: "user:alice"}
	bob := chat.StorageScope{Namespace: "tenant-a", Owner: "user:bob"}
	otherTenant := chat.StorageScope{Namespace: "tenant-b", Owner: "user:alice"}
	record := chat.StoredCompletion{ID: "chatcmpl-one", Created: 1000, ExpiresAt: 1060, Model: "native", Metadata: map[string]string{"label": "one"}, Completion: json.RawMessage(`{"id":"chatcmpl-one","choices":[]}`), Messages: []json.RawMessage{json.RawMessage(`{"role":"user","content":"hello"}`)}}
	require.NoError(t, store.Create(t.Context(), alice, record))
	_, err = store.Get(t.Context(), bob, record.ID)
	require.ErrorIs(t, err, chat.ErrStoredCompletionNotFound)
	_, err = store.Get(t.Context(), otherTenant, record.ID)
	require.ErrorIs(t, err, chat.ErrStoredCompletionNotFound)
	require.ErrorIs(t, store.Delete(t.Context(), bob, record.ID), chat.ErrStoredCompletionNotFound)
	_, err = store.UpdateMetadata(t.Context(), bob, record.ID, map[string]string{"hijack": "yes"})
	require.ErrorIs(t, err, chat.ErrStoredCompletionNotFound)
	list, err := store.List(t.Context(), bob)
	require.NoError(t, err)
	require.Empty(t, list)
	restarted := New()
	restarted.now = func() time.Time { return now }
	restarted.Bind(client)
	saved, err := restarted.Get(t.Context(), alice, record.ID)
	require.NoError(t, err)
	require.Equal(t, record.Messages, saved.Messages)
	updated, err := restarted.UpdateMetadata(t.Context(), alice, record.ID, map[string]string{"label": "two"})
	require.NoError(t, err)
	require.Equal(t, "two", updated.Metadata["label"])
	require.Contains(t, string(updated.Completion), `"label":"two"`)
	list, err = restarted.List(t.Context(), alice)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, restarted.Delete(t.Context(), alice, record.ID))
	require.NoError(t, restarted.Create(t.Context(), alice, record))
	now = time.Unix(1061, 0)
	_, err = restarted.Get(t.Context(), alice, record.ID)
	require.ErrorIs(t, err, chat.ErrStoredCompletionNotFound)
	now = time.Unix(1000, 0)
	require.NoError(t, restarted.Create(t.Context(), otherTenant, record))
	now = time.Unix(1061, 0)
	require.NoError(t, restarted.Sweep(t.Context()))
	require.Empty(t, objects)
}
