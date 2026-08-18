package openaicompatible

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoGenerateSendsCompatibleRequest(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		require.Equal(t, "trace", r.Header.Get("X-Test"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"created":1710000000,
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"prompt_tokens_details":{"cached_tokens":2},"queue_time":0.061348671}
		}`))
	}))
	defer server.Close()

	maxTokens := 12
	temp := 0.2
	topP := 0.9
	topK := 20
	strict := false

	model := New(
		"test-model",
		WithBaseURL(server.URL+"/v1"),
		WithAPIKey("test-key"),
		WithHeaders(map[string]string{"X-Test": "trace"}),
	)
	result, err := model.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewSystemMessage("system"),
			provider.UserText("say hello"),
		},
		Tools: []provider.Tool{
			{
				Type:        provider.ToolTypeFunction,
				Name:        "weather",
				Description: "Get weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
				Strict:      &strict,
			},
			{
				Type: provider.ToolTypeProvider,
				ID:   "web_search",
			},
		},
		ToolChoice:      &provider.ToolChoice{Type: provider.ToolChoiceTool, ToolName: "weather"},
		MaxOutputTokens: &maxTokens,
		Temperature:     &temp,
		TopP:            &topP,
		TopK:            &topK,
		ProviderOptions: provider.ProviderOptions{
			"openaiCompatible": provider.RawProviderOption{
				Key: "openaiCompatible",
				Raw: json.RawMessage(`{"user":"user-123","service_tier":"default","parallel_tool_calls":false}`),
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, "test-model", got["model"])
	require.Equal(t, "user-123", got["user"])
	require.Equal(t, float64(maxTokens), got["max_tokens"])
	require.Equal(t, temp, got["temperature"])
	require.Equal(t, topP, got["top_p"])
	require.Equal(t, false, got["parallel_tool_calls"])
	require.Equal(t, "default", got["service_tier"])

	messages := got["messages"].([]any)
	require.Equal(t, "system", messages[0].(map[string]any)["role"])
	require.Equal(t, "system", messages[0].(map[string]any)["content"])
	require.Equal(t, "user", messages[1].(map[string]any)["role"])
	require.Equal(t, "say hello", messages[1].(map[string]any)["content"])

	tools := got["tools"].([]any)
	require.Len(t, tools, 1)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	require.Equal(t, "weather", fn["name"])
	require.Equal(t, false, fn["strict"])

	choice := got["tool_choice"].(map[string]any)
	require.Equal(t, "function", choice["type"])
	require.Equal(t, "weather", choice["function"].(map[string]any)["name"])

	require.Len(t, result.Warnings, 2)
	require.Equal(t, provider.ContentText, result.Content[0].Type)
	require.Equal(t, "hello", result.Content[0].Text)
	require.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
	require.Equal(t, 7, *result.Usage.InputTokens.Total)
	require.Equal(t, 5, *result.Usage.InputTokens.NoCache)
	require.Equal(t, 2, *result.Usage.InputTokens.CacheRead)
	require.Equal(t, 3, *result.Usage.OutputTokens.Total)
	require.JSONEq(t, `{"prompt_tokens":7,"completion_tokens":3,"total_tokens":10,"prompt_tokens_details":{"cached_tokens":2},"queue_time":0.061348671}`, string(result.Usage.Raw))
	require.Equal(t, "chatcmpl_1", result.Response.ID)
	require.Equal(t, time.Unix(1710000000, 0).UTC(), result.Response.Timestamp)
}

func TestDoGeneratePreservesEpochTimestamp(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"created":0,
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	require.Equal(t, time.Unix(0, 0).UTC(), result.Response.Timestamp)
}

func TestPrepareToolsStrict(t *testing.T) {
	strictTrue := true
	strictFalse := false
	tests := []struct {
		name   string
		strict *bool
	}{
		{name: "absent"},
		{name: "true", strict: &strictTrue},
		{name: "false", strict: &strictFalse},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tools, _, warnings := prepareTools([]provider.Tool{{
				Type:   provider.ToolTypeFunction,
				Name:   "weather",
				Strict: tc.strict,
			}}, nil)
			require.Len(t, tools, 1)
			require.Equal(t, tc.strict, tools[0].Function.Strict)
			require.Empty(t, warnings)
		})
	}
}

func TestDoGenerateUsageDefaultsMissingTokenTotals(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":4,"total_tokens":4}
		}`))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	require.Equal(t, 4, *result.Usage.InputTokens.Total)
	require.Equal(t, 4, *result.Usage.InputTokens.NoCache)
	require.Equal(t, 0, *result.Usage.InputTokens.CacheRead)
	require.Equal(t, 0, *result.Usage.OutputTokens.Total)
	require.Equal(t, 0, *result.Usage.OutputTokens.Text)
	require.Equal(t, 0, *result.Usage.OutputTokens.Reasoning)
	require.JSONEq(t, `{"prompt_tokens":4,"total_tokens":4}`, string(result.Usage.Raw))
}

