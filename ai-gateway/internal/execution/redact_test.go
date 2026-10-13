package execution

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummaryRedactor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		keys  []string
		input string
		want  string
	}{
		{name: "no configured keys", input: "own-byok-key", want: "own-byok-key"},
		{name: "empty keys", keys: []string{"", ""}, input: "ordinary", want: "ordinary"},
		{name: "all configured accounts", keys: []string{"unused-key", "server-key"}, input: "server-key unused-key own-byok-key", want: "[REDACTED] [REDACTED] own-byok-key"},
		{name: "duplicates and overlaps", keys: []string{"key", "key-long", "", "key", "key-long"}, input: "key-long key key-long", want: "[REDACTED] [REDACTED] [REDACTED]"},
		{name: "JSON escapes", keys: []string{`key"<&\value`}, input: `echo key"<&\value`, want: "echo [REDACTED]"},
		{name: "configured ownership wins", keys: []string{"own-byok-key"}, input: "own-byok-key", want: "[REDACTED]"},
		{name: "marker collision", keys: []string{"REDACTED", "server-key"}, input: "server-key", want: ""},
		{name: "transformed values are out of scope", keys: []string{"key<value>"}, input: "key%3Cvalue%3E", want: "key%3Cvalue%3E"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, err := json.Marshal(tc.input)
			require.NoError(t, err)
			failure := &Failure{Message: tc.input, Type: tc.input, Code: code, StatusCode: 503}
			redactor := NewSummaryRedactor(tc.keys)
			for i := range tc.keys {
				tc.keys[i] = "mutated"
			}
			got := redactor.Redact(failure)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got.Message)
			assert.Equal(t, tc.want, got.Type)
			if tc.input != "" && tc.want == "" {
				assert.Empty(t, got.Code)
			} else {
				var gotCode string
				require.NoError(t, json.Unmarshal(got.Code, &gotCode))
				assert.Equal(t, tc.want, gotCode)
			}
			assert.Equal(t, 503, got.StatusCode)
			assert.Equal(t, &Failure{Message: tc.input, Type: tc.input, Code: code, StatusCode: 503}, failure)
			assert.Nil(t, redactor.Redact(nil))
		})
	}
	t.Run("numeric precision", func(t *testing.T) {
		code := json.RawMessage(`900719925474099312345`)
		got := NewSummaryRedactor([]string{string(code)}).Redact(&Failure{Code: code, StatusCode: 401})
		assert.Equal(t, &Failure{Code: code, StatusCode: 401}, got)
	})
}
