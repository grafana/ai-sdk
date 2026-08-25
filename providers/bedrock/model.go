package bedrock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/grafana/ai-sdk/provider"
)

// bearerTokenEnv is the env var consulted for a Bearer token when
// WithBearerToken is not used. Matches upstream env handling.
const bearerTokenEnv = "AWS_BEARER_TOKEN_BEDROCK"

// defaultSigningService is the AWS service name used in the SigV4 credential
// scope for standard Bedrock Runtime endpoints.
const defaultSigningService = "bedrock"

// mantleSigningService is the AWS service name used in the SigV4 credential
// scope for Bedrock Mantle endpoints. Mantle is a separate AWS service, so its
// signatures must be scoped to "bedrock-mantle" rather than "bedrock";
// signing with the wrong service name yields a signature-scope mismatch and
// authentication failure.
const mantleSigningService = "bedrock-mantle"

// mantleHostPrefix identifies Bedrock Mantle endpoints
// (bedrock-mantle.<region>.api.aws) for automatic signing-service selection.
const mantleHostPrefix = "bedrock-mantle."

// defaultRegion is the region used when none is configured and the env var
// AWS_REGION is unset. Mirrors the upstream fallback.
const defaultRegion = "us-east-1"

// model implements provider.LanguageModel against the AWS Bedrock Converse API.
//
// Construction happens via [New] plus zero or more [Option] values. All fields
// set during construction are read-only afterwards; the lazily-loaded default
// credential chain is guarded by credsOnce so concurrent DoStream/DoGenerate
// calls on a shared model are race-free.
type model struct {
	modelID     string
	region      string
	baseURL     string
	bearerToken string
	headers     map[string]string
	httpClient  *http.Client
	generateID  func() string

	// signingService overrides the AWS service name used in the SigV4
	// credential scope. When empty, the service is inferred from the endpoint
	// host (see resolveSigningService).
	signingService string

	// credentials is the AWS credential provider used for SigV4 signing. It is
	// either supplied via WithCredentials at construction or loaded lazily from
	// the default AWS chain on first signing call (guarded by credsOnce).
	credentials aws.CredentialsProvider
	// credsOnce guards lazy initialization of credentials from the default AWS
	// chain. credsErr captures any failure so it is returned consistently.
	credsOnce sync.Once
	credsErr  error
}

// New constructs a Bedrock language model. Without options, the provider
// reads the region from AWS_REGION (falling back to us-east-1) and loads
// credentials from the default AWS chain on first call.
//
// The returned value implements provider.LanguageModel. Anthropic-specific
// pass-throughs (thinking, effort, betas, native structured output) activate
// only when modelID identifies an Anthropic model (model ID contains
// "anthropic"); for other model families they emit warnings.
func New(modelID string, opts ...Option) provider.LanguageModel {
	m := &model{
		modelID:    modelID,
		region:     resolveDefaultRegion(),
		httpClient: http.DefaultClient,
		generateID: defaultGenerateID,
	}
	for _, opt := range opts {
		opt(m)
	}

	// Read AWS_BEARER_TOKEN_BEDROCK as a fallback when WithBearerToken is not
	// used. We trim because the upstream provider also rejects whitespace-only
	// values.
	if m.bearerToken == "" {
		if env := strings.TrimSpace(os.Getenv(bearerTokenEnv)); env != "" {
			m.bearerToken = env
		}
	}

	return m
}

// SpecificationVersion implements provider.LanguageModel.
func (m *model) SpecificationVersion() string { return specificationVersion }

// Provider implements provider.LanguageModel. The value matches upstream's
// `provider` field ("amazon-bedrock") so cross-SDK provider metadata keys
// remain compatible.
func (m *model) Provider() string { return providerName }

// ModelID implements provider.LanguageModel.
func (m *model) ModelID() string { return m.modelID }

var s3URLPattern = regexp.MustCompile(`^s3://`)

// SupportedURLs implements provider.LanguageModel.
func (m *model) SupportedURLs() map[string][]*regexp.Regexp {
	return map[string][]*regexp.Regexp{
		"image/*": {s3URLPattern},
		"video/*": {s3URLPattern},
	}
}

// DoGenerate is the non-streaming entry point. Implemented in http.go.
func (m *model) DoGenerate(ctx context.Context, params provider.CallOptions) (*provider.GenerateResult, error) {
	return m.doGenerate(ctx, params)
}

// DoStream is the streaming entry point. Implemented in http.go.
func (m *model) DoStream(ctx context.Context, params provider.CallOptions) (*provider.StreamResult, error) {
	return m.doStream(ctx, params)
}

// endpoint returns the base URL for outbound requests. WithBaseURL overrides
// the AWS-derived URL.
func (m *model) endpoint() string {
	if m.baseURL != "" {
		return strings.TrimRight(m.baseURL, "/")
	}
	return fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", m.region)
}

// resolveSigningService returns the AWS service name used in the SigV4
// credential scope. An explicit WithSigningService value always wins.
// Otherwise the service is inferred from the endpoint host: Bedrock Mantle
// endpoints (bedrock-mantle.<region>.api.aws) sign with "bedrock-mantle",
// while every other endpoint signs with the standard "bedrock" service.
func (m *model) resolveSigningService() string {
	if m.signingService != "" {
		return m.signingService
	}
	if isMantleEndpoint(m.endpoint()) {
		return mantleSigningService
	}
	return defaultSigningService
}

// isMantleEndpoint reports whether endpoint targets Bedrock Mantle, whose SigV4
// credential scope uses the "bedrock-mantle" service name. Mantle endpoints are
// hosted at bedrock-mantle.<region>.api.aws.
func isMantleEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	return strings.HasPrefix(u.Hostname(), mantleHostPrefix)
}

// resolveDefaultRegion reads AWS_REGION (or AWS_DEFAULT_REGION) and returns
// the first non-empty value. Falls back to a hard-coded default so test setups
// without an env work out of the box; the SigV4 signer will still need real
// credentials for real calls.
func resolveDefaultRegion() string {
	for _, key := range []string{"AWS_REGION", "AWS_DEFAULT_REGION"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return defaultRegion
}

// defaultGenerateID returns a short random hex string. Used for fallback
// tool-use IDs when neither the server nor the caller supply one. Best effort
// only -- if crypto/rand fails we return a sentinel.
func defaultGenerateID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "id_fallback"
	}
	return "tooluse_" + hex.EncodeToString(b[:])
}
