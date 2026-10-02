package v4

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nativeWarnings() []provider.Warning {
	return []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "native model ☃", Details: "limit=42", Message: "inactive"},
		{Type: provider.WarnCompatibility, Feature: "", Details: "", Setting: "inactive"},
		{Type: provider.WarnDeprecated, Setting: "", Message: "use native setting", Details: "inactive"},
		{Type: provider.WarnOther, Message: "", Feature: "inactive"},
		{Type: provider.WarnOther, Message: "ordinary token-looking text sk-application"},
	}
}

const nativeWarningsJSON = `[{"type":"unsupported","feature":"native model ☃","details":"limit=42"},{"type":"compatibility","feature":""},{"type":"deprecated","setting":"","message":"use native setting"},{"type":"other","message":""},{"type":"other","message":"ordinary token-looking text sk-application"}]`

func TestNativeResponse_Warnings(t *testing.T) {
	t.Run("unary", func(t *testing.T) {
		result := validGenerateResult()
		result.Warnings = nativeWarnings()
		mapped, err := mapUnarySuccess(result, 4096)
		require.NoError(t, err)
		body, ok := encodeUnarySuccess(mapped, 4096)
		require.True(t, ok)
		var value map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(body, &value))
		assert.JSONEq(t, nativeWarningsJSON, string(value["warnings"]))
	})
	t.Run("stream", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: nativeWarnings()}, finishPart())}, nil
		}
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		first := strings.TrimPrefix(strings.Split(body, "\n\n")[0], "data: ")
		assert.JSONEq(t, `{"type":"stream-start","warnings":`+nativeWarningsJSON+`}`, first)
	})
}

func TestNativeResponse_Sources(t *testing.T) {
	for _, id := range []string{"native-id", "", strings.Repeat("i", 1025)} {
		t.Run("id length "+strconv.Itoa(len(id)), func(t *testing.T) {
			parts := []provider.GenerateContentPart{
				{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: id, URL: "https://example.com", Title: "native"},
				{Type: provider.ContentSource, SourceType: provider.SourceTypeURL, ID: id, URL: "https://example.com"},
				{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: id, MediaType: "text/plain", Title: "file-native", Text: "legacy", Filename: "file-native", ProviderMetadata: provider.ProviderMetadata{"openai": json.RawMessage(`{"type":"file_path","index":0}`)}},
				{Type: provider.ContentSource, SourceType: provider.SourceTypeDocument, ID: id},
			}
			result := validGenerateResult()
			result.Content = parts
			mapped, err := mapUnarySuccess(result, 16384)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 16384)
			require.True(t, ok)
			var value struct {
				Content []map[string]any `json:"content"`
			}
			require.NoError(t, json.Unmarshal(body, &value))
			require.Len(t, value.Content, 4)
			for _, source := range value.Content {
				assert.Equal(t, id, source["id"])
			}
			assert.Equal(t, "file-native", value.Content[2]["title"])
			assert.Equal(t, "file-native", value.Content[2]["filename"])
			assert.Equal(t, "", value.Content[3]["title"])
			assert.NotContains(t, value.Content[3], "filename")
			harness := newRuntimeHarness(t, testLimits())
			streamParts := make([]provider.StreamPart, 0, len(parts)+1)
			for _, part := range parts {
				source := unarySource(part)
				streamParts = append(streamParts, provider.StreamPart{Type: provider.PartSource, Source: &source})
			}
			streamParts = append(streamParts, finishPart())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(streamParts...)}, nil
			}
			frames := strings.Split(strings.TrimSpace(harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()), "\n\n")
			require.Len(t, frames, 6)
			for i, source := range value.Content {
				raw, err := json.Marshal(source)
				require.NoError(t, err)
				assert.JSONEq(t, string(raw), strings.TrimPrefix(frames[i+1], "data: "))
			}
		})
	}
}

func TestNativeResponse_Identity(t *testing.T) {
	timestamp := time.Date(2026, 9, 30, 12, 34, 56, 123456789, time.FixedZone("offset", 3600))
	for _, tc := range []struct {
		name     string
		response *provider.GenerateResponse
		want     string
	}{
		{"absent", nil, ""},
		{"empty", &provider.GenerateResponse{}, `{}`},
		{"partial", &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-response"}}, `{"id":"native-response"}`},
		{"full", &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-response", ModelID: "native model/☃", Timestamp: timestamp, Provider: "not-transported"}, Headers: map[string]string{"Authorization": "not-transported"}, Body: json.RawMessage(`{"private":"not-transported"}`)}, `{"id":"native-response","modelId":"native model/☃","timestamp":"2026-09-30T11:34:56.123456789Z"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			result.Response = tc.response
			mapped, err := mapUnarySuccess(result, 4096)
			require.NoError(t, err)
			body, ok := encodeUnarySuccess(mapped, 4096)
			require.True(t, ok)
			var value map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &value))
			if tc.want == "" {
				assert.NotContains(t, value, "response")
			} else {
				assert.JSONEq(t, tc.want, string(value["response"]))
			}
			assert.NotContains(t, string(body), "not-transported")
			if tc.response == nil {
				return
			}
			harness := newRuntimeHarness(t, testLimits())
			meta := tc.response.ResponseMetadata
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: meta.ID, ModelID: meta.ModelID, Timestamp: meta.Timestamp}, finishPart())}, nil
			}
			frames := strings.Split(harness.serve(streamRequest(`{"prompt":[]}`)).Body.String(), "\n\n")
			var event map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[1], "data: ")), &event))
			assert.JSONEq(t, `"response-metadata"`, string(event["type"]))
			delete(event, "type")
			encoded, err := json.Marshal(event)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(encoded))
		})
	}
}
