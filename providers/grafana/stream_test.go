package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sseFrame(value string) string { return "data: " + value + "\n\n" }
func boolPointer(value bool) *bool { return &value }

func jsonMember(t *testing.T, body []byte, key string) string {
	t.Helper()
	var value map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &value))
	return string(value[key])
}

const finishEvent = `{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`

func collectParts(t *testing.T, result *provider.StreamResult) []provider.StreamPart {
	t.Helper()
	return collectPartsWithin(t, result, 3*time.Second)
}

func collectPartsWithin(t *testing.T, result *provider.StreamResult, timeout time.Duration) []provider.StreamPart {
	t.Helper()
	done := time.NewTimer(timeout)
	defer done.Stop()
	var parts []provider.StreamPart
	for {
		select {
		case part, ok := <-result.Stream:
			if !ok {
				return parts
			}
			parts = append(parts, part)
		case <-done.C:
			t.Fatal("stream did not close")
			return nil
		}
	}
}

func streamFromBody(t *testing.T, body io.ReadCloser) provider.LanguageModel {
	t.Helper()
	p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test/api/prefix", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "/api/prefix/language-model", req.URL.Path)
		assert.Equal(t, "true", req.Header.Get("Ai-Language-Model-Streaming"))
		assert.Equal(t, "text/event-stream", req.Header.Get("Accept"))
		assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}, "X-Server": {"stream"}, "X-Multi": {"one", "two"}}, Body: body, Request: req}, nil
	})}})
	require.NoError(t, err)
	m, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	return m
}

func TestModel_StreamNormalization(t *testing.T) {
	frames := []string{
		`{"type":"stream-start","warnings":[{"type":"other","message":"public warning"}]}`,
		`{"type":"response-metadata","id":"response-1","modelId":"assistant","timestamp":"2026-08-22T00:00:00.123Z","private":"do-not-expose"}`,
		`{"type":"text-start","id":"text-1"}`,
		`{"type":"text-delta","id":"text-1","delta":""}`,
		`{"type":"raw","rawValue":{"public":"raw"}}`,
		`{"type":"error","error":{"message":"upstream failure","type":"internal_server_error","param":null,"code":"upstream_error","statusCode":502,"retryable":true,"private":"do-not-expose"}}`,
		`{"type":"text-delta","id":"text-1","delta":"hello"}`,
		`{"type":"text-end","id":"text-1"}`,
		finishEvent, "[DONE]",
	}
	for _, includeRaw := range []bool{false, true} {
		t.Run(map[bool]string{false: "filter raw", true: "include raw"}[includeRaw], func(t *testing.T) {
			var payload strings.Builder
			payload.WriteString(":comment\n\n")
			for _, frame := range frames {
				payload.WriteString(sseFrame(frame))
			}
			body := &trackedBody{Reader: strings.NewReader(strings.ReplaceAll(payload.String(), "\n", "\r\n"))}
			m := streamFromBody(t, body)
			result, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}, IncludeRawChunks: includeRaw})
			require.NoError(t, err)
			require.NotNil(t, result.Request)
			assert.JSONEq(t, `[]`, jsonMember(t, result.Request.Body, "prompt"))
			assert.Equal(t, "stream", result.Response.Headers["X-Server"])
			assert.Equal(t, "one, two", result.Response.Headers["X-Multi"])
			parts := collectParts(t, result)
			want := 8
			if includeRaw {
				want = 9
			}
			require.Len(t, parts, want)
			assert.Equal(t, provider.PartStreamStart, parts[0].Type)
			assert.Equal(t, "public warning", parts[0].Warnings[0].Message)
			assert.Equal(t, "response-1", parts[1].ResponseID)
			assert.Equal(t, "assistant", parts[1].ModelID)
			assert.Equal(t, time.Date(2026, 8, 22, 0, 0, 0, 123000000, time.UTC), parts[1].Timestamp)
			assert.Nil(t, parts[1].ProviderMetadata)
			assert.Equal(t, provider.PartTextDelta, parts[3].Type)
			assert.Empty(t, parts[3].Delta)
			errIndex := 4
			if includeRaw {
				assert.Equal(t, provider.PartRaw, parts[4].Type)
				assert.JSONEq(t, `{"public":"raw"}`, string(parts[4].RawValue))
				errIndex = 5
			}
			assert.Equal(t, provider.PartError, parts[errIndex].Type)
			require.NotNil(t, parts[errIndex].APICallError)
			assert.True(t, parts[errIndex].APICallError.IsRetryable)
			assert.NotContains(t, parts[errIndex].APICallError.ResponseBody, "do-not-expose")
			assert.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
			assert.True(t, body.closed)
		})
	}
}

