package openai

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildBody is a test helper that runs buildParams and returns the marshaled
// request body as a generic map plus the warnings.
func buildBody(t *testing.T, modelID string, opts provider.CallOptions) (map[string]any, []provider.Warning) {
	t.Helper()
	body, warnings, _, err := buildParams(modelID, opts)
	require.NoError(t, err)
	b, err := json.Marshal(body)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m, warnings
}

func withOpenAIOptions(o OpenAIResponsesOptions) provider.ProviderOptions {
	return provider.BuildProviderOptions(o)
}

func withAzureOptions(t *testing.T, value any) provider.ProviderOptions {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return provider.ProviderOptions{"azure": provider.RawProviderOption{Key: "azure", Raw: raw}}
}

func TestBuildParams_SystemMessageModes(t *testing.T) {
	prompt := []provider.Message{
		provider.NewSystemMessage("be helpful"),
		provider.UserText("hi"),
	}

	t.Run("system mode (non-reasoning model)", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{Prompt: prompt})
		input := body["input"].([]any)
		sys := input[0].(map[string]any)
		assert.Equal(t, "system", sys["role"])
	})

	t.Run("developer mode (reasoning model)", func(t *testing.T) {
		body, _ := buildBody(t, "o3", provider.CallOptions{Prompt: prompt})
		input := body["input"].([]any)
		sys := input[0].(map[string]any)
		assert.Equal(t, "developer", sys["role"])
	})

	t.Run("remove mode drops system message + warning", func(t *testing.T) {
		body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt:          prompt,
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{SystemMessageMode: "remove"}),
		})
		input := body["input"].([]any)
		require.Len(t, input, 1)
		assert.Equal(t, "user", input[0].(map[string]any)["role"])
		assert.Contains(t, warningFeatures(warnings), "system")
	})
}

func TestBuildParams_PromptCacheBreakpoint(t *testing.T) {
	breakpoint := &PromptCacheBreakpoint{Mode: "explicit"}

	t.Run("system message", func(t *testing.T) {
		system := provider.NewSystemMessage("be helpful")
		system.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{PromptCacheBreakpoint: breakpoint})
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{Prompt: []provider.Message{system}})

		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		text := content[0].(map[string]any)
		assert.Equal(t, "input_text", text["type"])
		assert.Equal(t, "be helpful", text["text"])
		assert.Equal(t, map[string]any{"mode": "explicit"}, text["prompt_cache_breakpoint"])
	})

	t.Run("user text and file parts", func(t *testing.T) {
		textPart := provider.TextPart("look")
		textPart.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{PromptCacheBreakpoint: breakpoint})
		filePart := provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/x.png"})
		filePart.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{PromptCacheBreakpoint: breakpoint})

		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{Prompt: []provider.Message{provider.NewUserMessage(textPart, filePart)}})

		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		assert.Equal(t, map[string]any{"mode": "explicit"}, content[0].(map[string]any)["prompt_cache_breakpoint"])
		assert.Equal(t, map[string]any{"mode": "explicit"}, content[1].(map[string]any)["prompt_cache_breakpoint"])
	})
}

