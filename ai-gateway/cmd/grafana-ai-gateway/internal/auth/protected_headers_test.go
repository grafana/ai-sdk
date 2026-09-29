package auth

import (
	"context"
	"net/http"
	"testing"

	v4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every header name this edge refuses in outer headers must also be refused as a
// body-carried call header, or the body becomes a way around this check. Reading
// the edge's own behavior keeps the two from drifting: adding a name here without
// adding it to the runtime's protected set fails this test.
func TestInboundRefusedHeadersAreProtectedCallHeaders(t *testing.T) {
	candidates := []string{
		"Authorization", "X-Access-Token", "X-Grafana-Id",
		"X-Api-Key", "Api-Key", "Openai-Api-Key", "Anthropic-Api-Key", "Proxy-Authorization",
	}
	authenticate := func(name string) error {
		// A valid stack assertion, so only the candidate can cause a refusal.
		headers := http.Header{}
		headers.Set("X-Scope-OrgID", "1")
		if name != "" {
			headers.Set(name, "value")
		}
		_, err := (cloudProviderWireAuthenticator{}).Authenticate(context.Background(), headers)
		return err
	}
	require.NoError(t, authenticate(""), "the baseline request authenticates")
	require.NoError(t, authenticate("User-Agent"), "an ordinary header is accepted, so a refusal below is the candidate's")

	refused := 0
	for _, name := range candidates {
		if authenticate(name) == nil {
			continue
		}
		refused++
		assert.True(t, v4.IsProtectedCallHeader(name),
			"%s is refused in outer headers, so it must be refused in a request body too", name)
	}
	require.Positive(t, refused, "the edge refuses at least one credential header")
	require.Less(t, refused, len(candidates), "the edge refuses a subset, so this test can tell the two apart")
}