func TestModel_StreamRawUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "absent"},
		{name: "empty", raw: `{}`, want: `{}`},
		{name: "native", raw: `{"input_tokens":32,"service_tier":"standard","nested":[1,true]}`, want: `{"input_tokens":32,"service_tier":"standard","nested":[1,true]}`},
		{name: "valid surrogate", raw: `{"nested":{"\ud83d\ude00":1}}`, want: `{"nested":{"😀":1}}`},
		{name: "lone high key", raw: `{"nested":{"\ud800":1}}`, want: `{"nested":{"\ud800":1}}`},
		{name: "lone low nested", raw: `{"nested":["\udc00"]}`, want: `{"nested":["\udc00"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			finish := finishEvent
			if tc.raw != "" {
				finish = strings.Replace(finish, `"outputTokens":{}`, `"outputTokens":{},"raw":`+tc.raw, 1)
			}
			m := streamFromBody(t, io.NopCloser(strings.NewReader(sseFrame(`{"type":"raw","rawValue":{}}`)+sseFrame(finish))))
			result, err := m.DoStream(context.Background(), provider.CallOptions{IncludeRawChunks: false})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			require.Equal(t, provider.PartFinish, parts[0].Type)
			require.NotNil(t, parts[0].Usage)
			if tc.want == "" {
				assert.Nil(t, parts[0].Usage.Raw)
			} else {
				assert.JSONEq(t, tc.want, string(parts[0].Usage.Raw))
				if strings.Contains(tc.name, "lone") {
					assert.Equal(t, tc.raw, string(parts[0].Usage.Raw))
				}
			}
		})
	}
}

func TestModel_StreamValueLessRaw(t *testing.T) {
	for _, include := range []bool{true, false} {
		t.Run(fmt.Sprint(include), func(t *testing.T) {
			m := streamFromBody(t, io.NopCloser(strings.NewReader(sseFrame(`{"type":"raw"}`)+sseFrame(finishEvent))))
			result, err := m.DoStream(context.Background(), provider.CallOptions{IncludeRawChunks: include})
			require.NoError(t, err)
			parts := collectParts(t, result)
			if !include {
				require.Len(t, parts, 1)
				assert.Equal(t, provider.PartFinish, parts[0].Type)
				return
			}
			require.Len(t, parts, 2)
			assert.Equal(t, provider.PartRaw, parts[0].Type)
			assert.Nil(t, parts[0].RawValue)
			assert.Equal(t, provider.PartFinish, parts[1].Type)
		})
	}
}

func TestModel_StreamRawUsageSize(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantSize int
	}{
		{name: "at former cap", raw: `{"large":"` + strings.Repeat("x", formerRawUsageCap-len(`{"large":""}`)) + `"}`, wantSize: formerRawUsageCap},
		{name: "above former cap", raw: `{"large":"` + strings.Repeat("x", formerRawUsageCap+1-len(`{"large":""}`)) + `"}`, wantSize: formerRawUsageCap + 1},
		{name: "whitespace exact", raw: strings.Repeat(" ", formerRawUsageCap-2) + `{}`, wantSize: 2},
		{name: "leading whitespace", raw: strings.Repeat(" ", formerRawUsageCap-1) + `{}`, wantSize: 2},
		{name: "trailing whitespace", raw: `{}` + strings.Repeat(" ", formerRawUsageCap-1), wantSize: 2},
		{name: "interior whitespace", raw: `{"x":` + strings.Repeat(" ", formerRawUsageCap) + `0}`, wantSize: len(`{"x":0}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			finish := strings.Replace(finishEvent, `"outputTokens":{}`, `"outputTokens":{},"raw":`+tc.raw, 1)
			m := streamFromBody(t, io.NopCloser(strings.NewReader(sseFrame(finish))))
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			require.Equal(t, provider.PartFinish, parts[0].Type)
			require.NotNil(t, parts[0].Usage)
			assert.Len(t, parts[0].Usage.Raw, tc.wantSize)
			assert.JSONEq(t, tc.raw, string(parts[0].Usage.Raw))
		})
	}
}

