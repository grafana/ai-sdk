package grafana

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListModels_AtomicValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, media string
		valid             bool
	}{
		{"valid", discoveryFixture, "application/json", true},
		{"nullable description", strings.Replace(discoveryFixture, `"description":"public description"`, `"description":null`, 1), "application/json", true},
		{"wrong property casing", strings.Replace(discoveryFixture, `"models"`, `"Models"`, 1), "application/json", false},
		{"escaped lone surrogate", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"name":"\ud800"`, 1), "application/json", true},
		{"empty", `{"models":[]}`, "application/json", true},
		{"additive", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"private":{"backend":"do-not-expose"},"name":"Assistant"`, 1), "application/json", true},
		{"missing models", `{}`, "application/json", false},
		{"null models", `{"models":null}`, "application/json", false},
		{"null row", `{"models":[null]}`, "application/json", false},
		{"malformed", `{"models":[`, "application/json", false},
		{"trailing", discoveryFixture + ` {}`, "application/json", false},
		{"duplicate", strings.ReplaceAll(discoveryFixture, "grafana/assistant", "assistant"), "application/json", false},
		{"empty name", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"name":""`, 1), "application/json", false},
		{"missing name", strings.Replace(discoveryFixture, `"name":"Assistant",`, "", 1), "application/json", false},
		{"null name", strings.Replace(discoveryFixture, `"name":"Assistant"`, `"name":null`, 1), "application/json", false},
		{"invalid ID", strings.ReplaceAll(discoveryFixture, "grafana/assistant", "has space"), "application/json", false},
		{"wrong spec", strings.Replace(discoveryFixture, `"v4"`, `"v3"`, 1), "application/json", false},
		{"wrong provider", strings.Replace(discoveryFixture, `"provider":"grafana"`, `"provider":"private-backend"`, 1), "application/json", false},
		{"mismatched ID", strings.Replace(discoveryFixture, `"modelId":"assistant"`, `"modelId":"private-backend"`, 1), "application/json", false},
		{"wrong description", strings.Replace(discoveryFixture, `"description":"public description"`, `"description":2`, 1), "application/json", false},
		{"invalid UTF8", strings.Replace(discoveryFixture, "Assistant", string([]byte{255}), 1), "application/json", false},
		{"wrong media", discoveryFixture, "text/html", false},
		{"missing media", discoveryFixture, "", false},
		{"suffix media", discoveryFixture, "application/problem+json", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header()["Content-Type"] = []string{tc.media}
				_, _ = io.WriteString(w, tc.body)
			}, nil)
			rows, err := p.ListModels(context.Background())
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, rows)
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
				assert.NotContains(t, err.Error(), "private-backend")
				var api *provider.APICallError
				require.ErrorAs(t, err, &api)
				assert.False(t, api.IsRetryable)
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
func (b *trackedBody) Close() error { b.closed = true; return nil }

func TestListModels_BoundsAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int64
		body   string
		status int
		valid  bool
	}{
		{"exact", int64(len(discoveryFixture)), discoveryFixture, 200, true},
		{"one over", int64(len(discoveryFixture)) - 1, discoveryFixture, 200, false},
		{"large", 16, strings.Repeat("x", 100000), 200, false},
		{"error bound", 16, strings.Repeat("private-error", 10000), 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tc.body)}
			limits := DefaultLimits()
			limits.DiscoveryBytes = tc.limit
			limits.ErrorBytes = tc.limit
			p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", Limits: &limits, HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
			})}})
			require.NoError(t, err)
			rows, err := p.ListModels(context.Background())
			if tc.valid {
				require.NoError(t, err)
				assert.Len(t, rows, 2)
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
			}
			assert.True(t, body.closed)
			assert.LessOrEqual(t, body.read, int(tc.limit+1))
		})
	}
	t.Run("read failure closes body", func(t *testing.T) {
		cause := errors.New("read failure")
		body := &trackedBody{Reader: errorReader{cause}}
		p, err := NewWithAccessToken(AccessTokenConfig{AccessToken: "token", BaseURL: "https://example.test", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body, Request: req}, nil
		})}})
		require.NoError(t, err)
		_, err = p.ListModels(context.Background())
		assert.ErrorIs(t, err, cause)
		assert.True(t, body.closed)
	})
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

const configuredGatewayFixture = `{"canonicalModelId":"public","aliases":["alias"],"candidates":[{"providerInstance":"primary","provider":"anthropic","modelId":"native-primary"},{"providerInstance":"backup","provider":"openai","modelId":"native-backup"}]}`

func configuredDiscoveryFixture(t *testing.T) string {
	t.Helper()
	var gateway any
	require.NoError(t, json.Unmarshal([]byte(configuredGatewayFixture), &gateway))
	rows := []any{}
	for _, id := range []string{"alias", "public"} {
		rows = append(rows, map[string]any{"id": id, "name": "Model", "specification": map[string]any{"specificationVersion": "v4", "provider": "grafana", "modelId": id}, "gateway": gateway})
	}
	data, err := json.Marshal(map[string]any{"models": rows})
	require.NoError(t, err)
	return string(data)
}

func configuredRows(t *testing.T) []ModelInfo {
	t.Helper()
	var document struct {
		Models []ModelInfo `json:"models"`
	}
	require.NoError(t, json.Unmarshal([]byte(configuredDiscoveryFixture(t)), &document))
	return document.Models
}

func discoveryBody(t *testing.T, rows []ModelInfo) string {
	t.Helper()
	data, err := json.Marshal(struct {
		Models []ModelInfo `json:"models"`
	}{rows})
	require.NoError(t, err)
	return string(data)
}

func TestListModels_ConfiguredRetention(t *testing.T) {
	calls := 0
	p := testProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/aisdk/config", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, configuredDiscoveryFixture(t))
	}, nil)
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, rows, 2)
	data, err := json.Marshal(rows)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"gateway":`)
	assert.Contains(t, string(data), `"providerInstance":"backup"`)
	assert.Equal(t, 1, calls)
	require.NotNil(t, rows[0].Gateway)
	assert.Equal(t, "public", rows[0].Gateway.CanonicalModelID)
	assert.Equal(t, []string{"alias"}, rows[0].Gateway.Aliases)
	assert.Equal(t, "backup", rows[0].Gateway.Candidates[1].ProviderInstance)
	rows[0].Gateway.Candidates[0].ModelID = "caller mutation"
	assert.Equal(t, "native-primary", rows[1].Gateway.Candidates[0].ModelID)
	assert.EqualValues(t, 4<<20, DefaultLimits().DiscoveryBytes)
}

