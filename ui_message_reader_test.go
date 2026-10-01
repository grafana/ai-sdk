package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIMessageReader_PersistedToolFields(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprint(dynamic), func(t *testing.T) {
			var input []UIMessageChunk
			for _, raw := range []string{
				fmt.Sprintf(`{"type":"tool-input-start","toolCallId":"c","toolName":"lookup","dynamic":%t,"title":"","toolMetadata":{},"providerExecuted":true}`, dynamic),
				`{"type":"tool-input-available","toolCallId":"c","toolName":"lookup","input":{},"dynamic":` + fmt.Sprint(dynamic) + `}`,
				`{"type":"tool-approval-request","toolCallId":"c","approvalId":"a","approvalDescriptor":{"scope":"test"},"reason":"","signature":"sig"}`,
				`{"type":"tool-approval-response","approvalId":"a","approved":true,"reason":"","providerExecuted":false}`,
				`{"type":"tool-output-available","toolCallId":"c","output":null,"preliminary":false}`,
			} {
				var chunk UIMessageChunk
				require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
				encoded, err := json.Marshal(chunk)
				require.NoError(t, err)
				assert.JSONEq(t, raw, string(encoded))
				input = append(input, chunk)
			}
			message, err := AssembleUIMessage(chunks(input...))
			require.NoError(t, err)
			encoded, err := json.Marshal(message)
			require.NoError(t, err)
			var envelope struct {
				Parts []map[string]json.RawMessage `json:"parts"`
			}
			require.NoError(t, json.Unmarshal(encoded, &envelope))
			require.Len(t, envelope.Parts, 1)
			part := envelope.Parts[0]
			assert.Equal(t, `""`, string(part["title"]))
			assert.Equal(t, `{}`, string(part["toolMetadata"]))
			assert.Equal(t, `false`, string(part["preliminary"]))
			assert.JSONEq(t, `{"id":"a","approved":true,"descriptor":{"scope":"test"},"requestReason":"","reason":"","signature":"sig"}`, string(part["approval"]))
			models, err := ConvertToModelMessages([]UIMessage{message})
			require.NoError(t, err)
			require.Len(t, models, 2)
			assert.Equal(t, provider.RoleTool, models[1].Role)
		})
	}
}

func TestUIMessageReader_ProviderExecutionPresence(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		for _, kind := range []ChunkType{ChunkToolApprovalResponse, ChunkToolOutputAvailable, ChunkToolOutputError} {
			for _, supplied := range []bool{false, true} {
				t.Run(fmt.Sprintf("%t/%s/false-supplied-%t", dynamic, kind, supplied), func(t *testing.T) {
					initial := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: ToolStateApprovalRequested, Input: json.RawMessage(`{}`), ProviderExecuted: true, Approval: &ToolApproval{ID: "a", Descriptor: json.RawMessage(`{"scope":"all"}`), RequestReason: new("request"), Signature: "sig", IsAutomatic: true}}
					var part Part = ToolInvocationPart(initial)
					if dynamic {
						part = DynamicToolUIPart(initial)
					}
					fields := `"toolCallId":"c","approvalId":"a","approved":true,"output":"ok","errorText":""`
					if supplied {
						fields += `,"providerExecuted":false`
					}
					var chunk UIMessageChunk
					require.NoError(t, json.Unmarshal([]byte(`{"type":"`+string(kind)+`",`+fields+`}`), &chunk))
					input := []UIMessageChunk{chunk}
					if kind == ChunkToolApprovalResponse {
						input = append(input, UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c", Output: json.RawMessage(`"ok"`)})
					}
					message, err := AssembleUIMessage(chunks(input...), WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{part}}))
					require.NoError(t, err)
					var got toolPartFields
					switch p := message.Parts[0].(type) {
					case ToolInvocationPart:
						got = toolPartFields(p)
					case DynamicToolUIPart:
						got = toolPartFields(p)
					}
					assert.Equal(t, !supplied, got.ProviderExecuted)
					assert.Equal(t, "sig", got.Approval.Signature)
					models, err := ConvertToModelMessages([]UIMessage{message})
					require.NoError(t, err)
					if supplied {
						require.Len(t, models, 2)
						assert.Equal(t, provider.RoleTool, models[1].Role)
					} else {
						assert.Equal(t, provider.ContentPartTypeToolResult, models[0].Content[len(models[0].Content)-1].Type)
						if kind == ChunkToolApprovalResponse {
							require.Len(t, models, 2)
							assert.Equal(t, provider.ContentPartTypeToolApprovalResponse, models[1].Content[0].Type)
						} else {
							require.Len(t, models, 1)
						}
					}
				})
			}
		}
	}
}