func TestModel_StreamProviderToolParts(t *testing.T) {
	frames := []string{
		`{"type":"tool-input-start","id":"call","toolName":"echo","providerExecuted":true,"dynamic":false}`,
		`{"type":"tool-input-delta","id":"call","delta":""}`,
		`{"type":"tool-input-end","id":"call"}`,
		`{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}","providerExecuted":true,"dynamic":true,"providerMetadata":{"anthropic":{"type":"mcp-tool-use","serverName":"echo","private":"discard"}}}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":{"temperature":0},"isError":true,"preliminary":true,"providerMetadata":{"anthropic":{"type":"mcp-tool-use","serverName":"echo"}}}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":"final"}`,
		finishEvent,
	}
	var payload strings.Builder
	for _, frame := range frames {
		payload.WriteString(sseFrame(frame))
	}
	model := streamFromBody(t, io.NopCloser(strings.NewReader(payload.String())))
	stream, err := model.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	parts := collectParts(t, stream)
	require.Len(t, parts, len(frames))
	assert.True(t, parts[0].ProviderExecuted)
	assert.Equal(t, boolPointer(false), parts[0].Dynamic)
	assert.Equal(t, "", parts[1].Delta)
	assert.True(t, parts[3].ProviderExecuted)
	assert.Equal(t, boolPointer(true), parts[3].Dynamic)
	assert.JSONEq(t, `{"type":"mcp-tool-use","serverName":"echo","private":"discard"}`, string(parts[3].ProviderMetadata["anthropic"]))
	assert.Equal(t, boolPointer(true), parts[4].Preliminary)
	assert.True(t, parts[4].IsError)
	assert.JSONEq(t, `{"temperature":0}`, string(parts[4].Result))
	assert.Nil(t, parts[5].Preliminary)
	assert.Nil(t, parts[5].Dynamic)
}

func TestDecodeStreamPart_VariantLocalFields(t *testing.T) {
	for _, tc := range []struct {
		name, event, extra string
	}{
		{"text ignores tool fields", `{"type":"text-delta","id":"text","delta":"hello"}`, `"dynamic":{},"preliminary":true,"providerExecuted":"not-a-boolean","isError":[],"input":{},"result":false`},
		{"start ignores result fields", `{"type":"tool-input-start","id":"call","toolName":"echo"}`, `"preliminary":{},"isError":[],"result":false`},
		{"delta ignores start flags", `{"type":"tool-input-delta","id":"call","delta":""}`, `"providerExecuted":true,"dynamic":"not-a-boolean","preliminary":true,"toolName":[]`},
		{"end ignores start fields", `{"type":"tool-input-end","id":"call"}`, `"providerExecuted":true,"dynamic":true,"toolName":{},"delta":[]`},
		{"call ignores result fields", `{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}"}`, `"preliminary":{},"isError":[],"result":false`},
		{"result ignores call fields", `{"type":"tool-result","toolCallId":"call","toolName":"echo","result":false}`, `"input":{},"providerExecuted":"not-a-boolean","delta":[]`},
		{"null start markers normalize", `{"type":"tool-input-start","id":"call","toolName":"echo"}`, `"dynamic":null,"providerExecuted":null`},
		{"null result markers normalize", `{"type":"tool-result","toolCallId":"call","toolName":"echo","result":0}`, `"dynamic":null,"preliminary":null,"isError":null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := decodeStreamPart([]byte(tc.event))
			require.NoError(t, err)
			got, err := decodeStreamPart([]byte(strings.TrimSuffix(tc.event, "}") + "," + tc.extra + "}"))
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestDecodeStreamPart_ToolMarkerPresence(t *testing.T) {
	for _, event := range []string{
		`{"type":"tool-input-start","id":"call","toolName":"echo"}`,
		`{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}"}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":false}`,
	} {
		for _, tc := range []struct {
			value string
			want  *bool
		}{
			{value: "null"}, {value: "false", want: boolPointer(false)}, {value: "true", want: boolPointer(true)},
		} {
			part, err := decodeStreamPart([]byte(strings.TrimSuffix(event, "}") + `,"dynamic":` + tc.value + "}"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, part.Dynamic)
			if part.Type == provider.PartToolResult {
				part, err = decodeStreamPart([]byte(strings.TrimSuffix(event, "}") + `,"preliminary":` + tc.value + "}"))
				require.NoError(t, err)
				assert.Equal(t, tc.want, part.Preliminary)
			}
		}
	}
	for _, field := range []string{"dynamic", "preliminary", "isError"} {
		_, err := decodeStreamPart([]byte(`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":false,"` + field + `":"not-a-boolean"}`))
		require.Error(t, err)
	}
}

