package messages

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ToolChoice(t *testing.T) {
	for _, tc := range []struct {
		choice string
		want   provider.ToolChoice
	}{
		{`{"type":"auto"}`, provider.ToolChoice{Type: provider.ToolChoiceAuto}},
		{`{"type":"any"}`, provider.ToolChoice{Type: provider.ToolChoiceRequired}},
		{`{"type":"tool","name":"lookup"}`, provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "lookup"}},
		{`{"type":"none"}`, provider.ToolChoice{Type: provider.ToolChoiceNone}},
	} {
		t.Run(tc.choice, func(t *testing.T) {
			body := `{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}],"tool_choice":` + tc.choice + `}`
			seen := serveForTarget(t, execution.Target{Type: execution.TargetAnthropic}, body)
			require.NotNil(t, seen.ToolChoice)
			assert.Equal(t, tc.want, *seen.ToolChoice)
		})
	}
}

func TestHandler_InvalidEffort(t *testing.T) {
	for _, value := range []string{`""`, `"HIGH"`, `"unknown"`, `1`} {
		t.Run(value, func(t *testing.T) {
			h := testHandler(t, &fakeModel{generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
				require.FailNow(t, "invalid effort reached the provider")
				return nil, nil
			}})
			body := fmt.Sprintf(`{"model":"missing","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":%s}}`, value)
			req := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, req)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.JSONEq(t, `{"type":"error","error":{"type":"invalid_request_error","message":"invalid or unsupported request"}}`, response.Body.String())
		})
	}
}

func TestHandler_StopReasons(t *testing.T) {
	for _, tc := range []struct {
		reason provider.FinishReason
		want   anthropicsdk.StopReason
	}{
		{provider.FinishReason{Unified: provider.FinishReasonLength}, anthropicsdk.StopReasonMaxTokens},
		{provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "pause_turn"}, anthropicsdk.StopReasonPauseTurn},
		{provider.FinishReason{Unified: provider.FinishReasonStop, Raw: "stop_sequence"}, anthropicsdk.StopReasonStopSequence},
	} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.want, streaming), func(t *testing.T) {
				server := httptest.NewServer(testHandler(t, &fakeModel{
					generate: func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
						result := textResult()
						result.FinishReason = tc.reason
						return result, nil
					},
					stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						end := finish(tc.reason.Unified)
						end.FinishReason = &tc.reason
						return streamOf(
							provider.StreamPart{Type: provider.PartTextStart, ID: "t"},
							provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "hello"},
							provider.StreamPart{Type: provider.PartTextEnd, ID: "t"},
							end,
						), nil
					},
				}))
				defer server.Close()
				client := anthropicsdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"), option.WithMaxRetries(0))
				params := anthropicsdk.MessageNewParams{Model: "alias", MaxTokens: 8, Messages: []anthropicsdk.MessageParam{anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("hi"))}}
				var message anthropicsdk.Message
				if streaming {
					stream := client.Messages.NewStreaming(context.Background(), params)
					defer func() { assert.NoError(t, stream.Close()) }()
					var stopSequence json.RawMessage
					for stream.Next() {
						current := stream.Current()
						require.NoError(t, message.Accumulate(current))
						if current.Type == string(eventMessageDelta) {
							var event struct {
								Delta struct {
									StopSequence json.RawMessage `json:"stop_sequence"`
								} `json:"delta"`
							}
							require.NoError(t, json.Unmarshal([]byte(current.RawJSON()), &event))
							stopSequence = event.Delta.StopSequence
						}
					}
					require.NoError(t, stream.Err())
					assert.Equal(t, "null", string(stopSequence))
				} else {
					response, err := client.Messages.New(context.Background(), params)
					require.NoError(t, err)
					message = *response
					assert.Equal(t, "null", message.JSON.StopSequence.Raw())
				}
				assert.Equal(t, tc.want, message.StopReason)
				require.Len(t, message.Content, 1)
				assert.Equal(t, "hello", message.Content[0].Text)
			})
		}
	}
}

func TestHandler_StreamFailureAfterContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		part provider.StreamPart
		body string
	}{
		{"rate limit", provider.StreamPart{Type: provider.PartError, APICallError: &provider.APICallError{StatusCode: 429, Message: "private-provider-secret", ResponseBody: "private-provider-secret"}}, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limit exceeded"}}`},
		{"overloaded", provider.StreamPart{Type: provider.PartError, APICallError: &provider.APICallError{StatusCode: 529, Message: "private-provider-secret"}}, `{"type":"error","error":{"type":"overloaded_error","message":"service unavailable"}}`},
		{"unrepresented finish", finish(provider.FinishReasonError), `{"type":"error","error":{"type":"api_error","message":"upstream failure"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testHandler(t, &fakeModel{stream: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return streamOf(
					provider.StreamPart{Type: provider.PartTextStart, ID: "t"},
					provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "hello"},
					provider.StreamPart{Type: provider.PartTextEnd, ID: "t"},
					tc.part,
				), nil
			}})
			request := httptest.NewRequest(http.MethodPost, Path, strings.NewReader(`{"model":"alias","max_tokens":8,"messages":[{"role":"user","content":"hi"}],"stream":true}`))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			h.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, "text/event-stream", response.Header().Get("Content-Type"))
			body := response.Body.String()
			assert.Contains(t, body, `"text":"hello"`)
			assert.NotContains(t, body, "private-provider-secret")
			assert.NotContains(t, body, "event: message_stop")
			frames := strings.Split(strings.TrimSpace(body), "\n\n")
			last := frames[len(frames)-1]
			require.True(t, strings.HasPrefix(last, "event: error\ndata: "), last)
			assert.JSONEq(t, tc.body, strings.TrimPrefix(last, "event: error\ndata: "))
			assert.Equal(t, 1, strings.Count(body, "event: error\n"))
		})
	}
}
