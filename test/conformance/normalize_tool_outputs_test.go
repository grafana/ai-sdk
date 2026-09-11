//go:build conformance

package conformance

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func out(id string) map[string]any {
	return map[string]any{"type": "tool-output-available", "toolCallId": id}
}

func errOut(id string) map[string]any {
	return map[string]any{"type": "tool-output-error", "toolCallId": id}
}

func provOut(id string) map[string]any {
	return map[string]any{"type": "tool-output-available", "toolCallId": id, "providerExecuted": true}
}

func ids(chunks []map[string]any) []string {
	got := make([]string, 0, len(chunks))
	for _, c := range chunks {
		id, _ := c["toolCallId"].(string)
		if id == "" {
			id = c["type"].(string)
		}
		got = append(got, id)
	}
	return got
}

func TestNormalizeConcurrentToolOutputs(t *testing.T) {
	tests := []struct {
		name  string
		input []map[string]any
		want  []string
	}{
		{"empty", nil, []string{}},
		{
			"single chunk is untouched",
			[]map[string]any{out("b")},
			[]string{"b"},
		},
		{
			"run of one is untouched",
			[]map[string]any{{"type": "start-step"}, out("b"), {"type": "finish-step"}},
			[]string{"start-step", "b", "finish-step"},
		},
		{
			"adjacent local outputs sort by toolCallId",
			[]map[string]any{out("b"), out("a")},
			[]string{"a", "b"},
		},
		{
			"run at the very end of the slice",
			[]map[string]any{{"type": "start-step"}, out("c"), out("a"), out("b")},
			[]string{"start-step", "a", "b", "c"},
		},
		{
			"success and error outputs normalize as one run",
			[]map[string]any{out("c"), errOut("a"), out("b")},
			[]string{"a", "b", "c"},
		},
		{
			"provider-executed chunks keep recorded order and end a run",
			[]map[string]any{provOut("z"), provOut("a")},
			[]string{"z", "a"},
		},
		{
			"a provider-executed chunk splits two local runs",
			[]map[string]any{out("d"), out("c"), provOut("z"), out("b"), out("a")},
			[]string{"c", "d", "z", "a", "b"},
		},
		{
			"other chunk types are never absorbed",
			[]map[string]any{out("b"), {"type": "text-delta"}, out("a")},
			[]string{"b", "text-delta", "a"},
		},
		{
			"equal toolCallIds keep recorded order",
			[]map[string]any{
				{"type": "tool-output-available", "toolCallId": "a", "seq": 1},
				{"type": "tool-output-available", "toolCallId": "a", "seq": 2},
			},
			[]string{"a", "a"},
		},
		{
			"missing toolCallId sorts as empty string, no panic",
			[]map[string]any{out("b"), {"type": "tool-output-available"}},
			[]string{"tool-output-available", "b"},
		},
		{
			"non-string toolCallId sorts as empty string, no panic",
			[]map[string]any{out("b"), {"type": "tool-output-available", "toolCallId": 7}},
			[]string{"tool-output-available", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeConcurrentToolOutputs(tt.input)
			assert.Equal(t, tt.want, ids(got))
		})
	}

	t.Run("does not mutate the caller's slice", func(t *testing.T) {
		input := []map[string]any{out("b"), out("a")}
		_ = normalizeConcurrentToolOutputs(input)
		require.Equal(t, []string{"b", "a"}, ids(input), "caller's backing array was reordered")
	})

	t.Run("equal toolCallIds preserve relative order", func(t *testing.T) {
		input := []map[string]any{
			{"type": "tool-output-available", "toolCallId": "a", "seq": 1},
			{"type": "tool-output-available", "toolCallId": "a", "seq": 2},
		}
		got := normalizeConcurrentToolOutputs(input)
		assert.Equal(t, 1, got[0]["seq"])
		assert.Equal(t, 2, got[1]["seq"])
	})
}
