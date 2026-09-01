package bedrock

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/grafana/ai-sdk/provider"
)

// emptyPayloadSHA256 is the hex encoded SHA-256 of an empty string. Required
// by the SigV4 signer even for body-less requests (where it's used as
// :payloadHash).
const emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// userAgentSuffix appended to outbound requests for telemetry parity with
// upstream (`ai-sdk/amazon-bedrock/<version>`). Updated together with module
// releases.
const userAgentSuffix = "ai-sdk-go/bedrock"

// sigV4Now is the clock used for signing. Tests may override it.
var sigV4Now = time.Now

// nowSigner returns the signing time. Overridable from tests.
var nowSigner = func() time.Time { return sigV4Now() }

// signRequest authenticates and signs the request in place. The flow is:
//  1. Merge static headers from m.headers.
//  2. Append the User-Agent suffix.
//  3. When a bearer token is configured, set Authorization: Bearer ... and
//     return.
//  4. Otherwise: read the body fully so the SigV4 signer has a stable payload
//     hash, then call SignHTTP with credentials, the resolved signing service
//     ("bedrock" or "bedrock-mantle"), and the configured region.
//
// The function only signs `POST` requests with a body; everything else is
// passed through unsigned. This matches the upstream `createSigV4FetchFunction`
// behavior.
func (m *model) signRequest(ctx context.Context, req *http.Request) error {
	// Merge static headers first so they are part of the canonical request
	// hashed by SigV4.
	for k, v := range m.headers {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}

	// Append User-Agent suffix. We honor any preexisting User-Agent and
	// append. Upstream does the same so the signature is stable across
	// re-signs.
	if existing := req.Header.Get("User-Agent"); existing != "" {
		req.Header.Set("User-Agent", existing+" "+userAgentSuffix)
	} else {
		req.Header.Set("User-Agent", userAgentSuffix)
	}

	// Bearer-token shortcut.
	if m.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+m.bearerToken)
		return nil
	}

	// Non-POST or empty-body requests are not signed (matches upstream
	// shortcut). Bedrock Converse uses POST exclusively, but the shortcut is
	// preserved for completeness.
	if !strings.EqualFold(req.Method, http.MethodPost) || req.Body == nil {
		return nil
	}

	// Read body fully so we can hash it AND restore it for the downstream
	// http.Transport. SigV4 needs a stable payload hash before transport.
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("bedrock: reading body for signing: %w", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bodyBytes)), nil
	}
	req.ContentLength = int64(len(bodyBytes))

	payloadHash := emptyPayloadSHA256
	if len(bodyBytes) > 0 {
		sum := sha256.Sum256(bodyBytes)
		payloadHash = hex.EncodeToString(sum[:])
	}

	creds, err := m.resolveCredentials(ctx)
	if err != nil {
		// Surface credential errors as provider.APICallError so the retry
		// layer behaves consistently with HTTP-level errors. Credentials
		// failures are not retryable (config issue, not transient).
		retryable := false
		return provider.NewAPICallError(provider.APICallErrorOptions{
			Message:     fmt.Sprintf("bedrock: resolving AWS credentials: %v", err),
			URL:         req.URL.String(),
			IsRetryable: &retryable,
			Cause:       err,
		})
	}

	signer := v4.NewSigner()
	if err := signer.SignHTTP(ctx, creds, req, payloadHash, m.resolveSigningService(), m.region, nowSigner()); err != nil {
		return fmt.Errorf("bedrock: signing request: %w", err)
	}
	return nil
}

// resolveCredentials returns the AWS credentials to use for signing. Uses
// the explicit credentials provider when set; otherwise loads the default
// AWS chain (env, shared config, EC2 IRSA, etc.) lazily on first call and
// caches the resulting provider.
//
// Lazy initialization is guarded by sync.Once so a model shared across
// goroutines can have concurrent DoStream/DoGenerate calls without racing on
// m.credentials. The default-chain provider has its own internal cache, so
// subsequent calls reuse the AWS SDK's credential cache.
func (m *model) resolveCredentials(ctx context.Context) (aws.Credentials, error) {
	// Always go through credsOnce so the read/write of m.credentials is fully
	// synchronized. When WithCredentials supplied a provider at construction,
	// credsOnce is a no-op for it (m.credentials is already non-nil); otherwise
	// the default AWS chain is loaded exactly once.
	m.credsOnce.Do(func() {
		if m.credentials != nil {
			return
		}
		cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(m.region))
		if err != nil {
			m.credsErr = fmt.Errorf("loading default AWS config: %w", err)
			return
		}
		m.credentials = cfg.Credentials
	})
	if m.credsErr != nil {
		return aws.Credentials{}, m.credsErr
	}
	return m.credentials.Retrieve(ctx)
}
