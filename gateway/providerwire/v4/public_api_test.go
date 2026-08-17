package providerwirev4_test

import (
	"testing"

	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

var _ func(provider.CallOptions) ([]byte, error) = providerwirev4.EncodeCallOptions

func TestPublicAPI_RequestEncoder(t *testing.T) {
	assert.NotNil(t, providerwirev4.EncodeCallOptions)
}