func TestDoGenerateProviderOptionsMatchUpstreamKeys(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	model := New(
		"test-model",
		WithBaseURL(server.URL),
		WithProviderName("some-provider.chat"),
	)
	_, err := model.DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.ProviderOptions{
			"some-provider": provider.RawProviderOption{
				Key: "some-provider",
				Raw: json.RawMessage(`{"textVerbosity":"medium","customOption":"raw-value"}`),
			},
			"someProvider": provider.RawProviderOption{
				Key: "someProvider",
				Raw: json.RawMessage(`{"textVerbosity":"low","reasoningEffort":"high","customOption":"camel-value"}`),
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, "low", got["verbosity"])
	require.Equal(t, "high", got["reasoning_effort"])
	require.Equal(t, "camel-value", got["customOption"])
	require.NotContains(t, got, "textVerbosity")
	require.NotContains(t, got, "reasoningEffort")
}

func TestDoGenerateDeprecatedProviderOptionsKeyWarning(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.ProviderOptions{
			"openai-compatible": provider.RawProviderOption{
				Key: "openai-compatible",
				Raw: json.RawMessage(`{"user":"deprecated-user"}`),
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, "deprecated-user", got["user"])
	require.Equal(t, []provider.Warning{
		{
			Type:    provider.WarnDeprecated,
			Setting: "providerOptions key 'openai-compatible'",
			Message: "Use 'openaiCompatible' instead.",
		},
	}, result.Warnings)
}

func TestDoGenerateRawProviderOptionsKeyWarning(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	result, err := New(
		"test-model",
		WithBaseURL(server.URL),
		WithProviderName("test-provider.chat"),
	).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.ProviderOptions{
			"test-provider": provider.RawProviderOption{
				Key: "test-provider",
				Raw: json.RawMessage(`{"reasoningEffort":"high","someCustomOption":"test-value"}`),
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, "high", got["reasoning_effort"])
	require.Equal(t, "test-value", got["someCustomOption"])
	require.Equal(t, []provider.Warning{
		{
			Type:    provider.WarnDeprecated,
			Setting: "providerOptions key 'test-provider'",
			Message: "Use 'testProvider' instead.",
		},
	}, result.Warnings)
}

func TestDoGenerateOpenAIOptionsUseCanonicalProviderKey(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3,"completion_tokens_details":{"accepted_prediction_tokens":4,"rejected_prediction_tokens":5}}
		}`))
	}))
	defer server.Close()

	opts := provider.BuildProviderOptions(OpenAIOptions{TextVerbosity: "low"})
	require.Contains(t, opts, "openaiCompatible")
	require.NotContains(t, opts, "openai-compatible")

	result, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt:          []provider.Message{provider.UserText("hi")},
		ProviderOptions: opts,
	})
	require.NoError(t, err)

	require.Equal(t, "low", got["verbosity"])
	require.Contains(t, result.ProviderMetadata, "openaiCompatible")
	require.NotContains(t, result.ProviderMetadata, "openai-compatible")

	var responseMeta struct {
		AcceptedPredictionTokens int `json:"acceptedPredictionTokens"`
		RejectedPredictionTokens int `json:"rejectedPredictionTokens"`
	}
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["openaiCompatible"], &responseMeta))
	require.Equal(t, 4, responseMeta.AcceptedPredictionTokens)
	require.Equal(t, 5, responseMeta.RejectedPredictionTokens)
}

func TestDoGenerateSendsGoogleThoughtSignature(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	toolCall := provider.ToolCallPart("function-call-1", "check_flight", json.RawMessage(`{"flight":"AA100"}`))
	toolCall.ProviderOptions = provider.ProviderOptions{
		"google": provider.RawProviderOption{
			Key: "google",
			Raw: json.RawMessage(`{"thoughtSignature":"<Signature A>"}`),
		},
	}

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.NewAssistantMessage(toolCall)},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)
	message := messages[0].(map[string]any)
	toolCalls := message["tool_calls"].([]any)
	extra := toolCalls[0].(map[string]any)["extra_content"].(map[string]any)
	google := extra["google"].(map[string]any)
	require.Equal(t, "<Signature A>", google["thought_signature"])
}

func TestDoGenerateSendsOpenAICompatibleMessageMetadata(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	system := provider.NewSystemMessage("system")
	system.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"cacheControl":{"type":"ephemeral"}}`),
		},
	}

	userPart := provider.TextPart("hello")
	userPart.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"priority":"high"}`),
		},
	}

	toolCall := provider.ToolCallPart("function-call-1", "check_flight", json.RawMessage(`{"flight":"AA100"}`))
	toolCall.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"toolPriority":"critical","extra_content":{"google":{"thought_signature":"wrong"}}}`),
		},
		"google": provider.RawProviderOption{
			Key: "google",
			Raw: json.RawMessage(`{"thoughtSignature":"<Signature A>"}`),
		},
	}
	assistant := provider.NewAssistantMessage(toolCall)
	assistant.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"globalPriority":"high"}`),
		},
	}

	toolResult := provider.ToolResultPart("function-call-1", "check_flight", &provider.ToolResultOutput{
		Type: provider.ToolOutputJSON,
		JSON: json.RawMessage(`{"status":"delayed"}`),
	})
	toolResult.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"partial":true}`),
		},
	}

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			system,
			provider.NewUserMessage(userPart),
			assistant,
			provider.NewToolMessage(toolResult),
		},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)

	systemMessage := messages[0].(map[string]any)
	require.Equal(t, map[string]any{"type": "ephemeral"}, systemMessage["cacheControl"])

	userMessage := messages[1].(map[string]any)
	require.Equal(t, "hello", userMessage["content"])
	require.Equal(t, "high", userMessage["priority"])

	assistantMessage := messages[2].(map[string]any)
	require.Nil(t, assistantMessage["content"])
	require.Equal(t, "high", assistantMessage["globalPriority"])
	toolCalls := assistantMessage["tool_calls"].([]any)
	toolCallMessage := toolCalls[0].(map[string]any)
	require.Equal(t, "critical", toolCallMessage["toolPriority"])
	extra := toolCallMessage["extra_content"].(map[string]any)
	google := extra["google"].(map[string]any)
	require.Equal(t, "<Signature A>", google["thought_signature"])

	toolMessage := messages[3].(map[string]any)
	require.Equal(t, true, toolMessage["partial"])
}