func TestUIMessageReader_StaticErrorContinuation(t *testing.T) {
	for _, seeded := range []bool{false, true} {
		for _, raw := range []json.RawMessage{json.RawMessage(`"legacy"`), json.RawMessage(`{"q":"legacy"}`), json.RawMessage(`null`)} {
			t.Run(fmt.Sprintf("seeded-%t/%s", seeded, raw), func(t *testing.T) {
				var options []UIMessageReaderOption
				var updates []UIMessageChunk
				if seeded {
					options = append(options, WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, RawInput: raw, ErrorText: new("old")}}}))
				} else {
					updates = append(updates, UIMessageChunk{Type: ChunkToolInputError, ToolCallID: "c", ToolName: "lookup", Input: raw, ErrorText: "old"})
				}
				updates = append(updates, UIMessageChunk{Type: ChunkToolOutputError, ToolCallID: "c"})
				snapshots := collectMessages(StreamUIMessage(chunks(updates...), options...))
				require.NotEmpty(t, snapshots)
				for _, snapshot := range snapshots {
					part := snapshot.Parts[0].(ToolInvocationPart)
					assert.JSONEq(t, string(raw), string(part.RawInput))
				}
				message, err := AssembleUIMessage(chunks(updates...), options...)
				require.NoError(t, err)
				part := message.Parts[0].(ToolInvocationPart)
				assert.JSONEq(t, string(raw), string(part.RawInput))
				assert.Nil(t, part.Input)
				models, err := ConvertToModelMessages([]UIMessage{message})
				require.NoError(t, err)
				assert.JSONEq(t, string(raw), string(models[0].Content[0].Input))
			})
		}
	}
}

func TestUIMessageReader_InitialMessageIsolation(t *testing.T) {
	initial := UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, Title: new("title"), Input: json.RawMessage(`{}`), RawInput: json.RawMessage(`"raw"`), ErrorText: new("error"), ToolMetadata: map[string]json.RawMessage{"scope": json.RawMessage(`{"name":"all"}`)}, Approval: &ToolApproval{ID: "a", Approved: new(true), Reason: new("reason"), Descriptor: json.RawMessage(`{"scope":"all"}`)}}}}
	before, err := json.Marshal(initial)
	require.NoError(t, err)
	option := WithUIMessageReaderInitialMessage(initial)
	part := initial.Parts[0].(ToolInvocationPart)
	*part.Title = "mutated"
	part.ToolMetadata["scope"][0] = 'x'
	part.Approval.Descriptor[0] = 'x'
	message, err := AssembleUIMessage(chunks(), option)
	require.NoError(t, err)
	encoded, err := json.Marshal(message)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(encoded))
	part = message.Parts[0].(ToolInvocationPart)
	*part.ErrorText = "changed"
	*part.Approval.Reason = "changed"
	part.RawInput[0] = 'x'
	again, err := AssembleUIMessage(chunks(), option)
	require.NoError(t, err)
	encoded, err = json.Marshal(again)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(encoded))
	assert.Empty(t, collectMessages(StreamUIMessage(chunks(), option)))
	_, err = AssembleUIMessage(chunks(TextDeltaChunk("t", "invalid")), option)
	require.Error(t, err)
	for _, id := range []string{"user-id", ""} {
		seed := WithUIMessageReaderInitialMessage(UIMessage{ID: id, Role: RoleUser, Metadata: json.RawMessage(`{"ignored":true}`), Parts: []Part{TextPart{Text: "ignored"}}})
		got, err := AssembleUIMessage(chunks(), seed, WithUIMessageReaderGenerateID(func() string { return "generated" }))
		require.NoError(t, err)
		want := id
		if want == "" {
			want = "generated"
		}
		assert.Equal(t, want, got.ID)
		assert.Empty(t, got.Parts)
		assert.Empty(t, got.Metadata)
		got, err = AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkStart, MessageID: "replacement"}), seed)
		require.NoError(t, err)
		assert.Equal(t, "replacement", got.ID)
	}
	t.Run("data replacement", func(t *testing.T) {
		seed := WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{DataPart{DataName: "status", ID: "d", Data: json.RawMessage(`1`)}}})
		got, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkData, DataName: "status", ID: "d", Data: json.RawMessage(`2`)}), seed)
		require.NoError(t, err)
		require.Len(t, got.Parts, 1)
		assert.JSONEq(t, `2`, string(got.Parts[0].(DataPart).Data))
	})
}

func TestUIMessageReader_FileCloneIsolation(t *testing.T) {
	for i, references := range []map[string]string{nil, {}, {"test": "file"}} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			original := FilePart{Filename: new("original"), ProviderReference: references}
			cloned := clonePart(original).(FilePart)
			assert.Equal(t, original, cloned)
			*cloned.Filename = "changed"
			assert.Equal(t, "original", *original.Filename)
			if references != nil {
				before := references["test"]
				cloned.ProviderReference["test"] = "changed"
				assert.Equal(t, before, original.ProviderReference["test"])
			}
		})
	}
}

