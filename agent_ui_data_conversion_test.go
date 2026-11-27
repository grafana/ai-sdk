package aisdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ AgentUIStreamOption = (UIMessageStreamOption)(nil)

func TestAgentUIStream_DataConversion(t *testing.T) {
	for _, write := range []bool{false, true} {
		name := "create"
		if write {
			name = "write"
		}
		t.Run(name, func(t *testing.T) {
			userText := DataPart{DataName: "text", ID: "user-text", Data: json.RawMessage(`"user context"`)}
			userFile := DataPart{DataName: "file", ID: "user-file", Data: json.RawMessage(`"user.txt"`)}
			assistantText := DataPart{DataName: "text", ID: "assistant-text", Data: json.RawMessage(`"assistant context"`)}
			assistantFile := DataPart{DataName: "file", ID: "assistant-file", Data: json.RawMessage(`"assistant.txt"`)}
			skip := DataPart{DataName: "skip", ID: "local", Data: json.RawMessage(`null`)}
			messages := []UIMessage{
				{ID: "u", Role: RoleUser, Parts: []Part{TextPart{Text: "question"}, userText, userFile}},
				{ID: "a", Role: RoleAssistant, Parts: []Part{assistantText, TextPart{Text: "previous answer"}, skip, assistantFile}},
			}
			original, err := json.Marshal(messages)
			require.NoError(t, err)

			var prompt []provider.Message
			model := &mockModel{streamFunc: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
				prompt = options.Prompt
				return &provider.StreamResult{Stream: textStreamParts("continued")}, nil
			}}
			file := func(filename string) provider.ContentPart {
				p := provider.FilePart("text/plain", provider.BytesDataContent([]byte("document")))
				p.Filename = &filename
				return p
			}
			var seen []DataPart
			var retained json.RawMessage
			var finished UIMessageStreamOnFinishState
			opts := []AgentUIStreamOption{
				WithUIMessageStreamOriginalMessages(UIMessage{ID: "not-the-original", Role: RoleAssistant}),
				WithAgentUIStreamConvertDataPart(func(p DataPart) (*provider.ContentPart, error) {
					if retained != nil {
						retained[1] = '!'
					}
					seen = append(seen, DataPart{DataName: p.DataName, ID: p.ID, Data: cloneRawMessage(p.Data)})
					p.Data[0] = 'x'
					retained = p.Data
					switch p.ID {
					case userText.ID:
						return new(provider.TextPart("user context")), nil
					case assistantText.ID:
						return new(provider.TextPart("assistant context")), nil
					case userFile.ID:
						return new(file("user.txt")), nil
					case assistantFile.ID:
						return new(file("assistant.txt")), nil
					default:
						return nil, nil
					}
				}),
				nil,
				OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finished = state }),
			}
			agent := NewToolLoopAgent(model)
			if write {
				w := httptest.NewRecorder()
				require.NoError(t, WriteAgentUIStream(w, t.Context(), agent, messages, opts...))
				assert.Equal(t, "v1", w.Header().Get("x-vercel-ai-ui-message-stream"))
				assert.Contains(t, w.Body.String(), `"messageId":"a"`)
				assert.Contains(t, w.Body.String(), "data: [DONE]\n\n")
			} else {
				stream, err := CreateAgentUIStream(t.Context(), agent, messages, opts...)
				require.NoError(t, err)
				for range stream {
				}
			}

			assert.Equal(t, 1, model.callCount)
			assert.Equal(t, []provider.Message{
				provider.NewUserMessage(provider.TextPart("question"), provider.TextPart("user context"), file("user.txt")),
				provider.NewAssistantMessage(provider.TextPart("assistant context"), provider.TextPart("previous answer"), file("assistant.txt")),
			}, prompt)
			assert.Equal(t, []DataPart{userText, userFile, assistantText, skip, assistantFile}, seen)
			after, err := json.Marshal(messages)
			require.NoError(t, err)
			assert.JSONEq(t, string(original), string(after))

			require.NoError(t, finished.AssemblyError)
			assert.True(t, finished.IsContinuation)
			assert.Equal(t, "a", finished.ResponseMessage.ID)
			require.Len(t, finished.Messages, 2)
			assert.Equal(t, messages[0], finished.Messages[0])
			require.Greater(t, len(finished.ResponseMessage.Parts), len(messages[1].Parts))
			assert.Equal(t, messages[1].Parts, finished.ResponseMessage.Parts[:len(messages[1].Parts)])
			assert.Equal(t, finished.ResponseMessage, finished.Messages[1])
		})
	}
}