func TestBuildParams_UserImageAndFile(t *testing.T) {
	t.Run("text and image url", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.TextPart("look"),
					provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/x.png"}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		require.Len(t, content, 2)
		assert.Equal(t, "input_text", content[0].(map[string]any)["type"])
		img := content[1].(map[string]any)
		assert.Equal(t, "input_image", img["type"])
		assert.Equal(t, "https://example.com/x.png", img["image_url"])
	})

	t.Run("image provider reference uses file ID", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("image/png", provider.DataContent{Reference: json.RawMessage(`{"openai":"file-image-123"}`)}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		assert.Equal(t, "file-image-123", content[0].(map[string]any)["file_id"])
	})

	t.Run("PDF provider reference uses file ID", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("application/pdf", provider.DataContent{Reference: json.RawMessage(`{"openai":"file-pdf-123"}`)}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		assert.Equal(t, "file-pdf-123", content[0].(map[string]any)["file_id"])
	})

	t.Run("provider reference requires matching provider", func(t *testing.T) {
		_, _, _, err := buildParams("gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("application/pdf", provider.DataContent{Reference: json.RawMessage(`{"anthropic":"file-pdf-123"}`)}),
				),
			},
		})
		assert.EqualError(t, err, `openai: file reference has no "openai" provider entry`)
	})

	t.Run("empty provider reference does not fall back to URL", func(t *testing.T) {
		part := provider.FilePart("application/pdf", provider.DataContent{Reference: json.RawMessage(`{}`)})
		_, _, _, err := buildParams("gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{provider.NewUserMessage(part)},
		})
		assert.EqualError(t, err, `openai: file reference has no "openai" provider entry`)
	})

	t.Run("image file inline uses data URL", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("image/png", provider.DataContent{Base64: "AAECAw=="}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		img := content[0].(map[string]any)
		assert.Equal(t, "input_image", img["type"])
		assert.Equal(t, "data:image/png;base64,AAECAw==", img["image_url"])
	})

	t.Run("image file bytes inline uses data URL", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("image/jpeg", provider.DataContent{Bytes: []byte{0xff, 0xd8, 0xff, 0xe0}}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		img := content[0].(map[string]any)
		assert.Equal(t, "input_image", img["type"])
		assert.Equal(t, "data:image/jpeg;base64,/9j/4A==", img["image_url"])
	})

	t.Run("wildcard image media type detects inline bytes", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("image/*", provider.DataContent{Bytes: []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		img := content[0].(map[string]any)
		assert.Equal(t, "input_image", img["type"])
		assert.Equal(t, "data:image/png;base64,iVBORw0KGgo=", img["image_url"])
	})

	t.Run("top-level image media type URL is input image", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("image", provider.DataContent{URL: "https://example.com/x.png"}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		img := content[0].(map[string]any)
		assert.Equal(t, "input_image", img["type"])
		assert.Equal(t, "https://example.com/x.png", img["image_url"])
	})

	t.Run("wildcard image media type errors when inline bytes cannot be detected", func(t *testing.T) {
		_, _, _, err := buildParams("gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("image/*", provider.DataContent{Bytes: []byte{0x00, 0x01, 0x02, 0x03}}),
				),
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `media type "image/*"`)
	})

	t.Run("pdf file inline", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("application/pdf", provider.DataContent{Base64: "ZGF0YQ=="}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		f := content[0].(map[string]any)
		assert.Equal(t, "input_file", f["type"])
		assert.Contains(t, f["file_data"], "data:application/pdf;base64,")
		assert.Equal(t, "part-0.pdf", f["filename"])
	})

	t.Run("wildcard application media type detects pdf bytes", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("application/*", provider.DataContent{Bytes: []byte{0x25, 0x50, 0x44, 0x46}}),
				),
			},
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		f := content[0].(map[string]any)
		assert.Equal(t, "input_file", f["type"])
		assert.Equal(t, "data:application/pdf;base64,JVBERg==", f["file_data"])
		assert.Equal(t, "part-0.pdf", f["filename"])
	})

	t.Run("wildcard audio media type detects inline bytes when passed through", func(t *testing.T) {
		passThrough := true
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("audio/*", provider.DataContent{Bytes: []byte{0xff, 0xfb, 0x90, 0x64}}),
				),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PassThroughUnsupportedFiles: &passThrough}),
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		f := content[0].(map[string]any)
		assert.Equal(t, "input_file", f["type"])
		assert.Equal(t, "data:audio/mpeg;base64,//uQZA==", f["file_data"])
		assert.Equal(t, "part-0", f["filename"])
	})

	t.Run("wildcard M4A media type detects ftyp box when passed through", func(t *testing.T) {
		passThrough := true
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("audio/*", provider.DataContent{Bytes: []byte{0x00, 0x00, 0x00, 0x1c, 0x66, 0x74, 0x79, 0x70, 0x4d, 0x34, 0x41, 0x20}}),
				),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PassThroughUnsupportedFiles: &passThrough}),
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		file := content[0].(map[string]any)
		assert.Equal(t, "data:audio/mp4;base64,AAAAHGZ0eXBNNEEg", file["file_data"])
	})

	t.Run("wildcard audio media type strips id3 before detecting inline bytes", func(t *testing.T) {
		passThrough := true
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("audio/*", provider.DataContent{Base64: "SUQzBAAAAAAAAP/7kGQ="}),
				),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PassThroughUnsupportedFiles: &passThrough}),
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		f := content[0].(map[string]any)
		assert.Equal(t, "input_file", f["type"])
		assert.Equal(t, "data:audio/mpeg;base64,SUQzBAAAAAAAAP/7kGQ=", f["file_data"])
	})

	t.Run("wildcard video media type detects inline bytes when passed through", func(t *testing.T) {
		passThrough := true
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("video/*", provider.DataContent{Bytes: []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70}}),
				),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PassThroughUnsupportedFiles: &passThrough}),
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		f := content[0].(map[string]any)
		assert.Equal(t, "input_file", f["type"])
		assert.Equal(t, "data:video/mp4;base64,AAAAGGZ0eXA=", f["file_data"])
		assert.Equal(t, "part-0", f["filename"])
	})

	t.Run("detected audio media type errors without pass-through", func(t *testing.T) {
		_, _, _, err := buildParams("gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("audio/*", provider.DataContent{Bytes: []byte{0xff, 0xfb, 0x90, 0x64}}),
				),
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `media type "audio/mpeg"`)
	})

	t.Run("unsupported file media type pass-through uses resolved filename", func(t *testing.T) {
		passThrough := true
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("application/zip", provider.DataContent{Base64: "UEsDBA=="}),
				),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PassThroughUnsupportedFiles: &passThrough}),
		})
		input := body["input"].([]any)
		content := input[0].(map[string]any)["content"].([]any)
		f := content[0].(map[string]any)
		assert.Equal(t, "input_file", f["type"])
		assert.Equal(t, "data:application/zip;base64,UEsDBA==", f["file_data"])
		assert.Equal(t, "part-0", f["filename"])
	})

	t.Run("unsupported file media type errors", func(t *testing.T) {
		_, _, _, err := buildParams("gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewUserMessage(
					provider.FilePart("application/zip", provider.DataContent{Base64: "ZGF0YQ=="}),
				),
			},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `media type "application/zip"`)
	})
}

func TestBuildParams_StoredAssistantReference(t *testing.T) {
	assistantText := provider.ContentPart{
		Type:            provider.ContentPartTypeText,
		Text:            "prior answer",
		ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "msg_1"}),
	}
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("hi"),
			provider.NewAssistantMessage(assistantText),
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{}), // store defaults true
	})
	input := body["input"].([]any)
	last := input[len(input)-1].(map[string]any)
	assert.Equal(t, "item_reference", last["type"])
	assert.Equal(t, "msg_1", last["id"])
}

func TestBuildParams_FunctionCallRoundTrip(t *testing.T) {
	noStore := false
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("weather?"),
			provider.NewAssistantMessage(provider.ToolCallPart("call_1", "getWeather", json.RawMessage(`{"city":"SF"}`))),
			provider.NewToolMessage(provider.ToolResultPart("call_1", "getWeather", &provider.ToolResultOutput{
				Type: provider.ToolOutputJSON,
				JSON: json.RawMessage(`{"temp":20}`),
			})),
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
	})
	input := body["input"].([]any)
	var fcFound, foFound bool
	for _, it := range input {
		m := it.(map[string]any)
		switch m["type"] {
		case "function_call":
			fcFound = true
			assert.Equal(t, "call_1", m["call_id"])
			assert.Equal(t, "getWeather", m["name"])
			assert.Equal(t, `{"city":"SF"}`, m["arguments"])
		case "function_call_output":
			foFound = true
			assert.Equal(t, "call_1", m["call_id"])
			assert.Equal(t, `{"temp":20}`, m["output"])
		}
	}
	assert.True(t, fcFound, "function_call present")
	assert.True(t, foFound, "function_call_output present")
}

func TestBuildParams_ProviderToolContinuationTaxonomy(t *testing.T) {
	noStore := false

	t.Run("provider-executed call and result use one stored item reference", func(t *testing.T) {
		callID := "ci_123"
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.ContentPart{
						Type:             provider.ContentPartTypeToolCall,
						ToolCallID:       callID,
						ToolName:         "python",
						Input:            json.RawMessage(`{"code":"print(1)"}`),
						ProviderExecuted: true,
					},
					provider.ContentPart{
						Type:       provider.ContentPartTypeToolResult,
						ToolCallID: callID,
						ToolName:   "python",
						Output: &provider.ToolResultOutput{
							Type: provider.ToolOutputJSON,
							JSON: json.RawMessage(`{"outputs":[]}`),
						},
					},
				),
			},
			Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCodeInterpreter, Name: "python"}},
		})

		input := body["input"].([]any)
		require.Len(t, input, 1)
		assert.Equal(t, map[string]any{"type": "item_reference", "id": callID}, input[0])
	})

	t.Run("provider-executed call and result are omitted without storage", func(t *testing.T) {
		callID := "ws_123"
		body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(
					provider.TextPart("searching"),
					provider.ContentPart{
						Type:             provider.ContentPartTypeToolCall,
						ToolCallID:       callID,
						ToolName:         "search",
						Input:            json.RawMessage(`{"query":"news"}`),
						ProviderExecuted: true,
					},
					provider.ContentPart{
						Type:       provider.ContentPartTypeToolResult,
						ToolCallID: callID,
						ToolName:   "search",
						Output:     &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"sources":[]}`)},
					},
				),
			},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDWebSearch, Name: "search"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 1)
		assert.Equal(t, "assistant", input[0].(map[string]any)["role"])
		assert.NotContains(t, input[0].(map[string]any), "type")
		require.Len(t, warnings, 1)
		assert.Equal(t, "Results for OpenAI tool search are not sent to the API when store is false", warnings[0].Message)
	})

	t.Run("hosted tool search preserves distinct call and output items", func(t *testing.T) {
		call := provider.ContentPart{
			Type:             provider.ContentPartTypeToolCall,
			ToolCallID:       "tsc_123",
			ToolName:         "findTools",
			Input:            json.RawMessage(`{"arguments":{"paths":["get_weather"]},"call_id":null}`),
			ProviderExecuted: true,
			ProviderOptions:  provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "tsc_123"}),
		}
		result := provider.ContentPart{
			Type:            provider.ContentPartTypeToolResult,
			ToolCallID:      "tsc_123",
			ToolName:        "findTools",
			Output:          &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"tools":[{"type":"function","name":"get_weather","parameters":{"type":"object"}}]}`)},
			ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "tso_456"}),
		}
		body, _ := buildBody(t, "gpt-5", provider.CallOptions{
			Prompt:          []provider.Message{provider.NewAssistantMessage(call, result)},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "findTools"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 2)
		assert.Equal(t, "tool_search_call", input[0].(map[string]any)["type"])
		assert.Equal(t, "tsc_123", input[0].(map[string]any)["id"])
		assert.Equal(t, "server", input[0].(map[string]any)["execution"])
		assert.Equal(t, "tool_search_output", input[1].(map[string]any)["type"])
		assert.Equal(t, "tso_456", input[1].(map[string]any)["id"])
		assert.Len(t, input[1].(map[string]any)["tools"], 1)
	})

	t.Run("local shell call and output retain call id", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-5-codex", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolCall,
					ToolCallID: "call_local",
					ToolName:   "terminal",
					Input:      json.RawMessage(`{"action":{"type":"exec","command":["ls"]}}`),
				}),
				provider.NewToolMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "call_local",
					ToolName:   "terminal",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"output":"file.txt"}`)},
				}),
			},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDLocalShell, Name: "terminal"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 2)
		assert.Equal(t, "local_shell_call", input[0].(map[string]any)["type"])
		assert.Equal(t, "call_local", input[0].(map[string]any)["call_id"])
		assert.Equal(t, "local_shell_call_output", input[1].(map[string]any)["type"])
		assert.Equal(t, "call_local", input[1].(map[string]any)["call_id"])
	})

	t.Run("stored shell call pairs with reconstructed output", func(t *testing.T) {
		call := provider.ContentPart{
			Type:            provider.ContentPartTypeToolCall,
			ToolCallID:      "call_shell",
			ToolName:        "terminal",
			Input:           json.RawMessage(`{"action":{"commands":["uname -a"]}}`),
			ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "sh_123"}),
		}
		body, _ := buildBody(t, "gpt-5", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(call),
				provider.NewToolMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "call_shell",
					ToolName:   "terminal",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"output":[{"stdout":"x86_64\n","stderr":"","outcome":{"type":"exit","exitCode":0}}]}`)},
				}),
			},
			Tools: []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDShell, Name: "terminal"}},
		})

		input := body["input"].([]any)
		require.Len(t, input, 2)
		assert.Equal(t, map[string]any{"type": "item_reference", "id": "sh_123"}, input[0])
		output := input[1].(map[string]any)
		assert.Equal(t, "shell_call_output", output["type"])
		assert.Equal(t, "call_shell", output["call_id"])
	})

	t.Run("apply patch call and output retain provider taxonomy", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-5", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolCall,
					ToolCallID: "call_patch",
					ToolName:   "patch",
					Input:      json.RawMessage(`{"callId":"call_patch","operation":{"type":"create_file","path":"a.txt","diff":"+hello"}}`),
				}),
				provider.NewToolMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "call_patch",
					ToolName:   "patch",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"status":"completed","output":"created"}`)},
				}),
			},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDApplyPatch, Name: "patch"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 2)
		assert.Equal(t, "apply_patch_call", input[0].(map[string]any)["type"])
		assert.Equal(t, "call_patch", input[0].(map[string]any)["call_id"])
		assert.Equal(t, "apply_patch_call_output", input[1].(map[string]any)["type"])
		assert.Equal(t, "call_patch", input[1].(map[string]any)["call_id"])
	})

	t.Run("custom call and output retain provider taxonomy", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-5", provider.CallOptions{
			Prompt: []provider.Message{
				provider.NewAssistantMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolCall,
					ToolCallID: "call_custom",
					ToolName:   "write_sql",
					Input:      json.RawMessage(`"SELECT 1"`),
				}),
				provider.NewToolMessage(provider.ContentPart{
					Type:       provider.ContentPartTypeToolResult,
					ToolCallID: "call_custom",
					ToolName:   "write_sql",
					Output:     &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "ok"},
				}),
			},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 2)
		call := input[0].(map[string]any)
		assert.Equal(t, "custom_tool_call", call["type"])
		assert.Equal(t, "call_custom", call["call_id"])
		assert.Equal(t, "SELECT 1", call["input"])
		output := input[1].(map[string]any)
		assert.Equal(t, "custom_tool_call_output", output["type"])
		assert.Equal(t, "call_custom", output["call_id"])
	})

	t.Run("assistant execution denied result is omitted", func(t *testing.T) {
		body, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{provider.NewAssistantMessage(provider.ContentPart{
				Type:       provider.ContentPartTypeToolResult,
				ToolCallID: "denied",
				ToolName:   "search",
				Output:     &provider.ToolResultOutput{Type: provider.ToolOutputExecutionDenied, Reason: "no"},
			})},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDWebSearch, Name: "search"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})

		assert.Nil(t, body["input"])
		assert.Empty(t, warnings)
	})

	t.Run("non-JSON programmatic result is omitted", func(t *testing.T) {
		store := false
		body, warnings := buildBody(t, "gpt-5.6", provider.CallOptions{
			Prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolResultPart(
				"call_program", "program", &provider.ToolResultOutput{Type: provider.ToolOutputText, Text: "done"},
			))},
			Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDProgrammatic, Name: "program"}},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &store}),
		})

		assert.Nil(t, body["input"])
		assert.Empty(t, warnings)
	})
}

func TestBuildParams_ProviderToolContinuationProviderOptionsNamespace(t *testing.T) {
	t.Run("stored text reference", func(t *testing.T) {
		text := provider.TextPart("stored")
		text.ProviderOptions = withAzureOptions(t, OpenAIPartOptions{ItemID: "msg_azure"})
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt:          []provider.Message{provider.NewAssistantMessage(text)},
			ProviderOptions: withAzureOptions(t, OpenAIResponsesOptions{}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 1)
		assert.Equal(t, map[string]any{"type": "item_reference", "id": "msg_azure"}, input[0])
	})

	t.Run("system and user content options", func(t *testing.T) {
		breakpoint := &PromptCacheBreakpoint{Mode: "explicit"}
		system := provider.NewSystemMessage("be helpful")
		system.ProviderOptions = withAzureOptions(t, OpenAIPartOptions{PromptCacheBreakpoint: breakpoint})
		text := provider.TextPart("look")
		text.ProviderOptions = withAzureOptions(t, OpenAIPartOptions{PromptCacheBreakpoint: breakpoint})
		image := provider.FilePart("image/png", provider.DataContent{URL: "https://example.com/x.png"})
		image.ProviderOptions = withAzureOptions(t, OpenAIPartOptions{PromptCacheBreakpoint: breakpoint, ImageDetail: "high"})

		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt:          []provider.Message{system, provider.NewUserMessage(text, image)},
			ProviderOptions: withAzureOptions(t, OpenAIResponsesOptions{}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 2)
		systemContent := input[0].(map[string]any)["content"].([]any)
		assert.Equal(t, map[string]any{"mode": "explicit"}, systemContent[0].(map[string]any)["prompt_cache_breakpoint"])
		userContent := input[1].(map[string]any)["content"].([]any)
		assert.Equal(t, map[string]any{"mode": "explicit"}, userContent[0].(map[string]any)["prompt_cache_breakpoint"])
		assert.Equal(t, map[string]any{"mode": "explicit"}, userContent[1].(map[string]any)["prompt_cache_breakpoint"])
		assert.Equal(t, "high", userContent[1].(map[string]any)["detail"])
	})

	t.Run("expanded computer call", func(t *testing.T) {
		store := false
		call := provider.ToolCallPart("call_azure", "browser", json.RawMessage(`{"actions":[],"pendingSafetyChecks":[],"status":"completed"}`))
		call.ProviderOptions = withAzureOptions(t, OpenAIPartOptions{ItemID: "item_azure"})
		body, _ := buildBody(t, "computer-preview", provider.CallOptions{
			Prompt: []provider.Message{provider.NewAssistantMessage(call)},
			Tools:  []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}},
			ProviderOptions: withAzureOptions(t, OpenAIResponsesOptions{
				Store: &store,
			}),
		})

		input := body["input"].([]any)
		require.Len(t, input, 1)
		assert.Equal(t, "computer_call", input[0].(map[string]any)["type"])
		assert.Equal(t, "item_azure", input[0].(map[string]any)["id"])
	})
}

func TestBuildParams_ProviderToolContinuationValidation(t *testing.T) {
	noStore := false
	tests := []struct {
		name    string
		prompt  []provider.Message
		tools   []provider.Tool
		wantErr string
	}{
		{
			name: "local shell requires command",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart(
				"call_local", "terminal", json.RawMessage(`{"action":{"type":"exec"}}`),
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDLocalShell, Name: "terminal"}},
			wantErr: "action.command is required",
		},
		{
			name: "local shell rejects null option",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart(
				"call_local", "terminal", json.RawMessage(`{"action":{"type":"exec","command":["ls"],"timeoutMs":null}}`),
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDLocalShell, Name: "terminal"}},
			wantErr: "action.timeoutMs must be a number",
		},
		{
			name: "shell rejects unknown outcome",
			prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart(
				"call_shell", "terminal", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"output":[{"stdout":"","stderr":"","outcome":{"type":"unknown"}}]}`)},
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDShell, Name: "terminal"}},
			wantErr: "outcome.type is invalid",
		},
		{
			name: "shell rejects null option",
			prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart(
				"call_shell", "terminal", json.RawMessage(`{"action":{"commands":["pwd"],"maxOutputLength":null}}`),
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDShell, Name: "terminal"}},
			wantErr: "action.maxOutputLength must be a number",
		},
		{
			name: "apply patch validates status",
			prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart(
				"call_patch", "apply_patch", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"status":"unknown"}`)},
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDApplyPatch, Name: "apply_patch"}},
			wantErr: "status must be completed or failed",
		},
		{
			name: "apply patch rejects null output",
			prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart(
				"call_patch", "apply_patch", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"status":"completed","output":null}`)},
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDApplyPatch, Name: "apply_patch"}},
			wantErr: "output must be a string",
		},
		{
			name: "tool search requires tools",
			prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart(
				"call_search", "findTools", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{}`)},
			))},
			tools:   []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "findTools"}},
			wantErr: "tools is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := buildParams("gpt-5", provider.CallOptions{
				Prompt:          tc.prompt,
				Tools:           tc.tools,
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestBuildParams_ProviderToolContinuationFlexibleSchemas(t *testing.T) {
	noStore := false
	body, _ := buildBody(t, "gpt-5", provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewAssistantMessage(
				provider.ToolCallPart("call_local", "local", json.RawMessage(`{"action":{"type":"exec","command":["ls"],"timeoutMs":1.5}}`)),
				provider.ToolCallPart("call_shell", "shell", json.RawMessage(`{"action":{"commands":["pwd"],"maxOutputLength":2.5}}`)),
			),
			provider.NewToolMessage(
				provider.ToolResultPart("call_shell", "shell", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"output":[{"stdout":"ok","stderr":"","outcome":{"type":"exit","exitCode":3.5}}]}`)}),
				provider.ToolResultPart("call_search", "findTools", &provider.ToolResultOutput{Type: provider.ToolOutputJSON, JSON: json.RawMessage(`{"tools":[{"type":"future_tool","config":{"enabled":true}}]}`)}),
			),
		},
		Tools: []provider.Tool{
			{Type: provider.ToolTypeProvider, ID: toolIDLocalShell, Name: "local"},
			{Type: provider.ToolTypeProvider, ID: toolIDShell, Name: "shell"},
			{Type: provider.ToolTypeProvider, ID: toolIDToolSearch, Name: "findTools"},
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
	})

	input := body["input"].([]any)
	assert.Equal(t, 1.5, input[0].(map[string]any)["action"].(map[string]any)["timeout_ms"])
	assert.Equal(t, 2.5, input[1].(map[string]any)["action"].(map[string]any)["max_output_length"])
	assert.Equal(t, 3.5, input[2].(map[string]any)["output"].([]any)[0].(map[string]any)["outcome"].(map[string]any)["exit_code"])
	assert.Equal(t, "future_tool", input[3].(map[string]any)["tools"].([]any)[0].(map[string]any)["type"])
}

func TestShellInput_OnlyExposesCommands(t *testing.T) {
	assert.JSONEq(t,
		`{"action":{"commands":["echo hi"]}}`,
		string(shellInput(`{"commands":["echo hi"],"timeout_ms":1000.5,"max_output_length":2048.5}`, nil)),
	)
}

func TestProviderToolContinuationInput_PreservesShellOptions(t *testing.T) {
	assert.JSONEq(t,
		`{"action":{"type":"exec","command":["pwd"],"env":{"A":"B"},"timeoutMs":1000.5,"user":"nara","workingDirectory":"/tmp"}}`,
		string(localShellInput(`{"action":{"type":"exec","command":["pwd"],"env":{"A":"B"},"timeout_ms":1000.5,"user":"nara","working_directory":"/tmp"}}`)),
	)

	withoutEnv := localShellInput(`{"action":{"type":"exec","command":["pwd"],"timeout_ms":1000.5}}`)
	assert.JSONEq(t,
		`{"action":{"type":"exec","command":["pwd"],"timeoutMs":1000.5}}`,
		string(withoutEnv),
	)
	noStore := false
	body, _ := buildBody(t, "gpt-5", provider.CallOptions{
		Prompt: []provider.Message{provider.NewAssistantMessage(provider.ToolCallPart(
			"call_local", "terminal", withoutEnv,
		))},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDLocalShell, Name: "terminal"}},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
	})
	assert.NotContains(t, body["input"].([]any)[0].(map[string]any)["action"], "env")
}

func TestBuildParams_MCPApprovalContinuation(t *testing.T) {
	approved := provider.ToolApprovalResponsePart("approval_1", true, "")
	deniedResult := provider.ToolResultPart("call_1", "mcp_tool", &provider.ToolResultOutput{
		Type:            provider.ToolOutputExecutionDenied,
		Reason:          "denied",
		ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{ApprovalID: "approval_1"}),
	})
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(approved, approved, deniedResult)},
	})

	input := body["input"].([]any)
	require.Len(t, input, 2)
	assert.Equal(t, map[string]any{"type": "item_reference", "id": "approval_1"}, input[0])
	response := input[1].(map[string]any)
	assert.Equal(t, "mcp_approval_response", response["type"])
	assert.Equal(t, "approval_1", response["approval_request_id"])
	assert.Equal(t, true, response["approve"])
}

func TestBuildParams_CustomToolContentOptions(t *testing.T) {
	noStore := false
	breakpoint := &PromptCacheBreakpoint{Mode: "explicit"}
	emptyData := provider.Base64DataContent("")
	body, _ := buildBody(t, "gpt-5", provider.CallOptions{
		Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call_custom", "write_sql", &provider.ToolResultOutput{
			Type: provider.ToolOutputContent,
			Content: []provider.ToolResultContentValue{
				{
					Type:            provider.ToolContentText,
					Text:            "result",
					ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{PromptCacheBreakpoint: breakpoint}),
				},
				{
					Type:            provider.ToolContentFile,
					Data:            &provider.DataContent{URL: "https://example.com/image.png"},
					MediaType:       "image/png",
					ProviderOptions: provider.BuildProviderOptions(OpenAIPartOptions{ImageDetail: "high"}),
				},
				{
					Type:      provider.ToolContentFile,
					Data:      &provider.DataContent{Base64: "aW1hZ2U="},
					MediaType: "image/png",
				},
				{
					Type:      provider.ToolContentFile,
					Data:      &emptyData,
					MediaType: "image/png",
				},
			},
		}))},
		Tools:           []provider.Tool{{Type: provider.ToolTypeProvider, ID: toolIDCustom, Name: "write_sql"}},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
	})

	output := findInput(body, "custom_tool_call_output")["output"].([]any)
	require.Len(t, output, 4)
	assert.Equal(t, map[string]any{"mode": "explicit"}, output[0].(map[string]any)["prompt_cache_breakpoint"])
	assert.Equal(t, "https://example.com/image.png", output[1].(map[string]any)["image_url"])
	assert.Equal(t, "high", output[1].(map[string]any)["detail"])
	assert.Equal(t, "data:image/png;base64,aW1hZ2U=", output[2].(map[string]any)["image_url"])
	assert.Equal(t, "data:image/png;base64,", output[3].(map[string]any)["image_url"])
}

func TestBuildParams_ComputerCallRoundTrip(t *testing.T) {
	storedCall := provider.ToolCallPart("call_1", "browser", json.RawMessage(`{
		"actions":[
			{"type":"click","button":"left","x":10,"y":20,"keys":["SHIFT"]},
			{"type":"scroll","x":0,"y":1,"scrollX":2,"scrollY":3},
			{"type":"screenshot"}
		],
		"pendingSafetyChecks":[{"id":"safe_1","code":"policy","message":"confirm"}],
		"status":"completed"
	}`))
	storedCall.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "item_1"})
	result := provider.ToolResultPart("call_1", "browser", &provider.ToolResultOutput{
		Type: provider.ToolOutputJSON,
		JSON: json.RawMessage(`{
			"output":{"type":"computer_screenshot","imageUrl":"data:image/png;base64,abc","detail":"original"},
			"acknowledgedSafetyChecks":[{"id":"safe_1","code":"policy","message":"confirm"}]
		}`),
	})
	computerTool := provider.Tool{Type: provider.ToolTypeProvider, ID: toolIDComputer, Name: "browser"}

	t.Run("stored call uses item reference", func(t *testing.T) {
		body, _ := buildBody(t, "computer-preview", provider.CallOptions{
			Prompt: []provider.Message{
				provider.UserText("use browser"),
				provider.NewAssistantMessage(storedCall),
				provider.NewToolMessage(result),
			},
			Tools: []provider.Tool{computerTool},
		})
		input := body["input"].([]any)
		reference := input[1].(map[string]any)
		assert.Equal(t, "item_reference", reference["type"])
		assert.Equal(t, "item_1", reference["id"])
		output := input[2].(map[string]any)
		assert.Equal(t, "computer_call_output", output["type"])
		assert.Equal(t, "call_1", output["call_id"])
		screenshot := output["output"].(map[string]any)
		assert.Equal(t, "data:image/png;base64,abc", screenshot["image_url"])
		assert.Equal(t, "original", screenshot["detail"])
	})

	t.Run("non-stored call is expanded", func(t *testing.T) {
		store := false
		body, _ := buildBody(t, "computer-preview", provider.CallOptions{
			Prompt: []provider.Message{
				provider.UserText("use browser"),
				provider.NewAssistantMessage(storedCall),
			},
			Tools:           []provider.Tool{computerTool},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &store}),
		})
		call := findInput(body, "computer_call")
		require.NotNil(t, call)
		assert.Equal(t, "item_1", call["id"])
		assert.Equal(t, "call_1", call["call_id"])
		assert.Equal(t, "completed", call["status"])
		actions := call["actions"].([]any)
		require.Len(t, actions, 3)
		assert.Equal(t, float64(2), actions[1].(map[string]any)["scroll_x"])
		assert.Equal(t, float64(3), actions[1].(map[string]any)["scroll_y"])
		checks := call["pending_safety_checks"].([]any)
		assert.Equal(t, "safe_1", checks[0].(map[string]any)["id"])
	})

	t.Run("previous response omits stored call but sends output", func(t *testing.T) {
		body, _ := buildBody(t, "computer-preview", provider.CallOptions{
			Prompt: []provider.Message{
				provider.UserText("use browser"),
				provider.NewAssistantMessage(storedCall),
				provider.NewToolMessage(result),
			},
			Tools:           []provider.Tool{computerTool},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: "resp_prev"}),
		})
		assert.Nil(t, findInput(body, "computer_call"))
		assert.Nil(t, findInput(body, "item_reference"))
		require.NotNil(t, findInput(body, "computer_call_output"))
	})

	t.Run("file id screenshot", func(t *testing.T) {
		body, _ := buildBody(t, "computer-preview", provider.CallOptions{
			Prompt: []provider.Message{provider.NewToolMessage(provider.ToolResultPart("call_2", "browser", &provider.ToolResultOutput{
				Type: provider.ToolOutputJSON,
				JSON: json.RawMessage(`{"output":{"type":"computer_screenshot","fileId":"file_1","detail":"high"}}`),
			}))},
			Tools: []provider.Tool{computerTool},
		})
		output := findInput(body, "computer_call_output")
		require.NotNil(t, output)
		screenshot := output["output"].(map[string]any)
		assert.Equal(t, "file_1", screenshot["file_id"])
		assert.Equal(t, "high", screenshot["detail"])
	})
}

func TestComputerCallValidation(t *testing.T) {
	t.Run("fractional coordinates and explicit empty arrays", func(t *testing.T) {
		part := provider.ToolCallPart("call_1", "browser", json.RawMessage(`{
			"actions":[
				{"type":"click","button":"left","x":1.5,"y":2.25,"keys":[]},
				{"type":"drag","path":[]},
				{"type":"keypress","keys":[]}
			],
			"pendingSafetyChecks":[],
			"status":"completed"
		}`))
		item, err := computerCallInputItem(part, "")
		require.NoError(t, err)
		encoded, err := json.Marshal(item)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(encoded, &value))
		actions := value["actions"].([]any)
		assert.Equal(t, 1.5, actions[0].(map[string]any)["x"])
		assert.Equal(t, []any{}, actions[0].(map[string]any)["keys"])
		assert.Equal(t, []any{}, actions[1].(map[string]any)["path"])
		assert.Equal(t, []any{}, actions[2].(map[string]any)["keys"])
	})

	invalidInputs := []struct {
		name  string
		input string
	}{
		{name: "missing actions", input: `{"pendingSafetyChecks":[],"status":"completed"}`},
		{name: "missing pending safety checks", input: `{"actions":[],"status":"completed"}`},
		{name: "missing click button", input: `{"actions":[{"type":"click","x":1,"y":2}],"pendingSafetyChecks":[],"status":"completed"}`},
		{name: "invalid click button", input: `{"actions":[{"type":"click","button":"middle","x":1,"y":2}],"pendingSafetyChecks":[],"status":"completed"}`},
		{name: "missing type text", input: `{"actions":[{"type":"type"}],"pendingSafetyChecks":[],"status":"completed"}`},
		{name: "null optional action keys", input: `{"actions":[{"type":"click","button":"left","x":1,"y":2,"keys":null}],"pendingSafetyChecks":[],"status":"completed"}`},
		{name: "missing safety id", input: `{"actions":[],"pendingSafetyChecks":[{"code":"policy"}],"status":"completed"}`},
		{name: "null safety id", input: `{"actions":[],"pendingSafetyChecks":[{"id":null}],"status":"completed"}`},
		{name: "null safety code", input: `{"actions":[],"pendingSafetyChecks":[{"id":"safe_1","code":null}],"status":"completed"}`},
		{name: "null safety message", input: `{"actions":[],"pendingSafetyChecks":[{"id":"safe_1","message":null}],"status":"completed"}`},
	}
	for _, tc := range invalidInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := computerCallInputItem(provider.ToolCallPart("call_1", "browser", json.RawMessage(tc.input)), "")
			require.Error(t, err)
		})
	}

	t.Run("output preserves empty acknowledgements", func(t *testing.T) {
		part := provider.ToolResultPart("call_1", "browser", &provider.ToolResultOutput{
			Type: provider.ToolOutputJSON,
			JSON: json.RawMessage(`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":[]}`),
		})
		item, err := computerCallOutputItem(part)
		require.NoError(t, err)
		encoded, err := json.Marshal(item)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(encoded, &value))
		assert.Equal(t, []any{}, value["acknowledged_safety_checks"])
	})

	t.Run("empty screenshot identifiers are preserved", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			input   string
			wireKey string
		}{
			{name: "image URL", input: `{"output":{"type":"computer_screenshot","imageUrl":""}}`, wireKey: "image_url"},
			{name: "file ID", input: `{"output":{"type":"computer_screenshot","fileId":""}}`, wireKey: "file_id"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				part := provider.ToolResultPart("call_1", "browser", &provider.ToolResultOutput{
					Type: provider.ToolOutputJSON,
					JSON: json.RawMessage(tc.input),
				})
				item, err := computerCallOutputItem(part)
				require.NoError(t, err)
				encoded, err := json.Marshal(item)
				require.NoError(t, err)
				var value map[string]any
				require.NoError(t, json.Unmarshal(encoded, &value))
				output := value["output"].(map[string]any)
				assert.Contains(t, output, tc.wireKey)
				assert.Equal(t, "", output[tc.wireKey])
			})
		}
	})

	t.Run("empty safety check IDs are preserved", func(t *testing.T) {
		inputPart := provider.ToolCallPart("call_1", "browser", json.RawMessage(`{"actions":[],"pendingSafetyChecks":[{"id":""}],"status":"completed"}`))
		inputItem, err := computerCallInputItem(inputPart, "")
		require.NoError(t, err)
		inputJSON, err := json.Marshal(inputItem)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"computer_call","call_id":"call_1","actions":[],"pending_safety_checks":[{"id":""}],"status":"completed"}`, string(inputJSON))

		outputPart := provider.ToolResultPart("call_1", "browser", &provider.ToolResultOutput{
			Type: provider.ToolOutputJSON,
			JSON: json.RawMessage(`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":[{"id":""}]}`),
		})
		outputItem, err := computerCallOutputItem(outputPart)
		require.NoError(t, err)
		outputJSON, err := json.Marshal(outputItem)
		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"computer_call_output","call_id":"call_1","output":{"type":"computer_screenshot","file_id":"file_1"},"acknowledged_safety_checks":[{"id":""}]}`, string(outputJSON))
	})

	invalidOutputs := []string{
		`{"output":{"type":"computer_screenshot"}}`,
		`{"output":{"type":"computer_screenshot","imageUrl":"https://example.com/image.png","fileId":null}}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1","imageUrl":null}}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1","detail":null}}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1","detail":"maximum"}}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":null}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":[{}]}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":[{"id":null}]}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":[{"id":"safe_1","code":null}]}`,
		`{"output":{"type":"computer_screenshot","fileId":"file_1"},"acknowledgedSafetyChecks":[{"id":"safe_1","message":null}]}`,
	}
	for _, output := range invalidOutputs {
		_, err := computerCallOutputItem(provider.ToolResultPart("call_1", "browser", &provider.ToolResultOutput{
			Type: provider.ToolOutputJSON,
			JSON: json.RawMessage(output),
		}))
		require.Error(t, err)
	}
}

func TestBuildParams_EmptyToolInputSerializesEmptyObject(t *testing.T) {
	assert.Equal(t, "{}", serializeToolCallArguments(nil))
	assert.Equal(t, "{}", serializeToolCallArguments([]byte("null")))
	assert.Equal(t, `{"a":1}`, serializeToolCallArguments([]byte(`{"a":1}`)))
}

func TestBuildParams_UnsupportedSamplingParams(t *testing.T) {
	seed := 5
	topK := 3
	pp := 0.5
	fp := 0.5
	_, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt:           []provider.Message{provider.UserText("hi")},
		Seed:             &seed,
		TopK:             &topK,
		PresencePenalty:  &pp,
		FrequencyPenalty: &fp,
		StopSequences:    []string{"x"},
	})
	feats := warningFeatures(warnings)
	assert.Contains(t, feats, "seed")
	assert.Contains(t, feats, "topK")
	assert.Contains(t, feats, "presencePenalty")
	assert.Contains(t, feats, "frequencyPenalty")
	assert.Contains(t, feats, "stopSequences")
}

func TestBuildParams_JSONSchemaStructuredOutput(t *testing.T) {
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ResponseFormat: &provider.ResponseFormat{
			Type:   provider.ResponseFormatJSON,
			Name:   "weather",
			Schema: json.RawMessage(`{"type":"object"}`),
		},
	})
	text := body["text"].(map[string]any)
	format := text["format"].(map[string]any)
	assert.Equal(t, "json_schema", format["type"])
	assert.Equal(t, "weather", format["name"])
	assert.Equal(t, true, format["strict"])
	assert.NotNil(t, format["schema"])
}

func TestBuildParams_ReasoningModelStripsTemperature(t *testing.T) {
	temp := 0.7
	topP := 0.9
	body, warnings := buildBody(t, "o3", provider.CallOptions{
		Prompt:      []provider.Message{provider.UserText("hi")},
		Temperature: &temp,
		TopP:        &topP,
	})
	_, hasTemp := body["temperature"]
	_, hasTopP := body["top_p"]
	assert.False(t, hasTemp, "temperature should be stripped")
	assert.False(t, hasTopP, "top_p should be stripped")
	feats := warningFeatures(warnings)
	assert.Contains(t, feats, "temperature")
	assert.Contains(t, feats, "topP")
}

func TestBuildParams_ReasoningSummaryDefault(t *testing.T) {
	tests := []struct {
		name            string
		effort          string
		expectedEffort  string
		expectedSummary string
	}{
		{
			name:            "medium defaults to detailed summary",
			effort:          "medium",
			expectedEffort:  "medium",
			expectedSummary: "detailed",
		},
		{
			name:           "none does not default summary",
			effort:         "none",
			expectedEffort: "none",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := buildBody(t, "gpt-5.5", provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
				ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
					ReasoningEffort: tc.effort,
				}),
			})

			reasoning := body["reasoning"].(map[string]any)
			assert.Equal(t, tc.expectedEffort, reasoning["effort"])
			if tc.expectedSummary == "" {
				assert.NotContains(t, reasoning, "summary")
			} else {
				assert.Equal(t, tc.expectedSummary, reasoning["summary"])
			}
		})
	}
}

func TestBuildParams_ProviderOptions(t *testing.T) {
	t.Run("previousResponseId", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt:          []provider.Message{provider.UserText("hi")},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: "resp_prev"}),
		})
		assert.Equal(t, "resp_prev", body["previous_response_id"])
	})

	t.Run("previousResponseId skips stored reasoning references", func(t *testing.T) {
		reasoning := provider.ReasoningPart("thinking")
		reasoning.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "rs_prev"})
		body, warnings := buildBody(t, "o4-mini", provider.CallOptions{
			Prompt: []provider.Message{
				provider.UserText("continue"),
				provider.NewAssistantMessage(reasoning),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{PreviousResponseID: "resp_prev"}),
		})

		assert.Equal(t, "resp_prev", body["previous_response_id"])
		assert.Nil(t, findInput(body, "reasoning"))
		for _, it := range body["input"].([]any) {
			m := it.(map[string]any)
			assert.NotEqual(t, "rs_prev", m["id"])
		}
		assert.NotContains(t, warningFeatures(warnings), "reasoning")
	})

	t.Run("conversation and previousResponseId conflict warning", func(t *testing.T) {
		_, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt:          []provider.Message{provider.UserText("hi")},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Conversation: "conv_1", PreviousResponseID: "resp_prev"}),
		})
		assert.Contains(t, warningFeatures(warnings), "conversation")
	})

	t.Run("logprobs auto-include", func(t *testing.T) {
		n := int64(3)
		body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt:          []provider.Message{provider.UserText("hi")},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Logprobs: &LogprobsOption{Int: &n}}),
		})
		assert.EqualValues(t, 3, body["top_logprobs"])
		include := toStringSlice(body["include"])
		assert.Contains(t, include, "message.output_text.logprobs")
	})

	t.Run("gpt-5.6 prompt cache and reasoning options", func(t *testing.T) {
		body, _ := buildBody(t, "gpt-5.6", provider.CallOptions{
			Prompt: []provider.Message{provider.UserText("hi")},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
				PromptCacheOptions: &PromptCacheOptions{Mode: "explicit", TTL: "30m"},
				ReasoningEffort:    "max",
				ReasoningMode:      "pro",
				ReasoningContext:   "all_turns",
			}),
		})

		assert.Equal(t, map[string]any{"mode": "explicit", "ttl": "30m"}, body["prompt_cache_options"])
		reasoning := body["reasoning"].(map[string]any)
		assert.Equal(t, "max", reasoning["effort"])
		assert.Equal(t, "detailed", reasoning["summary"])
		assert.Equal(t, "pro", reasoning["mode"])
		assert.Equal(t, "all_turns", reasoning["context"])
	})

	t.Run("reasoning mode and context warn on non-reasoning model", func(t *testing.T) {
		_, warnings := buildBody(t, "gpt-4o", provider.CallOptions{
			Prompt: []provider.Message{provider.UserText("hi")},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{
				ReasoningMode:    "pro",
				ReasoningContext: "all_turns",
			}),
		})
		features := warningFeatures(warnings)
		assert.Contains(t, features, "reasoningMode")
		assert.Contains(t, features, "reasoningContext")
	})
}

func TestBuildParams_NonStoredToolCallOmitsItemID(t *testing.T) {
	noStore := false
	toolCall := provider.ToolCallPart("call_1", "getWeather", json.RawMessage(`{"city":"SF"}`))
	toolCall.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{ItemID: "fc_123", Namespace: "weather_ns"})
	body, _ := buildBody(t, "gpt-4o", provider.CallOptions{
		Prompt: []provider.Message{
			provider.UserText("weather?"),
			provider.NewAssistantMessage(toolCall),
		},
		ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
	})
	input := body["input"].([]any)
	var found bool
	for _, it := range input {
		m := it.(map[string]any)
		if m["type"] == "function_call" {
			found = true
			assert.NotContains(t, m, "id", "plain function calls omit the stored item id")
			assert.Equal(t, "weather_ns", m["namespace"])
		}
	}
	assert.True(t, found, "function_call present")
}

func TestBuildParams_NonStoredReasoningEmptySummaryIsEmptyArray(t *testing.T) {
	noStore := false
	encrypted := "enc_blob"
	t.Run("empty reasoning text -> empty summary array", func(t *testing.T) {
		part := provider.ReasoningPart("")
		part.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{
			ItemID:                    "rs_1",
			ReasoningEncryptedContent: &encrypted,
		})
		body, _ := buildBody(t, "o4-mini", provider.CallOptions{
			Prompt: []provider.Message{
				provider.UserText("hi"),
				provider.NewAssistantMessage(part),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})
		reasoning := findInput(body, "reasoning")
		require.NotNil(t, reasoning)
		summary, ok := reasoning["summary"].([]any)
		require.True(t, ok, "summary is an array")
		assert.Empty(t, summary, "empty reasoning text yields an empty summary array")
	})

	t.Run("non-empty reasoning text -> summary_text entry", func(t *testing.T) {
		part := provider.ReasoningPart("thinking...")
		part.ProviderOptions = provider.BuildProviderOptions(OpenAIPartOptions{
			ItemID:                    "rs_2",
			ReasoningEncryptedContent: &encrypted,
		})
		body, _ := buildBody(t, "o4-mini", provider.CallOptions{
			Prompt: []provider.Message{
				provider.UserText("hi"),
				provider.NewAssistantMessage(part),
			},
			ProviderOptions: withOpenAIOptions(OpenAIResponsesOptions{Store: &noStore}),
		})
		reasoning := findInput(body, "reasoning")
		require.NotNil(t, reasoning)
		summary := reasoning["summary"].([]any)
		require.Len(t, summary, 1)
		entry := summary[0].(map[string]any)
		assert.Equal(t, "summary_text", entry["type"])
		assert.Equal(t, "thinking...", entry["text"])
	})
}

func findInput(body map[string]any, itemType string) map[string]any {
	for _, it := range body["input"].([]any) {
		m := it.(map[string]any)
		if m["type"] == itemType {
			return m
		}
	}
	return nil
}

func warningFeatures(warnings []provider.Warning) []string {
	var out []string
	for _, w := range warnings {
		out = append(out, w.Feature)
	}
	return out
}

func toStringSlice(v any) []string {
	arr, _ := v.([]any)
	var out []string
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