func TestDoGenerateSingleTextUserMessageUsesPartMetadataOnly(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	part := provider.TextPart("hello")
	part.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"priority":"part"}`),
		},
	}
	msg := provider.NewUserMessage(part)
	msg.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{
			Key: "openaiCompatible",
			Raw: json.RawMessage(`{"cacheControl":{"type":"ephemeral"},"priority":"message"}`),
		},
	}

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{msg},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)
	userMessage := messages[0].(map[string]any)
	require.Equal(t, "hello", userMessage["content"])
	require.Equal(t, "part", userMessage["priority"])
	require.NotContains(t, userMessage, "cacheControl")
}

func TestDoGenerateSendsEmptyTextInMultipartUserMessage(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.TextPart(""),
				provider.TextPart("next"),
			),
		},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	first := content[0].(map[string]any)
	require.Equal(t, "text", first["type"])
	text, ok := first["text"]
	require.True(t, ok)
	require.Equal(t, "", text)
	require.Equal(t, "next", content[1].(map[string]any)["text"])
}

func TestDoGenerateResolvesInlineWildcardImageMediaType(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	pngBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	encodedPNG := base64.StdEncoding.EncodeToString(pngBytes)
	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.FilePart("image", provider.DataContent{Bytes: pngBytes}),
				provider.FilePart("image/*", provider.DataContent{Base64: encodedPNG}),
			),
		},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	for _, part := range content {
		imageURL := part.(map[string]any)["image_url"].(map[string]any)
		require.Equal(t, "data:image/png;base64,"+encodedPNG, imageURL["url"])
	}
}

func TestDoGenerateResolvesInlineWildcardAudioAndPDFMediaTypes(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	wavBytes := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x41, 0x56, 0x45}
	mp3Bytes := []byte{0xff, 0xfb, 0x90, 0x64}
	pdfBytes := []byte{0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x34}
	encodedWAV := base64.StdEncoding.EncodeToString(wavBytes)
	encodedMP3 := base64.StdEncoding.EncodeToString(mp3Bytes)
	encodedPDF := base64.StdEncoding.EncodeToString(pdfBytes)

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.FilePart("audio", provider.DataContent{Bytes: wavBytes}),
				provider.FilePart("audio/*", provider.DataContent{Base64: encodedMP3}),
				provider.FilePart("application", provider.DataContent{Bytes: pdfBytes}),
			),
		},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)

	wav := content[0].(map[string]any)["input_audio"].(map[string]any)
	require.Equal(t, "wav", wav["format"])
	require.Equal(t, encodedWAV, wav["data"])

	mp3 := content[1].(map[string]any)["input_audio"].(map[string]any)
	require.Equal(t, "mp3", mp3["format"])
	require.Equal(t, encodedMP3, mp3["data"])

	file := content[2].(map[string]any)["file"].(map[string]any)
	require.Equal(t, "document.pdf", file["filename"])
	require.Equal(t, "data:application/pdf;base64,"+encodedPDF, file["file_data"])
}

func TestDoGenerateParsesAudioAndPDFDataURLFileParts(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_1",
			"model":"test-model",
			"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]
		}`))
	}))
	defer server.Close()

	wavBytes := []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x41, 0x56, 0x45}
	pdfBytes := []byte{0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x34}
	encodedWAV := base64.StdEncoding.EncodeToString(wavBytes)
	encodedPDF := base64.StdEncoding.EncodeToString(pdfBytes)

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{
			provider.NewUserMessage(
				provider.FilePart("audio/*", provider.DataContent{URL: "data:audio/wav;base64," + encodedWAV}),
				provider.FilePart("application", provider.DataContent{URL: "data:application/pdf;base64," + encodedPDF}),
			),
		},
	})
	require.NoError(t, err)

	messages := got["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)

	audio := content[0].(map[string]any)["input_audio"].(map[string]any)
	require.Equal(t, "wav", audio["format"])
	require.Equal(t, encodedWAV, audio["data"])

	file := content[1].(map[string]any)["file"].(map[string]any)
	require.Equal(t, "document.pdf", file["filename"])
	require.Equal(t, "data:application/pdf;base64,"+encodedPDF, file["file_data"])
}

