// Package chatcompletionsadapter_test exercises the real command with the official Go SDK.
// The upstream below is a deterministic synthetic test server, not a recording.
package chatcompletionsadapter_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type upstream struct {
	mu          sync.Mutex
	requests    []map[string]any
	fail        bool
	hold        bool
	invalidJSON bool
	cancelled   chan struct{}
}

func (u *upstream) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		w.WriteHeader(400)
		return
	}
	u.mu.Lock()
	u.requests = append(u.requests, body)
	fail, hold, invalidJSON := u.fail, u.hold, u.invalidJSON
	u.mu.Unlock()
	if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer fake-backend-key" {
		w.WriteHeader(401)
		return
	}
	if fail {
		w.WriteHeader(503)
		_, _ = io.WriteString(w, `{"error":{"message":"private-upstream-secret","type":"server_error"}}`)
		return
	}
	if hold {
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: response.created\ndata: {\"type\":\"response.created\",\"sequence_number\":0,\"response\":{\"id\":\"resp_hold\",\"status\":\"in_progress\",\"output\":[]}}\n\n")
			_, _ = io.WriteString(w, "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"sequence_number\":1,\"output_index\":0,\"item\":{\"type\":\"message\",\"id\":\"msg_hold\",\"role\":\"assistant\",\"status\":\"in_progress\",\"content\":[]}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":2,\"item_id\":\"msg_hold\",\"output_index\":0,\"content_index\":0,\"delta\":\"hello\"}\n\n")
			w.(http.Flusher).Flush()
		}
		<-r.Context().Done()
		select {
		case u.cancelled <- struct{}{}:
		default:
		}
		return
	}
	text := "hello"
	if _, ok := body["text"]; ok {
		text = `{"answer":"yes"}`
		if invalidJSON {
			text = `{"answer":7}`
		}
	}
	encoded, _ := json.Marshal(body["input"])
	continuation := bytes.Contains(encoded, []byte("function_call_output"))
	tools, hasTools := body["tools"].([]any)
	function := hasTools && len(tools) > 0 && !continuation
	message := map[string]any{"id": "msg_test", "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}, "logprobs": []any{}}}}
	item := message
	if function {
		item = map[string]any{"id": "fc_private", "call_id": "call_weather", "type": "function_call", "name": "weather", "arguments": `{"city":"Rio"}`, "status": "completed"}
	}
	response := map[string]any{"id": "resp_private", "object": "response", "created_at": 1, "status": "completed", "model": body["model"], "output": []any{item}, "usage": map[string]any{"input_tokens": 3, "input_tokens_details": map[string]any{"cached_tokens": 1}, "output_tokens": 2, "output_tokens_details": map[string]any{"reasoning_tokens": 1}, "total_tokens": 5}}
	if body["stream"] != true {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	events := []map[string]any{{"type": "response.created", "response": map[string]any{"id": "resp_private", "model": body["model"], "created_at": 1, "status": "in_progress", "output": []any{}}}}
	start := map[string]any{}
	for k, v := range item {
		start[k] = v
	}
	if function {
		start["arguments"] = ""
	} else {
		start["content"] = []any{}
	}
	events = append(events, map[string]any{"type": "response.output_item.added", "output_index": 0, "item": start})
	if function {
		for _, fragment := range []string{`{"city":`, `"Rio"}`} {
			events = append(events, map[string]any{"type": "response.function_call_arguments.delta", "item_id": "fc_private", "output_index": 0, "delta": fragment})
		}
		events = append(events, map[string]any{"type": "response.function_call_arguments.done", "item_id": "fc_private", "output_index": 0, "arguments": item["arguments"]})
	} else {
		events = append(events, map[string]any{"type": "response.output_text.delta", "item_id": "msg_test", "output_index": 0, "content_index": 0, "delta": text, "logprobs": []any{}})
	}
	events = append(events, map[string]any{"type": "response.output_item.done", "output_index": 0, "item": item}, map[string]any{"type": "response.completed", "response": response})
	for i, event := range events {
		event["sequence_number"] = i
		b, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], b)
		w.(http.Flusher).Flush()
	}
}
func (u *upstream) last() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.requests[len(u.requests)-1]
}
func (u *upstream) count() int { u.mu.Lock(); defer u.mu.Unlock(); return len(u.requests) }

