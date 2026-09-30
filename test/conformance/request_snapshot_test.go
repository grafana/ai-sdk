//go:build conformance

package conformance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type requestTargetCase struct {
	Name         string `json:"name"`
	URL          string `json:"url"`
	ExpectedPath string `json:"expectedPath"`
}

func loadRequestTargetCases(t *testing.T) []requestTargetCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "request-snapshots", "cases.json"))
	require.NoError(t, err)
	var cases []requestTargetCase
	require.NoError(t, json.Unmarshal(data, &cases))
	require.NotEmpty(t, cases)
	return cases
}

func captureRequestTarget(t *testing.T, target string) RequestSnapshot {
	t.Helper()
	var req *http.Request
	if strings.Contains(target, "#") {
		parsed, err := url.Parse(target)
		require.NoError(t, err)
		req = httptest.NewRequest(http.MethodPost, "/", nil)
		req.URL = parsed
	} else {
		req = httptest.NewRequest(http.MethodPost, target, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	snapshot, err := newRequestSnapshot("anthropic", req, []byte(`{}`))
	require.NoError(t, err)
	return snapshot
}

func TestRequestSnapshot_Target(t *testing.T) {
	for _, tc := range loadRequestTargetCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			assert.Equal(t, tc.ExpectedPath, captureRequestTarget(t, tc.URL).Path)
		})
	}
	t.Run("empty path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.URL.Path = ""
		snapshot, err := newRequestSnapshot("anthropic", req, []byte(`{}`))
		require.NoError(t, err)
		assert.Equal(t, "/", snapshot.Path)
	})
}

func TestRequestSnapshot_TypeScriptGolden(t *testing.T) {
	cases := loadRequestTargetCases(t)
	expected, err := LoadExpectedRequests(filepath.Join("testdata", "request-snapshots", "expected-requests.jsonl"))
	require.NoError(t, err)
	require.Len(t, expected, len(cases))
	actual := make([]RequestSnapshot, 0, len(cases))
	for i, tc := range cases {
		require.Equal(t, tc.ExpectedPath, expected[i].Path, tc.Name)
		actual = append(actual, captureRequestTarget(t, tc.URL))
	}
	CompareRequestSnapshots(t, expected, actual)
}

func TestRequestSnapshot_QueryMismatch(t *testing.T) {
	const (
		helperEnv     = "AISDK_REQUEST_SNAPSHOT_TARGET"
		helperTimeout = 10 * time.Second
	)
	if target, ok := os.LookupEnv(helperEnv); ok {
		expected, err := LoadExpectedRequests(filepath.Join("testdata", "request-snapshots", "expected-requests.jsonl"))
		require.NoError(t, err)
		require.NotEmpty(t, expected)
		CompareRequestSnapshots(t, expected[:1], []RequestSnapshot{captureRequestTarget(t, target)})
		return
	}

	target := loadRequestTargetCases(t)[0].URL
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		target string
		match  bool
	}{
		{name: "matching control", target: target, match: true},
		{name: "api-version", target: strings.Replace(target, "2024-10-21", "2025-04-01-preview", 1)},
		{name: "repeated value order", target: strings.Replace(target, "feature=a&feature=b", "feature=b&feature=a", 1)},
		{name: "query missing", target: strings.SplitN(target, "?", 2)[0]},
		{name: "query escape spelling", target: strings.Replace(target, "2024-10-21", "2024%2D10%2D21", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), helperTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, executable, "-test.run=^TestRequestSnapshot_QueryMismatch$")
			cmd.Env = append(os.Environ(), helperEnv+"="+tc.target)
			output, err := cmd.CombinedOutput()
			if tc.match {
				require.NoError(t, err, "%s", output)
				return
			}
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr, "%s", output)
			assert.Contains(t, string(output), "request 0 path mismatch")
		})
	}
}

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

func TestRequestSnapshot_HeaderNormalization(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.test/v1/messages?stream=true", nil)
	req.Header.Set("Content-Type", " application/json ")
	req.Header.Set("X-API-Key", "secret")
	req.Header.Set("User-Agent", "go-test")

	snapshot, err := newRequestSnapshot("anthropic", req, []byte(`{}`))
	require.NoError(t, err)

	assert.Equal(t, "POST", snapshot.Method)
	assert.Equal(t, "/v1/messages?stream=true", snapshot.Path)
	assert.Equal(t, map[string]string{
		"content-type": "application/json",
		"x-api-key":    redactedHeaderValue,
	}, snapshot.Headers)
}