func TestDecodeStreamPart_DeferredAndMarkers(t *testing.T) {
	for _, tc := range []struct {
		name, event string
		dynamic     *bool
	}{
		{name: "absent", event: `{"type":"tool-input-start","id":"call","toolName":"echo"}`},
		{name: "false", event: `{"type":"tool-input-start","id":"call","toolName":"echo","dynamic":false}`, dynamic: boolPointer(false)},
		{name: "true", event: `{"type":"tool-input-start","id":"call","toolName":"echo","dynamic":true}`, dynamic: boolPointer(true)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			part, err := decodeStreamPart([]byte(tc.event))
			require.NoError(t, err)
			assert.Equal(t, tc.dynamic, part.Dynamic)
		})
	}
	result, err := decodeStreamPart([]byte(`{"type":"tool-result","toolCallId":"history-call","toolName":"echo","result":"error","isError":true}`))
	require.NoError(t, err)
	assert.Equal(t, "history-call", result.ToolCallID)
	assert.True(t, result.IsError)
	for _, event := range []string{
		`{"type":"tool-call","toolCallId":"call","toolName":"echo","input":"{}","providerExecuted":"true"}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":null}`,
		`{"type":"tool-result","toolCallId":"call","toolName":"echo","result":{},"providerMetadata":{"anthropic":null}}`,
	} {
		_, err := decodeStreamPart([]byte(event))
		require.Error(t, err, event)
	}
}

func TestModel_StreamEOF(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		count         int
	}{
		{"empty", "", 0}, {"DONE", sseFrame("[DONE]"), 0}, {"finish", sseFrame(finishEvent), 1}, {"without finish", sseFrame(`{"type":"text-start","id":"a"}`), 1}, {"after status-less error", sseFrame(`{"type":"text-start","id":"a"}`) + sseFrame(`{"type":"error","error":`+statusLessStreamError+`}}`), 2}, {"unterminated event", "data: {\"type\":\"text-start\",\"id\":\"a\"}", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := streamFromBody(t, io.NopCloser(strings.NewReader(tc.payload)))
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, tc.count)
			for _, part := range parts {
				if part.Type == provider.PartError {
					require.NotNil(t, part.APICallError)
					assert.Equal(t, "provider stream error", part.APICallError.Message)
				}
			}
		})
	}
}

type fragmentedReader struct{ io.Reader }

func (r fragmentedReader) Read(buffer []byte) (int, error) {
	if len(buffer) > 1 {
		buffer = buffer[:1]
	}
	return r.Reader.Read(buffer)
}

func TestModel_StreamFramingAndWarnings(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		t.Run(strings.ReplaceAll(strings.ReplaceAll(ending, "\r", "CR"), "\n", "LF"), func(t *testing.T) {
			payload := "\ufeff" + strings.Join([]string{":comment", "id: event-1", "event: message", "retry: 1000", `data: {"type":"text-start",`, `data: "id":"a"}`, "", ""}, ending)
			m := streamFromBody(t, io.NopCloser(fragmentedReader{strings.NewReader(payload)}))
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			assert.Equal(t, provider.PartTextStart, parts[0].Type)
			assert.Equal(t, "a", parts[0].ID)
		})
	}
	part, err := decodeStreamPart([]byte(`{"type":"stream-start","warnings":[{"type":"deprecated","setting":"model setting","message":"use another setting"},{"type":"other","message":""},{"type":"unsupported","feature":"","details":""}]}`))
	require.NoError(t, err)
	require.Len(t, part.Warnings, 3)
	assert.Equal(t, "model setting", part.Warnings[0].Setting)
	assert.Equal(t, "use another setting", part.Warnings[0].Message)
	for _, event := range []string{`{"type":"response-metadata","modelId":null}`, `{"type":"response-metadata","modelId":1}`, `{"type":"response-metadata","modelId":"assistant","timestamp":"2026-08-22T00:00:00,123Z"}`} {
		_, err := decodeStreamPart([]byte(event))
		require.Error(t, err)
	}
}