func startCommand(t *testing.T, backend, topology string, customConfig ...string) string {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "gateway")
	hostBinary := filepath.Join(dir, "plugin-host")
	for _, target := range []struct{ binary, source string }{{binary, "../../cmd/grafana-ai-gateway"}, {hostBinary, "../platformhost/cmd/plugin-host"}} {
		build := exec.Command("go", "build", "-race", "-mod=readonly", "-o", target.binary, target.source)
		build.Env = append(os.Environ(), "GOWORK=off")
		output, err := build.CombinedOutput()
		require.NoError(t, err, string(output))
	}
	config := fmt.Sprintf("providers:\n  test:\n    type: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s/v1\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: test\n      model: gpt-4.1\n    aliases: [chat]\n  public/reasoning:\n    name: Reasoning\n    primary:\n      provider: test\n      model: o3-mini\n", backend)
	if len(customConfig) > 0 {
		config = customConfig[0]
	}
	configPath := filepath.Join(dir, "models.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
	cmd := exec.Command(hostBinary, "--plugin", binary, "--topology", topology, "--namespace", "stacks-1", "--service-account-token", testToken, "--route-deadline", "10s")
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "GF_PLUGIN_") && !strings.HasPrefix(name, "GRAFANA_") && !strings.HasPrefix(name, "SIGIL_") && !strings.HasPrefix(name, "AGENTO11Y_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "CHAT_COMPLETIONS_ADAPTER_TEST_KEY=fake-backend-key", "GF_PLUGIN_CONFIG_FILE="+configPath, "GF_PLUGIN_ACCESS_MODE="+topology)
	if topology == "cloud" {
		cmd.Env = append(cmd.Env, "GF_PLUGIN_ACCESS_INTERNAL_SERVICES=integration")
	}
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case err := <-done:
				assert.NoError(t, err, stderr.String())
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				assert.Error(t, <-done)
				t.Error("shutdown exceeded bound")
			}
			assert.NotContains(t, stderr.String(), "fake-backend-key")
		})
	}
	t.Cleanup(stop)
	var ready struct {
		URL         string `json:"url"`
		RoutePrefix string `json:"routePrefix"`
	}
	readyDone := make(chan error, 1)
	go func() { readyDone <- json.NewDecoder(stdout).Decode(&ready) }()
	select {
	case err := <-readyDone:
		require.NoError(t, err)
	case <-time.After(30 * time.Second):
		stop()
		t.Fatal("platform host startup timed out")
	}
	return ready.URL + ready.RoutePrefix
}

const testToken = "glsa_chat_adapter_test"

