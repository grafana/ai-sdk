package bedrock

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixedNowOption fixes the signing time for deterministic header values.
func fixedNowOption(t time.Time) func() {
	old := sigV4Now
	sigV4Now = func() time.Time { return t }
	return func() { sigV4Now = old }
}

func newSigningTestModel(opts ...Option) *model {
	lm := New("anthropic.claude-sonnet-4-5-20250929-v1:0", opts...)
	return lm.(*model)
}

func TestSignRequest_SigV4HeadersOnPOST(t *testing.T) {
	defer fixedNowOption(time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC))()

	m := newSigningTestModel(
		WithRegion("us-east-1"),
		WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse",
		bytes.NewReader([]byte(`{"foo":"bar"}`)))
	require.NoError(t, err)

	require.NoError(t, m.signRequest(context.Background(), req))

	auth := req.Header.Get("Authorization")
	require.NotEmpty(t, auth, "Authorization header must be set after SigV4")
	assert.True(t, strings.HasPrefix(auth, "AWS4-HMAC-SHA256 "), "got: %s", auth)
	assert.Contains(t, auth, "Credential=AKID/")
	assert.Contains(t, auth, "/bedrock/aws4_request")
	assert.NotEmpty(t, req.Header.Get("X-Amz-Date"))

	// Body must still be readable after signing.
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"foo":"bar"}`, string(body))
}

func TestSignRequest_BearerTokenSkipsSigV4(t *testing.T) {
	m := newSigningTestModel(
		WithBearerToken("test-token"),
		WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse",
		bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	require.NoError(t, m.signRequest(context.Background(), req))

	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
	assert.Empty(t, req.Header.Get("X-Amz-Date"), "no SigV4 headers when bearer token is set")
}

func TestSignRequest_GETIsUnsigned(t *testing.T) {
	m := newSigningTestModel(
		WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/test", nil)
	require.NoError(t, err)

	require.NoError(t, m.signRequest(context.Background(), req))

	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestSignRequest_CustomHeadersPreserved(t *testing.T) {
	defer fixedNowOption(time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC))()

	m := newSigningTestModel(
		WithRegion("us-east-1"),
		WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
		WithHeaders(map[string]string{"X-Custom": "val"}),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse",
		bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	require.NoError(t, m.signRequest(context.Background(), req))

	assert.Equal(t, "val", req.Header.Get("X-Custom"))
	assert.Contains(t, req.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
}

func TestSignRequest_UserAgentAppended(t *testing.T) {
	m := newSigningTestModel(WithBearerToken("t"))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://example.com/", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)
	req.Header.Set("User-Agent", "my-app/1.0")

	require.NoError(t, m.signRequest(context.Background(), req))

	ua := req.Header.Get("User-Agent")
	assert.Contains(t, ua, "my-app/1.0")
	assert.Contains(t, ua, "ai-sdk-go/bedrock")
}

func TestSignRequest_CredentialProviderError(t *testing.T) {
	stub := stubCredentialsProvider{err: errors.New("creds boom")}
	m := newSigningTestModel(
		WithRegion("us-east-1"),
		WithCredentials(stub),
	)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://example.com/", bytes.NewReader([]byte(`{}`)))
	require.NoError(t, err)

	err = m.signRequest(context.Background(), req)
	require.Error(t, err)
	var apiErr *provider.APICallError
	require.True(t, errors.As(err, &apiErr), "got: %T %v", err, err)
	assert.False(t, apiErr.IsRetryable, "credential errors must not be retryable")
	assert.Contains(t, apiErr.Message, "resolving AWS credentials")
}

func TestResolveCredentials_ExplicitProvider(t *testing.T) {
	m := newSigningTestModel(
		WithCredentials(credentials.NewStaticCredentialsProvider("K", "S", "T")),
	)
	creds, err := m.resolveCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "K", creds.AccessKeyID)
	assert.Equal(t, "S", creds.SecretAccessKey)
	assert.Equal(t, "T", creds.SessionToken)
}

func TestResolveCredentials_ExplicitProviderCalledOncePerRequest(t *testing.T) {
	// Verify that with an explicit provider, resolveCredentials returns the
	// configured creds (the provider's own cache, if any, is its own concern).
	stub := &countingCredentialsProvider{
		creds: aws.Credentials{AccessKeyID: "K", SecretAccessKey: "S"},
	}
	m := newSigningTestModel(WithCredentials(stub))

	for i := 0; i < 3; i++ {
		creds, err := m.resolveCredentials(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "K", creds.AccessKeyID)
	}
	assert.Equal(t, 3, stub.calls, "explicit provider is called per Retrieve; caching is its responsibility")
}

// TestResolveCredentials_LazyChainRaceFree exercises the lazy default-chain
// initialization concurrently to verify credsOnce guards the shared model
// field. Run with `go test -race` to detect data races on m.credentials.
//
// The model has no explicit credentials, so the first call lazily loads the
// default AWS config. We only assert the calls return consistently (either all
// succeed or all fail the same way); the point is the absence of a data race.
func TestResolveCredentials_LazyChainRaceFree(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping lazy-chain race test in short mode (may touch AWS credential sources)")
	}
	m := newSigningTestModel(WithRegion("us-east-1"))

	const goroutines = 16
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, errs[idx] = m.resolveCredentials(ctx)
		}(i)
	}
	wg.Wait()

	// All goroutines must observe the same outcome (credsErr is set once).
	first := errs[0]
	for _, e := range errs[1:] {
		assert.Equal(t, first == nil, e == nil, "all concurrent calls must agree on success/failure")
	}
}

func TestSignRequest_ConcurrentCallsRaceFree(t *testing.T) {
	m := newSigningTestModel(
		WithRegion("us-east-1"),
		WithCredentials(credentials.NewStaticCredentialsProvider("AKID", "SECRET", "")),
	)
	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
				"https://bedrock-runtime.us-east-1.amazonaws.com/model/test/converse",
				bytes.NewReader([]byte(`{"foo":"bar"}`)))
			require.NoError(t, err)
			assert.NoError(t, m.signRequest(context.Background(), req))
		}()
	}
	wg.Wait()
}

type countingCredentialsProvider struct {
	creds aws.Credentials
	calls int
}

func (c *countingCredentialsProvider) Retrieve(_ context.Context) (aws.Credentials, error) {
	c.calls++
	return c.creds, nil
}
