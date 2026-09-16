package grafana

import (
	"context"
	"encoding/json"
	"errors"
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

func jsonMember(t *testing.T, body []byte, key string) string {
	t.Helper()
	var value map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &value))
	return string(value[key])
}

const finishEvent = `{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}}}`

func collectParts(t *testing.T, result *provider.StreamResult) []provider.StreamPart {
	t.Helper()
	done := time.NewTimer(3 * time.Second)
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

func streamFromBody(t *testing.T, body io.ReadCloser, limits *Limits) provider.LanguageModel {
	t.Helper()
	p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test/api/prefix", Limits: limits, HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
			m := streamFromBody(t, body, nil)
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

func TestModel_StreamEOF(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		count         int
	}{
		{"empty", "", 0}, {"DONE", sseFrame("[DONE]"), 0}, {"finish", sseFrame(finishEvent), 1}, {"without finish", sseFrame(`{"type":"text-start","id":"a"}`), 1}, {"unterminated event", "data: {\"type\":\"text-start\",\"id\":\"a\"}", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := streamFromBody(t, io.NopCloser(strings.NewReader(tc.payload)), nil)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, tc.count)
			for _, part := range parts {
				assert.NotEqual(t, provider.PartError, part.Type)
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
			m := streamFromBody(t, io.NopCloser(fragmentedReader{strings.NewReader(payload)}), nil)
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
	for _, event := range []string{`{"type":"response-metadata"}`, `{"type":"response-metadata","modelId":""}`, `{"type":"response-metadata","modelId":"assistant","timestamp":"2026-08-22T00:00:00,123Z"}`} {
		_, err := decodeStreamPart([]byte(event))
		require.Error(t, err)
	}
}

func TestModel_StreamInvalidEvents(t *testing.T) {
	for _, event := range []string{
		`{"type":"stream-start","warnings":[{"Type":"other","message":"safe"}]}`,
		`{"type":"stream-start","warnings":[{"type":"other","Message":"safe"}]}`,
		`{"type":"reasoning-start","id":"a"}`, `{"type":"text-start"}`, `{"type":"text-delta","id":"a"}`, `{"type":"text-end","id":null}`, `{"type":"finish"}`, `{"type":"stream-start"}`, `{"type":"stream-start","warnings":[{"type":"unknown"}]}`, `{"type":"response-metadata","timestamp":"bad"}`, `{"type":"raw"}`, `{"type":"error","error":{"message":"bad"}}`, `{"type":"error","error":{"message":"safe","type":"internal_server_error","param":null,"code":"upstream_error","statusCode":502,"retryable":false}}`, `{"type":"text-delta","id":"a","delta":1}`, `{"type":"finish","finishReason":{"unified":"stop"},"usage":{"inputTokens":{"total":-1},"outputTokens":{}}}`, `{"type":`, "[DONE] ", "",
	} {
		t.Run(event, func(t *testing.T) {
			payload := sseFrame(event) + sseFrame(finishEvent)
			body := &trackedBody{Reader: strings.NewReader(payload)}
			m := streamFromBody(t, body, nil)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			assert.Equal(t, provider.PartError, parts[0].Type)
			require.NotNil(t, parts[0].APICallError)
			assert.False(t, parts[0].APICallError.IsRetryable)
			assert.True(t, body.closed)
		})
	}
}

func TestModel_StreamBounds(t *testing.T) {
	frame := sseFrame(`{"type":"text-start","id":"a"}`)
	for _, tc := range []struct {
		name      string
		configure func(*Limits)
		payload   string
		count     int
		lastError bool
	}{
		{"exact total", func(l *Limits) { l.StreamBytes = int64(len(frame)); l.StreamEventBytes = l.StreamBytes }, frame, 1, false},
		{"one over total", func(l *Limits) { l.StreamBytes = int64(len(frame) - 1); l.StreamEventBytes = l.StreamBytes }, frame, 1, true},
		{"exact event", func(l *Limits) { l.StreamEventBytes = int64(len(frame)) }, frame, 1, false},
		{"one over event", func(l *Limits) { l.StreamEventBytes = int64(len(frame) - 1) }, frame, 1, true},
		{"long line", func(l *Limits) { l.StreamEventBytes = 16 }, "data:" + strings.Repeat("x", 100000), 1, true},
		{"exact count", func(l *Limits) { l.StreamEvents = 2 }, frame + frame, 2, false},
		{"one over count", func(l *Limits) { l.StreamEvents = 1 }, frame + frame, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := DefaultLimits()
			tc.configure(&limits)
			body := &trackedBody{Reader: strings.NewReader(tc.payload)}
			m := streamFromBody(t, body, &limits)
			result, err := m.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, tc.count)
			assert.Equal(t, tc.lastError, parts[len(parts)-1].Type == provider.PartError)
			if tc.lastError {
				assert.False(t, parts[len(parts)-1].APICallError.IsRetryable)
			}
			assert.True(t, body.closed)
			assert.LessOrEqual(t, int64(body.read), limits.StreamBytes+1)
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
			m := streamFromBody(t, body, nil)
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
		m := streamFromBody(t, body, nil)
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
		m := streamFromBody(t, body, nil)
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
		m := streamFromBody(t, body, nil)
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
		`{"type":"tool-input-start","id":"a","toolName":"f","providerExecuted":true}`,
		`{"type":"tool-call","toolCallId":"a","toolName":"f","input":"{}","dynamic":true}`,
		`{"type":"tool-result","toolCallId":"a","toolName":"f","result":null}`,
		`{"type":"tool-input-delta","id":"a"}`,
	} {
		_, err := decodeStreamPart([]byte(raw))
		require.Error(t, err)
	}
}
