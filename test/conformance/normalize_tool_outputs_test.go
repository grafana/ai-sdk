//go:build conformance

package conformance

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeConcurrentToolOutputs(t *testing.T) {
	chunk := func(typ, id string, providerExecuted bool) map[string]any {
		c := map[string]any{"type": typ, "toolCallId": id}
		if providerExecuted {
			c["providerExecuted"] = true
		}
		return c
	}
	out := func(id string) map[string]any { return chunk("tool-output-available", id, false) }
	ids := func(chunks []map[string]any) []string {
		var got []string
		for _, c := range chunks {
			id, _ := c["toolCallId"].(string)
			got = append(got, c["type"].(string)+":"+id)
		}
		return got
	}

	tests := []struct {
		name string
		in   []map[string]any
		want []string
	}{
		{"sorts adjacent local outputs", []map[string]any{out("b"), out("a")},
			[]string{"tool-output-available:a", "tool-output-available:b"}},
		{"sorts a success and an error together", []map[string]any{out("b"), chunk("tool-output-error", "a", false)},
			[]string{"tool-output-error:a", "tool-output-available:b"}},
		{"keeps provider-executed order", []map[string]any{chunk("tool-output-available", "b", true), chunk("tool-output-available", "a", true)},
			[]string{"tool-output-available:b", "tool-output-available:a"}},
		{"keeps rejected call outputs in place", []map[string]any{chunk("tool-input-error", "b", false), chunk("tool-output-error", "b", false), out("a")},
			[]string{"tool-input-error:b", "tool-output-error:b", "tool-output-available:a"}},
		{"does not sort across other chunks", []map[string]any{out("b"), {"type": "finish-step"}, out("a")},
			[]string{"tool-output-available:b", "finish-step:", "tool-output-available:a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ids(normalizeConcurrentToolOutputs(tt.in)))
		})
	}

	in := []map[string]any{out("b"), out("a")}
	normalizeConcurrentToolOutputs(in)
	assert.Equal(t, "b", in[0]["toolCallId"], "the caller's slice is not reordered")
}
