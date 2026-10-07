package aisdk

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToModelMessages_DataPartConverter(t *testing.T) {
	for _, role := range []Role{RoleUser, RoleAssistant, RoleSystem} {
		t.Run(string(role), func(t *testing.T) {
			calls := 0
			messages, err := ConvertToModelMessages([]UIMessage{{Role: role, Parts: []Part{TextPart{Text: "before"}, DataPart{DataName: "skip", Data: json.RawMessage(`1`)}, DataPart{DataName: "text", Data: json.RawMessage(`2`)}, DataPart{DataName: "file", Data: json.RawMessage(`3`)}, TextPart{Text: "after"}}}}, WithConvertDataPart(func(part DataPart) (*provider.ContentPart, error) {
				calls++
				switch part.DataName {
				case "text":
					return &provider.ContentPart{Type: provider.ContentPartTypeText, Text: "converted"}, nil
				case "file":
					return &provider.ContentPart{Type: provider.ContentPartTypeFile, MediaType: "text/plain", URL: "https://example.com/data"}, nil
				default:
					return nil, nil
				}
			}))
			require.NoError(t, err)
			require.Len(t, messages, 1)
			if role == RoleSystem {
				assert.Zero(t, calls)
				assert.Equal(t, "beforeafter", messages[0].Content[0].Text)
			} else {
				assert.Equal(t, 3, calls)
				require.Len(t, messages[0].Content, 4)
				assert.Equal(t, "before", messages[0].Content[0].Text)
				assert.Equal(t, "converted", messages[0].Content[1].Text)
				assert.Equal(t, provider.ContentPartTypeFile, messages[0].Content[2].Type)
				assert.Equal(t, "after", messages[0].Content[3].Text)
			}
		})
	}
	t.Run("empty text retains required field", func(t *testing.T) {
		messages, err := ConvertToModelMessages([]UIMessage{{Role: RoleUser, Parts: []Part{DataPart{DataName: "text", Data: json.RawMessage(`null`)}}}}, WithConvertDataPart(func(DataPart) (*provider.ContentPart, error) {
			return &provider.ContentPart{Type: provider.ContentPartTypeText, Text: ""}, nil
		}))
		require.NoError(t, err)
		encoded, err := json.Marshal(messages)
		require.NoError(t, err)
		assert.JSONEq(t, `[{"role":"user","content":[{"type":"text","text":""}]}]`, string(encoded))
	})
	t.Run("errors", func(t *testing.T) {
		sentinel := errors.New("converter failed")
		message := []UIMessage{{Role: RoleAssistant, Parts: []Part{DataPart{DataName: "test", Data: json.RawMessage(`1`)}}}}
		converted, err := ConvertToModelMessages(message, WithConvertDataPart(func(DataPart) (*provider.ContentPart, error) { return nil, sentinel }))
		assert.Nil(t, converted)
		assert.ErrorIs(t, err, sentinel)
		converted, err = ConvertToModelMessages(message, WithConvertDataPart(func(DataPart) (*provider.ContentPart, error) {
			return &provider.ContentPart{Type: provider.ContentPartTypeToolResult}, nil
		}))
		assert.Nil(t, converted)
		assert.ErrorContains(t, err, "unsupported content type")
	})
}

