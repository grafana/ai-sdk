package chatcompletions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"
)

// This in-memory fake is only a test double; production binds app resource storage.
type testCompletionStore struct {
	mu      sync.Mutex
	records map[string]StoredCompletion
	fail    bool
}

func (s *testCompletionStore) Ready(context.Context, StorageScope) error { return nil }
func (s *testCompletionStore) Create(_ context.Context, scope StorageScope, r StoredCompletion) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return ErrCompletionStorageUnavailable
	}
	s.records[scope.Namespace+"/"+scope.Owner+"/"+r.ID] = r
	return nil
}
func (s *testCompletionStore) Get(_ context.Context, scope StorageScope, id string) (StoredCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.records[scope.Namespace+"/"+scope.Owner+"/"+id]
	if !ok {
		return r, ErrStoredCompletionNotFound
	}
	return r, nil
}
func (s *testCompletionStore) List(_ context.Context, scope StorageScope, q StorageListOptions) (StoredCompletionPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []StoredCompletion
	for k, r := range s.records {
		if strings.HasPrefix(k, scope.Namespace+"/"+scope.Owner+"/") {
			result = append(result, r)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		less := result[i].Created < result[j].Created || result[i].Created == result[j].Created && result[i].ID < result[j].ID
		if q.Descending {
			return !less
		}
		return less
	})
	var page StoredCompletionPage
	after := q.After == ""
	for _, r := range result {
		if !after {
			if r.ID == q.After {
				after = true
			}
			continue
		}
		if q.Model != "" && q.Model != r.Model {
			continue
		}
		match := true
		for key, value := range q.Metadata {
			if actual, exists := r.Metadata[key]; !exists || actual != value {
				match = false
			}
		}
		if !match {
			continue
		}
		if len(page.Records) == q.Limit {
			page.HasMore = true
			break
		}
		page.Records = append(page.Records, r)
	}
	if !after {
		return page, ErrStoredCompletionNotFound
	}
	return page, nil
}

func (s *testCompletionStore) UpdateMetadata(ctx context.Context, scope StorageScope, id string, m map[string]string) (StoredCompletion, error) {
	r, err := s.Get(ctx, scope, id)
	if err != nil {
		return r, err
	}
	r.Metadata = m
	var out map[string]any
	_ = json.Unmarshal(r.Completion, &out)
	out["metadata"] = m
	r.Completion, _ = json.Marshal(out)
	return r, s.Create(ctx, scope, r)
}
func (s *testCompletionStore) Delete(_ context.Context, scope StorageScope, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := scope.Namespace + "/" + scope.Owner + "/" + id
	if _, ok := s.records[key]; !ok {
		return ErrStoredCompletionNotFound
	}
	delete(s.records, key)
	return nil
}

type storageTestOwner struct{}

