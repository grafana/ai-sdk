package chatcompletions

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func partsStream(parts ...provider.StreamPart) *provider.StreamResult {
	ch := make(chan provider.StreamPart, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	return &provider.StreamResult{Stream: ch}
}
func textParts() []provider.StreamPart {
	return []provider.StreamPart{{Type: provider.PartStreamStart}, {Type: provider.PartTextStart, ID: "text"}, {Type: provider.PartTextDelta, ID: "text", Delta: "hello"}, {Type: provider.PartTextEnd, ID: "text"}, {Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}, Usage: &textResult().Usage}}
}

func TestOfficialSDKStreaming(t *testing.T) {
	f := &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
		return textResult(), nil
	}, stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return partsStream(textParts()...), nil
	}}
	server := httptest.NewServer(testHandler(t, f))
	defer server.Close()
	client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("test"), option.WithMaxRetries(0))
	params := openaisdk.ChatCompletionNewParams{Model: "alias", Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hello")}}
	result, err := client.Chat.Completions.New(context.Background(), params)
	require.NoError(t, err)
	assert.Equal(t, "canonical", result.Model)
	assert.Equal(t, "hello", result.Choices[0].Message.Content)
	assert.Equal(t, int64(5), result.Usage.TotalTokens)
	assert.True(t, strings.HasPrefix(result.ID, "chatcmpl-"))
	params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{IncludeUsage: openaisdk.Bool(true)}
	stream := client.Chat.Completions.NewStreaming(context.Background(), params)
	defer func() { _ = stream.Close() }()
	text := ""
	finishes := 0
	usages := 0
	for stream.Next() {
		chunk := stream.Current()
		for _, choice := range chunk.Choices {
			text += choice.Delta.Content
			if choice.FinishReason != "" {
				finishes++
			}
		}
		if len(chunk.Choices) == 0 {
			usages++
			assert.Equal(t, int64(5), chunk.Usage.TotalTokens)
		}
	}
	require.NoError(t, stream.Err())
	assert.Equal(t, "hello", text)
	assert.Equal(t, 1, finishes)
	assert.Equal(t, 1, usages)
	params.StreamOptions = openaisdk.ChatCompletionStreamOptionsParam{}
	params.Model = "unknown"
	_, err = client.Chat.Completions.New(context.Background(), params)
	var api *openaisdk.Error
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 404, api.StatusCode)
	assert.NotContains(t, err.Error(), "private-provider")
}

func TestStreamFunctionState(t *testing.T) {
	r := requestWith(t, `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]`)
	s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	parts := []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "call", ToolName: "weather"}, {Type: provider.PartToolInputDelta, ID: "call", Delta: `{"x":`}, {Type: provider.PartToolInputDelta, ID: "call", Delta: `1}`}, {Type: provider.PartToolInputEnd, ID: "call"}, {Type: provider.PartToolCall, ToolCallID: "call", ToolName: "weather", Input: `{"x":1}`}}
	var deltas []*delta
	for _, p := range parts {
		d, err := s.consume(p, r)
		require.NoError(t, err)
		if d != nil {
			deltas = append(deltas, d)
		}
	}
	require.Len(t, deltas, 3)
	assert.Equal(t, "call", deltas[0].ToolCalls[0].ID)
	assert.Empty(t, deltas[1].ToolCalls[0].ID)
	_, _, err := s.finish(provider.StreamPart{FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, r)
	require.NoError(t, err)
	_, err = s.consume(parts[4], r)
	require.Error(t, err)
	bad := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	for _, p := range parts[:4] {
		_, err := bad.consume(p, r)
		require.NoError(t, err)
	}
	mismatch := parts[4]
	mismatch.Input = `{"x":2}`
	_, err = bad.consume(mismatch, r)
	require.Error(t, err)

	empty := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	for _, p := range []provider.StreamPart{{Type: provider.PartToolInputStart, ID: "empty", ToolName: "weather"}, {Type: provider.PartToolInputEnd, ID: "empty"}} {
		_, err = empty.consume(p, r)
		require.NoError(t, err)
	}
	d, err := empty.consume(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "empty", ToolName: "weather", Input: `{}`}, r)
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, "{}", *d.ToolCalls[0].Function.Arguments)
	_, _, err = empty.finish(provider.StreamPart{FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, r)
	require.NoError(t, err)
}

func TestRuntimeFailureBoundsAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []provider.StreamPart
	}{
		{name: "premature", parts: textParts()[:3]},
		{name: "unrepresented stop", parts: []provider.StreamPart{{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}}},
		{name: "duplicate finish", parts: append(textParts(), textParts()[4])},
		{name: "late nonterminal", parts: append(textParts(), provider.StreamPart{Type: provider.PartTextStart, ID: "late"})},
		{name: "second native segment", parts: append(textParts(), textParts()...)},
		{name: "invalid UTF8", parts: []provider.StreamPart{{Type: provider.PartTextStart, ID: "text"}, {Type: provider.PartTextDelta, ID: "text", Delta: string([]byte{0xff})}}},
		{name: "contradiction", parts: []provider.StreamPart{{Type: provider.PartTextDelta, ID: "missing", Delta: "secret"}}},
		{name: "unsupported content", parts: []provider.StreamPart{{Type: provider.PartSource}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(tc.parts...), nil
			}}
			server := httptest.NewServer(testHandler(t, f))
			defer server.Close()
			res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
			require.NoError(t, err)
			defer func() { _ = res.Body.Close() }()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), `"error"`)
			assert.NotContains(t, string(body), "[DONE]")
			assert.NotContains(t, string(body), "secret")
		})
	}
	t.Run("runtime idle timeout cancels provider", func(t *testing.T) {
		stopped := make(chan struct{})
		f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart)
			go func() {
				defer close(stopped)
				defer close(ch)
				<-ctx.Done()
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}
		h := testHandler(t, f)
		runtime, err := execution.New(execution.Config{Selector: func(context.Context, string, provider.CallOptions, json.RawMessage) (execution.Selection, error) {
			return execution.Selection{ID: "canonical", Model: f}, nil
		}, Limits: execution.Limits{StreamIdleDuration: 20 * time.Millisecond, StreamDrainDuration: time.Second}})
		require.NoError(t, err)
		h.runtime = runtime
		server := httptest.NewServer(h)
		defer server.Close()
		res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
		require.NoError(t, err)
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		require.NoError(t, err)
		assert.Contains(t, string(body), `"timeout"`)
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("producer not cancelled")
		}
	})
}

func TestLateSetupAndSlowClientAreBounded(t *testing.T) {
	t.Run("late setup ownership", func(t *testing.T) {
		started := make(chan struct{})
		drained := make(chan struct{})
		f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			close(started)
			<-ctx.Done()
			ch := make(chan provider.StreamPart)
			go func() {
				defer close(ch)
				ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "late"}
				close(drained)
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}
		h := testHandler(t, f)
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("POST", Path, strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		done := make(chan struct{})
		go func() { defer close(done); h.ServeHTTP(httptest.NewRecorder(), req) }()
		<-started
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cooperative setup did not return after cancellation")
		}
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("late stream was not drained")
		}
	})
	t.Run("slow socket client", func(t *testing.T) {
		stopped := make(chan struct{})
		f := &fakeModel{stream: func(ctx context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
			ch := make(chan provider.StreamPart)
			go func() {
				defer close(stopped)
				defer close(ch)
				select {
				case ch <- provider.StreamPart{Type: provider.PartTextStart, ID: "text"}:
				case <-ctx.Done():
					return
				}
				for {
					select {
					case <-ctx.Done():
						return
					case ch <- provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: strings.Repeat("x", 32768)}:
					}
				}
			}()
			return &provider.StreamResult{Stream: ch}, nil
		}}
		h := testHandler(t, f)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 150*time.Millisecond)
			defer cancel()
			h.ServeHTTP(w, r.WithContext(ctx))
		}))
		defer server.Close()
		conn, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
		require.NoError(t, err)
		defer func() { _ = conn.Close() }()
		body := strings.TrimSuffix(basic, "}") + `,"stream":true}`
		_, err = io.WriteString(conn, "POST "+Path+" HTTP/1.1\r\nHost: local\r\nContent-Type: application/json\r\nContent-Length: "+strconv.Itoa(len(body))+"\r\n\r\n"+body)
		require.NoError(t, err)
		select {
		case <-stopped:
		case <-time.After(time.Second):
			t.Fatal("slow client retained provider")
		}
	})
}

