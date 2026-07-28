//go:build conformance

package bedrock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	bedrockProvider "github.com/grafana/ai-sdk/providers/bedrock"
	"github.com/grafana/ai-sdk/test/conformance"
)

// TestConformance discovers Bedrock fixtures under upstream/ and recorded/
// and replays each through the Go Bedrock provider against a replay server
// using the AWS Smithy event-stream binary framing. The resulting
// UIMessageChunk sequence is compared byte-identical against
// expected.jsonl, which was produced by piping the same fixture chunks
// through the upstream TypeScript `@ai-sdk/amazon-bedrock` SDK.
//
// Cases without a generated expected.jsonl are skipped with a message so
// fixtures can be imported in stages (input.chunks.txt + config.yaml land
// first; expected.jsonl is generated via `mise run generate-conformance` after
// the TS tools support Bedrock).
func TestConformance(t *testing.T) {
	providerDir := "."
	cases := conformance.DiscoverTestCases(t, providerDir)

	if len(cases) == 0 {
		t.Skip("no conformance test cases found")
	}

	factory := func(baseURL string, cfg *conformance.Config) (provider.LanguageModel, error) {
		// Stub static credentials. SigV4 signing still runs but the replay
		// server does not validate signatures, so any plausible value
		// works.
		creds := credentials.NewStaticCredentialsProvider("AKID-test", "secret-test", "")
		model := bedrockProvider.New(
			cfg.Model,
			bedrockProvider.WithBaseURL(baseURL),
			bedrockProvider.WithRegion("us-east-1"),
			bedrockProvider.WithCredentials(creds),
		)
		return model, nil
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			expected := filepath.Join(tc.Dir, "expected.jsonl")
			if _, err := os.Stat(expected); err != nil {
				t.Skip("expected.jsonl missing; run `mise run generate-conformance` after TS tools support Bedrock")
			}
			conformance.RunTestCaseWithServer(t, tc, factory, conformance.BedrockTestServerFactory)
		})
	}
}