func TestUIMessageReader_ToolSnapshotIsolation(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		t.Run(map[bool]string{false: "static", true: "dynamic"}[dynamic], func(t *testing.T) {
			fields := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, Title: new("title"), Input: json.RawMessage(`{"q":"test"}`), RawInput: json.RawMessage(`"raw"`), Output: json.RawMessage(`null`), ErrorText: new(""), Preliminary: new(false), ToolMetadata: map[string]json.RawMessage{"key": json.RawMessage(`1`)}, CallProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"call":1}`)}, ResultProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"result":1}`)}, Approval: &ToolApproval{ID: "a", Approved: new(true), Descriptor: json.RawMessage(`{"scope":1}`), RequestReason: new("request"), Reason: new("reason")}}
			var part Part = ToolInvocationPart(fields)
			if dynamic {
				part = DynamicToolUIPart(fields)
			}
			state := newUIMessageReaderState(buildUIMessageReaderConfig([]UIMessageReaderOption{WithUIMessageReaderInitialMessage(UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{part}})}))
			first := state.snapshot()
			before, err := json.Marshal(first)
			require.NoError(t, err)
			var mutated toolPartFields
			switch p := first.Parts[0].(type) {
			case ToolInvocationPart:
				mutated = toolPartFields(p)
			case DynamicToolUIPart:
				mutated = toolPartFields(p)
			}
			*mutated.Title = "changed"
			*mutated.ErrorText = "changed"
			*mutated.Preliminary = true
			for _, raw := range []json.RawMessage{mutated.Input, mutated.RawInput, mutated.Output, mutated.ToolMetadata["key"], mutated.CallProviderMetadata["test"], mutated.ResultProviderMetadata["test"], mutated.Approval.Descriptor} {
				raw[0] = 'x'
			}
			*mutated.Approval.Approved = false
			*mutated.Approval.RequestReason = "changed"
			*mutated.Approval.Reason = "changed"
			after, err := json.Marshal(state.snapshot())
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
			write, err := state.apply(UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c", Output: json.RawMessage(`"final"`)})
			require.NoError(t, err)
			assert.True(t, write)
			later := state.snapshot()
			encoded, err := json.Marshal(later)
			require.NoError(t, err)
			assert.Contains(t, string(encoded), `"title":"title"`)
			assert.Contains(t, string(encoded), `"requestReason":"request"`)
		})
	}
}

func TestUtilityFunctionalOptions(t *testing.T) {
	t.Run("default calls compile and nil options are ignored", func(t *testing.T) {
		messages := []UIMessage{{ID: "u1", Role: RoleUser, Parts: []Part{TextPart{Text: "hi"}}}}
		modelMessages, err := ConvertToModelMessages(messages)
		require.NoError(t, err)
		require.Len(t, modelMessages, 1)

		var convertOpt ConvertOption
		modelMessages, err = ConvertToModelMessages(messages, convertOpt)
		require.NoError(t, err)
		require.Len(t, modelMessages, 1)

		var streamOpt UIMessageStreamOption
		cfg := buildUIMessageStreamConfig([]UIMessageStreamOption{streamOpt})
		assert.False(t, cfg.hasOriginalMessages)

		var readerOpt UIMessageReaderOption
		msg, err := AssembleUIMessage(chunks(), readerOpt, WithUIMessageReaderGenerateID(func() string { return "empty" }))
		require.NoError(t, err)
		assert.Equal(t, "empty", msg.ID)

		rec := httptest.NewRecorder()
		result := &StreamTextResult{fullStream: make(chan TextStreamPart), done: make(chan struct{})}
		close(result.fullStream)
		close(result.done)
		require.NoError(t, WriteUIMessageStream(rec, result))
	})

	t.Run("conversion option filters incomplete tool calls", func(t *testing.T) {
		messages := []UIMessage{{ID: "a1", Role: RoleAssistant, Parts: []Part{
			TextPart{Text: "checking"},
			ToolInvocationPart{ToolCallID: "c1", ToolName: "weather", State: ToolStateInputStreaming},
		}}}

		modelMessages, err := ConvertToModelMessages(messages, WithIgnoreIncompleteToolCalls())
		require.NoError(t, err)
		require.Len(t, modelMessages, 1)
		require.Len(t, modelMessages[0].Content, 1)
		assert.Equal(t, provider.ContentPartTypeText, modelMessages[0].Content[0].Type)
	})

	t.Run("ui message stream options apply in order", func(t *testing.T) {
		cfg := buildUIMessageStreamConfig([]UIMessageStreamOption{
			WithUIMessageStreamSources(false),
			WithUIMessageStreamSources(true),
			WithUIMessageStreamReasoning(true),
			WithUIMessageStreamReasoning(false),
		})

		require.NotNil(t, cfg.sendSources)
		assert.True(t, *cfg.sendSources)
		require.NotNil(t, cfg.sendReasoning)
		assert.False(t, *cfg.sendReasoning)
	})

	t.Run("callbacks use last value", func(t *testing.T) {
		cfg := buildUIMessageStreamConfig([]UIMessageStreamOption{
			OnUIMessageStreamError(func(error) string { return "first" }),
			OnUIMessageStreamError(func(error) string { return "second" }),
		})

		assert.Equal(t, "second", errorText(errors.New("boom"), cfg))
	})
}