func TestModel_StreamInvalidEvents(t *testing.T) {
	for _, event := range []string{
		strings.Replace(finishEvent, `"outputTokens":{}`, `"outputTokens":{},"raw":null`, 1),
		strings.Replace(finishEvent, `"outputTokens":{}`, `"outputTokens":{},"raw":[]`, 1),
		strings.Replace(finishEvent, `"outputTokens":{}`, `"outputTokens":{},"raw":{"nested":"`+string([]byte{0xff})+`"}`, 1),
		strings.Replace(finishEvent, `"outputTokens":{}`, `"outputTokens":{},"raw":{"private":`, 1),
		`{"type":"stream-start","warnings":[{"Type":"other","message":"safe"}]}`,
		`{"type":"stream-start","warnings":[{"type":"other","Message":"safe"}]}`,
		`{"type":"custom"}`, `{"type":"custom","kind":null}`, `{"type":"custom","kind":7}`, `{"type":"custom","kind":"x.y","providerMetadata":{"x":null}}`,
		`{"type":"reasoning-start"}`, `{"type":"text-start"}`, `{"type":"text-delta","id":"a"}`, `{"type":"text-end","id":null}`, `{"type":"finish"}`, `{"type":"stream-start"}`, `{"type":"stream-start","warnings":[{"type":"unknown"}]}`, `{"type":"response-metadata","timestamp":"bad"}`, `{"type":"error","error":{"message":"bad"}}`, `{"type":"error","error":{"message":"safe","type":"internal_server_error","param":null,"code":"upstream_error","statusCode":502,"retryable":false}}`, `{"type":"text-delta","id":"a","delta":1}`, `{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{"total":-1},"outputTokens":{}}}`, `{"type":`, "[DONE] ", "",
	} {
		name := event
		if len(name) > 120 {
			name = name[:120]
		}
		t.Run(name, func(t *testing.T) {
			payload := sseFrame(event) + sseFrame(finishEvent)
			body := &trackedBody{Reader: strings.NewReader(payload)}
			m := streamFromBody(t, body)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			assert.Equal(t, provider.PartError, parts[0].Type)
			require.NotNil(t, parts[0].APICallError)
			assert.False(t, parts[0].APICallError.IsRetryable)
			assert.NotContains(t, parts[0].APICallError.Error(), "private")
			assert.True(t, body.closed)
		})
	}
}

type generatedFrames struct {
	frame  func(index int) string
	count  int
	buffer string
	next   int
}

func (g *generatedFrames) Read(p []byte) (int, error) {
	if g.buffer == "" {
		if g.next == g.count {
			return 0, io.EOF
		}
		g.buffer = g.frame(g.next)
		g.next++
	}
	n := copy(p, g.buffer)
	g.buffer = g.buffer[n:]
	return n, nil
}

func TestModel_StreamAcceptsLargeStreams(t *testing.T) {
	const oneMiB = 1 << 20
	smallDelta := sseFrame(`{"type":"text-delta","id":"a","delta":"x"}`)
	largeDelta := sseFrame(`{"type":"text-delta","id":"a","delta":"` + strings.Repeat("x", oneMiB) + `"}`)
	comment := ":" + strings.Repeat("x", oneMiB) + "\n"
	for _, tc := range []struct {
		name      string
		frame     func(index int) string
		count     int
		wantParts int
	}{
		{"more than 100000 events", func(int) string { return smallDelta }, 100001, 100001},
		{"single event above 1 MiB", func(int) string { return largeDelta }, 1, 1},
		{"cumulative bytes above 64 MiB", func(index int) string {
			if index < 65 {
				return comment
			}
			return smallDelta
		}, 66, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: &generatedFrames{frame: tc.frame, count: tc.count}}
			m := streamFromBody(t, body)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectPartsWithin(t, result, time.Minute)
			require.Len(t, parts, tc.wantParts)
			for _, part := range parts {
				require.Equal(t, provider.PartTextDelta, part.Type)
			}
			assert.True(t, body.closed)
		})
	}
}

