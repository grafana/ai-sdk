package prometheus

import (
	"math"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func TestAPICallErrorOutcome_NormalizesStatusCode(t *testing.T) {
	for _, test := range []struct {
		name       string
		statusCode int
		want       string
	}{
		{name: "absent", statusCode: 0, want: statusCodeNone},
		{name: "negative", statusCode: -1, want: statusCodeOther},
		{name: "below HTTP range", statusCode: 99, want: statusCodeOther},
		{name: "minimum HTTP code", statusCode: 100, want: "100"},
		{name: "maximum HTTP code", statusCode: 599, want: "599"},
		{name: "above HTTP range", statusCode: 600, want: statusCodeOther},
		{name: "maximum integer", statusCode: math.MaxInt, want: statusCodeOther},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := apiCallErrorOutcome(&provider.APICallError{StatusCode: test.statusCode})
			assert.Equal(t, test.want, got.statusCode)
			assert.Equal(t, statusError, got.status)
			assert.Equal(t, errorTypeAPICallError, got.errorType)
		})
	}
}
