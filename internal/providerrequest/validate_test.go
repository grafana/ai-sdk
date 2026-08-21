package providerrequest

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func TestValidate_RequestArms(t *testing.T) {
	data := provider.TextDataContent("value")
	empty := ""
	valid := provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("system"),
			provider.NewUserMessage(provider.FilePartWithFilename("text/plain", data, "")),
			provider.NewAssistantMessage(provider.ToolCallPart("call-1", "lookup", json.RawMessage(`{}`))),
			provider.NewToolMessage(provider.ToolResultPart("call-1", "lookup", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"})),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeFunction, Name: "lookup", Description: &empty, InputSchema: json.RawMessage(`{}`)},
			{Type: provider.ToolTypeProvider, Name: "search", ID: "provider.search", ProviderOptions: provider.ProviderOptions{
				"provider": provider.RawProviderOption{Key: "provider", Raw: json.RawMessage(`{"enabled":true}`)},
			}},
		},
	}
	require.NoError(t, Validate(valid))

	invalid := []struct {
		name    string
		options provider.CallOptions
	}{
		{name: "system message with multiple parts", options: provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleSystem, Content: []provider.ContentPart{provider.TextPart("one"), provider.TextPart("two")}}}}},
		{name: "unsupported role", options: provider.CallOptions{Prompt: []provider.Message{{Role: provider.Role("unsupported")}}}},
		{name: "zero role", options: provider.CallOptions{Prompt: []provider.Message{{}}}},
		{name: "response filename on request file", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeFile, Data: &data, MediaType: "text/plain", Filename: "response.txt"})}}},
		{name: "inactive text field", options: provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(provider.ContentPart{Type: provider.ContentPartTypeText, Text: "value", ToolName: "inactive"})}}},
		{name: "inactive function tool field", options: provider.CallOptions{Tools: []provider.Tool{{Type: provider.ToolTypeFunction, Name: "lookup", ID: "inactive"}}}},
		{name: "inactive automatic tool name", options: provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceAuto, ToolName: "inactive"}}},
		{name: "unsupported tool choice", options: provider.CallOptions{ToolChoice: &provider.ToolChoice{Type: provider.ToolChoiceType("unsupported")}}},
		{name: "inactive text response format field", options: provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: provider.ResponseFormatText, Name: &empty}}},
		{name: "inactive tool result output field", options: provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{
			Type: provider.ToolOutputText, Text: "value", Reason: &empty,
		}))}}},
		{name: "inactive tool result content field", options: provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call", "tool", &provider.ToolResultOutput{
			Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{{
				Type: provider.ToolContentText, Text: "value", MediaType: "inactive",
			}},
		}))}}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, Validate(tc.options))
		})
	}
}

func TestValidate_DomainStructFieldExhaustiveness(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{
			name:  "call options",
			value: provider.CallOptions{},
			want: []string{
				"Prompt", "Tools", "ToolChoice", "MaxOutputTokens", "Temperature", "TopP", "TopK", "PresencePenalty", "FrequencyPenalty",
				"StopSequences", "ResponseFormat", "Seed", "Reasoning", "IncludeRawChunks", "Headers", "ProviderOptions",
			},
		},
		{
			name:  "content part",
			value: provider.ContentPart{},
			want: []string{
				"Type", "Text", "Data", "FilePartFilename", "Filename", "MediaType", "Kind", "SourceType", "ID", "URL", "Title",
				"ToolCallID", "ToolName", "Input", "Output", "ProviderExecuted", "ApprovalID", "Signature", "IsAutomatic", "Approved", "Reason", "ProviderOptions",
			},
		},
		{
			name:  "message",
			value: provider.Message{},
			want:  []string{"Role", "Content", "ProviderOptions"},
		},
		{
			name:  "tool",
			value: provider.Tool{},
			want:  []string{"Type", "Name", "Description", "InputSchema", "InputExamples", "Strict", "ID", "Args", "ProviderOptions"},
		},
		{
			name:  "tool choice",
			value: provider.ToolChoice{},
			want:  []string{"Type", "ToolName"},
		},
		{
			name:  "response format",
			value: provider.ResponseFormat{},
			want:  []string{"Type", "Schema", "Name", "Description"},
		},
		{
			name:  "tool result output",
			value: provider.ToolResultOutput{},
			want:  []string{"Type", "Text", "JSON", "Content", "Reason", "ProviderOptions"},
		},
		{
			name:  "tool result content",
			value: provider.ToolResultContentValue{},
			want:  []string{"Type", "Text", "Data", "MediaType", "Filename", "ProviderOptions"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			typeOf := reflect.TypeOf(tc.value)
			fields := make([]string, typeOf.NumField())
			for index := range typeOf.NumField() {
				fields[index] = typeOf.Field(index).Name
			}
			require.ElementsMatch(t, tc.want, fields, "update providerrequest validation for the changed domain struct")
		})
	}
}

func TestValidate_InvalidNumbers(t *testing.T) {
	invalid := provider.LanguageModelNumber{}
	tests := []struct {
		name    string
		options provider.CallOptions
	}{
		{name: "max output tokens", options: provider.CallOptions{MaxOutputTokens: &invalid}},
		{name: "top k", options: provider.CallOptions{TopK: &invalid}},
		{name: "seed", options: provider.CallOptions{Seed: &invalid}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, Validate(tc.options))
		})
	}
}
