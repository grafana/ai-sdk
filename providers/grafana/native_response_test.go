package grafana

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModel_NativeUnaryValues(t *testing.T) {
	body := `{"content":[{"type":"source","sourceType":"url","id":"same","url":"https://example.com"},{"type":"source","sourceType":"document","id":"same","mediaType":"text/plain","title":"file-native","filename":"file-native"},{"type":"source","sourceType":"document","id":"","mediaType":"","title":""}],"finishReason":{"unified":"stop"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[{"type":"unsupported","feature":"native","details":""},{"type":"compatibility","feature":""},{"type":"deprecated","setting":"","message":"native"},{"type":"other","message":""}],"response":{"id":"native-response","modelId":"native model ☃","timestamp":"2026-01-01T00:00:00Z"}}`
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}, nil)
	model, err := p.LanguageModel("assistant")
	require.NoError(t, err)
	result, err := model.DoGenerate(context.Background(), provider.CallOptions{})
	require.NoError(t, err)
	require.Len(t, result.Content, 3)
	assert.Equal(t, "same", result.Content[0].ID)
	assert.Equal(t, "same", result.Content[1].ID)
	assert.Empty(t, result.Content[2].ID)
	assert.Equal(t, "file-native", result.Content[1].Title)
	assert.Equal(t, "file-native", result.Content[1].Filename)
	assert.Equal(t, []provider.Warning{{Type: provider.WarnUnsupported, Feature: "native"}, {Type: provider.WarnCompatibility}, {Type: provider.WarnDeprecated, Message: "native"}, {Type: provider.WarnOther}}, result.Warnings)
	require.NotNil(t, result.Response)
	assert.JSONEq(t, body, string(result.Response.Body))
	assert.Empty(t, result.Response.ID)
	assert.Empty(t, result.Response.ModelID)
	assert.True(t, result.Response.Timestamp.IsZero())
}

func TestModel_NativeStreamIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, event, id, model string
		timestamp              time.Time
	}{
		{"absent", `{"type":"response-metadata"}`, "", "", time.Time{}},
		{"empty", `{"type":"response-metadata","id":"","modelId":""}`, "", "", time.Time{}},
		{"id only", `{"type":"response-metadata","id":"native response ☃"}`, "native response ☃", "", time.Time{}},
		{"model only", `{"type":"response-metadata","modelId":" native:model ☃ / "}`, "", " native:model ☃ / ", time.Time{}},
		{"timestamp only", `{"type":"response-metadata","timestamp":"2026-01-01T01:00:00.123456789+01:00"}`, "", "", time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(sseFrame(tc.event) + sseFrame(finishEvent))}
			model := streamFromBody(t, body, nil)
			result, err := model.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 2)
			assert.Equal(t, provider.PartResponseMeta, parts[0].Type)
			assert.Equal(t, tc.id, parts[0].ResponseID)
			assert.Equal(t, tc.model, parts[0].ModelID)
			assert.True(t, parts[0].Timestamp.Equal(tc.timestamp))
			assert.Equal(t, provider.PartFinish, parts[1].Type)
			assert.True(t, body.closed)
		})
	}
	for _, event := range []string{
		`{"type":"response-metadata","id":null}`, `{"type":"response-metadata","modelId":null}`, `{"type":"response-metadata","timestamp":null}`,
		`{"type":"response-metadata","id":1}`, `{"type":"response-metadata","modelId":{}}`, `{"type":"response-metadata","timestamp":false}`,
		`{"type":"response-metadata","timestamp":"bad"}`, `{"type":"response-metadata","modelId":"` + string([]byte{255}) + `"}`,
	} {
		t.Run(event, func(t *testing.T) {
			model := streamFromBody(t, io.NopCloser(strings.NewReader(sseFrame(event)+sseFrame(finishEvent))), nil)
			result, err := model.DoStream(context.Background(), provider.CallOptions{})
			require.NoError(t, err)
			parts := collectParts(t, result)
			require.Len(t, parts, 1)
			assert.Equal(t, provider.PartError, parts[0].Type)
		})
	}
}