func sdkParams(t *testing.T, extra string) openai.ChatCompletionNewParams {
	t.Helper()
	var params openai.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"chat","messages":[{"role":"user","content":"hello"}]`+extra+`}`), &params))
	return params
}

func TestOfficialSDKRealCommand(t *testing.T) {
	token := testToken
	fake := &upstream{cancelled: make(chan struct{}, 3)}
	up := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(up.Close)
	url := startCommand(t, up.URL, "self-hosted")
	client := openai.NewClient(option.WithBaseURL(url+"/v1/"), option.WithAPIKey(token), option.WithMaxRetries(0))
	t.Run("unary and native identity", func(t *testing.T) {
		result, err := client.Chat.Completions.New(context.Background(), sdkParams(t, ""))
		require.NoError(t, err)
		assert.Equal(t, "gpt-4.1", result.Model)
		assert.Equal(t, "resp_private", result.ID)
		assert.Equal(t, int64(1), result.Created)
		assert.Equal(t, "hello", result.Choices[0].Message.Content)
		assert.Equal(t, int64(5), result.Usage.TotalTokens)
		assert.Equal(t, false, fake.last()["store"])
		assert.Equal(t, "gpt-4.1", fake.last()["model"])
	})
	t.Run("warning extension through official SDK", func(t *testing.T) {
		params := sdkParams(t, `,"seed":7`)
		response, err := client.Chat.Completions.New(context.Background(), params)
		require.NoError(t, err)
		field, ok := response.JSON.ExtraFields["grafana"]
		require.True(t, ok)
		var extension struct {
			Warnings []struct{ Type, Feature string }
		}
		require.NoError(t, json.Unmarshal([]byte(field.Raw()), &extension))
		require.NotEmpty(t, extension.Warnings)
		assert.Equal(t, "seed", extension.Warnings[0].Feature)
		stream := client.Chat.Completions.NewStreaming(context.Background(), params)
		seen := false
		for stream.Next() {
			chunk := stream.Current()
			if field, ok := chunk.JSON.ExtraFields["grafana"]; ok {
				require.NoError(t, json.Unmarshal([]byte(field.Raw()), &extension))
				seen = true
			}
		}
		require.NoError(t, stream.Err())
		require.NoError(t, stream.Close())
		assert.True(t, seen)
	})
	tools := `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]`
	t.Run("raw semantic defaults across modes", func(t *testing.T) {
		for _, stream := range []bool{false, true} {
			for _, strict := range []string{"null", "false", "true"} {
				for _, store := range []string{"null", "false"} {
					body := fmt.Sprintf(`{"model":"chat","messages":[{"role":"user","content":"hello"}],"stream":%t,"store":%s,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}},"strict":%s}}]}`, stream, store, strict)
					req, err := http.NewRequest("POST", url+"/v1/chat/completions", strings.NewReader(body))
					require.NoError(t, err)
					req.Header.Set("Authorization", "Bearer "+token)
					req.Header.Set("Content-Type", "application/json")
					res, err := http.DefaultClient.Do(req)
					require.NoError(t, err)
					data, err := io.ReadAll(res.Body)
					_ = res.Body.Close()
					require.NoError(t, err)
					require.Equal(t, 200, res.StatusCode, string(data))
					if stream {
						assert.Contains(t, string(data), "[DONE]")
					}
					captured := fake.last()
					assert.Equal(t, false, captured["store"])
					assert.Equal(t, strict == "true", captured["tools"].([]any)[0].(map[string]any)["strict"])
				}
			}
		}
		before := fake.count()
		_, err := client.Chat.Completions.New(context.Background(), sdkParams(t, `,"store":true`))
		var api *openai.Error
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 400, api.StatusCode)
		assert.Equal(t, before, fake.count())
	})
	t.Run("tools strict default and continuation", func(t *testing.T) {
		result, err := client.Chat.Completions.New(context.Background(), sdkParams(t, tools))
		require.NoError(t, err)
		require.Len(t, result.Choices[0].Message.ToolCalls, 1)
		assert.Equal(t, "tool_calls", result.Choices[0].FinishReason)
		tool := fake.last()["tools"].([]any)[0].(map[string]any)
		assert.Equal(t, false, tool["strict"])
		params := sdkParams(t, tools)
		params.Messages = append(params.Messages, result.Choices[0].Message.ToParam(), openai.ToolMessage("sunny", "call_weather"))
		next, err := client.Chat.Completions.New(context.Background(), params)
		require.NoError(t, err)
		assert.Equal(t, "hello", next.Choices[0].Message.Content)
	})
	for _, extra := range []string{"", tools} {
		t.Run("stream"+extra, func(t *testing.T) {
			params := sdkParams(t, extra)
			params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
			stream := client.Chat.Completions.NewStreaming(context.Background(), params)
			defer func() { _ = stream.Close() }()
			var acc openai.ChatCompletionAccumulator
			usage := int64(0)
			for stream.Next() {
				c := stream.Current()
				acc.AddChunk(c)
				if len(c.Choices) == 0 {
					usage = c.Usage.TotalTokens
				}
			}
			require.NoError(t, stream.Err())
			require.Len(t, acc.Choices, 1)
			assert.Equal(t, int64(5), usage)
			if extra == "" {
				assert.Equal(t, "hello", acc.Choices[0].Message.Content)
			} else {
				require.Len(t, acc.Choices[0].Message.ToolCalls, 1)
				assert.JSONEq(t, `{"city":"Rio"}`, acc.Choices[0].Message.ToolCalls[0].Function.Arguments)
			}
		})
	}
	t.Run("structured and reasoning", func(t *testing.T) {
		params := sdkParams(t, `,"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`)
		result, err := client.Chat.Completions.New(context.Background(), params)
		require.NoError(t, err)
		assert.JSONEq(t, `{"answer":"yes"}`, result.Choices[0].Message.Content)
		format := fake.last()["text"].(map[string]any)["format"].(map[string]any)
		assert.Equal(t, true, format["strict"])
		params = sdkParams(t, `,"reasoning_effort":"high"`)
		params.Model = "public/reasoning"
		result, err = client.Chat.Completions.New(context.Background(), params)
		require.NoError(t, err)
		assert.Equal(t, "hello", result.Choices[0].Message.Content)
		assert.Equal(t, "high", fake.last()["reasoning"].(map[string]any)["effort"])
	})
	t.Run("structured strictness and invalid output", func(t *testing.T) {
		for _, strict := range []string{"", `,"strict":false`, `,"strict":true`} {
			params := sdkParams(t, `,"response_format":{"type":"json_schema","json_schema":{"name":"answer"`+strict+`,"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`)
			result, err := client.Chat.Completions.New(context.Background(), params)
			require.NoError(t, err)
			assert.JSONEq(t, `{"answer":"yes"}`, result.Choices[0].Message.Content)
			format := fake.last()["text"].(map[string]any)["format"].(map[string]any)
			assert.Equal(t, "answer", format["name"])
			assert.Equal(t, strict == `,"strict":true`, format["strict"])
		}
		fake.mu.Lock()
		fake.invalidJSON = true
		fake.mu.Unlock()
		defer func() { fake.mu.Lock(); fake.invalidJSON = false; fake.mu.Unlock() }()
		params := sdkParams(t, `,"response_format":{"type":"json_schema","json_schema":{"name":"answer","strict":true,"schema":{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"],"additionalProperties":false}}}`)
		_, err := client.Chat.Completions.New(context.Background(), params)
		var api *openai.Error
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 502, api.StatusCode)
		stream := client.Chat.Completions.NewStreaming(context.Background(), params)
		defer func() { _ = stream.Close() }()
		for stream.Next() {
		}
		require.Error(t, stream.Err())
	})
	t.Run("stream remains incremental and cancellation reaches backend", func(t *testing.T) {
		fake.mu.Lock()
		fake.hold = true
		fake.mu.Unlock()
		defer func() { fake.mu.Lock(); fake.hold = false; fake.mu.Unlock() }()
		before := fake.count()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		stream := client.Chat.Completions.NewStreaming(ctx, sdkParams(t, ""))
		defer func() { _ = stream.Close() }()
		text := ""
		for stream.Next() {
			for _, choice := range stream.Current().Choices {
				text += choice.Delta.Content
			}
			if text != "" {
				break
			}
		}
		require.Equal(t, "hello", text, "first text must arrive while upstream is still open")
		cancel()
		_ = stream.Close()
		select {
		case <-fake.cancelled:
		case <-time.After(time.Second):
			t.Fatal("backend did not observe cancellation")
		}
		assert.Equal(t, before+1, fake.count())
	})
	t.Run("auth resolution route and safe failure", func(t *testing.T) {
		before := fake.count()
		bad := openai.NewClient(option.WithBaseURL(url+"/v1/"), option.WithAPIKey("invalid"), option.WithMaxRetries(0))
		_, err := bad.Chat.Completions.New(context.Background(), sdkParams(t, ""))
		var api *openai.Error
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 401, api.StatusCode)
		assert.Equal(t, before, fake.count())
		params := sdkParams(t, "")
		params.Model = "missing"
		_, err = client.Chat.Completions.New(context.Background(), params)
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 404, api.StatusCode)
		for _, path := range []string{"/v1/models", "/v1/chat/completions"} {
			res, err := http.Get(url + path)
			require.NoError(t, err)
			data, _ := io.ReadAll(res.Body)
			_ = res.Body.Close()
			assert.GreaterOrEqual(t, res.StatusCode, 400)
			assert.NotContains(t, string(data), "private")
		}
		fake.mu.Lock()
		fake.fail = true
		fake.mu.Unlock()
		_, err = client.Chat.Completions.New(context.Background(), sdkParams(t, ""))
		require.ErrorAs(t, err, &api)
		assert.Equal(t, 503, api.StatusCode)
		assert.NotContains(t, err.Error(), "private-upstream-secret")
		fake.mu.Lock()
		fake.fail = false
		fake.mu.Unlock()
	})
}

