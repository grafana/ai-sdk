package aisdk

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type uiValidationAgentSpy struct {
	Agent
	streamCalls int
}

func (a *uiValidationAgentSpy) Stream(ctx context.Context, opts ...AgentStreamOption) *StreamTextResult {
	a.streamCalls++
	return a.Agent.Stream(ctx, opts...)
}

var _ Agent = (*uiValidationAgentSpy)(nil)

func TestValidateAgentUIMessages_Structure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message UIMessage
	}{
		{"unknown role", UIMessage{Role: Role("other")}},
		{"empty user", UIMessage{Role: RoleUser}},
		{"unknown part", UIMessage{Role: RoleAssistant, Parts: []Part{rawPart{Type_: "future", Raw: json.RawMessage(`{"type":"future"}`)}}}},
		{"invalid JSON", UIMessage{Role: RoleAssistant, Metadata: json.RawMessage(`invalid`)}},
		{"invalid payload", UIMessage{Role: RoleAssistant, Parts: []Part{DataPart{DataName: "x", Data: json.RawMessage(`invalid`)}}}},
		{"invalid metadata namespace", UIMessage{Role: RoleAssistant, Parts: []Part{TextPart{Text: "x", ProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`null`)}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages, err := validateAgentUIMessages([]UIMessage{tc.message}, nil)
			assert.Nil(t, messages)
			assert.ErrorContains(t, err, "validating UI message 0")
		})
	}
	for _, tc := range []struct {
		name   string
		fields toolPartFields
	}{
		{"missing error", toolPartFields{State: ToolStateOutputError}},
		{"unknown state", toolPartFields{State: ToolInvocationState("future")}},
		{"forbidden output", toolPartFields{State: ToolStateInputAvailable, Input: json.RawMessage(`{}`), Output: json.RawMessage(`null`)}},
		{"forbidden error", toolPartFields{State: ToolStateOutputAvailable, Input: json.RawMessage(`{}`), Output: json.RawMessage(`null`), ErrorText: new("")}},
		{"forbidden input approval", toolPartFields{State: ToolStateInputAvailable, Input: json.RawMessage(`{}`), Approval: &ToolApproval{ID: "a"}}},
		{"requested decision fields", toolPartFields{State: ToolStateApprovalRequested, Input: json.RawMessage(`{}`), Approval: &ToolApproval{ID: "a", Reason: new("")}}},
		{"requested approved", toolPartFields{State: ToolStateApprovalRequested, Input: json.RawMessage(`{}`), Approval: &ToolApproval{ID: "a", Approved: new(false)}}},
		{"success denied approval", toolPartFields{State: ToolStateOutputAvailable, Input: json.RawMessage(`{}`), Output: json.RawMessage(`null`), Approval: &ToolApproval{ID: "a", Approved: new(false)}}},
		{"error pending approval", toolPartFields{State: ToolStateOutputError, ErrorText: new(""), Approval: &ToolApproval{ID: "a"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.fields.ToolCallID = "c"
			tc.fields.ToolName = "lookup"
			messages, err := validateAgentUIMessages([]UIMessage{{Role: RoleAssistant, Parts: []Part{DynamicToolUIPart(tc.fields)}}}, nil)
			assert.Nil(t, messages)
			assert.ErrorContains(t, err, "part 0")
		})
	}
}

