package grafana

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidJSON_Unicode(t *testing.T) {
	for _, tc := range []struct {
		input string
		valid bool
	}{
		{`{"x":"\ud83d\ude00"}`, true}, {`{"x":"😀"}`, true}, {`{"x":"\ud800"}`, false}, {`{"x":"\udc00"}`, false}, {`{"x":"\ud800\u0061"}`, false}, {`{"x":"\\ud800"}`, true}, {`{"x":"\"quoted\""}`, true}, {`"\u0061"`, true}, {`"\ud800\ud800"`, false}, {`"\u`, false},
	} {
		t.Run(tc.input, func(t *testing.T) { assert.Equal(t, tc.valid, validJSON([]byte(tc.input))) })
	}
}