func TestUIMessageStreamOriginalMessagesOption(t *testing.T) {
	t.Run("empty original messages are explicitly present", func(t *testing.T) {
		cfg := buildUIMessageStreamConfig([]UIMessageStreamOption{WithUIMessageStreamOriginalMessages()})
		assert.True(t, cfg.hasOriginalMessages)
		assert.NotNil(t, cfg.originalMessages)
		assert.Empty(t, cfg.originalMessages)
	})

	t.Run("nil slice expansion is explicitly present", func(t *testing.T) {
		var original []UIMessage
		cfg := buildUIMessageStreamConfig([]UIMessageStreamOption{WithUIMessageStreamOriginalMessages(original...)})
		assert.True(t, cfg.hasOriginalMessages)
		assert.NotNil(t, cfg.originalMessages)
		assert.Empty(t, cfg.originalMessages)
	})

	t.Run("finish callback receives continuation response", func(t *testing.T) {
		stream := make(chan TextStreamPart, 4)
		stream <- StreamStart{}
		stream <- StreamTextStart{ID: "t1"}
		stream <- StreamTextDelta{ID: "t1", Text: "hi"}
		stream <- StreamTextEnd{ID: "t1"}
		close(stream)

		var finish UIMessageStreamOnFinishState
		result := &StreamTextResult{fullStream: stream, done: make(chan struct{})}
		close(result.done)
		for range result.ToUIMessageStream(
			WithUIMessageStreamOriginalMessages(),
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		assert.False(t, finish.IsContinuation)
		require.Len(t, finish.Messages, 1)
		assert.Equal(t, "generated", finish.ResponseMessage.ID)
		assertTextPart(t, finish.ResponseMessage, "hi", "done")
	})
}

func TestToUIMessageStreamFinishState(t *testing.T) {
	t.Run("callback without originals", func(t *testing.T) {
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(StreamStart{}, StreamTextStart{ID: "t1"}, StreamTextDelta{ID: "t1", Text: "hi"}, StreamTextEnd{ID: "t1"})
		for range result.ToUIMessageStream(
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		assert.False(t, finish.IsContinuation)
		require.Len(t, finish.Messages, 1)
		assert.Equal(t, "generated", finish.ResponseMessage.ID)
		assertTextPart(t, finish.ResponseMessage, "hi", "done")
	})

	t.Run("empty originals generate new persisted response", func(t *testing.T) {
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(StreamStart{}, StreamTextStart{ID: "t1"}, StreamTextDelta{ID: "t1", Text: "hi"}, StreamTextEnd{ID: "t1"})
		for range result.ToUIMessageStream(
			WithUIMessageStreamOriginalMessages(),
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		assert.False(t, finish.IsContinuation)
		require.Len(t, finish.Messages, 1)
		assert.Equal(t, "generated", finish.ResponseMessage.ID)
	})

	t.Run("user-last originals append response", func(t *testing.T) {
		original := UIMessage{ID: "u1", Role: RoleUser, Parts: []Part{TextPart{Text: "hello"}}}
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(StreamStart{}, StreamTextStart{ID: "t1"}, StreamTextDelta{ID: "t1", Text: "hi"}, StreamTextEnd{ID: "t1"})
		for range result.ToUIMessageStream(
			WithUIMessageStreamOriginalMessages(original),
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		assert.False(t, finish.IsContinuation)
		require.Len(t, finish.Messages, 2)
		assert.Equal(t, "u1", finish.Messages[0].ID)
		assert.Equal(t, "generated", finish.Messages[1].ID)
	})

	t.Run("assistant-last originals continue response", func(t *testing.T) {
		calls := 0
		original := UIMessage{ID: "a1", Role: RoleAssistant, Parts: []Part{TextPart{Text: "old", State: "done"}}}
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(StreamStart{}, StreamTextStart{ID: "t1"}, StreamTextDelta{ID: "t1", Text: " new"}, StreamTextEnd{ID: "t1"})
		for range result.ToUIMessageStream(
			WithUIMessageStreamOriginalMessages(original),
			WithUIMessageStreamGenerateID(func() string { calls++; return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		assert.Zero(t, calls)
		assert.True(t, finish.IsContinuation)
		require.Len(t, finish.Messages, 1)
		assert.Equal(t, "a1", finish.ResponseMessage.ID)
		require.Len(t, finish.ResponseMessage.Parts, 2)
		assertTextPart(t, finish.ResponseMessage, "old", "done")
	})

	t.Run("finish callback assembles filtered chunks", func(t *testing.T) {
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(
			StreamStart{},
			StreamReasoningStart{ID: "r1"},
			StreamReasoningDelta{ID: "r1", Text: "hidden"},
			StreamReasoningEnd{ID: "r1"},
			StreamReasoningFile{File: GeneratedFile{Base64: "AQID", MediaType: "image/png"}},
			StreamTextStart{ID: "t1"},
			StreamTextDelta{ID: "t1", Text: "shown"},
			StreamTextEnd{ID: "t1"},
		)
		for range result.ToUIMessageStream(
			WithUIMessageStreamReasoning(false),
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		require.Len(t, finish.ResponseMessage.Parts, 1)
		assertTextPart(t, finish.ResponseMessage, "shown", "done")
	})

	t.Run("abort chunk marks finish state aborted", func(t *testing.T) {
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(StreamStart{}, StreamTextStart{ID: "t1"}, StreamAbort{Reason: "client"})
		for range result.ToUIMessageStream(
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		) {
		}

		assert.True(t, finish.IsAborted)
	})

	t.Run("message metadata chunks emitted for non lifecycle parts", func(t *testing.T) {
		var finish UIMessageStreamOnFinishState
		result := streamTextResultFromParts(StreamStart{}, StreamTextStart{ID: "t1"}, StreamTextDelta{ID: "t1", Text: "hi"})
		stream := result.ToUIMessageStream(
			WithUIMessageStreamGenerateID(func() string { return "generated" }),
			WithUIMessageStreamMessageMetadata(func(part TextStreamPart) json.RawMessage {
				if _, ok := part.(StreamTextDelta); ok {
					return json.RawMessage(`{"nested":{"x":1}}`)
				}
				return nil
			}),
			OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finish = state }),
		)
		var sawMetadata bool
		for chunk := range stream {
			if chunk.Type == ChunkMessageMetadata {
				sawMetadata = true
				assert.JSONEq(t, `{"nested":{"x":1}}`, string(chunk.MessageMetadata))
			}
		}
		assert.True(t, sawMetadata)
		assert.JSONEq(t, `{"nested":{"x":1}}`, string(finish.ResponseMessage.Metadata))
	})
}

func TestStreamUIMessage_ConcurrentReasoningReplacement(t *testing.T) {
	old := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"old","reasoningEncryptedContent":null}`)}
	final := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"final"}`)}
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkStart, MessageID: "m"},
		UIMessageChunk{Type: ChunkReasoningStart, ID: "same", ProviderMetadata: old},
		UIMessageChunk{Type: ChunkReasoningStart, ID: "other", ProviderMetadata: old},
		UIMessageChunk{Type: ChunkTextStart, ID: "same"},
		UIMessageChunk{Type: ChunkReasoningDelta, ID: "same", Delta: "one"},
		UIMessageChunk{Type: ChunkReasoningDelta, ID: "other", Delta: "two"},
		UIMessageChunk{Type: ChunkReasoningFile, MediaType: "image/png", URL: "data:image/png;base64,", ProviderMetadata: final},
		UIMessageChunk{Type: ChunkTextDelta, ID: "same", Delta: "answer"},
		UIMessageChunk{Type: ChunkReasoningEnd, ID: "other", ProviderMetadata: provider.ProviderMetadata{}},
		UIMessageChunk{Type: ChunkReasoningEnd, ID: "same", ProviderMetadata: final},
		UIMessageChunk{Type: ChunkTextEnd, ID: "same"},
		UIMessageChunk{Type: ChunkFinish},
	)))
	parts := messages[len(messages)-1].Parts
	require.Len(t, parts, 4)
	assert.Equal(t, "one", parts[0].(ReasoningPart).Text)
	assert.Equal(t, final, parts[0].(ReasoningPart).ProviderMetadata)
	assert.Equal(t, "two", parts[1].(ReasoningPart).Text)
	assert.Empty(t, parts[1].(ReasoningPart).ProviderMetadata)
	assert.Equal(t, "answer", parts[2].(TextPart).Text)
	assert.Equal(t, "data:image/png;base64,", parts[3].(ReasoningFilePart).URL)
	assert.Equal(t, final, parts[3].(ReasoningFilePart).ProviderMetadata)
}

