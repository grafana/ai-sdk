package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.ReplaceAll(body, tc.old, tc.replacement)
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
	for i := range maxDiscoveryCandidates {
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

func TestListModels_ConfiguredCardinalityAndStringBounds(t *testing.T) {
	for _, dimension := range []string{"rows", "aliases", "candidates", "name", "description", "instance", "provider", "model"} {
		t.Run(dimension, func(t *testing.T) {
			for _, extra := range []int{0, 1} {
				route := ConfiguredRoute{CanonicalModelID: "public", Aliases: []string{}, Candidates: []ConfiguredCandidate{{ProviderInstance: "p", Provider: "anthropic", ModelID: "native"}}}
				rows := []ModelInfo{{ID: "public", Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: "public"}, Gateway: &route}}
				switch dimension {
				case "rows":
					rows = nil
					for i := range maxDiscoveryRows + extra {
						id := fmt.Sprintf("row-%04d", i)
						rows = append(rows, ModelInfo{ID: id, Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: id}})
					}
				case "aliases":
					for i := range maxDiscoveryAliases + extra {
						id := fmt.Sprintf("alias-%d", i)
						route.Aliases = append(route.Aliases, id)
						rows = append(rows, ModelInfo{ID: id, Name: "Model", Specification: ModelSpecification{SpecificationVersion: "v4", Provider: "grafana", ModelID: id}, Gateway: &route})
					}
				case "candidates":
					route.Candidates = nil
					for i := range maxDiscoveryCandidates + extra {
						route.Candidates = append(route.Candidates, ConfiguredCandidate{ProviderInstance: "p", Provider: "anthropic", ModelID: fmt.Sprintf("model-%d", i)})
					}
				case "name":
					rows[0].Name = strings.Repeat("a", maxDiscoveryStringBytes+extra)
				case "description":
					value := strings.Repeat("a", maxDiscoveryStringBytes+extra)
					rows[0].Description = &value
				case "instance":
					route.Candidates[0].ProviderInstance = strings.Repeat("a", maxDiscoveryStringBytes+extra)
				case "provider":
					route.Candidates[0].Provider = strings.Repeat("a", maxDiscoveryStringBytes+extra)
				case "model":
					route.Candidates[0].ModelID = strings.Repeat("é", maxDiscoveryStringBytes/2) + strings.Repeat("a", extra)
				}
				body := discoveryBody(t, rows)
				p := testProvider(t, func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, body)
				}, nil)
				got, err := p.ListModels(context.Background())
				if extra == 0 {
					require.NoError(t, err)
					require.Len(t, got, len(rows))
				} else {
					require.Error(t, err)
					assert.Nil(t, got)
				}
			}
		})
	}
}