func TestConvertToModelMessages_CallbackOrder(t *testing.T) {
	dataError, outputError := errors.New("data failed"), errors.New("output failed")
	for _, dynamic := range []bool{false, true} {
		for _, tc := range []struct {
			name                                     string
			executed, separate, failData, failOutput bool
			trace                                    []string
			text                                     string
			err                                      error
		}{
			{name: "local", trace: []string{"data", "output"}, text: "0"},
			{name: "inline", executed: true, trace: []string{"output", "data"}, text: "1"},
			{name: "separate steps", separate: true, trace: []string{"output", "data"}, text: "1"},
			{name: "data failure", failData: true, trace: []string{"data"}, err: dataError},
			{name: "both fail", failData: true, failOutput: true, trace: []string{"data"}, err: dataError},
			{name: "output failure", failOutput: true, trace: []string{"data", "output"}, err: outputError},
			{name: "inline data failure", executed: true, failData: true, trace: []string{"output", "data"}, err: dataError},
			{name: "inline both fail", executed: true, failData: true, failOutput: true, trace: []string{"output"}, err: outputError},
		} {
			t.Run(fmt.Sprintf("dynamic-%t/%s", dynamic, tc.name), func(t *testing.T) {
				fields := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: ToolStateOutputAvailable, Input: json.RawMessage(`{}`), Output: json.RawMessage(`"ok"`), ProviderExecuted: tc.executed}
				var part Part = ToolInvocationPart(fields)
				if dynamic {
					part = DynamicToolUIPart(fields)
				}
				parts := []Part{part}
				if tc.separate {
					parts = append(parts, StepStartPart{})
				}
				parts = append(parts, DataPart{DataName: "count", Data: json.RawMessage(`null`)})
				calls := 0
				var trace []string
				got, err := ConvertToModelMessages([]UIMessage{{Role: RoleAssistant, Parts: parts}}, WithTools(ToolSet{"lookup": {ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
					trace = append(trace, "output")
					calls++
					if tc.failOutput {
						return nil, outputError
					}
					return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "mapped"}, nil
				}}}), WithConvertDataPart(func(DataPart) (*provider.ContentPart, error) {
					trace = append(trace, "data")
					if tc.failData {
						return nil, dataError
					}
					return &provider.ContentPart{Type: provider.ContentPartTypeText, Text: strconv.Itoa(calls)}, nil
				}))
				assert.Equal(t, tc.trace, trace)
				if tc.err != nil {
					assert.ErrorIs(t, err, tc.err)
					assert.Nil(t, got)
				} else {
					require.NoError(t, err)
					var texts []string
					for _, message := range got {
						for _, content := range message.Content {
							if content.Type == provider.ContentPartTypeText {
								texts = append(texts, content.Text)
							}
						}
					}
					assert.Equal(t, []string{tc.text}, texts)
				}
			})
		}
	}
}

func TestConvertToModelMessages_ToolFilteringCallbacks(t *testing.T) {
	states := []ToolInvocationState{ToolStateInputStreaming, ToolStateInputAvailable, ToolStateApprovalRequested, ToolStateApprovalResponded, ToolStateOutputAvailable, ToolStateOutputError, ToolStateOutputDenied}
	for _, dynamic := range []bool{false, true} {
		for _, state := range states {
			for preliminaryIndex, preliminary := range []*bool{nil, new(false), new(true)} {
				for _, ignore := range []bool{false, true} {
					t.Run(fmt.Sprintf("dynamic-%t/%s/preliminary-%d/ignore-%t", dynamic, state, preliminaryIndex, ignore), func(t *testing.T) {
						fields := toolPartFields{ToolCallID: "c", ToolName: "lookup", State: state, Input: json.RawMessage(`{}`)}
						if state == ToolStateOutputAvailable {
							fields.Output = json.RawMessage(`"ok"`)
							fields.Preliminary = preliminary
						}
						if state == ToolStateOutputError {
							fields.ErrorText = new("")
						}
						if state == ToolStateApprovalRequested {
							fields.Approval = &ToolApproval{ID: "a"}
						}
						if state == ToolStateApprovalResponded || state == ToolStateOutputDenied {
							fields.Approval = &ToolApproval{ID: "a", Approved: new(false)}
						}
						var part Part = ToolInvocationPart(fields)
						if dynamic {
							part = DynamicToolUIPart(fields)
						}
						calls := 0
						options := []ConvertOption{WithTools(ToolSet{"lookup": {ToModelOutput: func(ToolOutputContext) (*provider.ToolResultOutput, error) {
							calls++
							return &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "mapped"}, nil
						}}})}
						if ignore {
							options = append(options, WithIgnoreIncompleteToolCalls())
						}
						messages, err := ConvertToModelMessages([]UIMessage{{Role: RoleAssistant, Parts: []Part{part}}}, options...)
						require.NoError(t, err)
						filtered := state == ToolStateInputStreaming || (ignore && (state == ToolStateInputAvailable || state == ToolStateApprovalRequested || (state == ToolStateOutputAvailable && preliminary != nil && *preliminary)))
						if filtered {
							assert.Empty(t, messages)
						} else {
							assert.NotEmpty(t, messages)
						}
						wantCalls := 0
						if !filtered && state == ToolStateOutputAvailable {
							wantCalls = 1
						}
						assert.Equal(t, wantCalls, calls)
					})
				}
			}
		}
	}
}