func TestValidateAgentUIMessages_NormalizedClone(t *testing.T) {
	part := ToolInvocationPart{ToolCallID: "c", ToolName: "obsolete", State: ToolStateOutputError, Title: new(""), ToolMetadata: map[string]json.RawMessage{"x": json.RawMessage(`1`)}, RawInput: json.RawMessage(`"legacy"`), ErrorText: new(""), CallProviderMetadata: provider.ProviderMetadata{}, ResultProviderMetadata: provider.ProviderMetadata{"test": json.RawMessage(`{"x":1}`)}, Approval: &ToolApproval{ID: "a", Approved: new(true), Descriptor: json.RawMessage(`null`), RequestReason: new(""), Reason: new(""), Signature: "sig", IsAutomatic: true}}
	original := []UIMessage{{ID: "m", Role: RoleAssistant, Parts: []Part{part}, Metadata: json.RawMessage(`{"opaque":null}`)}}
	before, err := json.Marshal(original)
	require.NoError(t, err)
	normalized, err := validateAgentUIMessages(original, nil)
	require.NoError(t, err)
	got, ok := normalized[0].Parts[0].(DynamicToolUIPart)
	require.True(t, ok)
	assert.Equal(t, toolPartFields(part), toolPartFields(got))
	*got.Title = "changed"
	*got.ErrorText = "changed"
	got.RawInput[0] = 'x'
	got.ToolMetadata["x"][0] = 'x'
	got.ResultProviderMetadata["test"][0] = 'x'
	got.Approval.Descriptor[0] = 'x'
	*got.Approval.Approved = false
	*got.Approval.RequestReason = "changed"
	*got.Approval.Reason = "changed"
	normalized[0].Metadata[0] = 'x'
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
}

func TestValidateAgentUIMessages_SchemaBoundaries(t *testing.T) {
	calls := 0
	tools := ToolSet{"lookup": {Type: UserToolProvider, ValidateInput: func(json.RawMessage) error { calls++; return nil }}}
	messages, err := validateAgentUIMessages([]UIMessage{{Role: RoleAssistant, Parts: []Part{ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: ToolStateInputAvailable, Input: json.RawMessage(`null`)}, DataPart{DataName: "opaque", Data: json.RawMessage(`null`)}}}}, tools)
	require.NoError(t, err)
	require.Len(t, messages[0].Parts, 2)
	assert.Zero(t, calls)
}

func TestCreateAgentUIStream_EmptyHistory(t *testing.T) {
	for _, tc := range []struct {
		name     string
		messages []UIMessage
	}{{"nil", nil}, {"empty", []UIMessage{}}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				stream := make(chan provider.StreamPart, 1)
				stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}}
			spy := &uiValidationAgentSpy{Agent: NewToolLoopAgent(model)}
			stream, err := CreateAgentUIStream(t.Context(), spy, tc.messages)
			if stream != nil {
				for range stream {
				}
			}
			assert.Error(t, err)
			assert.Nil(t, stream)
			assert.Zero(t, spy.streamCalls)
			assert.Zero(t, calls)
		})
	}
}

func TestCreateAgentUIStream_DynamicRawInputContinuation(t *testing.T) {
	initial := UIMessage{ID: "m", Role: RoleAssistant, Parts: []Part{DynamicToolUIPart{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputError, Input: json.RawMessage(`{"q":"old"}`), RawInput: json.RawMessage(`"legacy"`), ErrorText: new(""), Title: new(""), ToolMetadata: map[string]json.RawMessage{}}}}
	assembled, err := AssembleUIMessage(chunks(UIMessageChunk{Type: ChunkToolOutputAvailable, ToolCallID: "c", Output: json.RawMessage(`"final"`)}), WithUIMessageReaderInitialMessage(initial))
	require.NoError(t, err)
	encoded, err := json.Marshal(assembled)
	require.NoError(t, err)
	var persisted UIMessage
	require.NoError(t, json.Unmarshal(encoded, &persisted))
	normalized, err := validateAgentUIMessages([]UIMessage{persisted}, nil)
	require.NoError(t, err)
	part := normalized[0].Parts[0].(DynamicToolUIPart)
	assert.Nil(t, part.RawInput)
	assert.JSONEq(t, `"legacy"`, string(persisted.Parts[0].(DynamicToolUIPart).RawInput))
	*part.Title = "changed"
	assert.Equal(t, "", *persisted.Parts[0].(DynamicToolUIPart).Title)
	calls := 0
	model := &mockModel{streamFunc: func(_ context.Context, options provider.CallOptions) (*provider.StreamResult, error) {
		calls++
		assert.JSONEq(t, `{"q":"old"}`, string(options.Prompt[0].Content[0].Input))
		stream := make(chan provider.StreamPart, 1)
		stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
		close(stream)
		return &provider.StreamResult{Stream: stream}, nil
	}}
	spy := &uiValidationAgentSpy{Agent: NewToolLoopAgent(model)}
	var finished UIMessageStreamOnFinishState
	stream, err := CreateAgentUIStream(t.Context(), spy, []UIMessage{persisted}, OnUIMessageStreamFinish(func(state UIMessageStreamOnFinishState) { finished = state }))
	require.NoError(t, err)
	for range stream {
	}
	assert.Equal(t, 1, spy.streamCalls)
	assert.Equal(t, 1, calls)
	assert.Nil(t, finished.ResponseMessage.Parts[0].(DynamicToolUIPart).RawInput)
	after, err := json.Marshal(persisted)
	require.NoError(t, err)
	assert.JSONEq(t, string(encoded), string(after))
}