func TestDoGenerateStructuredOutputAndToolCallResponse(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_tools",
			"model":"test-model",
			"choices":[{
				"message":{
					"role":"assistant",
					"tool_calls":[{
						"id":"call_1",
						"type":"function",
						"function":{"name":"weather","arguments":"{\"city\":\"Paris\"}"}
					}]
				},
				"finish_reason":"tool_calls"
			}]
		}`))
	}))
	defer server.Close()

	strict := false
	result, err := New(
		"test-model",
		WithBaseURL(server.URL),
		WithStructuredOutputs(true),
	).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("use a tool")},
		ResponseFormat: &provider.ResponseFormat{
			Type:        provider.ResponseFormatJSON,
			Name:        "weather_response",
			Description: "Weather response",
			Schema:      json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		},
		ProviderOptions: provider.BuildProviderOptions(OpenAIOptions{
			StrictJSONSchema: &strict,
		}),
	})
	require.NoError(t, err)

	format := got["response_format"].(map[string]any)
	require.Equal(t, "json_schema", format["type"])
	schema := format["json_schema"].(map[string]any)
	require.Equal(t, "weather_response", schema["name"])
	require.Equal(t, false, schema["strict"])

	require.Equal(t, provider.FinishReasonToolCalls, result.FinishReason.Unified)
	require.Len(t, result.Content, 1)
	require.Equal(t, provider.ContentToolCall, result.Content[0].Type)
	require.Equal(t, "call_1", result.Content[0].ToolCallID)
	require.Equal(t, "weather", result.Content[0].ToolName)
	require.JSONEq(t, `{"city":"Paris"}`, string(result.Content[0].Input))
}

func TestDoGenerateResponseOrderAndProviderMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_tools",
			"created":1710000000,
			"model":"test-model",
			"choices":[{
				"message":{
					"role":"assistant",
					"content":"answer",
					"reasoning_content":"thinking",
					"tool_calls":[{
						"id":"function-call-1",
						"type":"function",
						"function":{"name":"check_flight","arguments":"{\"flight\":\"AA100\"}"},
						"extra_content":{"google":{"thought_signature":"<Signature A>"}}
					}]
				},
				"finish_reason":"tool_calls"
			}],
			"usage":{
				"prompt_tokens":10,
				"completion_tokens":20,
				"total_tokens":30,
				"completion_tokens_details":{
					"reasoning_tokens":3,
					"accepted_prediction_tokens":4,
					"rejected_prediction_tokens":5
				}
			}
		}`))
	}))
	defer server.Close()

	result, err := New(
		"test-model",
		WithBaseURL(server.URL),
		WithProviderName("google.generative-ai"),
	).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	require.Len(t, result.Content, 3)
	require.Equal(t, provider.ContentText, result.Content[0].Type)
	require.Equal(t, provider.ContentReasoning, result.Content[1].Type)
	require.Equal(t, provider.ContentToolCall, result.Content[2].Type)

	var toolMeta struct {
		ThoughtSignature string `json:"thoughtSignature"`
	}
	require.NoError(t, json.Unmarshal(result.Content[2].ProviderMetadata["google"], &toolMeta))
	require.Equal(t, "<Signature A>", toolMeta.ThoughtSignature)

	var responseMeta struct {
		AcceptedPredictionTokens int `json:"acceptedPredictionTokens"`
		RejectedPredictionTokens int `json:"rejectedPredictionTokens"`
	}
	require.NoError(t, json.Unmarshal(result.ProviderMetadata["google"], &responseMeta))
	require.Equal(t, 4, responseMeta.AcceptedPredictionTokens)
	require.Equal(t, 5, responseMeta.RejectedPredictionTokens)
}

func TestDoStreamTextAndUsage(t *testing.T) {
	t.Parallel()

	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "text/event-stream", r.Header.Get("Accept"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_stream","created":1710000000,"model":"test-model","choices":[{"delta":{"role":"assistant"},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_stream","created":1710000000,"model":"test-model","choices":[{"delta":{"content":"hel"},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_stream","created":1710000000,"model":"test-model","choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"queue_time":0.061348671}}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL), WithIncludeUsage(true)).DoStream(context.Background(), provider.CallOptions{
		Prompt:           []provider.Message{provider.UserText("hi")},
		IncludeRawChunks: true,
	})
	require.NoError(t, err)

	require.Equal(t, true, got["stream"])
	require.Equal(t, map[string]any{"include_usage": true}, got["stream_options"])

	parts := collectStreamParts(result)
	require.Equal(t, provider.PartStreamStart, parts[0].Type)
	require.Equal(t, provider.PartRaw, parts[1].Type)
	require.Equal(t, provider.PartResponseMeta, parts[2].Type)
	require.Equal(t, "chatcmpl_stream", parts[2].ResponseID)
	require.Equal(t, time.Unix(1710000000, 0).UTC(), parts[2].Timestamp)
	require.Equal(t, provider.PartTextStart, parts[4].Type)
	require.Equal(t, provider.PartTextDelta, parts[5].Type)
	require.Equal(t, "hel", parts[5].Delta)
	require.Equal(t, provider.PartTextDelta, parts[7].Type)
	require.Equal(t, "lo", parts[7].Delta)
	require.Equal(t, provider.PartTextEnd, parts[len(parts)-2].Type)
	require.Equal(t, provider.PartFinish, parts[len(parts)-1].Type)
	require.Equal(t, provider.FinishReasonStop, parts[len(parts)-1].FinishReason.Unified)
	require.Equal(t, 2, *parts[len(parts)-1].Usage.InputTokens.Total)
	require.Equal(t, 3, *parts[len(parts)-1].Usage.OutputTokens.Total)
	require.JSONEq(t, `{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"queue_time":0.061348671}`, string(parts[len(parts)-1].Usage.Raw))
}

