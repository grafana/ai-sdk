package grafana

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/require"
)

func checkRequestContractArms[T ~string](t *testing.T, arms map[T]bool, request func(T) provider.CallOptions) {
	t.Helper()
	for arm, rejected := range arms {
		t.Run(string(arm), func(t *testing.T) {
			body, err := encodeRequest(request(arm))
			if rejected {
				require.ErrorIs(t, err, errRequest)
				require.Nil(t, body)
				return
			}
			require.NoError(t, err)
			require.True(t, json.Valid(body))
		})
	}
}

func TestRequestContract_FiniteDiscriminators(t *testing.T) {
	t.Run("Role", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.Role]bool{
			provider.RoleSystem: false, provider.RoleUser: false, provider.RoleAssistant: false, provider.RoleTool: false,
		}, func(role provider.Role) provider.CallOptions {
			part := provider.TextPart("")
			if role == provider.RoleTool {
				part = provider.ToolResultPart("id", "tool", &provider.ToolResultOutput{Type: provider.ToolOutputText})
			}
			return provider.CallOptions{Prompt: []provider.Message{{Role: role, Content: []provider.ContentPart{part}}}}
		})
	})
	t.Run("ContentPartType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ContentPartType]bool{
			provider.ContentPartTypeText: false, provider.ContentPartTypeReasoning: false,
			provider.ContentPartTypeFile: false, provider.ContentPartTypeReasoningFile: false,
			provider.ContentPartTypeCustom: false, provider.ContentPartTypeToolCall: false,
			provider.ContentPartTypeToolResult: false, provider.ContentPartTypeToolApprovalResponse: false,
			provider.ContentPartTypeSource: true, provider.ContentPartTypeToolApprovalRequest: true,
		}, func(arm provider.ContentPartType) provider.CallOptions {
			part := provider.ContentPart{Type: arm}
			role := provider.RoleAssistant
			switch arm {
			case provider.ContentPartTypeFile, provider.ContentPartTypeReasoningFile:
				part.Data = &provider.DataContent{Bytes: []byte{}}
				part.MediaType = "text/plain"
			case provider.ContentPartTypeCustom:
				part.Kind = "test.custom"
			case provider.ContentPartTypeToolCall:
				part.ToolCallID, part.ToolName, part.Input = "id", "tool", json.RawMessage(`{}`)
			case provider.ContentPartTypeToolResult:
				part.ToolCallID, part.ToolName = "id", "tool"
				part.Output = &provider.ToolResultOutput{Type: provider.ToolOutputText}
			case provider.ContentPartTypeToolApprovalResponse:
				role = provider.RoleTool
				approved := false
				part.ApprovalID, part.Approved = "approval", &approved
			}
			return provider.CallOptions{Prompt: []provider.Message{{Role: role, Content: []provider.ContentPart{part}}}}
		})
	})
	t.Run("ReasoningEffort", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ReasoningEffort]bool{
			provider.ReasoningProviderDefault: false, provider.ReasoningNone: false, provider.ReasoningMinimal: false,
			provider.ReasoningLow: false, provider.ReasoningMedium: false, provider.ReasoningHigh: false, provider.ReasoningXHigh: false,
		}, func(arm provider.ReasoningEffort) provider.CallOptions { return provider.CallOptions{Reasoning: arm} })
		body, err := encodeRequest(provider.CallOptions{Reasoning: provider.ReasoningProviderDefault})
		require.NoError(t, err)
		require.JSONEq(t, `{}`, string(body))
	})
	t.Run("ResponseFormatType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ResponseFormatType]bool{
			provider.ResponseFormatText: false, provider.ResponseFormatJSON: false,
		}, func(arm provider.ResponseFormatType) provider.CallOptions {
			return provider.CallOptions{ResponseFormat: &provider.ResponseFormat{Type: arm}}
		})
	})
	t.Run("ToolChoiceType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ToolChoiceType]bool{
			provider.ToolChoiceAuto: false, provider.ToolChoiceNone: false, provider.ToolChoiceRequired: false, provider.ToolChoiceTool: false,
		}, func(arm provider.ToolChoiceType) provider.CallOptions {
			choice := &provider.ToolChoice{Type: arm}
			if arm == provider.ToolChoiceTool {
				choice.ToolName = "tool"
			}
			return provider.CallOptions{ToolChoice: choice}
		})
	})
	t.Run("ToolType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ToolType]bool{
			provider.ToolTypeFunction: false, provider.ToolTypeProvider: false,
		}, func(arm provider.ToolType) provider.CallOptions {
			tool := provider.Tool{Type: arm, Name: "tool"}
			if arm == provider.ToolTypeFunction {
				tool.InputSchema = json.RawMessage(`{}`)
			} else {
				tool.ID, tool.Args = "test.tool", map[string]json.RawMessage{}
			}
			return provider.CallOptions{Tools: []provider.Tool{tool}}
		})
	})
	t.Run("ToolResultOutputType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ToolResultOutputType]bool{
			provider.ToolOutputText: false, provider.ToolOutputJSON: false, provider.ToolOutputContent: false,
			provider.ToolOutputExecutionDenied: false, provider.ToolOutputErrorText: false, provider.ToolOutputErrorJSON: false,
		}, func(arm provider.ToolResultOutputType) provider.CallOptions {
			output := &provider.ToolResultOutput{Type: arm}
			if arm == provider.ToolOutputJSON || arm == provider.ToolOutputErrorJSON {
				output.JSON = json.RawMessage(`null`)
			}
			if arm == provider.ToolOutputContent {
				output.Content = []provider.ToolResultContentValue{}
			}
			return provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("id", "tool", output))}}
		})
	})
	t.Run("ToolResultContentType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.ToolResultContentType]bool{
			provider.ToolContentText: false, provider.ToolContentFile: false, provider.ToolContentCustom: false,
			provider.ToolContentFileData: true, provider.ToolContentFileURL: true, provider.ToolContentFileReference: true,
		}, func(arm provider.ToolResultContentType) provider.CallOptions {
			content := provider.ToolResultContentValue{Type: arm}
			if arm == provider.ToolContentFile {
				content.Data, content.MediaType = &provider.DataContent{Bytes: []byte{}}, "text/plain"
			}
			return provider.CallOptions{Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("id", "tool", &provider.ToolResultOutput{
				Type: provider.ToolOutputContent, Content: []provider.ToolResultContentValue{content},
			}))}}
		})
	})
	t.Run("SourceType", func(t *testing.T) {
		checkRequestContractArms(t, map[provider.SourceType]bool{
			provider.SourceTypeURL: true, provider.SourceTypeDocument: true,
		}, func(arm provider.SourceType) provider.CallOptions {
			return provider.CallOptions{Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{Type: provider.ContentPartTypeSource, SourceType: arm})}}
		})
	})
}
