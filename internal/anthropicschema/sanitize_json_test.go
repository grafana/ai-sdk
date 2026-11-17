package anthropicschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeJSON(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "keeps property order at every depth",
			in:   `{"type":"object","properties":{"zulu":{"type":"string"},"alpha":{"type":"object","properties":{"yankee":{"type":"string"},"bravo":{"type":"string"}}}},"required":["zulu","alpha"]}`,
			want: `{"type":"object","properties":{"zulu":{"type":"string"},"alpha":{"type":"object","properties":{"yankee":{"type":"string"},"bravo":{"type":"string"}},"additionalProperties":false}},"required":["zulu","alpha"],"additionalProperties":false}`,
		},
		{
			name: "keeps order inside items, oneOf rewritten as anyOf, and $defs",
			in:   `{"$defs":{"z":{"type":"object","properties":{"y":{"type":"string"},"b":{"type":"string"}}}},"type":"object","properties":{"list":{"type":"array","items":{"type":"object","properties":{"y":{"type":"string"},"b":{"type":"string"}}}},"choice":{"oneOf":[{"type":"object","properties":{"y":{"type":"string"},"b":{"type":"string"}}}]}}}`,
			want: `{"$defs":{"z":{"type":"object","properties":{"y":{"type":"string"},"b":{"type":"string"}},"additionalProperties":false}},"type":"object","properties":{"list":{"type":"array","items":{"type":"object","properties":{"y":{"type":"string"},"b":{"type":"string"}},"additionalProperties":false}},"choice":{"anyOf":[{"type":"object","properties":{"y":{"type":"string"},"b":{"type":"string"}},"additionalProperties":false}]}},"additionalProperties":false}`,
		},
		{
			name: "moves stripped constraints into the description",
			in:   `{"type":"object","properties":{"z":{"type":"string","maxLength":3},"a":{"type":"integer","minimum":5}}}`,
			want: `{"type":"object","properties":{"z":{"type":"string","description":"max length: 3."},"a":{"type":"integer","description":"minimum: 5."}},"additionalProperties":false}`,
		},
		{
			name: "a repeated member keeps its first position and last value",
			in:   `{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"},"z":{"type":"integer"}}}`,
			want: `{"type":"object","properties":{"z":{"type":"integer"},"a":{"type":"string"}},"additionalProperties":false}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizeJSON(json.RawMessage(tt.in))
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))

			// The result matches what Sanitize returns, apart from order.
			schema := map[string]any{}
			require.NoError(t, json.Unmarshal([]byte(tt.in), &schema))
			wantMap, err := json.Marshal(Sanitize(schema))
			require.NoError(t, err)
			assert.JSONEq(t, string(wantMap), string(got))
		})
	}
}

func TestSanitizeJSON_RejectsInvalidSchema(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `{`, `{} {}`} {
		_, err := SanitizeJSON(json.RawMessage(raw))
		assert.Error(t, err, raw)
	}
}