func TestOfficialStoredCompletionLifecycle(t *testing.T) {
	store := &testCompletionStore{records: map[string]StoredCompletion{}}
	f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		result := textResult()
		result.ProviderMetadata = provider.ProviderMetadata{"private": json.RawMessage(`{"secret":"native-diagnostic"}`)}
		return result, nil
	}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(textParts()...), nil
	}}
	h := testHandler(t, f)
	require.NoError(t, WithStorage(StorageConfig{Store: store, Retention: time.Hour, ResolveScope: func(ctx context.Context) (StorageScope, bool) {
		return StorageScope{Namespace: "tenant", Owner: ctx.Value(storageTestOwner{}).(string)}, true
	}})(h))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		owner := r.Header.Get("Test-Owner")
		if owner == "" {
			owner = "alice"
		}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), storageTestOwner{}, owner)))
	}))
	defer server.Close()
	client := openai.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	params := openai.ChatCompletionNewParams{Model: "alias", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}, Store: openai.Bool(true), Metadata: map[string]string{"label": "one"}}
	first, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	got, err := client.Chat.Completions.Get(t.Context(), first.ID)
	require.NoError(t, err)
	require.Equal(t, first.Choices[0].Message.Content, got.Choices[0].Message.Content)
	messages, err := client.Chat.Completions.Messages.List(t.Context(), first.ID, openai.ChatCompletionMessageListParams{})
	require.NoError(t, err)
	require.Len(t, messages.Data, 1)
	require.Equal(t, "hello", messages.Data[0].Content)
	updated, err := client.Chat.Completions.Update(t.Context(), first.ID, openai.ChatCompletionUpdateParams{Metadata: map[string]string{"label": "two"}})
	require.NoError(t, err)
	require.Equal(t, "two", updated.Metadata["label"])
	second, err := client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	page, err := client.Chat.Completions.List(t.Context(), openai.ChatCompletionListParams{Limit: openai.Int(1)})
	require.NoError(t, err)
	require.Len(t, page.Data, 1)
	require.True(t, page.HasMore)
	next, err := client.Chat.Completions.List(t.Context(), openai.ChatCompletionListParams{After: openai.String(page.Data[0].ID)})
	require.NoError(t, err)
	require.Len(t, next.Data, 1)
	require.NotEqual(t, page.Data[0].ID, next.Data[0].ID)
	_, err = client.Chat.Completions.Get(t.Context(), first.ID, option.WithHeader("Test-Owner", "bob"))
	var api *openai.Error
	require.ErrorAs(t, err, &api)
	require.Equal(t, 404, api.StatusCode)
	deleted, err := client.Chat.Completions.Delete(t.Context(), first.ID)
	require.NoError(t, err)
	require.True(t, deleted.Deleted)
	_, err = client.Chat.Completions.Get(t.Context(), first.ID)
	require.ErrorAs(t, err, &api)
	require.Equal(t, 404, api.StatusCode)
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeObfuscation: openai.Bool(false)}
	stream := client.Chat.Completions.NewStreaming(t.Context(), params)
	streamID := ""
	for stream.Next() {
		chunk := stream.Current()
		streamID = chunk.ID
		require.NotContains(t, chunk.RawJSON(), `"obfuscation"`)
		require.False(t, chunk.Usage.JSON.TotalTokens.Valid(), "usage remains hidden unless requested")
	}
	require.NoError(t, stream.Err())
	streamed, err := client.Chat.Completions.Get(t.Context(), streamID)
	require.NoError(t, err)
	require.NotEmpty(t, streamed.Choices[0].Message.Content)
	require.NotZero(t, streamed.Usage.TotalTokens)
	record, err := store.Get(t.Context(), StorageScope{Namespace: "tenant", Owner: "alice"}, second.ID)
	require.NoError(t, err)
	require.NotContains(t, string(record.Completion), "native-diagnostic")
	require.NotContains(t, string(record.Completion), "grafana")
	store.mu.Lock()
	before := len(store.records)
	store.mu.Unlock()
	params.StreamOptions = openai.ChatCompletionStreamOptionsParam{}
	params.Store = openai.Bool(false)
	_, err = client.Chat.Completions.New(t.Context(), params)
	require.NoError(t, err)
	store.mu.Lock()
	require.Len(t, store.records, before)
	store.fail = true
	store.mu.Unlock()
	params.Store = openai.Bool(true)
	_, err = client.Chat.Completions.New(t.Context(), params)
	require.ErrorAs(t, err, &api)
	require.Equal(t, 503, api.StatusCode)
	stream = client.Chat.Completions.NewStreaming(t.Context(), params)
	for stream.Next() {
	}
	require.Error(t, stream.Err())
}