func TestOfficialSDK_HeterogeneousFallback(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	var paths []string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		requests = append(requests, body)
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		if r.URL.Path == "/v1/responses" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"error":{"message":"retry","type":"server_error"}}`)
			return
		}
		call := map[string]any{"id": "call_weather", "type": "function", "function": map[string]any{"name": "weather", "arguments": `{"city":"Rio"}`}}
		base := map[string]any{"id": "compatible-native", "model": "new-custom-model", "created": 123}
		w.Header().Set("Content-Type", "application/json")
		if body["stream"] != true {
			base["object"] = "chat.completion"
			base["choices"] = []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "tool_calls": []any{call}}, "finish_reason": "tool_calls"}}
			_ = json.NewEncoder(w).Encode(base)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		call["index"] = 0
		for _, choice := range []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{call}}}, map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}} {
			base["object"] = "chat.completion.chunk"
			base["choices"] = []any{choice}
			b, _ := json.Marshal(base)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer backend.Close()
	token := testToken
	config := fmt.Sprintf("providers:\n  first:\n    type: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s/v1\n  second:\n    type: openai-compatible\n    providerName: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s/v1\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: first\n      model: gpt-4.1\n    fallback:\n      - provider: second\n        model: new-custom-model\n    aliases: [chat]\n", backend.URL, backend.URL)
	address := startCommand(t, backend.URL, "self-hosted", config)
	client := openai.NewClient(option.WithBaseURL(address+"/v1/"), option.WithAPIKey(token), option.WithMaxRetries(0))
	for _, streaming := range []bool{false, true} {
		params := sdkParams(t, `,"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]`)
		if !streaming {
			result, err := client.Chat.Completions.New(context.Background(), params)
			require.NoError(t, err)
			assert.Equal(t, "compatible-native", result.ID)
			assert.Equal(t, "new-custom-model", result.Model)
			require.Len(t, result.Choices[0].Message.ToolCalls, 1)
		} else {
			stream := client.Chat.Completions.NewStreaming(context.Background(), params)
			var acc openai.ChatCompletionAccumulator
			for stream.Next() {
				chunk := stream.Current()
				assert.Equal(t, "compatible-native", chunk.ID)
				assert.Equal(t, int64(123), chunk.Created)
				acc.AddChunk(chunk)
			}
			require.NoError(t, stream.Err())
			require.NoError(t, stream.Close())
			require.Len(t, acc.Choices[0].Message.ToolCalls, 1)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 4)
	assert.Equal(t, []string{"/v1/responses", "/v1/chat/completions", "/v1/responses", "/v1/chat/completions"}, paths)
	for _, body := range requests {
		assert.Equal(t, false, body["store"])
		assert.Equal(t, false, body["parallel_tool_calls"])
		assert.NotContains(t, body, "parallelToolCalls")
	}
}

