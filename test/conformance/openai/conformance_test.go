//go:build conformance

package openai_test

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	openaiProvider "github.com/grafana/ai-sdk/providers/openai"
	"github.com/grafana/ai-sdk/test/conformance"
	"github.com/openai/openai-go/v3/option"
)

func TestConformance(t *testing.T) {
	providerDir := "."
	cases := conformance.DiscoverTestCases(t, providerDir)

	if len(cases) == 0 {
		t.Skip("no conformance test cases found")
	}

	idGen := conformanceIDGenerator("src")

	factory := func(baseURL string, cfg *conformance.Config) (provider.LanguageModel, error) { //nolint:unparam
		// Upstream createOpenAI uses a base URL that includes the /v1 prefix; the
		// replay server accepts any path, so append /v1 to match the upstream
		// request path (/v1/responses).
		model := openaiProvider.NewResponses(
			"test-api-key",
			cfg.Model,
			openaiProvider.WithRequestOptions(option.WithBaseURL(baseURL+"/v1")),
			openaiProvider.WithGenerateID(idGen()),
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

// conformanceIDGenerator returns a factory that, on each call, yields a fresh
// deterministic sequential ID generator with the given prefix. A fresh
// generator per test case keeps source IDs byte-stable across runs.
func conformanceIDGenerator(prefix string) func() func() string {
	return func() func() string {
		n := 0
		return func() string {
			id := prefix + "-" + itoa(n)
			n++
			return id
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