func TestStoredStreamPreservesIndexedCustomAndAudio(t *testing.T) {
	recorder := httptest.NewRecorder()
	var stored []byte
	writer := newStoredStreamWriter(recorder, func(raw []byte) error { stored = raw; return nil }, false)
	frames := []string{
		`{"id":"multi","created":1,"model":"native","choices":[{"index":1,"delta":{"role":"assistant","audio":{"id":"audio","data":"YQ==","transcript":"hel"}}},{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"same","type":"custom","custom":{"name":"write","input":"one"}}]}}]}`,
		`{"id":"multi","created":1,"model":"native","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"custom":{"input":"two"}}]},"finish_reason":"tool_calls"},{"index":1,"delta":{"audio":{"data":"Yg==","transcript":"lo","expires_at":99},"annotations":[{"type":"url_citation","url_citation":{"url":"https://example.com","title":"source","start_index":0,"end_index":1}}]},"logprobs":{"content":[{"token":"hello","logprob":-0.1,"bytes":null,"top_logprobs":[]}]},"finish_reason":"stop"}]}`,
		`{"id":"multi","created":1,"model":"native","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":2,"total_tokens":4}}`,
		`[DONE]`,
	}
	for _, frame := range frames {
		_, err := writer.Write([]byte("data: " + frame + "\n\n"))
		require.NoError(t, err)
	}
	var result struct {
		Choices []struct {
			Index   int
			Message struct {
				Audio       map[string]any
				ToolCalls   []struct{ Custom struct{ Name, Input string } } `json:"tool_calls"`
				Annotations []any
			}
			Logprobs any
		}
		Usage struct {
			Total int `json:"total_tokens"`
		}
	}
	require.NoError(t, json.Unmarshal(stored, &result))
	require.Len(t, result.Choices, 2)
	require.Equal(t, "onetwo", result.Choices[0].Message.ToolCalls[0].Custom.Input)
	require.Equal(t, "write", result.Choices[0].Message.ToolCalls[0].Custom.Name)
	require.Equal(t, "YWI=", result.Choices[1].Message.Audio["data"])
	require.Equal(t, "hello", result.Choices[1].Message.Audio["transcript"])
	require.Len(t, result.Choices[1].Message.Annotations, 1)
	require.NotNil(t, result.Choices[1].Logprobs)
	require.Equal(t, 4, result.Usage.Total)
}
func TestStoredMessagesPreserveMultipartAndPaging(t *testing.T) {
	store := &testCompletionStore{records: map[string]StoredCompletion{}}
	scope := StorageScope{Namespace: "tenant", Owner: "alice"}
	h := testHandler(t, &fakeModel{})
	require.NoError(t, WithStorage(StorageConfig{Store: store, Retention: time.Hour, ResolveScope: func(context.Context) (StorageScope, bool) { return scope, true }})(h))
	r := requestWith(t, `,"store":true`)
	r.originalMessages = []json.RawMessage{json.RawMessage(`{"role":"user","name":"alice","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}`), json.RawMessage(`{"role":"function","name":"weather","content":"sunny"}`)}
	require.NoError(t, h.persistCompletion(t.Context(), r, scope, []byte(`{"id":"stored","created":1,"model":"native","choices":[]}`)))
	req := httptest.NewRequest("GET", Path+"/stored/messages?limit=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var page struct {
		Data []map[string]any
		Last string `json:"last_id"`
		More bool   `json:"has_more"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page))
	require.Len(t, page.Data, 1)
	require.Nil(t, page.Data[0]["content"])
	require.Len(t, page.Data[0]["content_parts"], 2)
	require.Equal(t, "alice", page.Data[0]["name"])
	require.True(t, page.More)
	req = httptest.NewRequest("GET", Path+"/stored/messages?after="+page.Last, nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Contains(t, w.Body.String(), `"role":"function"`)
	require.Contains(t, w.Body.String(), `"name":"weather"`)
}

func TestOfficialTypeScriptStoredCompletionLifecycle(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("official TypeScript fixture requires Node.js")
	}
	fixture, err := filepath.Abs("../../test/openai-chat-completions-adapter/package.json")
	require.NoError(t, err)
	if _, err = os.Stat(filepath.Join(filepath.Dir(fixture), "node_modules/openai")); err != nil {
		t.Skip("install the official Chat client fixture dependencies")
	}
	store := &testCompletionStore{records: map[string]StoredCompletion{}}
	f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return textResult(), nil
	}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(textParts()...), nil
	}}
	h := testHandler(t, f)
	require.NoError(t, WithStorage(StorageConfig{Store: store, Retention: time.Hour, ResolveScope: func(context.Context) (StorageScope, bool) {
		return StorageScope{Namespace: "tenant", Owner: "typescript"}, true
	}})(h))
	server := httptest.NewServer(h)
	defer server.Close()
	script := `import {createRequire} from 'node:module';
import assert from 'node:assert/strict';
const require = createRequire(process.argv[2]);
const OpenAI = require('openai').default;
const client: InstanceType<typeof OpenAI> = new OpenAI({baseURL:process.argv[3]+'/v1',apiKey:'synthetic',maxRetries:0});
const request = {model:'alias',messages:[{role:'user',content:'hello'}],store:true,metadata:{label:'typescript'}};
const created = await client.chat.completions.create(request);
const got = await client.chat.completions.retrieve(created.id);
assert.equal(got.choices[0].message.content,created.choices[0].message.content);
const messages = await client.chat.completions.messages.list(created.id,{limit:1});
assert.equal(messages.data[0].content,'hello');
const updated = await client.chat.completions.update(created.id,{metadata:{label:'updated'}});
assert.equal(updated.metadata.label,'updated');
const listed = await client.chat.completions.list({metadata:{label:'updated'}});
assert.equal(listed.data.length,1);
const stream = await client.chat.completions.create({...request,stream:true});
let id = ''; for await(const chunk of stream){id=chunk.id;assert.equal(chunk.usage,null);}
const streamed = await client.chat.completions.retrieve(id);assert.ok(streamed.usage.total_tokens>0);
assert.equal((await client.chat.completions.delete(created.id)).deleted,true);
await assert.rejects(()=>client.chat.completions.retrieve(created.id),(error:any)=>error.status===404);
`
	path := filepath.Join(t.TempDir(), "lifecycle.mts")
	require.NoError(t, os.WriteFile(path, []byte(script), 0600))
	output, err := exec.CommandContext(t.Context(), node, path, fixture, server.URL).CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestStoredStreamLargeToolIndexAndModeration(t *testing.T) {
	writer := newStoredStreamWriter(httptest.NewRecorder(), nil, false)
	require.NoError(t, writer.consume([]byte(`{"id":"stored","moderation":{"model":"native"},"choices":[{"index":0,"delta":{"tool_calls":[{"index":128,"id":"call128","type":"function","function":{"name":"weather","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)))
	raw, err := writer.complete()
	require.NoError(t, err)
	require.Contains(t, string(raw), `"moderation":{"model":"native"}`)
	require.Contains(t, string(raw), `"id":"call128"`)
}

func TestStoredRecordBoundsAndDiagnosticRemoval(t *testing.T) {
	store := &testCompletionStore{records: map[string]StoredCompletion{}}
	h := testHandler(t, &fakeModel{})
	scope := StorageScope{Namespace: "tenant", Owner: "alice"}
	require.NoError(t, WithStorage(StorageConfig{Store: store, Retention: time.Hour, ResolveScope: func(context.Context) (StorageScope, bool) { return scope, true }})(h))
	r := mappedRequest{}
	r.originalMessages = []json.RawMessage{json.RawMessage(`{"role":"user","content":"keep user text"}`)}
	raw := []byte(`{"id":"record","created":1,"model":"model","grafana":{"secret":"root-secret"},"choices":[{"index":0,"grafana":{"secret":"choice-secret"},"message":{"role":"assistant","content":"public output"},"finish_reason":"stop"}]}`)
	require.NoError(t, h.persistCompletion(t.Context(), r, scope, raw))
	record, err := store.Get(t.Context(), scope, "record")
	require.NoError(t, err)
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "root-secret")
	require.NotContains(t, string(encoded), "choice-secret")
	require.Contains(t, string(encoded), "keep user text")
	r.originalMessages = []json.RawMessage{json.RawMessage(`{"role":"user","content":"` + strings.Repeat("a", MaxStoredCompletionBytes) + `"}`)}
	require.ErrorIs(t, h.persistCompletion(t.Context(), r, scope, raw), ErrCompletionStorageUnavailable)
	unchanged, err := store.Get(t.Context(), scope, "record")
	require.NoError(t, err)
	require.Equal(t, record, unchanged)
}

func TestStoredInputQuotaBeforeModelInvocation(t *testing.T) {
	store := &testCompletionStore{records: map[string]StoredCompletion{}}
	f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		t.Fatal("oversized stored input must not invoke provider")
		return nil, nil
	}}
	h := testHandler(t, f)
	require.NoError(t, WithStorage(StorageConfig{Store: store, Retention: time.Hour, ResolveScope: func(context.Context) (StorageScope, bool) {
		return StorageScope{Namespace: "tenant", Owner: "alice"}, true
	}})(h))
	body := `{"model":"alias","store":true,"messages":[{"role":"user","content":"` + strings.Repeat("a", MaxStoredCompletionBytes) + `"}]}`
	req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	require.Empty(t, store.records)
}

func TestStoredAudioRejectsContradictoryIdentity(t *testing.T) {
	for _, field := range []string{"id", "expires_at"} {
		t.Run(field, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writer := newStoredStreamWriter(recorder, func([]byte) error { t.Fatal("invalid audio must not persist"); return nil }, false)
			_, err := writer.Write([]byte("data: " + `{"choices":[{"index":0,"delta":{"audio":{"id":"one","expires_at":1,"data":"YQ=="}}}]}` + "\n\n"))
			require.NoError(t, err)
			value := `"two"`
			if field == "expires_at" {
				value = "2"
			}
			_, err = writer.Write([]byte("data: " + `{"choices":[{"index":0,"delta":{"audio":{"` + field + `":` + value + `}}}]}` + "\n\n"))
			require.Error(t, err)
			require.Contains(t, recorder.Body.String(), `"error"`)
			require.NotContains(t, recorder.Body.String(), "[DONE]")
		})
	}
}

func TestStoredStreamQuotaMeasuresLogicalState(t *testing.T) {
	t.Run("tiny frames and repeated padding", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		var saved []byte
		writer := newStoredStreamWriter(recorder, func(raw []byte) error { saved = raw; return nil }, false)
		frame := []byte("data: " + `{"id":"tiny","created":1,"model":"m","obfuscation":"` + strings.Repeat("x", 256) + `","moderation":{"model":"repeated"},"choices":[{"index":0,"delta":{"content":"x"}}]}` + "\n\n")
		for i := 0; i < 8192; i++ {
			_, err := writer.Write(frame)
			require.NoError(t, err)
		}
		_, err := writer.Write([]byte("data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n"))
		require.NoError(t, err)
		_, err = writer.Write([]byte("data: [DONE]\n\n"))
		require.NoError(t, err)
		require.Less(t, len(saved), 10<<10)
		require.Greater(t, recorder.Body.Len(), 2<<20)
	})
	t.Run("logical overflow", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		writer := newStoredStreamWriter(recorder, func([]byte) error { t.Fatal("oversized state must not save"); return nil }, false)
		frame := []byte("data: " + `{"choices":[{"index":0,"delta":{"content":"` + strings.Repeat("x", 64<<10) + `"}}]}` + "\n\n")
		var err error
		for i := 0; i < 10 && err == nil; i++ {
			_, err = writer.Write(frame)
		}
		require.Error(t, err)
		require.Contains(t, recorder.Body.String(), `"error"`)
		require.NotContains(t, recorder.Body.String(), "[DONE]")
	})
}
