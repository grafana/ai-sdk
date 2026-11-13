package v4

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionOutcomeDocuments(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		body   []byte
		frame  []byte
	}{
		{name: "denial", err: fmt.Errorf("quota exceeded for private-tenant: %w", extension.ErrDenied), status: http.StatusForbidden, body: canonicalPermissionError, frame: canonicalPermissionStreamErrorFrame},
		{name: "unavailable", err: extension.ErrUnavailable, status: http.StatusServiceUnavailable, body: canonicalUnavailableError, frame: canonicalUnavailableStreamErrorFrame},
	} {
		t.Run(tc.name+" unary", func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.generate = func(context.Context, provider.CallOptions) (*provider.GenerateResult, error) { return nil, tc.err }
			response := harness.serve(validRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.Bytes())
			assert.NotContains(t, response.Body.String(), "private-tenant")
		})
		t.Run(tc.name+" stream setup", func(t *testing.T) {
			harness := newRuntimeHarness(t, testLimits())
			harness.model.stream = func(context.Context, provider.CallOptions) (*provider.StreamResult, error) { return nil, tc.err }
			response := harness.serve(streamRequest(`{"prompt":[]}`))
			assert.Equal(t, tc.status, response.Code)
			assert.Equal(t, tc.body, response.Body.Bytes())
		})
		t.Run(tc.name+" in band", func(t *testing.T) {
			part := provider.NewAPICallError(provider.APICallErrorOptions{Message: "private-tenant secret", Cause: tc.err})
			body := serveSegmentParts(t, false,
				provider.StreamPart{Type: provider.PartStreamStart},
				provider.StreamPart{Type: provider.PartTextStart, ID: "t"},
				provider.StreamPart{Type: provider.PartTextDelta, ID: "t", Delta: "hello"},
				provider.StreamPart{Type: provider.PartError, APICallError: part},
			)
			require.True(t, strings.HasSuffix(body, string(tc.frame)), body)
			assert.Equal(t, []string{"stream-start", "text-start", "text-delta", "error"}, segmentFrameTypes(t, body))
			assert.NotContains(t, body, "private-tenant")
		})
	}
}