func TestConvertToModelMessages_MetadataPresence(t *testing.T) {
	metadata := []provider.ProviderMetadata{nil, {}, {"test": json.RawMessage(`{"source":"present"}`)}}
	for _, state := range []ToolInvocationState{ToolStateOutputAvailable, ToolStateOutputError} {
		for _, executed := range []bool{false, true} {
			for callIndex, call := range metadata {
				for resultIndex, result := range metadata {
					t.Run(fmt.Sprintf("%s/provider-%t/call-%d/result-%d", state, executed, callIndex, resultIndex), func(t *testing.T) {
						part := ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: state, Input: json.RawMessage(`{}`), ProviderExecuted: executed, CallProviderMetadata: call, ResultProviderMetadata: result}
						if state == ToolStateOutputAvailable {
							part.Output = json.RawMessage(`"ok"`)
						} else {
							part.ErrorText = new("")
						}
						messages, err := ConvertToModelMessages([]UIMessage{{Role: RoleAssistant, Parts: []Part{part}}})
						require.NoError(t, err)
						callSource := call
						if state == ToolStateOutputError && callSource == nil {
							callSource = result
						}
						assert.Equal(t, providerMetadataToOptions(callSource), messages[0].Content[0].ProviderOptions)
						if executed {
							resultSource := result
							if resultSource == nil {
								resultSource = call
							}
							assert.Equal(t, providerMetadataToOptions(resultSource), messages[0].Content[1].ProviderOptions)
						} else {
							assert.Equal(t, providerMetadataToOptions(call), messages[1].Content[0].ProviderOptions)
						}
					})
				}
			}
		}
	}
}

func TestConvertToModelMessages_MetadataPlacement(t *testing.T) {
	call := provider.ProviderMetadata{"test": json.RawMessage(`{"source":"call"}`)}
	result := provider.ProviderMetadata{"test": json.RawMessage(`{"source":"result"}`)}
	for _, state := range []ToolInvocationState{ToolStateOutputAvailable, ToolStateOutputError} {
		for _, executed := range []bool{false, true} {
			for _, hasCall := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/provider-%t/call-%t", state, executed, hasCall), func(t *testing.T) {
					fields := ToolInvocationPart{ToolCallID: "c", ToolName: "lookup", State: state, Input: json.RawMessage(`{}`), Output: json.RawMessage(`"ok"`), ErrorText: new("failed"), ProviderExecuted: executed, ResultProviderMetadata: result, Approval: &ToolApproval{ID: "a", Approved: new(true), RequestReason: new("request")}}
					if state == ToolStateOutputAvailable {
						fields.ErrorText = nil
					} else {
						fields.Output = nil
					}
					if hasCall {
						fields.CallProviderMetadata = call
					}
					messages, err := ConvertToModelMessages([]UIMessage{{Role: RoleAssistant, Parts: []Part{fields}}})
					require.NoError(t, err)
					wantCall := providerMetadataToOptions(fields.CallProviderMetadata)
					if state == ToolStateOutputError && !hasCall {
						wantCall = providerMetadataToOptions(result)
					}
					assert.Equal(t, wantCall, messages[0].Content[0].ProviderOptions)
					assert.Nil(t, messages[0].Content[1].ProviderOptions)
					assert.Equal(t, "request", messages[0].Content[1].Reason)
					if executed {
						assert.Equal(t, providerMetadataToOptions(result), messages[0].Content[2].ProviderOptions)
					} else {
						assert.Equal(t, providerMetadataToOptions(fields.CallProviderMetadata), messages[1].Content[1].ProviderOptions)
					}
				})
			}
		}
	}
}