func TestOfficialSDK_CloudAccess(t *testing.T) {
	fake := &upstream{}
	up := httptest.NewServer(http.HandlerFunc(fake.handle))
	defer up.Close()
	base := startCommand(t, up.URL, "cloud")
	client := openai.NewClient(option.WithBaseURL(base+"/v1/"), option.WithAPIKey(testToken), option.WithMaxRetries(0))
	_, err := client.Chat.Completions.New(context.Background(), sdkParams(t, ""))
	var api *openai.Error
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 400, api.StatusCode)
	assert.Zero(t, fake.count())
	hostURL := strings.Split(base, "/apis/")[0]
	res, err := http.Post(hostURL+"/platformhost/credentials", "application/json", strings.NewReader(`{"kind":"service","service":"integration"}`))
	require.NoError(t, err)
	defer func() { assert.NoError(t, res.Body.Close()) }()
	require.Equal(t, 200, res.StatusCode)
	var credential struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&credential))
	internal := openai.NewClient(option.WithBaseURL(base+"/v1/"), option.WithAPIKey("unused"), option.WithHeaderDel("Authorization"), option.WithHeader("X-Access-Token", credential.Token), option.WithMaxRetries(0))
	result, err := internal.Chat.Completions.New(context.Background(), sdkParams(t, ""))
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
	assert.Equal(t, 1, fake.count())
}

