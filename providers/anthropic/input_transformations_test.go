package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	droppedBlock = `{"type":"thinking_block_dropped","path":"messages[1].content[0]","reason":"prefix_mismatch"}`
	droppedWant  = `[{"type":"thinking_block_dropped","path":"messages[1].content[0]","reason":"prefix_mismatch"}]`
)

func anthropicMetadata(t *testing.T, metadata provider.ProviderMetadata) map[string]json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(metadata["anthropic"], &fields))
	return fields
}

func TestConvertResponse_InputTransformations(t *testing.T) {
	response := func(field string) string {
		return `{"id":"msg_1","type":"message","role":"assistant","model":"claude-fable-5-1","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}` + field + `}`
	}

	for _, tc := range []struct {
		name    string
		field   string
		want    string
		wantKey bool
	}{
		{name: "present", field: `,"input_transformations":[` + droppedBlock + `]`, want: droppedWant, wantKey: true},
		{name: "unknown fields are dropped", field: `,"input_transformations":[{"type":"t","path":"p","reason":"r","extra":{"a":1}}]`, want: `[{"type":"t","path":"p","reason":"r"}]`, wantKey: true},
		{name: "empty array is kept", field: `,"input_transformations":[]`, want: `[]`, wantKey: true},
		{name: "null is omitted", field: `,"input_transformations":null`},
		{name: "absent is omitted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := convertResponse(unmarshalMessage(t, response(tc.field)), toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
			require.NoError(t, err)
			fields := anthropicMetadata(t, result.ProviderMetadata)
			if !tc.wantKey {
				assert.NotContains(t, fields, "inputTransformations")
				return
			}
			assert.JSONEq(t, tc.want, string(fields["inputTransformations"]))
		})
	}

	for _, malformed := range []string{
		`,"input_transformations":[{"type":"t","path":"p"}]`,
		`,"input_transformations":[{"type":1,"path":"p","reason":"r"}]`,
		`,"input_transformations":{"type":"t"}`,
	} {
		t.Run("malformed "+malformed, func(t *testing.T) {
			result, err := convertResponse(unmarshalMessage(t, response(malformed)), toolNameMapping{}, false, nil, defaultGenerateID, "anthropic", false)
			require.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestStreamAdapter_InputTransformations(t *testing.T) {
	start := func(id, extra string) string {
		return `{"type":"message_start","message":{"id":"` + id + `","type":"message","role":"assistant","model":"claude-fable-5-1","content":[],"usage":{"input_tokens":1,"output_tokens":0}` + extra + `}}`
	}
	delta := func(extra string) string {
		return `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}` + extra + `}`
	}
	const stop = `{"type":"message_stop"}`

	finishFields := func(t *testing.T, events ...string) []map[string]json.RawMessage {
		t.Helper()
		var parsed []anthropic.BetaRawMessageStreamEventUnion
		for _, raw := range events {
			parsed = append(parsed, unmarshalEvent(t, raw))
		}
		var out []map[string]json.RawMessage
		for _, part := range collectParts(parsed) {
			if part.Type == provider.PartFinish {
				out = append(out, anthropicMetadata(t, part.ProviderMetadata))
			}
		}
		return out
	}
	transformations := `,"input_transformations":[` + droppedBlock + `]`

	t.Run("from message start", func(t *testing.T) {
		finishes := finishFields(t, start("msg_1", transformations), delta(""), stop)
		require.Len(t, finishes, 1)
		assert.JSONEq(t, droppedWant, string(finishes[0]["inputTransformations"]))
	})

	t.Run("a delta replaces them", func(t *testing.T) {
		finishes := finishFields(t, start("msg_1", transformations), delta(`,"input_transformations":[{"type":"t","path":"p","reason":"r"}]`), stop)
		require.Len(t, finishes, 1)
		assert.JSONEq(t, `[{"type":"t","path":"p","reason":"r"}]`, string(finishes[0]["inputTransformations"]))
	})

	t.Run("a null or absent delta field keeps them", func(t *testing.T) {
		for _, field := range []string{``, `,"input_transformations":null`} {
			finishes := finishFields(t, start("msg_1", transformations), delta(field), stop)
			require.Len(t, finishes, 1)
			assert.JSONEq(t, droppedWant, string(finishes[0]["inputTransformations"]))
		}
	})

	t.Run("they carry into the next message", func(t *testing.T) {
		finishes := finishFields(t, start("msg_1", ""), delta(transformations), stop, start("msg_2", ""), delta(""), stop)
		require.Len(t, finishes, 2)
		assert.JSONEq(t, droppedWant, string(finishes[0]["inputTransformations"]))
		assert.JSONEq(t, droppedWant, string(finishes[1]["inputTransformations"]))
	})

	t.Run("omitted when never supplied", func(t *testing.T) {
		finishes := finishFields(t, start("msg_1", ""), delta(""), stop)
		require.Len(t, finishes, 1)
		assert.NotContains(t, finishes[0], "inputTransformations")
	})

	t.Run("malformed values fail at the event", func(t *testing.T) {
		for _, events := range [][]string{
			{start("msg_1", `,"input_transformations":[{"type":"t","path":"p"}]`)},
			{start("msg_1", ""), delta(`,"input_transformations":[{"type":"t"}]`)},
		} {
			adapter := &streamAdapter{blocks: map[int64]*blockState{}, serverToolCalls: map[string]string{}, mcpToolCalls: map[string]mcpToolCallInfo{}, generateID: defaultGenerateID}
			parts := make(chan provider.StreamPart, 16)
			var failed error
			for _, raw := range events {
				if err := adapter.handleEvent(unmarshalEvent(t, raw), parts); err != nil {
					failed = err
				}
			}
			require.Error(t, failed)
			assert.Contains(t, failed.Error(), "input_transformations")
		}
	})
}