func TestCreateAgentUIStream_PersistedValidation(t *testing.T) {
	inputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`))
	require.NoError(t, err)
	outputSchema, err := schema.SchemaFromJSON(json.RawMessage(`{"type":"string"}`))
	require.NoError(t, err)
	for _, tc := range []struct {
		name, part string
		valid      bool
	}{
		{"valid input", `"state":"input-available","input":{"q":"x"}`, true},
		{"invalid input", `"state":"input-available","input":{"q":1}`, false},
		{"partial exempt", `"state":"input-streaming","input":{"q":1}`, true},
		{"dynamic exempt", `"state":"input-available","input":{"q":1},"type":"dynamic-tool"`, true},
		{"empty terminal normalized", `"state":"output-available","input":{},"output":"ok"`, true},
		{"nonempty terminal invalid", `"state":"output-available","input":{"q":1},"output":"ok"`, false},
		{"output checked before normalization", `"state":"output-available","input":{},"output":1`, false},
		{"failed input normalized", `"state":"output-error","input":{"q":1},"errorText":""`, true},
		{"legacy error", `"state":"output-error","rawInput":"legacy","errorText":""`, true},
		{"contradictory output", `"state":"input-available","input":{"q":"x"},"output":"bad"`, false},
		{"missing error", `"state":"output-error"`, false},
		{"denied missing approval", `"state":"output-denied","input":{"q":"x"}`, false},
		{"denied wrong approval", `"state":"output-denied","input":{"q":"x"},"approval":{"id":"a","approved":true}`, false},
		{"valid denied", `"state":"output-denied","input":{"q":"x"},"approval":{"id":"a","approved":false,"reason":""}`, true},
		{"unknown terminal normalized", `"state":"output-available","input":{},"output":1,"toolName":"missing","type":"tool-missing"`, true},
		{"unknown nonterminal rejected", `"state":"input-available","input":{},"toolName":"missing","type":"tool-missing"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var message UIMessage
			require.NoError(t, json.Unmarshal([]byte(`{"id":"m","role":"assistant","parts":[{"type":"tool-lookup","toolName":"lookup","toolCallId":"c",`+tc.part+`}]}`), &message))
			before, err := json.Marshal(message)
			require.NoError(t, err)
			calls := 0
			model := &mockModel{streamFunc: func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				calls++
				stream := make(chan provider.StreamPart, 1)
				stream <- provider.StreamPart{Type: provider.PartFinish, FinishReason: &provider.FinishReason{Unified: provider.FinishReasonStop}}
				close(stream)
				return &provider.StreamResult{Stream: stream}, nil
			}}
			agent := NewToolLoopAgent(model, WithToolLoopAgentOptions(WithTools(ToolSet{"lookup": {InputSchema: inputSchema, OutputSchema: outputSchema, Execute: func(context.Context, json.RawMessage, ToolExecutionOptions) (json.RawMessage, error) {
				return json.RawMessage(`"ok"`), nil
			}}})))
			spy := &uiValidationAgentSpy{Agent: agent}
			stream, err := CreateAgentUIStream(t.Context(), spy, []UIMessage{message})
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, 1, spy.streamCalls)
				for range stream {
				}
				if tc.name != "valid input" && tc.name != "dynamic exempt" {
					assert.Equal(t, 1, calls)
				}
			} else {
				require.Error(t, err)
				assert.Nil(t, stream)
				assert.Equal(t, 0, calls)
				assert.Zero(t, spy.streamCalls)
			}
			after, err := json.Marshal(message)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}