func TestStreamingPlatformOwnsOutputBounds(t *testing.T) {
	t.Run("large frame and many parts", func(t *testing.T) {
		large := strings.Repeat("x", 2<<20)
		parts := []provider.StreamPart{{Type: provider.PartTextStart, ID: "text"}, {Type: provider.PartTextDelta, ID: "text", Delta: large}}
		for range 1100 {
			parts = append(parts, provider.StreamPart{Type: provider.PartTextDelta, ID: "text", Delta: "x"})
		}
		parts = append(parts, provider.StreamPart{Type: provider.PartTextEnd, ID: "text"}, textParts()[4])
		f := &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return partsStream(parts...), nil
		}}
		server := httptest.NewServer(testHandler(t, f))
		defer server.Close()
		res, err := http.Post(server.URL+Path, "application/json", strings.NewReader(strings.TrimSuffix(basic, "}")+`,"stream":true}`))
		require.NoError(t, err)
		defer func() { _ = res.Body.Close() }()
		body, err := io.ReadAll(res.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), large)
		assert.Contains(t, string(body), "[DONE]")
		assert.NotContains(t, string(body), `"error"`)
	})
	t.Run("text and tool cardinality", func(t *testing.T) {
		r := requestWith(t, `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]`)
		s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
		for i := 0; i < 1100; i++ {
			id := strconv.Itoa(i)
			for _, p := range []provider.StreamPart{{Type: provider.PartTextStart, ID: id}, {Type: provider.PartTextDelta, ID: id, Delta: "x"}, {Type: provider.PartTextEnd, ID: id}} {
				_, err := s.consume(p, r)
				require.NoError(t, err)
			}
		}
		for i := 0; i < 130; i++ {
			_, err := s.consume(provider.StreamPart{Type: provider.PartToolCall, ToolCallID: strconv.Itoa(i), ToolName: "weather", Input: `{}`}, r)
			require.NoError(t, err)
		}
		_, _, err := s.finish(provider.StreamPart{FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls}}, r)
		require.NoError(t, err)
	})
}

func TestStreamingRejectsTextEncodedFunctionInput(t *testing.T) {
	s := streamState{textIDs: map[string]bool{}, tools: map[string]*toolState{}}
	r := requestWith(t, `,"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}]`)
	_, err := s.consume(provider.StreamPart{Type: provider.PartToolInputStart, ID: "call", ToolName: "weather", InputEncoding: provider.ToolInputEncodingText}, r)
	require.Error(t, err)
}

func TestNativeContract_StreamingSDK(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint(late), func(t *testing.T) {
			parts := textParts()
			parts[0].Warnings = []provider.Warning{{Type: provider.WarnUnsupported, Feature: "seed", Details: "ignored <seed>"}}
			meta := provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "native-id", ModelID: "native-model", Timestamp: time.Unix(123, 0)}
			index := 1
			if late {
				index = 3
			}
			parts = append(parts[:index], append([]provider.StreamPart{meta}, parts[index:]...)...)
			server := httptest.NewServer(testHandler(t, &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return partsStream(parts...), nil
			}}))
			defer server.Close()
			client := openaisdk.NewClient(option.WithBaseURL(server.URL+"/v1"), option.WithAPIKey("dummy"), option.WithMaxRetries(0))
			stream := client.Chat.Completions.NewStreaming(context.Background(), openaisdk.ChatCompletionNewParams{Model: "alias", Messages: []openaisdk.ChatCompletionMessageParamUnion{openaisdk.UserMessage("hello")}})
			defer func() { require.NoError(t, stream.Close()) }()
			id, model := "", ""
			var created int64
			warningsSeen, nativeSeen := false, false
			for stream.Next() {
				c := stream.Current()
				if id == "" {
					id, model, created = c.ID, c.Model, c.Created
				}
				assert.Equal(t, id, c.ID)
				assert.Equal(t, model, c.Model)
				assert.Equal(t, created, c.Created)
				if field, ok := c.JSON.ExtraFields["grafana"]; ok {
					var extension diagnostics
					require.NoError(t, json.Unmarshal([]byte(field.Raw()), &extension))
					if len(extension.Warnings) > 0 {
						warningsSeen = true
						assert.Equal(t, "ignored <seed>", extension.Warnings[0].Details)
					}
					if extension.NativeResponse != nil {
						nativeSeen = true
						assert.Equal(t, "native-id", extension.NativeResponse.ID)
					}
				}
			}
			require.NoError(t, stream.Err())
			assert.True(t, warningsSeen)
			if late {
				assert.True(t, nativeSeen)
				assert.NotEqual(t, "native-id", id)
			} else {
				assert.Equal(t, "native-id", id)
				assert.Equal(t, "native-model", model)
				assert.Equal(t, int64(123), created)
			}
		})
	}
}
