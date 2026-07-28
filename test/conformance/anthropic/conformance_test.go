//go:build conformance

package anthropic_test

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/grafana/ai-sdk/provider"
	anthropicProvider "github.com/grafana/ai-sdk/providers/anthropic"
	"github.com/grafana/ai-sdk/test/conformance"
)

func TestConformance(t *testing.T) {
	providerDir := "."
	cases := conformance.DiscoverTestCases(t, providerDir)

	if len(cases) == 0 {
		t.Skip("no conformance test cases found")
	}

	factory := func(baseURL string, cfg *conformance.Config) (provider.LanguageModel, error) {
		model := anthropicProvider.New(
			"test-api-key",
			cfg.Model,
			anthropicProvider.WithRequestOptions(
				option.WithBaseURL(baseURL),
			),
		)
		return model, nil
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			conformance.RunTestCase(t, tc, factory)
		})
	}
}
