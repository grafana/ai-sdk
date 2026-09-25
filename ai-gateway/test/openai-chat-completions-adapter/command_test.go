// Package chatcompletionsadapter_test exercises the real command with the official Go SDK.
// The upstream below is a deterministic synthetic test server, not a recording.
package chatcompletionsadapter_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
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
	mu        sync.Mutex
	requests  []map[string]any
	fail      bool
	hold      bool
	cancelled chan struct{}
}

func (u *upstream) handle(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		w.WriteHeader(400)
		return
	}
	u.mu.Lock()
	u.requests = append(u.requests, body)
	fail, hold := u.fail, u.hold
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

func startCommand(t *testing.T, backend, jwks string) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "gateway")
	build := exec.Command("go", "build", "-race", "-mod=readonly", "-o", binary, "../../cmd/grafana-ai-gateway")
	build.Env = append(os.Environ(), "GOWORK=off")
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	config := fmt.Sprintf("providers:\n  test:\n    type: openai\n    apiKeyEnv: CHAT_COMPLETIONS_ADAPTER_TEST_KEY\n    baseURL: %s/v1\nmodels:\n  public/chat:\n    name: Chat\n    primary:\n      provider: test\n      model: gpt-4.1\n    aliases: [chat]\n  public/reasoning:\n    name: Reasoning\n    primary:\n      provider: test\n      model: o3-mini\n", backend)
	configPath := filepath.Join(dir, "models.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0600))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	operationalAddress := address
	args := []string{"--config.file=" + configPath, "--deployment.mode=development", "--server.listen-address=" + address, "--server.shutdown-timeout=2s"}
	if jwks != "" {
		args = append(args, "--auth.jwks-url="+jwks)
	} else {
		operational, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		operationalAddress = operational.Addr().String()
		require.NoError(t, operational.Close())
		args = append(args, "--auth.mode=cloud-gateway", "--server.operational-listen-address="+operationalAddress)
	}
	cmd := exec.Command(binary, args...)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(name, "GRAFANA_AI_GATEWAY_") && !strings.HasPrefix(name, "SIGIL_") && !strings.HasPrefix(name, "AGENTO11Y_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "CHAT_COMPLETIONS_ADAPTER_TEST_KEY=fake-backend-key")
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
				<-done
				t.Error("shutdown exceeded bound")
			}
			assert.NotContains(t, stderr.String(), "fake-backend-key")
		})
	}
	t.Cleanup(stop)
	url := "http://" + address
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		res, err := http.Get("http://" + operationalAddress + "/ready")
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == 200 {
				return url, stop
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("command did not become ready")
	return "", stop
}
func verifiedToken(t *testing.T) (string, string, *httptest.Server) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	enc := base64.RawURLEncoding.EncodeToString
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "EC", "crv": "P-256", "kid": "adapter", "alg": "ES256", "use": "sig", "x": enc(key.X.FillBytes(make([]byte, 32))), "y": enc(key.Y.FillBytes(make([]byte, 32)))}}})
	}))
	t.Cleanup(jwks.Close)
	header := enc([]byte(`{"alg":"ES256","typ":"at+jwt","kid":"adapter"}`))
	signToken := func(expiry time.Time) string {
		payload, _ := json.Marshal(map[string]any{"sub": "access-policy:adapter", "aud": []string{"ai-sdk"}, "exp": expiry.Unix(), "namespace": "stack-adapter", "serviceIdentity": "adapter-test"})
		unsigned := header + "." + enc(payload)
		digest := sha256.Sum256([]byte(unsigned))
		r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
		require.NoError(t, err)
		sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
		return unsigned + "." + enc(sig)
	}
	return signToken(time.Now().Add(time.Hour)), signToken(time.Now().Add(-time.Hour)), jwks
}
func sdkParams(t *testing.T, extra string) openai.ChatCompletionNewParams {
	t.Helper()
	var params openai.ChatCompletionNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"chat","messages":[{"role":"user","content":"hello"}]`+extra+`}`), &params))
	return params
}

func TestOfficialSDKRealCommand(t *testing.T) {
	token, expired, jwks := verifiedToken(t)
	fake := &upstream{cancelled: make(chan struct{}, 3)}
	up := httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(up.Close)
	url, stop := startCommand(t, up.URL, jwks.URL)
	client := openai.NewClient(option.WithBaseURL(url+"/v1"), option.WithAPIKey(token), option.WithMaxRetries(0))
	t.Run("adapter malformed and expired credentials", func(t *testing.T) {
		before := fake.count()
		for _, headers := range []http.Header{{}, {"Authorization": {"Bearer " + expired}}, {"Authorization": {"Bearer " + token, "Bearer " + token}}, {"Authorization": {"Bearer " + token + ", other"}}, {"Authorization": {"Bearer " + token}, "X-Access-Token": {"other"}}, {"Authorization": {"Bearer " + token}, "X-Grafana-Id": {"other"}}, {"Authorization": {"Bearer " + token}, "X-Scope-OrgID": {"42"}}} {
			req, err := http.NewRequest("POST", url+"/v1/chat/completions", strings.NewReader(`not even JSON`))
			require.NoError(t, err)
			req.Header = headers
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(res.Body)
			_ = res.Body.Close()
			require.NoError(t, err)
			assert.Equal(t, 401, res.StatusCode)
			assert.Equal(t, `{"error":{"message":"authentication failed","type":"authentication_error","param":null,"code":"invalid_api_key"}}`, string(body))
		}
		assert.Equal(t, before, fake.count())
	})
	t.Run("unary and canonical identity", func(t *testing.T) {
		result, err := client.Chat.Completions.New(context.Background(), sdkParams(t, ""))
		require.NoError(t, err)
		assert.Equal(t, "public/chat", result.Model)
		assert.Equal(t, "hello", result.Choices[0].Message.Content)
		assert.Equal(t, int64(5), result.Usage.TotalTokens)
		assert.Equal(t, false, fake.last()["store"])
		assert.Equal(t, "gpt-4.1", fake.last()["model"])
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
	t.Run("auth resolution route and safe failure", func(t *testing.T) {
		before := fake.count()
		bad := openai.NewClient(option.WithBaseURL(url+"/v1"), option.WithAPIKey("invalid"), option.WithMaxRetries(0))
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
			assert.Contains(t, string(data), `"error"`)
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
	t.Run("bounded concurrent load", func(t *testing.T) {
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				result, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{Model: "chat", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}})
				if assert.NoError(t, err) {
					assert.Equal(t, "hello", result.Choices[0].Message.Content)
				}
			})
		}
		for range 8 {
			wg.Go(func() {
				req, err := http.NewRequest("POST", url+"/api/v1/aisdk/language-model", strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
				if !assert.NoError(t, err) {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Access-Token", token)
				req.Header.Set("AI-Language-Model-Id", "chat")
				req.Header.Set("AI-Language-Model-Specification-Version", "4")
				req.Header.Set("AI-Language-Model-Streaming", "false")
				res, err := http.DefaultClient.Do(req)
				if !assert.NoError(t, err) {
					return
				}
				body, _ := io.ReadAll(res.Body)
				_ = res.Body.Close()
				assert.Equal(t, 200, res.StatusCode, string(body))
			})
		}
		for _, path := range []string{"/live", "/ready", "/metrics"} {
			res, err := http.Get(url + path)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			assert.Equal(t, 200, res.StatusCode)
		}
		wg.Wait()
	})
	t.Run("shutdown cancels active upstream", func(t *testing.T) {
		fake.mu.Lock()
		fake.hold = true
		fake.mu.Unlock()
		done := make(chan error, 3)
		before := fake.count()
		streamReady := make(chan struct{})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{Model: "chat", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}})
			defer func() { _ = stream.Close() }()
			if stream.Next() {
				close(streamReady)
			}
			for stream.Next() {
			}
			done <- stream.Err()
		}()
		go func() {
			_, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{Model: "chat", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hello")}})
			done <- err
		}()
		go func() {
			req, err := http.NewRequest("POST", url+"/api/v1/aisdk/language-model", strings.NewReader(`{"prompt":[{"role":"user","content":[{"type":"text","text":"hello"}]}]}`))
			if err != nil {
				done <- err
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Access-Token", token)
			req.Header.Set("AI-Language-Model-Id", "chat")
			req.Header.Set("AI-Language-Model-Specification-Version", "4")
			req.Header.Set("AI-Language-Model-Streaming", "false")
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				_, err = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
				if res.StatusCode >= 400 {
					err = fmt.Errorf("status %d", res.StatusCode)
				}
			}
			done <- err
		}()
		require.Eventually(t, func() bool { return fake.count() >= before+3 }, time.Second, 10*time.Millisecond)
		select {
		case <-streamReady:
		case <-time.After(time.Second):
			t.Fatal("adapter SSE did not commit before shutdown")
		}
		stop()
		for range 3 {
			select {
			case <-fake.cancelled:
			case <-time.After(time.Second):
				t.Fatal("upstream was not cancelled")
			}
			select {
			case err := <-done:
				assert.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("client did not terminate")
			}
		}
	})
}

func TestOfficialSDKCloudCommand(t *testing.T) {
	fake := &upstream{}
	up := httptest.NewServer(http.HandlerFunc(fake.handle))
	defer up.Close()
	base, stop := startCommand(t, up.URL, "")
	defer stop()
	direct := openai.NewClient(option.WithBaseURL(base+"/v1"), option.WithAPIKey("edge-must-strip"), option.WithHeader("X-Scope-OrgID", "42"), option.WithMaxRetries(0))
	_, err := direct.Chat.Completions.New(context.Background(), sdkParams(t, ""))
	var api *openai.Error
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 401, api.StatusCode)
	assert.Zero(t, fake.count())
	target, err := url.Parse(base)
	require.NoError(t, err)
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		r.Out.Header.Del("Authorization")
		r.Out.Header.Del("X-Access-Token")
		r.Out.Header.Del("X-Grafana-Id")
		r.Out.Header.Set("X-Scope-OrgID", "42")
	}}
	// This dummy edge proves stripping/composition only, not production CAP policy.
	edge := httptest.NewServer(proxy)
	defer edge.Close()
	client := openai.NewClient(option.WithBaseURL(edge.URL+"/v1"), option.WithAPIKey("dummy-edge-key"), option.WithMaxRetries(0))
	result, err := client.Chat.Completions.New(context.Background(), sdkParams(t, ""))
	require.NoError(t, err)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
	stream := client.Chat.Completions.NewStreaming(context.Background(), sdkParams(t, ""))
	defer func() { _ = stream.Close() }()
	var text string
	for stream.Next() {
		for _, choice := range stream.Current().Choices {
			text += choice.Delta.Content
		}
	}
	require.NoError(t, stream.Err())
	assert.Equal(t, "hello", text)
}
