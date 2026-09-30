package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grafana/ai-sdk/provider"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCallerResponse_WarningsAndIdentity(t *testing.T) {
	warnings := []provider.Warning{
		{Type: provider.WarnUnsupported, Feature: "", Details: "detail", Message: "inactive"},
		{Type: provider.WarnCompatibility, Feature: "feature"},
		{Type: provider.WarnDeprecated, Setting: "", Message: "", Details: "inactive"},
		{Type: provider.WarnOther, Message: "caller warning <>&", Feature: "inactive"},
	}
	timestamp := time.Date(2026, 9, 29, 12, 30, 0, 123000000, time.FixedZone("native", 3600))
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "unary", true: "stream"}[streaming], func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			result := validGenerateResult()
			result.Warnings = warnings
			result.Response = &provider.GenerateResponse{ResponseMetadata: provider.ResponseMetadata{ID: "native-id", ModelID: "native-model", Timestamp: timestamp, Provider: "excluded-provider"}, Headers: map[string]string{"Authorization": "excluded-key"}, Body: json.RawMessage(`{"excluded":"body"}`)}
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return result, nil }
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: warnings}, provider.StreamPart{Type: provider.PartResponseMeta, ResponseID: "native-id", ModelID: "native-model", Timestamp: timestamp}, finishPart())}, nil
			}
			request := validRequest(`{"prompt":[]}`)
			if streaming {
				request = streamRequest(`{"prompt":[]}`)
			}
			response := harness.serve(request)
			require.Equal(t, http.StatusOK, response.Code)
			var warningJSON, identityJSON json.RawMessage
			if streaming {
				requireStreamBodyMatchesSchema(t, response.Body.String())
				frames := strings.Split(strings.TrimSpace(response.Body.String()), "\n\n")
				var start map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(frames[0], "data: ")), &start))
				warningJSON = start["warnings"]
				identityJSON = json.RawMessage(strings.TrimPrefix(frames[1], "data: "))
				assert.JSONEq(t, `{"type":"response-metadata","id":"native-id","modelId":"native-model","timestamp":"2026-09-29T11:30:00.123Z"}`, string(identityJSON))
			} else {
				compiled, err := schema.CompileSchema(unarySuccessSchemaJSON)
				require.NoError(t, err)
				require.NoError(t, compiled.Validate(response.Body.Bytes()))
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				warningJSON, identityJSON = body["warnings"], body["response"]
				assert.JSONEq(t, `{"id":"native-id","modelId":"native-model","timestamp":"2026-09-29T11:30:00.123Z"}`, string(identityJSON))
			}
			assert.JSONEq(t, `[{"type":"unsupported","feature":"","details":"detail"},{"type":"compatibility","feature":"feature"},{"type":"deprecated","setting":"","message":""},{"type":"other","message":"caller warning <>&"}]`, string(warningJSON))
			for _, excluded := range []string{"inactive", "excluded", "canonical/model"} {
				assert.NotContains(t, response.Body.String(), excluded)
			}
		})
	}
}

func TestCallerResponse_InvalidWarningsAndIdentity(t *testing.T) {
	invalid := string([]byte{0xff})
	for _, tc := range []struct {
		name    string
		warning provider.Warning
	}{
		{name: "unknown", warning: provider.Warning{Type: provider.WarningType("future")}},
		{name: "utf8", warning: provider.Warning{Type: provider.WarnOther, Message: invalid}},
		{name: "message bytes", warning: provider.Warning{Type: provider.WarnOther, Message: strings.Repeat("x", 4097)}},
		{name: "feature bytes", warning: provider.Warning{Type: provider.WarnUnsupported, Feature: strings.Repeat("☃", 1366)}},
		{name: "details bytes", warning: provider.Warning{Type: provider.WarnCompatibility, Details: strings.Repeat("x", 4097)}},
		{name: "setting bytes", warning: provider.Warning{Type: provider.WarnDeprecated, Setting: strings.Repeat("x", 4097)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := validGenerateResult()
			result.Warnings = []provider.Warning{tc.warning}
			_, err := mapUnarySuccess(result, 1<<20)
			require.Error(t, err)
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
				return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartStreamStart, Warnings: result.Warnings}, finishPart())}, nil
			}
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), response.Body.String())
		})
	}
	for _, metadata := range []provider.ResponseMetadata{
		{ID: invalid}, {ModelID: invalid}, {Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)},
	} {
		result := validGenerateResult()
		result.Response = &provider.GenerateResponse{ResponseMetadata: metadata}
		_, err := mapUnarySuccess(result, 1<<20)
		require.Error(t, err)
	}
	t.Run("empty identity is not canonical", func(t *testing.T) {
		harness := newRuntimeHarness(t, testLimits())
		harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
			return &provider.StreamResult{Stream: makeStream(provider.StreamPart{Type: provider.PartResponseMeta}, finishPart())}, nil
		}
		body := harness.serve(streamRequest(`{"prompt":[]}`)).Body.String()
		assert.Contains(t, body, `data: {"type":"response-metadata"}`)
		assert.NotContains(t, body, "modelId")
		requireStreamBodyMatchesSchema(t, body)
	})
	t.Run("escaped warning complete bound", func(t *testing.T) {
		result := validGenerateResult()
		result.Warnings = []provider.Warning{{Type: provider.WarnOther, Message: strings.Repeat("<", 4096)}}
		mapped, err := mapUnarySuccess(result, 1<<20)
		require.NoError(t, err)
		encoded, ok := encodeUnarySuccess(mapped, 1<<20)
		require.True(t, ok)
		_, ok = encodeUnarySuccess(mapped, int64(len(encoded)-1))
		assert.False(t, ok)
		_, err = mapUnarySuccess(result, 4095)
		require.Error(t, err)
	})
}