func TestListModels_ConfiguredAtomicValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]ModelInfo) []ModelInfo
	}{
		{"absent extension", func(rows []ModelInfo) []ModelInfo {
			for i := range rows {
				rows[i].Gateway = nil
			}
			return rows
		}},
		{"missing canonical", func(rows []ModelInfo) []ModelInfo { return rows[:1] }},
		{"missing alias", func(rows []ModelInfo) []ModelInfo { return rows[1:] }},
		{"contradictory projection", func(rows []ModelInfo) []ModelInfo { rows[1].Gateway.Candidates[0].ModelID = "other"; return rows }},
		{"undeclared row", func(rows []ModelInfo) []ModelInfo {
			rows[0].ID = "other"
			rows[0].Specification.ModelID = "other"
			return rows
		}},
		{"duplicate rows", func(rows []ModelInfo) []ModelInfo { return append(rows, rows[0]) }},
		{"duplicate aliases", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.Aliases = []string{"alias", "alias"}; return rows }},
		{"canonical alias collision", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.Aliases = []string{"public"}; return rows }},
		{"invalid canonical", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.CanonicalModelID = "bad id"; return rows }},
		{"empty candidates", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.Candidates = []ConfiguredCandidate{}; return rows }},
		{"null aliases", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.Aliases = nil; return rows }},
		{"null candidates", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.Candidates = nil; return rows }},
		{"blank identity", func(rows []ModelInfo) []ModelInfo { rows[0].Gateway.Candidates[0].ProviderInstance = " "; return rows }},
		{"duplicate tuple", func(rows []ModelInfo) []ModelInfo {
			candidate := rows[0].Gateway.Candidates[0]
			candidate.Provider = "other"
			rows[0].Gateway.Candidates = append(rows[0].Gateway.Candidates, candidate)
			return rows
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := discoveryBody(t, tc.mutate(configuredRows(t)))
			p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}, nil)
			rows, err := p.ListModels(context.Background())
			if tc.name == "absent extension" {
				require.NoError(t, err)
				assert.Nil(t, rows[0].Gateway)
			} else {
				require.Error(t, err)
				assert.Nil(t, rows)
				assert.NotContains(t, err.Error(), "native-primary")
			}
		})
	}
	body := discoveryBody(t, configuredRows(t))
	for _, tc := range []struct{ name, old, replacement string }{
		{"null gateway", configuredGatewayFixture, "null"},
		{"missing aliases", `"aliases":["alias"],`, ""},
		{"null candidate", `{"providerInstance":"primary","provider":"anthropic","modelId":"native-primary"}`, "null"},
		{"null model", `"modelId":"native-primary"`, `"modelId":null`},
		{"missing model", `,"modelId":"native-primary"`, ""},
		{"wrong candidate shape", `"providerInstance":"primary"`, `"providerInstance":2`},
		{"wrong candidate casing", `"providerInstance":"primary"`, `"ProviderInstance":"primary"`},
		{"wrong route casing", `"canonicalModelId":"public"`, `"CanonicalModelId":"public"`},
		{"normalized duplicate tuple", `"modelId":"native-primary"`, `"modelId":"\ud800"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.ReplaceAll(body, tc.old, tc.replacement)
			if tc.name == "normalized duplicate tuple" {
				changed = strings.ReplaceAll(changed, `"providerInstance":"backup"`, `"providerInstance":"primary"`)
				changed = strings.ReplaceAll(changed, `"modelId":"native-backup"`, `"modelId":"\ufffd"`)
			}
			require.NotEqual(t, body, changed)
			p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, changed)
			}, nil)
			rows, err := p.ListModels(context.Background())
			require.Error(t, err)
			assert.Nil(t, rows)
		})
	}
}

func TestListModels_ConfiguredUnknownAdditionsAreNotPromoted(t *testing.T) {
	body := discoveryBody(t, configuredRows(t))
	body = strings.ReplaceAll(body, `"gateway":{`, `"gateway":{"future":{"apiKey":"ignored-dummy-secret"},`)
	body = strings.ReplaceAll(body, `"providerInstance":`, `"future":"ignored-dummy-secret","providerInstance":`)
	body = strings.ReplaceAll(body, `"models":[`, `"future":"ignored-dummy-secret","models":[`)
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}, nil)
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, configuredRows(t), rows)
	encoded, err := json.Marshal(rows)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "ignored-dummy-secret")
}

func TestListModels_ClientAllowanceDoesNotSetServerCapacity(t *testing.T) {
	route := ConfiguredRoute{CanonicalModelID: "public", Aliases: []string{}, Candidates: []ConfiguredCandidate{}}
	for i := range 16 {
		route.Candidates = append(route.Candidates, ConfiguredCandidate{ProviderInstance: "instance", Provider: "anthropic", ModelID: fmt.Sprintf("%d-", i) + strings.Repeat("a", 1048)})
	}
	for i := range 64 {
		route.Aliases = append(route.Aliases, fmt.Sprintf("alias-%d", i))
	}
	var models []ModelInfo
	for _, id := range append([]string{route.CanonicalModelID}, route.Aliases...) {
		models = append(models, ModelInfo{ID: id, Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: id}, Gateway: &route})
	}
	body := discoveryBody(t, models)
	require.Greater(t, len(body), 1<<20)
	require.Less(t, len(body), 4<<20)
	p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}, nil)
	rows, err := p.ListModels(context.Background())
	require.NoError(t, err)
	assert.Len(t, rows, len(models))
	p.limits.DiscoveryBytes = 1 << 20
	rows, err = p.ListModels(context.Background())
	require.Error(t, err)
	assert.Nil(t, rows)
}

func TestListModels_ClientDoesNotDuplicateServerPolicy(t *testing.T) {
	for _, dimension := range []string{"rows", "aliases", "candidates", "name", "description", "instance", "provider", "model"} {
		t.Run(dimension, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				route := ConfiguredRoute{CanonicalModelID: "public", Aliases: []string{}, Candidates: []ConfiguredCandidate{{ProviderInstance: "p", Provider: "anthropic", ModelID: "native"}}}
				rows := []ModelInfo{{ID: "public", Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: "public"}, Gateway: &route}}
				switch dimension {
				case "rows":
					rows = nil
					for i := range 1024 + extra {
						id := fmt.Sprintf("row-%04d", i)
						rows = append(rows, ModelInfo{ID: id, Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: id}})
					}
				case "aliases":
					for i := range 128 + extra {
						id := fmt.Sprintf("alias-%d", i)
						route.Aliases = append(route.Aliases, id)
						rows = append(rows, ModelInfo{ID: id, Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: id}, Gateway: &route})
					}
				case "candidates":
					route.Candidates = nil
					for i := range 16 + extra {
						route.Candidates = append(route.Candidates, ConfiguredCandidate{ProviderInstance: "p", Provider: "anthropic", ModelID: fmt.Sprintf("model-%d", i)})
					}
				case "name":
					rows[0].Name = strings.Repeat("a", 2048+extra)
				case "description":
					value := strings.Repeat("a", 2048+extra)
					rows[0].Description = &value
				case "instance":
					route.Candidates[0].ProviderInstance = strings.Repeat("a", 2048+extra)
				case "provider":
					route.Candidates[0].Provider = strings.Repeat("a", 2048+extra)
				case "model":
					route.Candidates[0].ModelID = strings.Repeat("é", 1024) + strings.Repeat("a", extra)
				}
				body := discoveryBody(t, rows)
				p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, body)
				}, nil)
				got, err := p.ListModels(context.Background())
				require.NoError(t, err)
				assert.Equal(t, rows, got)
			}
		})
	}
}

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

func TestListModels_DiscoveryStandardJSONUnicode(t *testing.T) {
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
	}{
		{"lone high", `"\ud800"`, "�"},
		{"lone low", `"\udc00"`, "�"},
		{"reversed pair", `"\udc00\ud800"`, "��"},
		{"unmatched before text", `"\ud800x"`, "�x"},
		{"unmatched after pair", `"\ud83d\ude80\ud800"`, "🚀�"},
		{"valid pair", `"\ud83d\ude80"`, "🚀"},
		{"literal replacement", `"�"`, "�"},
		{"escaped replacement", `"\ufffd"`, "�"},
		{"escaped backslash", `"\\ud800"`, `\ud800`},
		{"escaped backslash and slash", `"\\/"`, `\/`},
		{"Unicode escaped backslash", `"\u005cud800"`, `\ud800`},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			for _, tc := range values {
				t.Run(tc.name, func(t *testing.T) {
					key := strings.SplitN(field.old, ":", 2)[0]
					document := strings.Replace(unicodeDiscoveryFixture, field.old, key+":"+tc.raw, 1)
					require.NotEqual(t, unicodeDiscoveryFixture, document)
					rows, err := listUnicodeDiscovery(t, document)
					require.NoError(t, err)
					require.Len(t, rows, 1)
					assert.Equal(t, tc.expected, field.value(rows[0]))
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
		name, replacement, expected string
	}{
		{"last ordinary", `"modelId":"\ud800","modelId":"native"`, "native"},
		{"last normalized", `"modelId":"native","modelId":"\ud800"`, "�"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := strings.Replace(unicodeDiscoveryFixture, `"modelId":"native"`, tc.replacement, 1)
			rows, err := listUnicodeDiscovery(t, document)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, tc.expected, rows[0].Gateway.Candidates[0].ModelID)
		})
	}
}
