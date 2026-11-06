package gateway

import (
	"encoding/json"
	"fmt"
	stored "github.com/grafana/ai-sdk/ai-gateway/pkg/internal/storedchatcompletion/v1alpha1"
	"github.com/grafana/ai-sdk/ai-gateway/test/platformhost"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// The API here simulates resource persistence, not host RBAC deployment.
func TestGatewayStoredLifecycleThroughAuthenticatedSDK(t *testing.T) {
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
				if filter != "" && filter != "aigateway.ext.grafana.app/owner"+"="+object.Spec.Owner {
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

	t.Cleanup(server.Close)
	h := newHarnessWithOptions(t, platformhost.TopologySelfHosted, harnessOptions{kubeConfig: rest.Config{Host: server.URL}})
	token := h.host.AddServiceAccount("alice", testNamespace)
	client := openai.NewClient(option.WithBaseURL(h.url("v1")), option.WithAPIKey(token), option.WithMaxRetries(0))
	params := openai.ChatCompletionNewParams{Model: "public", Store: openai.Bool(true), Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, Metadata: map[string]string{"label": "original"}}
	completion, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	fetched, err := client.Chat.Completions.Get(t.Context(), completion.ID)
	require.NoError(t, err)
	require.Equal(t, completion.ID, fetched.ID)
	updated, err := client.Chat.Completions.Update(t.Context(), completion.ID, openai.ChatCompletionUpdateParams{Metadata: map[string]string{"label": "changed"}})
	require.NoError(t, err)
	require.Equal(t, "changed", updated.Metadata["label"])
	page, err := client.Chat.Completions.List(t.Context(), openai.ChatCompletionListParams{})
	require.NoError(t, err)
	require.Len(t, page.Data, 1)
	messages, err := client.Chat.Completions.Messages.List(t.Context(), completion.ID, openai.ChatCompletionMessageListParams{})
	require.NoError(t, err)
	require.Len(t, messages.Data, 1)
	require.Equal(t, "hello", messages.Data[0].Content)
	other := openai.NewClient(option.WithBaseURL(h.url("v1")), option.WithAPIKey(h.host.AddServiceAccount("bob", testNamespace)), option.WithMaxRetries(0))
	_, err = other.Chat.Completions.Get(t.Context(), completion.ID)
	var api *openai.Error
	require.ErrorAs(t, err, &api)
	require.Equal(t, 404, api.StatusCode)
	// This deterministic native fixture returns msg_1 every time; delete before reuse.
	deleted, err := client.Chat.Completions.Delete(t.Context(), completion.ID)
	require.NoError(t, err)
	require.True(t, deleted.Deleted)
	stream := client.Chat.Completions.NewStreaming(t.Context(), params)
	id := ""
	for stream.Next() {
		id = stream.Current().ID
	}
	require.NoError(t, stream.Err())
	streamed, err := client.Chat.Completions.Get(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, "firstsecond", streamed.Choices[0].Message.Content)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, objects, 1)
	for _, object := range objects {
		require.NotContains(t, object.Spec.Record, providerSecret)
		require.NotContains(t, object.Spec.Record, "providerOptions")
	}
}