func TestDoStreamRecoversAfterMalformedChunk(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: not-json` + "\n\n" +
				`data: {"id":"chatcmpl_stream","created":0,"model":"test-model","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt:           []provider.Message{provider.UserText("hi")},
		IncludeRawChunks: true,
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	rawParts := findParts(parts, provider.PartRaw)
	require.Len(t, rawParts, 2)
	assert.Empty(t, rawParts[0].RawValue)
	_, err = json.Marshal(rawParts[0])
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"chatcmpl_stream","created":0,"model":"test-model","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, string(rawParts[1].RawValue))

	var sawError, sawText bool
	for _, part := range parts {
		switch part.Type {
		case provider.PartError:
			sawError = true
		case provider.PartTextDelta:
			sawText = sawText || part.Delta == "ok"
		}
	}
	assert.True(t, sawError)
	assert.True(t, sawText)
	responseMeta := findPart(parts, provider.PartResponseMeta)
	require.Equal(t, provider.PartResponseMeta, responseMeta.Type)
	assert.Equal(t, time.Unix(0, 0).UTC(), responseMeta.Timestamp)
	finish := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, finish.Type)
	assert.Equal(t, provider.FinishReasonStop, finish.FinishReason.Unified)
	require.NotNil(t, finish.Usage)
	assert.Nil(t, finish.Usage.InputTokens.Total)
	assert.Nil(t, finish.Usage.OutputTokens.Total)
}

func TestDoStreamRecoversAfterStructurallyInvalidChunk(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		chunk string
	}{
		{name: "missing choices", chunk: `{}`},
		{name: "null choices", chunk: `{"choices":null}`},
		{name: "null choice", chunk: `{"choices":[null]}`},
		{name: "invalid role", chunk: `{"choices":[{"delta":{"role":"user"}}]}`},
		{name: "tool call missing function", chunk: `{"choices":[{"delta":{"tool_calls":[{"id":"call_1"}]}}]}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(
					"data: " + tc.chunk + "\n\n" +
						`data: {"id":"chatcmpl_stream","model":"test-model","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n" +
						"data: [DONE]\n\n",
				))
			}))
			defer server.Close()

			result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("hi")},
			})
			require.NoError(t, err)

			parts := collectStreamParts(result)
			require.Len(t, parts, 7)
			assert.Equal(t, provider.PartStreamStart, parts[0].Type)
			assert.Equal(t, provider.PartError, parts[1].Type)
			assert.Equal(t, provider.PartResponseMeta, parts[2].Type)
			assert.Equal(t, provider.PartTextDelta, parts[4].Type)
			assert.Equal(t, "ok", parts[4].Delta)
			require.Equal(t, provider.PartFinish, parts[6].Type)
			assert.Equal(t, provider.FinishReasonStop, parts[6].FinishReason.Unified)
		})
	}
}

func TestDoStreamRawChunkPreservesStructurallyInvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt:           []provider.Message{provider.UserText("hi")},
		IncludeRawChunks: true,
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	rawParts := findParts(parts, provider.PartRaw)
	require.Len(t, rawParts, 1)
	assert.JSONEq(t, `{}`, string(rawParts[0].RawValue))
	require.Len(t, findParts(parts, provider.PartError), 1)
}

func TestDoStreamRawProviderOptionsKeyWarning(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_stream","created":1710000000,"model":"test-model","choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New(
		"test-model",
		WithBaseURL(server.URL),
		WithProviderName("test-provider.chat"),
	).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
		ProviderOptions: provider.ProviderOptions{
			"test-provider": provider.RawProviderOption{
				Key: "test-provider",
				Raw: json.RawMessage(`{"reasoningEffort":"high"}`),
			},
		},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	require.Equal(t, provider.PartStreamStart, parts[0].Type)
	require.Equal(t, []provider.Warning{
		{
			Type:    provider.WarnDeprecated,
			Setting: "providerOptions key 'test-provider'",
			Message: "Use 'testProvider' instead.",
		},
	}, parts[0].Warnings)
}

func TestDoStreamUsageClampsNegativeTextTokens(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_stream","model":"grok-3-mini","choices":[{"delta":{"content":"hi"},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_stream","model":"grok-3-mini","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":2,"total_tokens":354,"prompt_tokens_details":{"cached_tokens":11},"completion_tokens_details":{"reasoning_tokens":340},"cost_in_usd_ticks":1721250}}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL), WithIncludeUsage(true)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	finish := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, finish.Type)
	require.Equal(t, 12, *finish.Usage.InputTokens.Total)
	require.Equal(t, 1, *finish.Usage.InputTokens.NoCache)
	require.Equal(t, 11, *finish.Usage.InputTokens.CacheRead)
	require.Equal(t, 2, *finish.Usage.OutputTokens.Total)
	require.Equal(t, 0, *finish.Usage.OutputTokens.Text)
	require.Equal(t, 340, *finish.Usage.OutputTokens.Reasoning)
	require.JSONEq(t, `{"prompt_tokens":12,"completion_tokens":2,"total_tokens":354,"prompt_tokens_details":{"cached_tokens":11},"completion_tokens_details":{"reasoning_tokens":340},"cost_in_usd_ticks":1721250}`, string(finish.Usage.Raw))
}

