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
// using the AWS Smithy event-stream binary framing. Streaming cases compare
// UIMessageChunk sequences against expected.jsonl; unary cases compare the
// provider result against expected-generate.json. Both are generated through
// the pinned upstream TypeScript `@ai-sdk/amazon-bedrock` SDK.
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
			cfg, err := conformance.LoadConfig(filepath.Join(tc.Dir, "config.yaml"))
			if err != nil {
				t.Fatalf("loading config: %v", err)
			}
			expectedName := "expected.jsonl"
			if cfg.Operation == conformance.OperationGenerate {
				expectedName = "expected-generate.json"
			}
			if _, err := os.Stat(filepath.Join(tc.Dir, expectedName)); err != nil {
				t.Skipf("%s missing; run `mise run generate-conformance`", expectedName)
			}
			conformance.RunTestCaseWithServer(t, tc, factory, conformance.BedrockTestServerFactory)
		})
	}
}