func TestStreamUIMessage_ProgressiveReasoningAndNonText(t *testing.T) {
	meta := provider.ProviderMetadata{"anthropic": json.RawMessage(`{"signature":"sig"}`)}
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkStart, MessageID: "msg-1", MessageMetadata: json.RawMessage(`{"a":1}`)},
		UIMessageChunk{Type: ChunkReasoningStart, ID: "r1"},
		UIMessageChunk{Type: ChunkReasoningDelta, ID: "r1", Delta: "think", ProviderMetadata: meta},
		UIMessageChunk{Type: ChunkReasoningEnd, ID: "r1"},
		UIMessageChunk{Type: ChunkFile, URL: "https://example.com/file.png", MediaType: "image/png", ProviderMetadata: meta},
		UIMessageChunk{Type: ChunkReasoningFile, URL: "https://example.com/reasoning.png", MediaType: "image/png", ProviderMetadata: meta},
		UIMessageChunk{Type: ChunkSourceURL, SourceID: "s1", URL: "https://example.com", Title: "Example"},
		UIMessageChunk{Type: ChunkSourceDocument, SourceID: "s2", MediaType: "text/plain", Title: "Doc", Filename: "doc.txt"},
		UIMessageChunk{Type: ChunkData, DataName: "weather", ID: "d1", Data: json.RawMessage(`{"temp":70}`)},
		UIMessageChunk{Type: ChunkData, DataName: "weather", ID: "d1", Data: json.RawMessage(`{"temp":72}`)},
		UIMessageChunk{Type: ChunkData, DataName: "transient", Data: json.RawMessage(`{"skip":true}`), Transient: true},
		UIMessageChunk{Type: ChunkMessageMetadata, MessageMetadata: json.RawMessage(`{"b":2}`)},
		UIMessageChunk{Type: ChunkFinish, MessageMetadata: json.RawMessage(`{"c":3}`)},
	)))

	require.Len(t, messages, 12)
	last := messages[len(messages)-1]
	assert.JSONEq(t, `{"a":1,"b":2,"c":3}`, string(last.Metadata))
	require.Len(t, last.Parts, 6)
	rp, ok := last.Parts[0].(ReasoningPart)
	require.True(t, ok)
	assert.Equal(t, "r1", rp.ID)
	assert.Equal(t, "think", rp.Text)
	assert.Equal(t, "done", rp.State)
	assert.Equal(t, meta, rp.ProviderMetadata)
	_, ok = last.Parts[1].(FilePart)
	assert.True(t, ok)
	rfp, ok := last.Parts[2].(ReasoningFilePart)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/reasoning.png", rfp.URL)
	assert.Equal(t, meta, rfp.ProviderMetadata)
	_, ok = last.Parts[3].(SourceURLPart)
	assert.True(t, ok)
	documentSource, ok := last.Parts[4].(SourceDocumentPart)
	require.True(t, ok)
	assert.Equal(t, "doc.txt", documentSource.Filename)
	dp, ok := last.Parts[5].(DataPart)
	require.True(t, ok)
	assert.JSONEq(t, `{"temp":72}`, string(dp.Data))
}