type atomicBody struct {
	io.ReadCloser
	closed atomic.Int32
}

func (b *atomicBody) Close() error { b.closed.Add(1); return b.ReadCloser.Close() }

func TestModel_StreamCancellationAndTransport(t *testing.T) {
	t.Run("ready send cancellation race", func(t *testing.T) {
		for range 50 {
			body := &atomicBody{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat(sseFrame(`{"type":"text-start","id":"a"}`), 10)))}
			m := streamFromBody(t, body)
			ctx, cancel := context.WithCancel(context.Background())
			result, err := m.DoStream(ctx, provider.CallOptions{})
			require.NoError(t, err)
			cancel()
			parts := collectParts(t, result)
			for _, part := range parts {
				assert.NotEqual(t, provider.PartError, part.Type)
			}
			assert.Equal(t, int32(1), body.closed.Load())
		}
	})
	t.Run("blocked reader", func(t *testing.T) {
		reader, writer := io.Pipe()
		defer func() { _ = writer.Close() }()
		body := &atomicBody{ReadCloser: reader}
		m := streamFromBody(t, body)
		ctx, cancel := context.WithCancel(context.Background())
		result, err := m.DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)
		cancel()
		parts := collectParts(t, result)
		assert.Empty(t, parts)
		assert.Equal(t, int32(1), body.closed.Load())
	})
	t.Run("blocked delivery", func(t *testing.T) {
		payload := strings.Repeat(sseFrame(`{"type":"text-delta","id":"a","delta":"hello"}`), 1000)
		body := &atomicBody{ReadCloser: io.NopCloser(strings.NewReader(payload))}
		m := streamFromBody(t, body)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result, err := m.DoStream(ctx, provider.CallOptions{})
		require.NoError(t, err)
		require.Eventually(t, func() bool { return len(result.Stream) == 64 }, time.Second, time.Millisecond)
		cancel()
		parts := collectParts(t, result)
		for _, part := range parts {
			assert.NotEqual(t, provider.PartError, part.Type)
		}
		assert.Equal(t, int32(1), body.closed.Load())
	})
	t.Run("transport after output", func(t *testing.T) {
		cause := errors.New("private transport error")
		body := &trackedBody{Reader: io.MultiReader(strings.NewReader(sseFrame(`{"type":"text-start","id":"a"}`)), errorReader{cause})}
		m := streamFromBody(t, body)
		result, err := m.DoStream(context.Background(), provider.CallOptions{})
		require.NoError(t, err)
		parts := collectParts(t, result)
		require.Len(t, parts, 2)
		assert.Equal(t, provider.PartTextStart, parts[0].Type)
		assert.ErrorIs(t, parts[1].APICallError, cause)
		assert.True(t, parts[1].APICallError.IsRetryable)
		assert.NotContains(t, parts[1].APICallError.Error(), "private transport error")
		assert.True(t, body.closed)
	})
	t.Run("real transport after output does not replay", func(t *testing.T) {
		var calls atomic.Int32
		p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Content-Length", "4096")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, sseFrame(`{"type":"text-start","id":"a"}`))
			w.(http.Flusher).Flush()
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijacking response: %v", err)
				return
			}
			_ = conn.Close()
		})
		m, err := p.LanguageModel("assistant")
		require.NoError(t, err)
		result, err := m.DoStream(context.Background(), provider.CallOptions{Prompt: []provider.Message{}})
		require.NoError(t, err)
		parts := collectParts(t, result)
		require.Len(t, parts, 2)
		assert.Equal(t, provider.PartTextStart, parts[0].Type)
		assert.Equal(t, provider.PartError, parts[1].Type)
		assert.ErrorIs(t, parts[1].APICallError, io.ErrUnexpectedEOF)
		assert.Equal(t, int32(1), calls.Load())
	})
}

func TestModel_StreamSetupFailure(t *testing.T) {
	for _, status := range []int{200, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader("private-body")}
			p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"text/html"}}, Body: body, Request: req}, nil
			})}})
			require.NoError(t, err)
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.True(t, body.closed)
			assert.NotContains(t, err.Error(), "private-body")
		})
	}
}