func TestAgentUIStream_DataConversionDefaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []AgentUIStreamOption
	}{
		{name: "omitted"},
		{name: "nil option", opts: []AgentUIStreamOption{nil}},
		{name: "nil callback", opts: []AgentUIStreamOption{WithAgentUIStreamConvertDataPart(nil)}},
		{name: "ignored result", opts: []AgentUIStreamOption{WithAgentUIStreamConvertDataPart(func(DataPart) (*provider.ContentPart, error) {
			return nil, nil
		})}},
	} {
		for _, helper := range []string{"create", "write"} {
			t.Run(helper+"/"+tc.name, func(t *testing.T) {
				var prompt []provider.Message
				model := &mockModel{streamFunc: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
					prompt = options.Prompt
					return &provider.StreamResult{Stream: finishStreamParts()}, nil
				}}
				original := UIMessage{ID: "a", Role: RoleAssistant, Parts: []Part{TextPart{Text: "answer"}, DataPart{DataName: "local", Data: json.RawMessage(`true`)}}}
				history := []UIMessage{{ID: "u", Role: RoleUser, Parts: []Part{TextPart{Text: "question"}}}, original}
				var finished UIMessageStreamOnFinishState
				opts := append(tc.opts, OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finished = state }))
				agent := NewToolLoopAgent(model)
				if helper == "write" {
					require.NoError(t, WriteAgentUIStream(httptest.NewRecorder(), t.Context(), agent, history, opts...))
				} else {
					stream, err := CreateAgentUIStream(t.Context(), agent, history, opts...)
					require.NoError(t, err)
					for range stream {
					}
				}

				assert.Equal(t, []provider.Message{provider.UserText("question"), provider.NewAssistantMessage(provider.TextPart("answer"))}, prompt)
				require.NoError(t, finished.AssemblyError)
				assert.True(t, finished.IsContinuation)
				require.Len(t, finished.Messages, len(history))
				require.GreaterOrEqual(t, len(finished.Messages[1].Parts), len(original.Parts))
				assert.Equal(t, original.Parts, finished.Messages[1].Parts[:len(original.Parts)])
			})
		}
	}
}

func TestAgentUIStream_DataConversionError(t *testing.T) {
	sentinel := errors.New("aisdk: test conversion failure")
	for _, tc := range []struct {
		name       string
		converter  func(DataPart) (*provider.ContentPart, error)
		badHistory bool
		wantError  string
		wantCause  error
	}{
		{name: "callback error", converter: func(DataPart) (*provider.ContentPart, error) { return nil, sentinel }, wantError: `converting data part "context"`, wantCause: sentinel},
		{name: "unsupported content", converter: func(DataPart) (*provider.ContentPart, error) {
			return &provider.ContentPart{Type: provider.ContentPartTypeReasoning}, nil
		}, wantError: `data part "context" converter returned unsupported content type`},
		{name: "invalid history", badHistory: true, wantError: "validating UI message 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := &mockModel{streamFunc: func(_ context.Context, _ provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: finishStreamParts()}, nil
			}}
			agent := &uiValidationAgentSpy{Agent: NewToolLoopAgent(model)}
			history := []UIMessage{{ID: "u", Role: RoleUser, Parts: []Part{DataPart{DataName: "context", Data: json.RawMessage(`true`)}}}}
			converter := tc.converter
			if tc.badHistory {
				history = append(history, UIMessage{ID: "invalid", Role: RoleUser})
				converter = func(DataPart) (*provider.ContentPart, error) {
					t.Error("converter ran before history validation completed")
					return nil, nil
				}
			}
			opt := WithAgentUIStreamConvertDataPart(converter)
			stream, err := CreateAgentUIStream(t.Context(), agent, history, opt)
			require.ErrorContains(t, err, tc.wantError)
			assert.Nil(t, stream)
			w := httptest.NewRecorder()
			writeErr := WriteAgentUIStream(w, t.Context(), agent, history, opt)
			require.ErrorContains(t, writeErr, tc.wantError)
			if tc.wantCause != nil {
				assert.ErrorIs(t, err, tc.wantCause)
				assert.ErrorIs(t, writeErr, tc.wantCause)
			}
			assert.Empty(t, w.Header())
			assert.Empty(t, w.Body.String())
			assert.Zero(t, agent.streamCalls)
			assert.Zero(t, model.callCount)
		})
	}
}
