package openaicompatible

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeFileEncodingOptIn(t *testing.T) {
	filename := "note.txt"
	for _, enabled := range []bool{false, true} {
		part := provider.FilePart("text/plain", provider.Base64DataContent("aGVsbG8="))
		part.Filename = &filename
		if enabled {
			part.ProviderOptions = provider.BuildProviderOptions(ChatCompletionFileOptions{PreserveNativeFile: true})
		}
		got, err := convertFileContent(part, "custom")
		require.NoError(t, err)
		raw, err := json.Marshal(got)
		require.NoError(t, err)
		if enabled {
			assert.JSONEq(t, `{"type":"file","file":{"filename":"note.txt","file_data":"data:text/plain;base64,aGVsbG8="}}`, string(raw))
		} else {
			assert.JSONEq(t, `{"type":"text","text":"hello"}`, string(raw))
		}
	}
	part := provider.FilePart("application/pdf", provider.ReferenceDataContent(json.RawMessage(`{"custom":"file-native"}`)))
	part.Filename = &filename
	part.ProviderOptions = provider.BuildProviderOptions(ChatCompletionFileOptions{PreserveNativeFile: true})
	got, err := convertFileContent(part, "custom")
	require.NoError(t, err)
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"file","file":{"filename":"note.txt","file_id":"file-native"}}`, string(raw))
	part.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"preserveNativeFile":"true"}`)}}
	_, err = convertFileContent(part, "custom")
	require.Error(t, err)
}

func TestLocalCodecFlagsAreConsumedCaseInsensitively(t *testing.T) {
	part := provider.FilePart("text/plain", provider.Base64DataContent("aGVsbG8="))
	part.ProviderOptions = provider.ProviderOptions{"openaiCompatible": provider.RawProviderOption{Key: "openaiCompatible", Raw: json.RawMessage(`{"PreserveNativeFile":true}`)}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotContains(t, body, "PreserveChatCompletionMetadata")
		assert.JSONEq(t, `[{"role":"user","content":[{"type":"file","file":{"file_data":"data:text/plain;base64,aGVsbG8="}}]}]`, string(body["messages"]))
		assert.JSONEq(t, `"native"`, string(body["VendorField"]))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	model := New("native", WithBaseURL(server.URL), WithProviderName("vendor"))
	_, err := model.DoGenerate(t.Context(), provider.CallOptions{Prompt: []provider.Message{{Role: provider.RoleUser, Content: []provider.ContentPart{part}}}, ProviderOptions: provider.ProviderOptions{"vendor": provider.RawProviderOption{Key: "vendor", Raw: json.RawMessage(`{"PreserveChatCompletionMetadata":true,"VendorField":"native"}`)}}})
	require.NoError(t, err)
}
