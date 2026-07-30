package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	aisdk "github.com/grafana/ai-sdk"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedModel struct {
	callCount int
	calls     []provider.CallOptions
}

func (m *scriptedModel) SpecificationVersion() string               { return "v4" }
func (m *scriptedModel) Provider() string                           { return "test" }
func (m *scriptedModel) ModelID() string                            { return "weather-script" }
func (m *scriptedModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *scriptedModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	return nil, nil
}
func (m *scriptedModel) DoStream(_ context.Context, opts provider.CallOptions) (*provider.StreamResult, error) {
	m.callCount++
	m.calls = append(m.calls, opts)
	if m.callCount == 1 {
		return &provider.StreamResult{Stream: streamParts(
			provider.StreamPart{Type: provider.PartToolCall, ToolCallID: "weather-1", ToolName: "get_weather", Input: `{"city":"Paris"}`},
			provider.StreamPart{
				Type:         provider.PartFinish,
				FinishReason: &provider.FinishReason{Unified: provider.FinishReasonToolCalls},
				Usage:        testUsage(8, 3),
			},
		)}, nil
	}

	return &provider.StreamResult{Stream: streamParts(
		provider.StreamPart{Type: provider.PartTextStart, ID: "text-1"},
		provider.StreamPart{Type: provider.PartTextDelta, ID: "text-1", Delta: "Paris is 18°C and partly cloudy."},
		provider.StreamPart{Type: provider.PartTextEnd, ID: "text-1"},
		provider.StreamPart{
			Type:         provider.PartFinish,
			FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop},
			Usage:        testUsage(12, 7),
		},
	)}, nil
}

func streamParts(parts ...provider.StreamPart) <-chan provider.StreamPart {
	stream := make(chan provider.StreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	return stream
}

func testUsage(input, output int) *provider.Usage {
	return &provider.Usage{
		InputTokens:  provider.InputTokenUsage{Total: &input},
		OutputTokens: provider.OutputTokenUsage{Total: &output},
	}
}

func TestChatHandler(t *testing.T) {
	t.Run("rejects invalid JSON", func(t *testing.T) {
		agent, err := newAgent(&scriptedModel{})
		require.NoError(t, err)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader("{"))
		newChatHandler(agent).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "invalid request\n", recorder.Body.String())
	})

	t.Run("rejects trailing JSON", func(t *testing.T) {
		agent, err := newAgent(&scriptedModel{})
		require.NoError(t, err)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"messages":[]} {}`))
		newChatHandler(agent).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "invalid request\n", recorder.Body.String())
	})

	t.Run("rejects invalid messages", func(t *testing.T) {
		tests := []struct {
			name string
			body string
		}{
			{name: "empty history", body: `{"messages":[]}`},
			{name: "unknown role", body: `{"messages":[{"id":"message-1","role":"unknown","parts":[]}]}`},
			{name: "invalid tool state", body: `{"messages":[{"id":"assistant-1","role":"assistant","parts":[{"type":"tool-get_weather","toolCallId":"weather-1","state":"input-available"}]}]}`},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				agent, err := newAgent(&scriptedModel{})
				require.NoError(t, err)

				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(tc.body))
				newChatHandler(agent).ServeHTTP(recorder, request)

				assert.Equal(t, http.StatusBadRequest, recorder.Code)
				assert.Equal(t, "invalid messages\n", recorder.Body.String())
			})
		}
	})

	t.Run("executes tool and streams final answer", func(t *testing.T) {
		model := &scriptedModel{}
		agent, err := newAgent(model)
		require.NoError(t, err)

		body := `{"messages":[{"id":"user-1","role":"user","parts":[{"type":"text","text":"What is the weather in Paris?"}]}]}`
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
		newChatHandler(agent).ServeHTTP(recorder, request)

		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
		assert.Equal(t, "v1", recorder.Header().Get("x-vercel-ai-ui-message-stream"))
		assert.Equal(t, 2, model.callCount)
		require.Len(t, model.calls, 2)
		require.Len(t, model.calls[0].Tools, 1)
		assert.Equal(t, "get_weather", model.calls[0].Tools[0].Name)
		require.NotEmpty(t, model.calls[0].Prompt)
		assert.Equal(t, provider.RoleUser, model.calls[0].Prompt[len(model.calls[0].Prompt)-1].Role)

		var toolResult *provider.ContentPart
		for _, message := range model.calls[1].Prompt {
			for i := range message.Content {
				if message.Content[i].Type == provider.ContentPartTypeToolResult {
					toolResult = &message.Content[i]
				}
			}
		}
		require.NotNil(t, toolResult)
		require.NotNil(t, toolResult.Output)
		assert.Equal(t, provider.ToolOutputJSON, toolResult.Output.Type)
		assert.JSONEq(t, `{"city":"Paris","celsius":18,"conditions":"partly cloudy"}`, string(toolResult.Output.JSON))

		chunks, done := decodeSSEChunks(t, recorder.Body.String())
		assert.True(t, done)

		var chunkTypes []aisdk.ChunkType
		var text string
		var toolInput, toolOutput json.RawMessage
		for _, chunk := range chunks {
			chunkTypes = append(chunkTypes, chunk.Type)
			switch chunk.Type {
			case aisdk.ChunkTextDelta:
				text += chunk.Delta
			case aisdk.ChunkToolInputAvailable:
				assert.Equal(t, "get_weather", chunk.ToolName)
				toolInput = chunk.Input
			case aisdk.ChunkToolOutputAvailable:
				toolOutput = chunk.Output
			}
		}

		assert.Contains(t, chunkTypes, aisdk.ChunkToolInputAvailable)
		assert.Contains(t, chunkTypes, aisdk.ChunkToolOutputAvailable)
		assert.Contains(t, chunkTypes, aisdk.ChunkTextDelta)
		assert.Contains(t, chunkTypes, aisdk.ChunkFinish)
		assert.JSONEq(t, `{"city":"Paris"}`, string(toolInput))
		assert.JSONEq(t, `{"city":"Paris","celsius":18,"conditions":"partly cloudy"}`, string(toolOutput))
		assert.Equal(t, "Paris is 18°C and partly cloudy.", text)
	})
}

func decodeSSEChunks(t *testing.T, body string) ([]aisdk.UIMessageChunk, bool) {
	t.Helper()
	var chunks []aisdk.UIMessageChunk
	var done bool
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			done = true
			continue
		}
		var chunk aisdk.UIMessageChunk
		require.NoError(t, json.Unmarshal([]byte(data), &chunk))
		chunks = append(chunks, chunk)
	}
	return chunks, done
}
