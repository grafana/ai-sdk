package openaicompatible

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserFileReferences_Request(t *testing.T) {
	tests := []struct{ name, providerName, reference, wantID string }{
		{"configured namespace", "custom-provider.chat", `{"custom-provider":"file-123","customProvider":"wrong","openai":"other"}`, "file-123"},
		{"trimmed first segment", " custom-provider .chat.extra", `{"custom-provider":"file-123"," custom-provider ":"wrong"}`, "file-123"},
		{"case preserved", "Custom-Provider.chat", `{"Custom-Provider":"file-123","custom-provider":"wrong"}`, "file-123"},
		{"underscore preserved", "custom_provider.chat", `{"custom_provider":"file-123","customProvider":"wrong"}`, "file-123"},
		{"default namespace", "", `{"openai-compatible":"file-123","openaiCompatible":"wrong"}`, "file-123"},
		{"empty ID", "custom-provider.chat", `{"custom-provider":""}`, ""},
		{"opaque ID", "custom-provider.chat", `{"custom-provider":"  file-☃/\"\\\n  "}`, "  file-☃/\"\\\n  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, mediaType := range []string{"application/pdf", "image/png", "audio/wav", "text/plain", "video/mp4", "image/*", "application/zip", ""} {
				t.Run(mediaType, func(t *testing.T) {
					part := provider.FilePart(mediaType, provider.ReferenceDataContent(json.RawMessage(tc.reference)))
					filename := "ignored.pdf"
					part.Filename = &filename
					wantID, err := json.Marshal(tc.wantID)
					require.NoError(t, err)
					checkUserFileReferenceRequest(t, tc.providerName, []provider.Message{provider.NewUserMessage(part)},
						`[{"role":"user","content":[{"type":"file","file":{"file_id":`+string(wantID)+`}}]}]`, "")
				})
			}
		})
	}
}

func TestUserFileReferences_Invalid(t *testing.T) {
	tests := []struct {
		name, reference, wantError string
		conflict                   bool
	}{
		{name: "missing namespace", reference: `{"other":"file-other"}`, wantError: `no file reference for provider "custom-provider"`},
		{name: "empty object", reference: `{}`, wantError: `no file reference for provider "custom-provider"`},
		{name: "camel case is not an alias", reference: `{"customProvider":"file-123"}`, wantError: `no file reference for provider "custom-provider"`},
		{name: "full name is not an alias", reference: `{"custom-provider.chat":"file-123"}`, wantError: `no file reference for provider "custom-provider"`},
		{name: "case is not normalized", reference: `{"Custom-Provider":"file-123"}`, wantError: `no file reference for provider "custom-provider"`},
		{name: "key whitespace is not trimmed", reference: `{" custom-provider ":"file-123"}`, wantError: `no file reference for provider "custom-provider"`},
		{name: "null reference", reference: `null`, wantError: "invalid file input"},
		{name: "array reference", reference: `[]`, wantError: "invalid file input"},
		{name: "malformed reference", reference: `{`, wantError: "invalid file input"},
		{name: "null identifier", reference: `{"custom-provider":null}`, wantError: "invalid file input"},
		{name: "numeric identifier", reference: `{"custom-provider":123}`, wantError: "invalid file input"},
		{name: "reserved type member", reference: `{"type":"file-123","custom-provider":"file-123"}`, wantError: "invalid file input"},
		{name: "conflicting data arm", reference: `{"custom-provider":"file-123"}`, conflict: true, wantError: "invalid file input"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := provider.ReferenceDataContent(json.RawMessage(tc.reference))
			if tc.conflict {
				data.URL = "https://example.com/image.png"
			}
			checkUserFileReferenceRequest(t, "custom-provider.chat", []provider.Message{provider.NewUserMessage(provider.FilePart("image/png", data))}, "", tc.wantError)
		})
	}
}

func TestUserFileReferences_MixedContentMetadata(t *testing.T) {
	reference := provider.FilePart("image/png", provider.ReferenceDataContent(json.RawMessage(`{"custom-provider":"file-123"}`)))
	reference.ProviderOptions = provider.ProviderOptions{
		"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"customOption":{"keep":true}}`)},
		"custom-provider":  provider.RawProviderOption{Key: "custom-provider", Raw: json.RawMessage(`{"ignored":true}`)},
	}
	text := provider.TextPart("before")
	text.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"tag":"text"}`)}}
	message := provider.NewUserMessage(text, reference,
		provider.FilePart("application/pdf", provider.Base64DataContent("")),
		provider.FilePart("image/png", provider.URLDataContent("https://example.com/image.png")),
		provider.FilePart("image/png", provider.BytesDataContent([]byte{1, 2})),
		provider.FilePart("text/plain", provider.URLDataContent("https://example.com/text.txt")),
		provider.TextPart("after"))
	message.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"tag":"message"}`)}}
	checkUserFileReferenceRequest(t, "custom-provider.chat", []provider.Message{message}, `[{"role":"user","tag":"message","content":[
  {"type":"text","text":"before","tag":"text"},
  {"type":"file","file":{"file_id":"file-123"},"customOption":{"keep":true}},
  {"type":"file","file":{"filename":"document.pdf","file_data":"data:application/pdf;base64,"}},
  {"type":"image_url","image_url":{"url":"https://example.com/image.png"}},
  {"type":"image_url","image_url":{"url":"data:image/png;base64,AQI="}},
  {"type":"text","text":"https://example.com/text.txt"},
  {"type":"text","text":"after"}
 ]}]`, "")
	t.Run("part metadata overrides generated file", func(t *testing.T) {
		part := reference
		part.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"file":{"file_id":"override"}}`)}}
		checkUserFileReferenceRequest(t, "custom-provider.chat", []provider.Message{provider.NewUserMessage(part)}, `[{"role":"user","content":[{"type":"file","file":{"file_id":"override"}}]}]`, "")
	})
}

func checkUserFileReferenceRequest(t *testing.T, providerName string, prompt []provider.Message, wantMessages, wantError string) {
	t.Helper()
	for _, streaming := range []bool{false, true} {
		name := "generate"
		if streaming {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			before, err := json.Marshal(prompt)
			if wantError == "" {
				require.NoError(t, err)
			}
			var got []byte
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				assert.Equal(t, "https://example.com/v1/chat/completions", req.URL.String())
				var readErr error
				got, readErr = io.ReadAll(req.Body)
				require.NoError(t, readErr)
				body := `{"id":"test","model":"test-model","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
				contentType := "application/json"
				if streaming {
					body = "data: " + `{"id":"test","model":"test-model","choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
					contentType = "text/event-stream"
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}
			options := []Option{WithBaseURL("https://example.com/v1"), WithHTTPClient(client)}
			if providerName != "" {
				options = append(options, WithProviderName(providerName))
			}
			model := New("test-model", options...)
			var requestBody json.RawMessage
			if streaming {
				result, callErr := model.DoStream(context.Background(), provider.CallOptions{Prompt: prompt})
				err = callErr
				if result != nil {
					requestBody = result.Request.Body
					parts := collectStreamParts(result)
					assert.Empty(t, findParts(parts, provider.PartError))
					assert.Equal(t, provider.FinishReasonStop, findPart(parts, provider.PartFinish).FinishReason.Unified)
				}
			} else {
				result, callErr := model.DoGenerate(context.Background(), provider.CallOptions{Prompt: prompt})
				err = callErr
				if result != nil {
					requestBody = result.Request.Body
					assert.Equal(t, provider.FinishReasonStop, result.FinishReason.Unified)
				}
			}
			if wantError != "" {
				require.ErrorContains(t, err, wantError)
				assert.Zero(t, requests)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, 1, requests)
			wantBody := `{"model":"test-model","messages":` + wantMessages
			if streaming {
				wantBody += `,"stream":true`
			}
			wantBody += `}`
			assert.JSONEq(t, wantBody, string(got))
			assert.Equal(t, string(got), string(requestBody))
			after, err := json.Marshal(prompt)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after))
		})
	}
}
