package providerwirev4_test

import (
	providerwirev4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
)

var _ func(provider.CallOptions) ([]byte, error) = providerwirev4.EncodeCallOptions
