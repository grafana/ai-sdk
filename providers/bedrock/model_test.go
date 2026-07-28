package bedrock

import (
	"context"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time check that *model satisfies provider.LanguageModel.
var _ provider.LanguageModel = (*model)(nil)

func TestModel_Identity(t *testing.T) {
	m := New("anthropic.claude-sonnet-4-5-20250929-v1:0",
		WithRegion("us-east-1"),
	)
	assert.Equal(t, "v4", m.SpecificationVersion())
	assert.Equal(t, "amazon-bedrock", m.Provider())
	assert.Equal(t, "anthropic.claude-sonnet-4-5-20250929-v1:0", m.ModelID())
	patterns := m.SupportedURLs()["image/*"]
	require.Len(t, patterns, 1)
	assert.True(t, patterns[0].MatchString("s3://bucket/image.png"))
}

func TestModel_Construction(t *testing.T) {
	cases := []struct {
		name  string
		opts  []Option
		check func(t *testing.T, m *model)
	}{
		{
			name: "default region falls back to us-east-1",
			opts: nil,
			check: func(t *testing.T, m *model) {
				// AWS_REGION may be set in the test environment; either the
				// env value or the fallback is acceptable. We only assert the
				// region is non-empty and the endpoint is well-formed.
				require.NotEmpty(t, m.region)
				assert.Contains(t, m.endpoint(), "bedrock-runtime")
			},
		},
		{
			name: "explicit region wins",
			opts: []Option{WithRegion("eu-west-1")},
			check: func(t *testing.T, m *model) {
				assert.Equal(t, "eu-west-1", m.region)
				assert.Equal(t, "https://bedrock-runtime.eu-west-1.amazonaws.com", m.endpoint())
			},
		},
		{
			name: "WithBaseURL overrides default endpoint",
			opts: []Option{WithBaseURL("https://custom.example.com/")},
			check: func(t *testing.T, m *model) {
				assert.Equal(t, "https://custom.example.com", m.endpoint())
			},
		},
		{
			name: "WithBearerToken sets token",
			opts: []Option{WithBearerToken("test-token")},
			check: func(t *testing.T, m *model) {
				assert.Equal(t, "test-token", m.bearerToken)
			},
		},
		{
			name: "WithCredentials wires aws.CredentialsProvider",
			opts: []Option{WithCredentials(credentials.NewStaticCredentialsProvider("k", "s", ""))},
			check: func(t *testing.T, m *model) {
				require.NotNil(t, m.credentials)
				creds, err := m.credentials.Retrieve(context.Background())
				require.NoError(t, err)
				assert.Equal(t, "k", creds.AccessKeyID)
				assert.Equal(t, "s", creds.SecretAccessKey)
			},
		},
		{
			name: "WithHTTPClient overrides default",
			opts: []Option{WithHTTPClient(&http.Client{})},
			check: func(t *testing.T, m *model) {
				assert.NotSame(t, http.DefaultClient, m.httpClient)
			},
		},
		{
			name: "WithHeaders merges static headers",
			opts: []Option{
				WithHeaders(map[string]string{"X-Trace": "abc"}),
				WithHeaders(map[string]string{"X-Other": "y"}),
			},
			check: func(t *testing.T, m *model) {
				assert.Equal(t, "abc", m.headers["X-Trace"])
				assert.Equal(t, "y", m.headers["X-Other"])
			},
		},
		{
			name: "WithGenerateID overrides default generator",
			opts: []Option{WithGenerateID(func() string { return "fixed-id" })},
			check: func(t *testing.T, m *model) {
				assert.Equal(t, "fixed-id", m.generateID())
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lm := New("anthropic.claude-sonnet-4-5-20250929-v1:0", tc.opts...)
			m, ok := lm.(*model)
			require.True(t, ok, "New must return *model")
			tc.check(t, m)
		})
	}
}

func TestModel_BearerTokenFromEnv(t *testing.T) {
	t.Setenv(bearerTokenEnv, "  env-token  ")
	lm := New("anthropic.claude-sonnet-4-5-20250929-v1:0")
	m := lm.(*model)
	assert.Equal(t, "env-token", m.bearerToken, "bearer token should be trimmed from env value")
}

func TestModel_ExplicitBearerWinsOverEnv(t *testing.T) {
	t.Setenv(bearerTokenEnv, "env-token")
	lm := New("anthropic.claude-sonnet-4-5-20250929-v1:0", WithBearerToken("explicit"))
	m := lm.(*model)
	assert.Equal(t, "explicit", m.bearerToken)
}

func TestBedrockOptions_ProviderKey(t *testing.T) {
	assert.Equal(t, "amazonBedrock", BedrockOptions{}.ProviderKey())
	assert.Equal(t, "amazonBedrock", FilePartOptions{}.ProviderKey())
	assert.Equal(t, "amazonBedrock", ReasoningMetadata{}.ProviderKey())
}

func TestDefaultGenerateID(t *testing.T) {
	id := defaultGenerateID()
	assert.True(t, len(id) > len("tooluse_"))
	assert.NotEqual(t, id, defaultGenerateID(), "successive IDs should differ")
}

// stubCredentialsProvider returns fixed credentials for tests that need to
// construct a provider without contacting AWS.
type stubCredentialsProvider struct {
	creds aws.Credentials
	err   error
}

func (s stubCredentialsProvider) Retrieve(_ context.Context) (aws.Credentials, error) {
	if s.err != nil {
		return aws.Credentials{}, s.err
	}
	return s.creds, nil
}