func TestModel_StreamSetupErrorBackstop(t *testing.T) {
	const errorBody = `{"error":{"message":"private-message","type":"internal_server_error","code":"internal_error","param":null}}`
	for _, delta := range []int64{0, -1} {
		t.Run(fmt.Sprintf("delta=%d", delta), func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(errorBody)}
			p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 500, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
			})}})
			require.NoError(t, err)
			p.bufferedReadLimit = int64(len(errorBody)) + delta
			m, err := p.LanguageModel("assistant")
			require.NoError(t, err)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.Error(t, err)
			assert.Nil(t, result)
			assert.True(t, body.closed)
			var gatewayErr *GatewayError
			if delta == 0 {
				require.ErrorAs(t, err, &gatewayErr)
				return
			}
			assert.False(t, errors.As(err, &gatewayErr))
			var api *provider.APICallError
			require.ErrorAs(t, err, &api)
			assert.False(t, api.IsRetryable)
			assert.NotContains(t, err.Error(), "private-message")
			assert.LessOrEqual(t, body.read, len(errorBody))
		})
	}
}

func TestDecodeStreamPart_FunctionTools(t *testing.T) {
	for _, raw := range []string{
		`{"type":"tool-input-start","id":"a","toolName":"f"}`,
		`{"type":"tool-input-delta","id":"a","delta":""}`,
		`{"type":"tool-input-end","id":"a"}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":"{}"}`,
		`{"type":"tool-result","toolCallId":"a","toolName":"f","result":false,"isError":true}`,
	} {
		part, err := decodeStreamPart([]byte(raw))
		require.NoError(t, err)
		assert.NotEmpty(t, part.Type)
	}
	for _, raw := range []string{
		`{"type":"tool-call","toolCallId":"a","toolName":"f"}`,
		`{"type":"tool-result","toolCallId":"a","toolName":"f","result":null}`,
		`{"type":"tool-input-delta","id":"a"}`,
	} {
		_, err := decodeStreamPart([]byte(raw))
		require.Error(t, err)
	}
	for _, input := range []string{"", `{"service":`, "\"\\\n\t<>&\u2028\u2029"} {
		raw, err := json.Marshal(map[string]string{"type": "tool-call", "toolCallId": "a", "toolName": "f", "input": input})
		require.NoError(t, err)
		part, err := decodeStreamPart(raw)
		require.NoError(t, err)
		assert.Equal(t, input, part.Input)
	}
}

func TestModel_StreamCustomParts(t *testing.T) {
	frames := []string{
		`{"type":"text-start","id":"t"}`,
		`{"type":"text-delta","id":"t","delta":"a"}`,
		`{"type":"custom","kind":"anthropic.message_start","providerMetadata":{"anthropic":{"id":"msg_1","usage":{"input_tokens":3}}}}`,
		`{"type":"text-delta","id":"t","delta":"b"}`,
		`{"type":"custom","kind":"example.bare"}`,
		`{"type":"custom","kind":"example.empty","providerMetadata":{}}`,
		`{"type":"custom","kind":"","text":7}`,
		`{"type":"text-end","id":"t"}`,
		finishEvent,
	}
	var payload strings.Builder
	for _, frame := range frames {
		payload.WriteString(sseFrame(frame))
	}
	model := streamFromBody(t, io.NopCloser(strings.NewReader(payload.String())))
	stream, err := model.DoStream(context.Background(), provider.CallOptions{})
	require.NoError(t, err)

	parts := collectParts(t, stream)

	require.Len(t, parts, len(frames))
	types := make([]provider.StreamPartType, len(parts))
	for i, part := range parts {
		types[i] = part.Type
	}
	assert.Equal(t, []provider.StreamPartType{
		provider.PartTextStart, provider.PartTextDelta, provider.PartCustom, provider.PartTextDelta, provider.PartCustom,
		provider.PartCustom, provider.PartCustom, provider.PartTextEnd, provider.PartFinish,
	}, types)
	assert.Equal(t, "anthropic.message_start", parts[2].Kind)
	assert.JSONEq(t, `{"id":"msg_1","usage":{"input_tokens":3}}`, string(parts[2].ProviderMetadata["anthropic"]))
	assert.Equal(t, "example.bare", parts[4].Kind)
	assert.Nil(t, parts[4].ProviderMetadata)
	assert.NotNil(t, parts[5].ProviderMetadata)
	assert.Empty(t, parts[5].ProviderMetadata)
	assert.Equal(t, "", parts[6].Kind)
}