func TestDoStreamUsageIgnoresTotalTokensForGenericReasoningAccounting(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_stream","model":"test-model","choices":[{"delta":{"content":"hi"},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_stream","model":"test-model","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":40,"completion_tokens_details":{"reasoning_tokens":10}}}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL), WithIncludeUsage(true)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	finish := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, finish.Type)
	require.Equal(t, 10, *finish.Usage.InputTokens.Total)
	require.Equal(t, 20, *finish.Usage.OutputTokens.Total)
	require.Equal(t, 10, *finish.Usage.OutputTokens.Text)
	require.Equal(t, 10, *finish.Usage.OutputTokens.Reasoning)
}

func TestDoStreamProcessesFinalDataLineAtEOF(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_stream","created":1710000000,"model":"test-model","choices":[{"delta":{"content":"hello"},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_stream","created":1710000000,"model":"test-model","choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL), WithIncludeUsage(true)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	finish := parts[len(parts)-1]
	require.Equal(t, provider.PartFinish, finish.Type)
	require.Equal(t, provider.FinishReasonStop, finish.FinishReason.Unified)
	require.NotNil(t, finish.Usage)
	require.Equal(t, 2, *finish.Usage.InputTokens.Total)
	require.Equal(t, 3, *finish.Usage.OutputTokens.Total)
}