func TestStreamUIMessage_CustomChunk(t *testing.T) {
	meta := provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"cmp-1"}`)}
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkStart, MessageID: "msg-1"},
		UIMessageChunk{Type: ChunkCustom, Kind: "openai.compaction", ProviderMetadata: meta},
		UIMessageChunk{Type: ChunkFinish},
	)))

	require.Len(t, messages, 2)
	last := messages[len(messages)-1]
	require.Len(t, last.Parts, 1)
	custom, ok := last.Parts[0].(CustomPart)
	require.True(t, ok)
	assert.Equal(t, "openai.compaction", custom.Kind)
	assert.Equal(t, meta, custom.ProviderMetadata)
}

func TestStreamUIMessage_ProgressiveToolLifecycle(t *testing.T) {
	dynamic := true
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: "c1", ToolName: "weather"},
		UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "c1", InputTextDelta: `{"city":"Por`},
		UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "c1", InputTextDelta: `tland"}`},
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c1", ToolName: "weather", Input: json.RawMessage(`{"city":"Portland"}`)},
		UIMessageChunk{Type: ChunkToolApprovalRequest, ToolCallID: "c1", ApprovalID: "apr", Signature: "sig"},
		UIMessageChunk{Type: ChunkToolApprovalResponse, ApprovalID: "apr", Approved: true, Reason: "ok"},
		UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c1", Output: json.RawMessage(`{"temp":72}`)},
		UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: "c2", ToolName: "lookup", Dynamic: &dynamic},
		UIMessageChunk{Type: ChunkToolOutputError, ToolCallID: "c2", ErrorText: "failed"},
	)))

	require.Len(t, messages, 9)
	static := requireToolInvocationPart(t, messages[0], 0)
	assert.Equal(t, ToolStateInputStreaming, static.State)
	assert.Nil(t, static.Input)

	static = requireToolInvocationPart(t, messages[1], 0)
	assert.JSONEq(t, `{"city":"Por"}`, string(static.Input))

	static = requireToolInvocationPart(t, messages[2], 0)
	assert.JSONEq(t, `{"city":"Portland"}`, string(static.Input))

	static = requireToolInvocationPart(t, messages[6], 0)
	assert.Equal(t, ToolStateOutputAvailable, static.State)
	assert.JSONEq(t, `{"temp":72}`, string(static.Output))
	require.NotNil(t, static.Approval)
	require.NotNil(t, static.Approval.Approved)
	assert.True(t, *static.Approval.Approved)

	dyn := requireDynamicToolPart(t, messages[8], 1)
	assert.Equal(t, ToolStateOutputError, dyn.State)
	assert.Equal(t, new("failed"), dyn.ErrorText)
}

