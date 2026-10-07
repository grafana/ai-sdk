package service

import (
	"context"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGuardDecodeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body, action string
		present            bool
		bad                bool
		badTransform       bool
	}{
		{name: "allow", body: `{"action":"allow"}`, action: "allow"},
		{name: "deny first", body: `{"action":"deny","reason":[],"evaluations":42,"transformed_input":null}`, action: "deny", present: true},
		{name: "deny malformed Unicode diagnostic", body: `{"action":"deny","reason":"\ud800"}`, action: "deny"},
		{name: "deny duplicate diagnostic members", body: `{"action":"deny","evaluations":[{"passed":true,"passed":false}]}`, action: "deny"},
		{name: "allow null reason", body: `{"action":"allow","reason":null}`, action: "allow"},
		{name: "allow malformed evaluations", body: `{"action":"allow","evaluations":[{"passed":"yes"}]}`, action: "allow"},
		{name: "null transform retained", body: `{"action":"allow","transformed_input":null}`, action: "allow", present: true},
		{name: "unusable transform retained", body: `{"action":"allow","transformed_input":42}`, action: "allow", present: true},
		{name: "missing", body: `{}`, bad: true},
		{name: "null", body: `{"action":null}`, bad: true},
		{name: "unknown", body: `{"action":"warn"}`, bad: true},
		{name: "no normalization", body: `{"action":" Allow "}`, bad: true},
		{name: "wrong case key", body: `{"Action":"allow"}`, bad: true},
		{name: "duplicate action", body: `{"action":"deny","action":"allow"}`, bad: true},
		{name: "escaped duplicate", body: `{"action":"deny","\u0061ction":"deny"}`, bad: true},
		{name: "deny duplicate transform", body: `{"action":"deny","transformed_input":{},"transformed_input":null}`, action: "deny", present: true},
		{name: "allow duplicate transform", body: `{"action":"allow","transformed_input":{},"transformed_input":null}`, badTransform: true},
		{name: "bad diagnostics allow", body: `{"action":"allow","reason":[]}`, action: "allow"},
		{name: "duplicate diagnostics allow", body: `{"action":"allow","reason":[],"reason":42}`, action: "allow"},
		{name: "array", body: `[]`, bad: true},
		{name: "trailing", body: `{"action":"deny"}{}`, bad: true},
		{name: "broken JSON", body: `{"action":"deny","reason":`, bad: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeGuardRuntimeResponseContext(context.Background(), []byte(tc.body))
			if tc.bad {
				require.ErrorIs(t, err, errGuardResponse)
				return
			}
			if tc.badTransform {
				require.ErrorIs(t, err, errGuardTransform)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.action, string(got.action))
			assert.Equal(t, tc.present, got.transformPresent)
			if tc.present {
				assert.NotEmpty(t, got.transform)
			}
		})
	}
}

func TestGuardNumericValidation_Exactness(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		equal         bool
	}{
		{"9007199254740993", "9007199254740993.000", true},
		{"9007199254740993", "9007199254740992", false},
		{"0.1234567890123456789", "0.12345678901234568", false},
		{"1.2500", "125e-2", true},
		{"-1.25", "-1250e-3", true},
		{"-1.25", "1.25", false},
		{"-0e10000", "0.000e-10000", true},
		{"1e10000", "10e9999", true},
		{"1e-10000", "0.1e-9999", true},
		{"1e-10000", "1e-9999", false},
		{strings.Repeat("9", 4096), strings.Repeat("9", 4095) + "8", false},
	} {
		t.Run(tc.before[:min(len(tc.before), 24)]+"/"+tc.after[:min(len(tc.after), 24)], func(t *testing.T) {
			left, err := guardJSONValue([]byte(tc.before))
			require.NoError(t, err)
			right, err := guardJSONValue([]byte(tc.after))
			require.NoError(t, err)
			l, ok := left.(json.Number)
			require.True(t, ok)
			r, ok := right.(json.Number)
			require.True(t, ok)
			wantLeft, ok := new(big.Rat).SetString(tc.before)
			require.True(t, ok)
			wantRight, ok := new(big.Rat).SetString(tc.after)
			require.True(t, ok)
			assert.Equal(t, tc.equal, wantLeft.Cmp(wantRight) == 0)
			assert.Equal(t, tc.equal, guardNumber(l) == guardNumber(r))
			assert.Equal(t, tc.equal, guardSafeJSON([]byte(tc.before), []byte(tc.after)))
		})
	}
	for _, raw := range []string{"1e10001", "1e-10001", strings.Repeat("9", 4097), "01", "+1", "1.", "NaN", "1e"} {
		_, err := guardJSONValue([]byte(raw))
		assert.ErrorIs(t, err, errGuardTransform)
	}
	raw := []byte(`{"unusable":[` + strings.Repeat("1e10000,", 30000) + `1e10000]}`)
	value, err := guardJSONValue(raw)
	require.NoError(t, err)
	object, ok := value.(map[string]any)
	require.True(t, ok)
	values, ok := object["unusable"].([]any)
	require.True(t, ok)
	require.Len(t, values, 30001)
	assert.Equal(t, json.Number("1e10000"), values[30000])
	var transformed guardResponseInput
	assert.ErrorIs(t, decodeGuardJSON(raw, &transformed), errGuardTransform)
}

func TestGuardImportedResponses(t *testing.T) {
	data, err := os.ReadFile("testdata/guards/responses.json")
	require.NoError(t, err)
	var responses map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &responses))
	for name, raw := range responses {
		t.Run(name, func(t *testing.T) {
			verdict, err := decodeGuardRuntimeResponseContext(context.Background(), raw)
			require.NoError(t, err)
			assert.Equal(t, name == "allow_with_transformed_input", verdict.transformPresent)
			if verdict.transformPresent {
				var input guardResponseInput
				require.NoError(t, decodeGuardJSON(verdict.transform, &input))
				assert.JSONEq(t, `{"command":"rm -rf /tmp/cache"}`, string(input.Messages[1].Parts[1].ToolCall.InputJSON))
			}
		})
	}
}
