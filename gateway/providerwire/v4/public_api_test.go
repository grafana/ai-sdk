package providerwirev4_test

import (
	"testing"

	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
)

func TestPublicAPI_RequestEncoder(t *testing.T) {
	var encode func(provider.CallOptions) ([]byte, error) = providerwirev4.EncodeCallOptions
	assert.NotNil(t, encode)
}