func TestOfficialSDK_AnthropicFallback(t *testing.T) {
	var mu sync.Mutex
	var requests []map[string]any
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		requests = append(requests, body)
		mu.Unlock()
		if r.URL.Path == "/v1/responses" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			_, _ = io.WriteString(w, `{"error":{"message":"retry","type":"server_error"}}`)
			return
		}
		if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "fake-backend-key" {
			w.WriteHeader(401)
			return
		}
		messages, _ := json.Marshal(body["messages"])
		tool := body["tools"] != nil && !bytes.Contains(messages, []byte("tool_result"))
		content := map[string]any{"type": "text", "text": "hello"}
		stop := "end_turn"
		if tool {
			content = map[string]any{"type": "tool_use", "id": "call_weather", "name": "weather", "input": map[string]any{"city": "Rio"}}
			stop = "tool_use"
		}
		message := map[string]any{"id": "msg_native", "type": "message", "role": "assistant", "model": body["model"], "content": []any{content}, "stop_reason": stop, "stop_sequence": nil, "usage": map[string]any{"input_tokens": 3, "output_tokens": 2}}
		if body["stream"] != true {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(message)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		start := map[string]any{}
		for key, value := range message {
			start[key] = value
		}
		start["content"] = []any{}
		start["stop_reason"] = nil
		start["usage"] = map[string]any{"input_tokens": 3, "output_tokens": 0}
		block := map[string]any{"type": "text", "text": ""}
		delta := map[string]any{"type": "text_delta", "text": "hello"}
		if tool {
			block = map[string]any{"type": "tool_use", "id": "call_weather", "name": "weather", "input": map[string]any{}}
			delta = map[string]any{"type": "input_json_delta", "partial_json": `{"city":"Rio"}`}
		}
		events := []map[string]any{{"type": "message_start", "message": start}, {"type": "content_block_start", "index": 0, "content_block": block}, {"type": "content_block_delta", "index": 0, "delta": delta}, {"type": "content_block_stop", "index": 0}, {"type": "message_delta", "delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 2}}, {"type": "message_stop"}}
		for _, event := range events {
			data, _ := json.Marshal(event)
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event["type"], data)
			w.(http.Flusher).Flush()
		}
	}))
	defer backend.Close()
	config := fmt.Sprintf("providers:\n  first:\n    type: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s/v1\n  second:\n    type: anthropic\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: first\n      model: gpt-4.1\n    fallback:\n      - provider: second\n        model: claude-sonnet-4-20250514\n    aliases: [chat]\n", backend.URL, backend.URL)
	base := startCommand(t, backend.URL, "self-hosted", config)
	client := openai.NewClient(option.WithBaseURL(base+"/v1/"), option.WithAPIKey(testToken), option.WithMaxRetries(0))
	for _, tools := range []string{"", `,"parallel_tool_calls":false,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}]`} {
		params := sdkParams(t, tools)
		result, err := client.Chat.Completions.New(context.Background(), params)
		require.NoError(t, err)
		assert.Equal(t, "msg_native", result.ID)
		assert.Equal(t, "claude-sonnet-4-20250514", result.Model)
		assert.Equal(t, int64(5), result.Usage.TotalTokens)
		if tools == "" {
			assert.Equal(t, "hello", result.Choices[0].Message.Content)
		} else {
			require.Len(t, result.Choices[0].Message.ToolCalls, 1)
			continuation := params
			continuation.Messages = append(continuation.Messages, result.Choices[0].Message.ToParam(), openai.ToolMessage("sunny", "call_weather"))
			next, err := client.Chat.Completions.New(context.Background(), continuation)
			require.NoError(t, err)
			assert.Equal(t, "hello", next.Choices[0].Message.Content)
		}
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)}
		stream := client.Chat.Completions.NewStreaming(context.Background(), params)
		var acc openai.ChatCompletionAccumulator
		var usage int64
		for stream.Next() {
			chunk := stream.Current()
			assert.Equal(t, "msg_native", chunk.ID)
			acc.AddChunk(chunk)
			if len(chunk.Choices) == 0 {
				usage = chunk.Usage.TotalTokens
			}
		}
		require.NoError(t, stream.Err())
		require.NoError(t, stream.Close())
		require.Len(t, acc.Choices, 1)
		assert.Equal(t, int64(5), usage)
		if tools == "" {
			assert.Equal(t, "hello", acc.Choices[0].Message.Content)
		} else {
			require.Len(t, acc.Choices[0].Message.ToolCalls, 1)
			assert.JSONEq(t, `{"city":"Rio"}`, acc.Choices[0].Message.ToolCalls[0].Function.Arguments)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 10)
	for i, body := range requests {
		if i%2 == 0 {
			assert.Equal(t, false, body["store"])
		} else if body["tools"] != nil {
			assert.Equal(t, true, body["tool_choice"].(map[string]any)["disable_parallel_tool_use"])
		}
	}
}

func TestOfficialSDK_HostDeadline(t *testing.T) {
	fake := &upstream{hold: true, cancelled: make(chan struct{}, 1)}
	backend := httptest.NewServer(http.HandlerFunc(fake.handle))
	defer backend.Close()
	base := startCommand(t, backend.URL, "self-hosted")
	client := openai.NewClient(option.WithBaseURL(base+"/v1/"), option.WithAPIKey(testToken), option.WithMaxRetries(0))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	started := time.Now()
	_, err := client.Chat.Completions.New(ctx, sdkParams(t, ""))
	require.Error(t, err)
	assert.NoError(t, ctx.Err(), "the host deadline must expire before the client deadline")
	assert.Less(t, time.Since(started), 12*time.Second)
	select {
	case <-fake.cancelled:
	case <-time.After(time.Second):
		t.Fatal("native request survived host deadline")
	}
	assert.Equal(t, 1, fake.count())
}