func TestStreamUIMessage_RepeatedToolCallIDAcrossSteps(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkStartStep},
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "call-0", ToolName: "recordStep", Input: json.RawMessage(`{"step":1}`), ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc-step-1"}`)}},
		UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "call-0", Output: json.RawMessage(`{"recorded":1}`)},
		UIMessageChunk{Type: ChunkFinishStep},
		UIMessageChunk{Type: ChunkStartStep},
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "call-0", ToolName: "recordStep", Input: json.RawMessage(`{"step":2}`), ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"itemId":"fc-step-2"}`)}},
		UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "call-0", Output: json.RawMessage(`{"recorded":2}`)},
		UIMessageChunk{Type: ChunkFinishStep},
	)))

	require.Len(t, messages, 4)
	last := messages[len(messages)-1]
	require.Len(t, last.Parts, 4)
	first := requireToolInvocationPart(t, last, 1)
	assert.Equal(t, ToolStateOutputAvailable, first.State)
	assert.JSONEq(t, `{"step":1}`, string(first.Input))
	assert.JSONEq(t, `{"recorded":1}`, string(first.Output))
	assert.JSONEq(t, `{"itemId":"fc-step-1"}`, string(first.CallProviderMetadata["openai"]))
	second := requireToolInvocationPart(t, last, 3)
	assert.Equal(t, ToolStateOutputAvailable, second.State)
	assert.JSONEq(t, `{"step":2}`, string(second.Input))
	assert.JSONEq(t, `{"recorded":2}`, string(second.Output))
	assert.JSONEq(t, `{"itemId":"fc-step-2"}`, string(second.CallProviderMetadata["openai"]))
}

func TestStreamUIMessage_ToolApprovalResponsePreservesRequestSignature(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c1", ToolName: "weather", Input: json.RawMessage(`{}`)},
		UIMessageChunk{Type: ChunkToolApprovalRequest, ToolCallID: "c1", ApprovalID: "apr", Signature: "sig", IsAutomatic: true},
		UIMessageChunk{Type: ChunkToolApprovalResponse, ApprovalID: "apr", Approved: true, Reason: "ok"},
	)))

	require.Len(t, messages, 3)
	part := requireToolInvocationPart(t, messages[2], 0)
	require.NotNil(t, part.Approval)
	assert.Equal(t, "apr", part.Approval.ID)
	assert.True(t, part.Approval.IsAutomatic)
	assert.Equal(t, "sig", part.Approval.Signature)
	require.NotNil(t, part.Approval.Approved)
	assert.True(t, *part.Approval.Approved)
}

func TestStreamUIMessage_ToolInputErrorAndOutputDenied(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkToolInputError, ToolCallID: "c1", ToolName: "weather", Input: json.RawMessage(`{"city":1}`), ErrorText: "bad input"},
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c2", ToolName: "delete", Input: json.RawMessage(`{}`)},
		UIMessageChunk{Type: ChunkToolOutputDenied, ToolCallID: "c2"},
	)))

	require.Len(t, messages, 3)
	inputErr := requireToolInvocationPart(t, messages[0], 0)
	assert.Equal(t, ToolStateOutputError, inputErr.State)
	assert.Equal(t, new("bad input"), inputErr.ErrorText)
	assert.Nil(t, inputErr.Input)
	assert.JSONEq(t, `{"city":1}`, string(inputErr.RawInput))

	denied := requireToolInvocationPart(t, messages[2], 1)
	assert.Equal(t, ToolStateOutputDenied, denied.State)
}

func TestStreamUIMessage_PartialToolInputInvalidJSONOmitted(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkToolInputStart, ToolCallID: "c1", ToolName: "weather"},
		UIMessageChunk{Type: ChunkToolInputDelta, ToolCallID: "c1", InputTextDelta: `!`},
	)))

	require.Len(t, messages, 2)
	part := requireToolInvocationPart(t, messages[1], 0)
	assert.Equal(t, ToolStateInputStreaming, part.State)
	assert.Nil(t, part.Input)
	_, err := json.Marshal(messages[1])
	require.NoError(t, err)
}

func TestStreamUIMessage_MetadataMerge(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkStart, MessageID: "msg-1", MessageMetadata: json.RawMessage(`{"nested":{"x":1},"keep":true}`)},
		UIMessageChunk{Type: ChunkMessageMetadata, MessageMetadata: json.RawMessage(`{"nested":{"y":2},"arr":[1]}`)},
		UIMessageChunk{Type: ChunkFinish, MessageMetadata: json.RawMessage(`{"nested":{"z":3},"arr":[2]}`)},
	)))

	require.Len(t, messages, 3)
	assert.JSONEq(t, `{"nested":{"x":1,"y":2,"z":3},"keep":true,"arr":[2]}`, string(messages[2].Metadata))
}

func TestStreamUIMessage_SnapshotIsolation(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkStart, MessageID: "msg-1", MessageMetadata: json.RawMessage(`{"a":1}`)},
		TextStartChunk("t1"),
		TextDeltaChunk("t1", "hello"),
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c1", ToolName: "weather", Input: json.RawMessage(`{"city":"NYC"}`), ProviderMetadata: provider.ProviderMetadata{"p": json.RawMessage(`{"x":1}`)}},
		UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c1", Output: json.RawMessage(`{"temp":72}`)},
	)))
	require.Len(t, messages, 5)

	messages[0].Metadata[0] = '['
	tp := messages[2].Parts[0].(TextPart)
	tp.Text = "mutated"
	messages[2].Parts[0] = tp
	toolSnapshot := requireToolInvocationPart(t, messages[3], 1)
	toolSnapshot.Input[0] = '['
	toolSnapshot.CallProviderMetadata["p"][0] = '['

	assert.Equal(t, "msg-1", messages[4].ID)
	assert.JSONEq(t, `{"a":1}`, string(messages[4].Metadata))
	assertTextPart(t, messages[4], "hello", "streaming")
	laterTool := requireToolInvocationPart(t, messages[4], 1)
	assert.JSONEq(t, `{"city":"NYC"}`, string(laterTool.Input))
	assert.JSONEq(t, `{"x":1}`, string(laterTool.CallProviderMetadata["p"]))
	assert.JSONEq(t, `{"temp":72}`, string(laterTool.Output))

	fresh, err := AssembleUIMessage(chunks(
		UIMessageChunk{Type: ChunkStart, MessageID: "msg-1", MessageMetadata: json.RawMessage(`{"a":1}`)},
		TextStartChunk("t1"),
		TextDeltaChunk("t1", "hello"),
		UIMessageChunk{Type: ChunkToolInputAvailable, ToolCallID: "c1", ToolName: "weather", Input: json.RawMessage(`{"city":"NYC"}`), ProviderMetadata: provider.ProviderMetadata{"p": json.RawMessage(`{"x":1}`)}},
		UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c1", Output: json.RawMessage(`{"temp":72}`)},
	))
	require.NoError(t, err)
	assert.Equal(t, messages[4], fresh)

	reasoningMessages := collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkReasoningFile, URL: "https://example.com/reasoning.png", MediaType: "image/png", ProviderMetadata: provider.ProviderMetadata{"p": json.RawMessage(`{"x":1}`)}},
		UIMessageChunk{Type: ChunkFile, URL: "https://example.com/file.png", MediaType: "image/png"},
	)))
	require.Len(t, reasoningMessages, 2)
	reasoningSnapshot := reasoningMessages[0].Parts[0].(ReasoningFilePart)
	reasoningSnapshot.ProviderMetadata["p"][0] = '['
	laterReasoning := reasoningMessages[1].Parts[0].(ReasoningFilePart)
	assert.JSONEq(t, `{"x":1}`, string(laterReasoning.ProviderMetadata["p"]))
}

func TestStreamUIMessage_EmptyAndNonWritingStreams(t *testing.T) {
	assert.Empty(t, collectMessages(StreamUIMessage(chunks())))
	assert.Empty(t, collectMessages(StreamUIMessage(chunks(
		UIMessageChunk{Type: ChunkError, ErrorText: "boom"},
		UIMessageChunk{Type: ChunkStartStep},
		UIMessageChunk{Type: ChunkFinish},
	))))
}

func TestStreamUIMessage_InvalidOrderCloses(t *testing.T) {
	messages := collectMessages(StreamUIMessage(chunks(
		TextDeltaChunk("missing", "nope"),
		TextStartChunk("t1"),
	)))
	assert.Empty(t, messages)
}

func TestAssembleUIMessage_RejectsUnknownChunkType(t *testing.T) {
	_, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkType("future-chunk")}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported UI message chunk type "future-chunk"`)
}

