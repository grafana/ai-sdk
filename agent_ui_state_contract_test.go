package aisdk

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
