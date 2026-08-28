//go:build conformance

package conformance

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestSnapshot_BodyObjectOrderIgnored(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.test/v1/messages", nil)

	expected, err := newRequestSnapshot("anthropic", req, []byte(`{"a":1,"b":2}`))
	require.NoError(t, err)
	actual, err := newRequestSnapshot("anthropic", req, []byte(`{"b":2,"a":1}`))
	require.NoError(t, err)

	assert.Equal(t, expected.Body, actual.Body)
}

func TestRequestSnapshot_ArrayOrderEnforced(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.test/v1/messages", nil)

	expected, err := newRequestSnapshot("anthropic", req, []byte(`{"items":[1,2]}`))
	require.NoError(t, err)
	actual, err := newRequestSnapshot("anthropic", req, []byte(`{"items":[2,1]}`))
	require.NoError(t, err)

	assert.NotEqual(t, expected.Body, actual.Body)
}

func TestNormalizeToolResultText_InvalidInputDiagnostics(t *testing.T) {
	upstream := `AI_InvalidToolInputError: Invalid input for tool weather: AI_TypeValidationError: details`
	goError := `invalid input for tool weather: schema: schema validation failed`

	assert.Equal(t, normalizeToolResultText(upstream), normalizeToolResultText(goError))
	assert.Equal(t, "invalid input for tool weather: <validator-diagnostics>", normalizeToolResultText(upstream))
	assert.NotEqual(t, normalizeToolResultText(upstream), normalizeToolResultText(`invalid input for tool search: details`))
	assert.Equal(t, "ordinary error", normalizeToolResultText("ordinary error"))
}

func TestRequestSnapshot_HeaderNormalization(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.test/v1/messages?stream=true", nil)
	req.Header.Set("Content-Type", " application/json ")
	req.Header.Set("X-API-Key", "secret")
	req.Header.Set("User-Agent", "go-test")

	snapshot, err := newRequestSnapshot("anthropic", req, []byte(`{}`))
	require.NoError(t, err)

	assert.Equal(t, "POST", snapshot.Method)
	assert.Equal(t, "/v1/messages", snapshot.Path)
	assert.Equal(t, map[string]string{
		"content-type": "application/json",
		"x-api-key":    redactedHeaderValue,
	}, snapshot.Headers)
}