func TestAssembleUIMessage_Behavior(t *testing.T) {
	t.Run("matches last snapshot when no later non-writing mutations occur", func(t *testing.T) {
		input := []UIMessageChunk{TextStartChunk("t1"), TextDeltaChunk("t1", "hi"), TextEndChunk("t1")}
		progressive := collectMessages(StreamUIMessage(chunks(input...), WithUIMessageReaderGenerateID(func() string { return "id" })))
		require.NotEmpty(t, progressive)
		msg, err := AssembleUIMessage(chunks(input...), WithUIMessageReaderGenerateID(func() string { return "id" }))
		require.NoError(t, err)
		assert.Equal(t, progressive[len(progressive)-1], msg)
	})

	t.Run("empty stream returns generated assistant", func(t *testing.T) {
		calls := 0
		msg, err := AssembleUIMessage(chunks(), WithUIMessageReaderGenerateID(func() string {
			calls++
			return "generated"
		}))
		require.NoError(t, err)
		assert.Equal(t, "generated", msg.ID)
		assert.Equal(t, RoleAssistant, msg.Role)
		assert.Empty(t, msg.Parts)
		assert.Equal(t, 1, calls)
	})

	t.Run("includes later non-writing state mutations", func(t *testing.T) {
		input := []UIMessageChunk{TextStartChunk("t1"), TextDeltaChunk("t1", "hi"), UIMessageChunk{Type: ChunkStartStep}}
		progressive := collectMessages(StreamUIMessage(chunks(input...), WithUIMessageReaderGenerateID(func() string { return "id" })))
		require.Len(t, progressive, 2)
		msg, err := AssembleUIMessage(chunks(input...), WithUIMessageReaderGenerateID(func() string { return "id" }))
		require.NoError(t, err)
		require.Len(t, msg.Parts, 2)
		_, ok := msg.Parts[1].(StepStartPart)
		assert.True(t, ok)
	})

	t.Run("returns stream error with final state", func(t *testing.T) {
		msg, err := AssembleUIMessage(chunks(TextStartChunk("t1"), UIMessageChunk{Type: ChunkError, ErrorText: "boom"}, TextDeltaChunk("t1", "hi")))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
		assertTextPart(t, msg, "hi", "streaming")
	})

	t.Run("invalid order returns error", func(t *testing.T) {
		_, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "missing", Output: json.RawMessage(`{}`)}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "aisdk:")
	})
}

func TestUIMessageReaderLazyIDGeneration(t *testing.T) {
	t.Run("start chunk avoids fallback generator", func(t *testing.T) {
		calls := 0
		messages := collectMessages(StreamUIMessage(chunks(UIMessageChunk{Type: ChunkStart, MessageID: "from-stream"}), WithUIMessageReaderGenerateID(func() string {
			calls++
			return "fallback"
		})))
		require.Len(t, messages, 1)
		assert.Equal(t, "from-stream", messages[0].ID)
		assert.Zero(t, calls)
	})

	t.Run("fallback generator called once", func(t *testing.T) {
		calls := 0
		messages := collectMessages(StreamUIMessage(chunks(TextStartChunk("t1"), TextDeltaChunk("t1", "hi")), WithUIMessageReaderGenerateID(func() string {
			calls++
			return "fallback"
		})))
		require.Len(t, messages, 2)
		assert.Equal(t, "fallback", messages[0].ID)
		assert.Equal(t, "fallback", messages[1].ID)
		assert.Equal(t, 1, calls)
	})

	t.Run("assemble start chunk avoids fallback generator", func(t *testing.T) {
		calls := 0
		msg, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkStart, MessageID: "from-stream"}), WithUIMessageReaderGenerateID(func() string {
			calls++
			return "fallback"
		}))
		require.NoError(t, err)
		assert.Equal(t, "from-stream", msg.ID)
		assert.Zero(t, calls)
	})
}

func streamTextResultFromParts(parts ...TextStreamPart) *StreamTextResult {
	stream := make(chan TextStreamPart, len(parts))
	for _, part := range parts {
		stream <- part
	}
	close(stream)
	done := make(chan struct{})
	close(done)
	return &StreamTextResult{fullStream: stream, done: done}
}

func requireToolInvocationPart(t *testing.T, msg UIMessage, idx int) ToolInvocationPart {
	t.Helper()
	require.Greater(t, len(msg.Parts), idx)
	part, ok := msg.Parts[idx].(ToolInvocationPart)
	require.True(t, ok, "expected ToolInvocationPart at %d, got %T", idx, msg.Parts[idx])
	return part
}

func requireDynamicToolPart(t *testing.T, msg UIMessage, idx int) DynamicToolUIPart {
	t.Helper()
	require.Greater(t, len(msg.Parts), idx)
	part, ok := msg.Parts[idx].(DynamicToolUIPart)
	require.True(t, ok, "expected DynamicToolUIPart at %d, got %T", idx, msg.Parts[idx])
	return part
}
