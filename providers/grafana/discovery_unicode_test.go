package grafana

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unicodeDiscoveryFixture = `{"models":[{"id":"public","name":"Model","description":"Description","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"public"},"gateway":{"canonicalModelId":"public","aliases":[],"candidates":[{"providerInstance":"primary","provider":"anthropic","modelId":"native"}]}}]}`

func listUnicodeDiscovery(t *testing.T, document string) ([]ModelInfo, error) {
	t.Helper()
	body := &trackedBody{Reader: strings.NewReader(document)}
	p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "dummy", BaseURL: "https://configured.invalid", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
	})}})
	require.NoError(t, err)
	rows, err := p.ListModels(context.Background())
	assert.True(t, body.closed)
	return rows, err
}

func TestListModels_DiscoveryUnicodeScalars(t *testing.T) {
	fields := []struct {
		name, old string
		value     func(ModelInfo) string
	}{
		{"name", `"name":"Model"`, func(row ModelInfo) string { return row.Name }},
		{"description", `"description":"Description"`, func(row ModelInfo) string { return *row.Description }},
		{"instance", `"providerInstance":"primary"`, func(row ModelInfo) string { return row.Gateway.Candidates[0].ProviderInstance }},
		{"provider", `"provider":"anthropic"`, func(row ModelInfo) string { return row.Gateway.Candidates[0].Provider }},
		{"model", `"modelId":"native"`, func(row ModelInfo) string { return row.Gateway.Candidates[0].ModelID }},
	}
	values := []struct {
		name, raw, expected string
		valid               bool
	}{
		{"lone high", `"\ud800"`, "", false},
		{"lone low", `"\udc00"`, "", false},
		{"reversed pair", `"\udc00\ud800"`, "", false},
		{"unmatched before text", `"\ud800x"`, "", false},
		{"unmatched after pair", `"\ud83d\ude80\ud800"`, "", false},
		{"valid pair", `"\ud83d\ude80"`, "🚀", true},
		{"literal replacement", `"�"`, "�", true},
		{"escaped replacement", `"\ufffd"`, "�", true},
		{"escaped backslash", `"\\ud800"`, `\ud800`, true},
		{"escaped backslash and slash", `"\\/"`, `\/`, true},
		{"Unicode escaped backslash", `"\u005cud800"`, `\ud800`, true},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, tc := range values {
				t.Run(tc.name, func(t *testing.T) {
					key := strings.SplitN(field.old, ":", 2)[0]
					document := strings.Replace(unicodeDiscoveryFixture, field.old, key+":"+tc.raw, 1)
					require.NotEqual(t, unicodeDiscoveryFixture, document)
					rows, err := listUnicodeDiscovery(t, document)
					if tc.valid {
						require.NoError(t, err)
						require.Len(t, rows, 1)
						assert.Equal(t, tc.expected, field.value(rows[0]))
					} else {
						require.Error(t, err)
						assert.Nil(t, rows)
					}
				})
			}
		})
	}
}

func TestListModels_DiscoveryUnicodeIgnoredAndDuplicateMembers(t *testing.T) {
	document := strings.Replace(unicodeDiscoveryFixture, `"models":[`, `"future":"\ud800","models":[`, 1)
	document = strings.Replace(document, `"name":"Model"`, `"future":"\udc00","name":"Model"`, 1)
	document = strings.Replace(document, `"gateway":{`, `"gateway":{"future":"\ud800",`, 1)
	document = strings.Replace(document, `{"providerInstance":`, `{"future":"\udc00","providerInstance":`, 1)
	rows, err := listUnicodeDiscovery(t, document)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "native", rows[0].Gateway.Candidates[0].ModelID)
	for _, tc := range []struct {
		name, replacement string
		valid             bool
	}{
		{"last valid", `"modelId":"\ud800","modelId":"native"`, true},
		{"last malformed", `"modelId":"native","modelId":"\ud800"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := strings.Replace(unicodeDiscoveryFixture, `"modelId":"native"`, tc.replacement, 1)
			rows, err := listUnicodeDiscovery(t, document)
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, "native", rows[0].Gateway.Candidates[0].ModelID)
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
			}
		})
	}
}