func TestDoStreamStructuredError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		errorJSON   string
		wantMessage string
	}{
		{
			name:        "custom error without message",
			errorJSON:   `{"status":"failed","details":{"errorMessage":"blocked","extra":true},"note":"vendor"}`,
			wantMessage: "openai: stream error:",
		},
		{
			name:        "OpenAI error preserves unknown fields",
			errorJSON:   `{"message":"rate limited","type":"rate_limit","code":"slow_down","unknown":{"retryAfter":5}}`,
			wantMessage: "openai: rate limited",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			frame := `{"error":` + tc.errorJSON + `}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Provider", "test")
				_, _ = w.Write([]byte("data: " + frame + "\n\n"))
			}))
			defer server.Close()

			result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
				Prompt:           []provider.Message{provider.UserText("hi")},
				IncludeRawChunks: true,
			})
			require.NoError(t, err)

			parts := collectStreamParts(result)
			require.Len(t, findParts(parts, provider.PartRaw), 1)
			errors := findParts(parts, provider.PartError)
			require.Len(t, errors, 1)
			require.NotNil(t, errors[0].APICallError)
			apiErr := errors[0].APICallError
			require.Contains(t, apiErr.Message, tc.wantMessage)
			require.Equal(t, server.URL+"/chat/completions", apiErr.URL)
			require.False(t, apiErr.IsRetryable)
			require.Equal(t, []string{"test"}, apiErr.ResponseHeaders["X-Provider"])
			require.JSONEq(t, tc.errorJSON, string(apiErr.Data))
			require.JSONEq(t, frame, apiErr.ResponseBody)
			require.Contains(t, string(apiErr.RequestBodyValues), `"model":"test-model"`)
			require.Equal(t, provider.FinishReasonError, parts[len(parts)-1].FinishReason.Unified)
		})
	}
}

func TestDoStreamStructuredErrorContinues(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"error":{"message":"transient provider event","code":"NOTICE"}}` + "\n\n" +
				`data: {"id":"chatcmpl_stream","model":"test-model","choices":[{"delta":{"content":"recovered"},"finish_reason":"stop"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.NoError(t, err)
	parts := collectStreamParts(result)
	require.Len(t, findParts(parts, provider.PartError), 1)
	text := findPart(parts, provider.PartTextDelta)
	require.Equal(t, "recovered", text.Delta)
	require.Equal(t, provider.FinishReasonStop, parts[len(parts)-1].FinishReason.Unified)
}

func TestDoStreamToolCallDeltas(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"arguments":"{\"city\""}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"weather","arguments":":\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("weather")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	requireContainsPart(t, parts, provider.PartToolInputStart)
	requireContainsPart(t, parts, provider.PartToolInputDelta)
	requireContainsPart(t, parts, provider.PartToolInputEnd)
	requireContainsPart(t, parts, provider.PartToolCall)

	toolCall := findPart(parts, provider.PartToolCall)
	require.Equal(t, "call_1", toolCall.ToolCallID)
	require.Equal(t, "weather", toolCall.ToolName)
	require.JSONEq(t, `{"city":"Paris"}`, toolCall.Input)
	require.Equal(t, provider.FinishReasonToolCalls, parts[len(parts)-1].FinishReason.Unified)
}

func TestDoStreamToolCallIndexTracking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		chunks   string
		expected []provider.StreamPart
	}{
		{
			name: "non-zero index",
			chunks: `data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
			expected: []provider.StreamPart{{ToolCallID: "call_1", ToolName: "read_file", Input: `{"path":"a.txt"}`}},
		},
		{
			name: "reused index",
			chunks: `data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"first","arguments":"{\"value\":1}"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_2","type":"function","function":{"name":"second","arguments":"{\"value\":2}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
			expected: []provider.StreamPart{
				{ToolCallID: "call_1", ToolName: "first", Input: `{"value":1}`},
				{ToolCallID: "call_2", ToolName: "second", Input: `{"value":2}`},
			},
		},
		{
			name: "omitted continuation index",
			chunks: `data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":7,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"pa"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"function":{"arguments":"th\":\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
			expected: []provider.StreamPart{{ToolCallID: "call_1", ToolName: "read_file", Input: `{"path":"a.txt"}`}},
		},
		{
			name: "late id with index",
			chunks: `data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
			expected: []provider.StreamPart{{ToolCallID: "call_1", ToolName: "read_file", Input: `{"path":"a.txt"}`}},
		},
		{
			name:     "empty id",
			chunks:   `data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"","type":"function","function":{"name":"read_file","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
			expected: []provider.StreamPart{{ToolCallID: "", ToolName: "read_file", Input: `{}`}},
		},
		{
			name:     "empty name",
			chunks:   `data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n",
			expected: []provider.StreamPart{{ToolCallID: "call_1", ToolName: "", Input: `{}`}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte(tc.chunks + `data: [DONE]` + "\n\n"))
			}))
			defer server.Close()

			result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
				Prompt: []provider.Message{provider.UserText("use tools")},
			})
			require.NoError(t, err)

			parts := collectStreamParts(result)
			toolCalls := findParts(parts, provider.PartToolCall)
			require.Len(t, toolCalls, len(tc.expected))
			for i, expected := range tc.expected {
				assert.Equal(t, expected.ToolCallID, toolCalls[i].ToolCallID)
				assert.Equal(t, expected.ToolName, toolCalls[i].ToolName)
				assert.JSONEq(t, expected.Input, toolCalls[i].Input)
			}
			finish := parts[len(parts)-1]
			require.NotNil(t, finish.FinishReason)
			assert.Equal(t, provider.FinishReasonToolCalls, finish.FinishReason.Unified)
		})
	}
}

func TestDoStreamRejectsUnindexedToolCallWithoutName(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"arguments":"{\"path\":"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"read_file","arguments":"\"a.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("use tools")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	errors := findParts(parts, provider.PartError)
	require.Len(t, errors, 1)
	require.NotNil(t, errors[0].APICallError)
	assert.Contains(t, errors[0].APICallError.Message, "missing function name")
	assert.Empty(t, findParts(parts, provider.PartToolCall))
	finish := parts[len(parts)-1]
	require.NotNil(t, finish.FinishReason)
	assert.Equal(t, provider.FinishReasonError, finish.FinishReason.Unified)
}

func TestDoStreamRejectsUnindexedToolCallWithoutIDOrName(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"type":"function","function":{"arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("use tools")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	errors := findParts(parts, provider.PartError)
	require.Len(t, errors, 1)
	require.NotNil(t, errors[0].APICallError)
	assert.Contains(t, errors[0].APICallError.Message, "missing id")
	assert.Empty(t, findParts(parts, provider.PartToolCall))
	finish := parts[len(parts)-1]
	require.NotNil(t, finish.FinishReason)
	assert.Equal(t, provider.FinishReasonError, finish.FinishReason.Unified)
}

func TestDoStreamSkipsInitialEmptyToolInputDelta(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"weather","arguments":""}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("weather")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	toolInputDeltas := findParts(parts, provider.PartToolInputDelta)
	require.Len(t, toolInputDeltas, 1)
	require.Equal(t, `{"city":"Paris"}`, toolInputDeltas[0].Delta)

	toolCall := findPart(parts, provider.PartToolCall)
	require.Equal(t, "call_1", toolCall.ToolCallID)
	require.JSONEq(t, `{"city":"Paris"}`, toolCall.Input)
}

func TestDoStreamDoesNotFinishToolCallBeforeFlush(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"query\":\"test\"}"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":""}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"content":"after"},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"limit\":10}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("search")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	toolCallIndex := findPartIndex(parts, provider.PartToolCall)
	textEndIndex := findPartIndex(parts, provider.PartTextEnd)
	require.NotEqual(t, -1, toolCallIndex)
	require.NotEqual(t, -1, textEndIndex)
	require.Less(t, textEndIndex, toolCallIndex)

	toolInputDeltas := findParts(parts, provider.PartToolInputDelta)
	require.Len(t, toolInputDeltas, 3)
	require.Equal(t, "", toolInputDeltas[1].Delta)

	toolCall := parts[toolCallIndex]
	require.Equal(t, "call_1", toolCall.ToolCallID)
	require.Equal(t, "search", toolCall.ToolName)
	require.Equal(t, `{"query":"test"},"limit":10}`, toolCall.Input)
}

func TestDoStreamMissingToolCallIDEmitsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"name":"weather","arguments":""}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"type":"function","function":{"arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("weather")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	errors := findParts(parts, provider.PartError)
	toolCalls := findParts(parts, provider.PartToolCall)
	finish := parts[len(parts)-1]

	require.Len(t, errors, 1)
	require.Contains(t, errors[0].APICallError.Message, "missing id")
	require.Empty(t, toolCalls)
	require.Equal(t, provider.PartFinish, finish.Type)
	require.Equal(t, provider.FinishReasonError, finish.FinishReason.Unified)
}

func TestDoStreamUnindexedParallelToolCalls(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"id":"call_paris","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Paris\"}"}},{"id":"call_london","type":"function","function":{"name":"weather","arguments":"{\"city\":\"London\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("weather")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	toolCalls := findParts(parts, provider.PartToolCall)
	require.Len(t, toolCalls, 2)
	require.Equal(t, "call_paris", toolCalls[0].ToolCallID)
	require.Equal(t, "weather", toolCalls[0].ToolName)
	require.JSONEq(t, `{"city":"Paris"}`, toolCalls[0].Input)
	require.Equal(t, "call_london", toolCalls[1].ToolCallID)
	require.Equal(t, "weather", toolCalls[1].ToolName)
	require.JSONEq(t, `{"city":"London"}`, toolCalls[1].Input)
	require.Equal(t, provider.FinishReasonToolCalls, parts[len(parts)-1].FinishReason.Unified)
}

func TestDoStreamDistinctUnindexedToolCallsAcrossChunks(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"id":"call_paris","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Paris\"}"}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"id":"call_london","type":"function","function":{"name":"weather","arguments":"{\"city\":\"London\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New("test-model", WithBaseURL(server.URL)).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("weather")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	toolCalls := findParts(parts, provider.PartToolCall)
	require.Len(t, toolCalls, 2)
	require.Equal(t, "call_paris", toolCalls[0].ToolCallID)
	require.JSONEq(t, `{"city":"Paris"}`, toolCalls[0].Input)
	require.Equal(t, "call_london", toolCalls[1].ToolCallID)
	require.JSONEq(t, `{"city":"London"}`, toolCalls[1].Input)
	require.Equal(t, provider.FinishReasonToolCalls, parts[len(parts)-1].FinishReason.Unified)
}

func TestDoStreamToolCallAndFinishProviderMetadata(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"id":"function-call-1","type":"function","function":{"name":"check_flight","arguments":""},"extra_content":{"google":{"thought_signature":"<Signature A>"}}}]},"finish_reason":null}]}` + "\n\n" +
				`data: {"id":"chatcmpl_tools","model":"test-model","choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"flight\":\"AA100\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"completion_tokens_details":{"accepted_prediction_tokens":4,"rejected_prediction_tokens":5}}}` + "\n\n" +
				`data: [DONE]` + "\n\n",
		))
	}))
	defer server.Close()

	result, err := New(
		"test-model",
		WithBaseURL(server.URL),
		WithProviderName("google.generative-ai"),
	).DoStream(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("weather")},
	})
	require.NoError(t, err)

	parts := collectStreamParts(result)
	toolCall := findPart(parts, provider.PartToolCall)
	require.Equal(t, "function-call-1", toolCall.ToolCallID)

	var toolMeta struct {
		ThoughtSignature string `json:"thoughtSignature"`
	}
	require.NoError(t, json.Unmarshal(toolCall.ProviderMetadata["google"], &toolMeta))
	require.Equal(t, "<Signature A>", toolMeta.ThoughtSignature)

	finish := parts[len(parts)-1]
	var responseMeta struct {
		AcceptedPredictionTokens int `json:"acceptedPredictionTokens"`
		RejectedPredictionTokens int `json:"rejectedPredictionTokens"`
	}
	require.NoError(t, json.Unmarshal(finish.ProviderMetadata["google"], &responseMeta))
	require.Equal(t, 4, responseMeta.AcceptedPredictionTokens)
	require.Equal(t, 5, responseMeta.RejectedPredictionTokens)
}

func TestDoGenerateAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error","code":"rate_limit"}}`))
	}))
	defer server.Close()

	_, err := New("test-model", WithBaseURL(server.URL)).DoGenerate(context.Background(), provider.CallOptions{
		Prompt: []provider.Message{provider.UserText("hi")},
	})
	require.Error(t, err)

	var apiErr *provider.APICallError
	require.True(t, errors.As(err, &apiErr))
	require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
	require.True(t, apiErr.IsRetryable)
	require.Contains(t, apiErr.Message, "rate limited")
	require.JSONEq(t, `{"message":"rate limited","type":"rate_limit_error","code":"rate_limit"}`, string(apiErr.Data))
}

func collectStreamParts(result *provider.StreamResult) []provider.StreamPart {
	var parts []provider.StreamPart
	for part := range result.Stream {
		parts = append(parts, part)
	}
	return parts
}

func requireContainsPart(t *testing.T, parts []provider.StreamPart, partType provider.StreamPartType) {
	t.Helper()
	for _, part := range parts {
		if part.Type == partType {
			return
		}
	}
	require.Failf(t, "missing stream part", "missing %s in %#v", partType, parts)
}

func findPart(parts []provider.StreamPart, partType provider.StreamPartType) provider.StreamPart {
	for _, part := range parts {
		if part.Type == partType {
			return part
		}
	}
	return provider.StreamPart{}
}

func findPartIndex(parts []provider.StreamPart, partType provider.StreamPartType) int {
	for i, part := range parts {
		if part.Type == partType {
			return i
		}
	}
	return -1
}

func findParts(parts []provider.StreamPart, partType provider.StreamPartType) []provider.StreamPart {
	var matches []provider.StreamPart
	for _, part := range parts {
		if part.Type == partType {
			matches = append(matches, part)
		}
	}
	return matches
}
