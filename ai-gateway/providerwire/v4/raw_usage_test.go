package v4

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawUsageResponses(t *testing.T) {
	native := json.RawMessage(`{"input_tokens":32,"cache_read_input_tokens":18,"service_tier":"standard","inference_geo":"us","nested":{"tokens":[1,2]}}`)
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{name: "native", raw: native, want: string(native)},
		{name: "empty object", raw: json.RawMessage(`{}`), want: `{}`},
		{name: "absent"},
		{name: "valid surrogate pair", raw: json.RawMessage(`{"nested":{"\ud83d\ude00":"\ud83d\ude00"}}`), want: `{"nested":{"😀":"😀"}}`},
		{name: "lone high surrogate", raw: json.RawMessage(`{"\ud800":1}`), want: `{"\ud800":1}`},
		{name: "lone low surrogate", raw: json.RawMessage(`{"nested":["\udc00"]}`), want: `{"nested":["\udc00"]}`},
	} {
		for _, streaming := range []bool{false, true} {
			name := "unary"
			if streaming {
				name = "stream"
			}
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				if streaming {
					harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						part := finishPart()
						part.Usage.Raw = tc.raw
						return &provider.StreamResult{Stream: makeStream(part)}, nil
					}
				} else {
					harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
						result := validGenerateResult()
						result.Usage.Raw = tc.raw
						return result, nil
					}
				}
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request = streamRequest(`{"prompt":[]}`)
				}
				response := harness.serve(request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				body := response.Body.String()
				if streaming {
					requireStreamBodyMatchesSchema(t, body)
					require.NotContains(t, body, `"code":"internal_error"`)
					body = strings.TrimSuffix(strings.Split(body, "data: ")[2], "\n\n")
				}
				var value struct {
					Usage struct {
						Raw          json.RawMessage `json:"raw"`
						InputTokens  map[string]any  `json:"inputTokens"`
						OutputTokens map[string]any  `json:"outputTokens"`
					} `json:"usage"`
				}
				require.NoError(t, json.Unmarshal([]byte(body), &value))
				assert.NotNil(t, value.Usage.InputTokens)
				assert.NotNil(t, value.Usage.OutputTokens)
				if tc.raw == nil {
					assert.Nil(t, value.Usage.Raw)
				} else {
					assert.JSONEq(t, tc.want, string(value.Usage.Raw))
					if strings.Contains(tc.name, "lone") {
						assert.Equal(t, string(tc.raw), string(value.Usage.Raw))
					}
				}
			})
		}
	}
}

func TestRawUsageBoundaries(t *testing.T) {
	payload := `{"x":"` + strings.Repeat("x", maxRawUsageBytes-len(`{"x":""}`)) + `"}`
	require.Len(t, payload, maxRawUsageBytes)
	assert.True(t, validRawUsage(json.RawMessage(payload), 2<<20))
	assert.False(t, validRawUsage(json.RawMessage(payload+" "), 2<<20))
	assert.False(t, validRawUsage(json.RawMessage(payload), int64(len(payload)-1)))

	result := validGenerateResult()
	result.Usage.Raw = json.RawMessage(payload)
	mapped, err := mapUnarySuccess(result, 2<<20)
	require.NoError(t, err)
	encoded, ok := encodeUnarySuccess(mapped, 2<<20)
	require.True(t, ok)
	_, ok = encodeUnarySuccess(mapped, int64(len(encoded)-1))
	assert.False(t, ok)

	part := finishPart()
	part.Usage.Raw = json.RawMessage(payload)
	harness := newRuntimeHarness(t, func() Limits {
		limits := testLimits()
		limits.StreamFrameBytes = 2 << 20
		return limits
	}())
	harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
		return &provider.StreamResult{Stream: makeStream(part)}, nil
	}
	response := harness.serve(streamRequest(`{"prompt":[]}`))
	require.Contains(t, response.Body.String(), `"type":"finish"`)
	assert.NotContains(t, response.Body.String(), `"code":"internal_error"`)

	frame, ok := encodeStreamFrame(streamEvent{typeName: provider.PartFinish, finishReason: *part.FinishReason, rawUsage: part.Usage.Raw}, 2<<20)
	require.True(t, ok)
	_, ok = encodeStreamFrame(streamEvent{typeName: provider.PartFinish, finishReason: *part.FinishReason, rawUsage: part.Usage.Raw}, int64(len(frame)-1))
	assert.False(t, ok)

	escaped := validGenerateResult()
	escaped.Usage.Raw = json.RawMessage(`{"html":"<>&"}`)
	mapped, err = mapUnarySuccess(escaped, 1<<20)
	require.NoError(t, err)
	encoded, ok = encodeUnarySuccess(mapped, 1<<20)
	require.True(t, ok)
	assert.Contains(t, string(encoded), `\u003c\u003e\u0026`)
	_, ok = encodeUnarySuccess(mapped, int64(len(encoded)-1))
	assert.False(t, ok)
}

func TestRawUsageRejectsInvalidProviderData(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  json.RawMessage
	}{
		{name: "null", raw: json.RawMessage(`null`)},
		{name: "scalar", raw: json.RawMessage(`12`)},
		{name: "array", raw: json.RawMessage(`[]`)},
		{name: "malformed", raw: json.RawMessage(`{"a":`)},
		{name: "trailing object", raw: json.RawMessage(`{} {}`)},
		{name: "whitespace only", raw: json.RawMessage(`  `)},
		{name: "invalid UTF-8 key", raw: append([]byte(`{"`), append([]byte{0xff}, []byte(`":1}`)...)...)},
		{name: "invalid UTF-8 nested", raw: append([]byte(`{"nested":["`), append([]byte{0xff}, []byte(`"]}`)...)...)},
		{name: "raw input ceiling", raw: json.RawMessage(`{"large":"` + strings.Repeat("x", 1<<20) + `"}`)},
	} {
		for _, streaming := range []bool{false, true} {
			name := "unary"
			if streaming {
				name = "stream"
			}
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				harness := newRuntimeHarness(t, testLimits())
				if streaming {
					harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
						part := finishPart()
						part.Usage.Raw = tc.raw
						return &provider.StreamResult{Stream: makeStream(part)}, nil
					}
				} else {
					harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
						result := validGenerateResult()
						result.Usage.Raw = tc.raw
						return result, nil
					}
				}
				request := validRequest(`{"prompt":[]}`)
				if streaming {
					request = streamRequest(`{"prompt":[]}`)
				}
				response := harness.serve(request)
				if streaming {
					assert.Equal(t, http.StatusOK, response.Code)
					assert.Equal(t, string(canonicalEmptyStartFrame)+string(canonicalInternalStreamErrorFrame), response.Body.String())
				} else {
					assert.Equal(t, http.StatusInternalServerError, response.Code)
					assert.Equal(t, string(canonicalInternalError), response.Body.String())
				}
			})
		}
	}
}
