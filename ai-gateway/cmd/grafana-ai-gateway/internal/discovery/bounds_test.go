package discovery

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configuredInfo() catalog.ModelInfo {
	return catalog.ModelInfo{ID: "public", Name: "Model", Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "instance", Provider: "anthropic", ModelID: "native"}}}
}

func TestHandler_ConfiguredBounds(t *testing.T) {
	tests := []struct {
		name  string
		build func(int) []catalog.ModelInfo
		exact int
	}{
		{"rows", func(n int) []catalog.ModelInfo {
			infos := make([]catalog.ModelInfo, n)
			for i := range infos {
				infos[i] = catalog.ModelInfo{ID: fmt.Sprintf("model-%04d", i), Name: "Model"}
			}
			return infos
		}, MaxModelRows},
		{"aliases", func(n int) []catalog.ModelInfo {
			info := configuredInfo()
			for i := 0; i < n; i++ {
				info.Aliases = append(info.Aliases, fmt.Sprintf("alias-%03d", i))
			}
			return []catalog.ModelInfo{info}
		}, MaxAliases},
		{"candidates", func(n int) []catalog.ModelInfo {
			info := configuredInfo()
			info.Candidates = nil
			for i := 0; i < n; i++ {
				info.Candidates = append(info.Candidates, catalog.ConfiguredCandidate{ProviderInstance: "instance", Provider: "anthropic", ModelID: fmt.Sprintf("native-%d", i)})
			}
			return []catalog.ModelInfo{info}
		}, MaxCandidates},
	}
	for _, field := range []string{"name", "description", "instance", "provider", "model"} {
		tests = append(tests, struct {
			name  string
			build func(int) []catalog.ModelInfo
			exact int
		}{field, func(n int) []catalog.ModelInfo {
			info := configuredInfo()
			value := strings.Repeat("a", n)
			switch field {
			case "name":
				info.Name = value
			case "description":
				info.Description = value
			case "instance":
				info.Candidates[0].ProviderInstance = value
			case "provider":
				info.Candidates[0].Provider = value
			case "model":
				info.Candidates[0].ModelID = value
			}
			return []catalog.ModelInfo{info}
		}, MaxStringBytes})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, Validate(tc.build(tc.exact), 4<<20))
			require.Error(t, Validate(tc.build(tc.exact+1), 4<<20))
			response := httptest.NewRecorder()
			newTestHandler(t, &testLister{models: tc.build(tc.exact + 1)}, 4<<20).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
			assert.Equal(t, 500, response.Code)
			assert.NotContains(t, response.Body.String(), `"models"`)
		})
	}
	t.Run("UTF-8 bytes not rune count", func(t *testing.T) {
		info := configuredInfo()
		info.Candidates[0].ModelID = strings.Repeat("é", MaxStringBytes/2)
		require.NoError(t, Validate([]catalog.ModelInfo{info}, 4<<20))
		info.Candidates[0].ModelID += "é"
		require.Error(t, Validate([]catalog.ModelInfo{info}, 4<<20))
	})
}

func TestHandler_AggregateAliasExpansionLimit(t *testing.T) {
	for _, extra := range []int{0, 1} {
		t.Run(fmt.Sprintf("extra-%d", extra), func(t *testing.T) {
			infos := make([]catalog.ModelInfo, 8)
			for i := range infos {
				infos[i] = configuredInfo()
				infos[i].ID = fmt.Sprintf("route-%d", i)
				aliases := 127
				if i == 0 {
					aliases += extra
				}
				for j := range aliases {
					infos[i].Aliases = append(infos[i].Aliases, fmt.Sprintf("alias-%d-%03d", i, j))
				}
			}
			response := httptest.NewRecorder()
			newTestHandler(t, &testLister{models: infos}, 4<<20).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
			if extra == 0 {
				require.Equal(t, 200, response.Code)
				var document struct {
					Models []json.RawMessage `json:"models"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
				assert.Len(t, document.Models, MaxModelRows)
			} else {
				assert.Equal(t, 500, response.Code)
				assert.NotContains(t, response.Body.String(), `"models"`)
			}
		})
	}
}

func TestHandler_ConfiguredAtomicValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*catalog.ModelInfo)
	}{
		{"alias canonical collision", func(info *catalog.ModelInfo) { info.Aliases = []string{"public"} }},
		{"duplicate alias", func(info *catalog.ModelInfo) { info.Aliases = []string{"alias", "alias"} }},
		{"duplicate tuple different provider", func(info *catalog.ModelInfo) {
			c := info.Candidates[0]
			c.Provider = "openai"
			info.Candidates = append(info.Candidates, c)
		}},
		{"blank instance", func(info *catalog.ModelInfo) { info.Candidates[0].ProviderInstance = " " }},
		{"empty model", func(info *catalog.ModelInfo) { info.Candidates[0].ModelID = "" }},
		{"invalid UTF-8", func(info *catalog.ModelInfo) { info.Candidates[0].Provider = string([]byte{0xff}) }},
		{"invalid alias", func(info *catalog.ModelInfo) { info.Aliases = []string{"has space"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := configuredInfo()
			tc.change(&info)
			assertInternalOnly(t, newTestHandler(t, &testLister{models: []catalog.ModelInfo{{ID: "first", Name: "First"}, info}}, 4<<20))
		})
	}
	t.Run("late duplicate row", func(t *testing.T) {
		info := configuredInfo()
		assertInternalOnly(t, newTestHandler(t, &testLister{models: []catalog.ModelInfo{info, info}}, 4<<20))
	})
	t.Run("cross route alias collision", func(t *testing.T) {
		info := configuredInfo()
		info.Aliases = []string{"other"}
		assertInternalOnly(t, newTestHandler(t, &testLister{models: []catalog.ModelInfo{info, {ID: "other", Name: "Other"}}}, 4<<20))
	})
}

func TestHandler_ConfiguredEncodedBudget(t *testing.T) {
	info := configuredInfo()
	info.Name = `quote" slash\ controls` + "\x00\n<>&"
	info.Aliases = []string{"alias"}
	infos := []catalog.ModelInfo{info}
	document, err := newTestHandler(t, &testLister{}, 4<<20).encode(infos)
	require.NoError(t, err)
	require.NoError(t, Validate(infos, int64(len(document))))
	require.Error(t, Validate(infos, int64(len(document)-1)))
	compiled, err := schema.CompileSchema(discoverySchemaJSON)
	require.NoError(t, err)
	require.NoError(t, compiled.Validate(json.RawMessage(document)))
	info.Candidates = nil
	for i := range MaxCandidates {
		info.Candidates = append(info.Candidates, catalog.ConfiguredCandidate{ProviderInstance: "instance", Provider: "anthropic", ModelID: fmt.Sprintf("%d-", i) + strings.Repeat("a", 1048)})
	}
	info.Aliases = nil
	for i := range 64 {
		info.Aliases = append(info.Aliases, fmt.Sprintf("alias-%d", i))
	}
	require.Error(t, Validate([]catalog.ModelInfo{info}, 1<<20))
	require.NoError(t, Validate([]catalog.ModelInfo{info}, 4<<20))
	for _, limit := range []int64{0, -1, int64(^uint64(0) >> 1)} {
		require.Error(t, Validate(infos, limit))
	}
}

func TestHandler_ConfiguredPreflightAllocation(t *testing.T) {
	oversized := configuredInfo()
	oversized.Candidates = make([]catalog.ConfiguredCandidate, MaxCandidates+1)
	oversized.Candidates[0].ModelID = strings.Repeat("a", 1<<20)
	invalid := false
	allocations := testing.AllocsPerRun(100, func() {
		rows, err := prepareRows([]catalog.ModelInfo{oversized}, 1<<20)
		if rows != nil || err != errResponseLimit {
			invalid = true
		}
	})
	require.False(t, invalid)
	assert.Zero(t, allocations, "impossible candidate counts fail before expansion or string encoding")
}

func FuzzHandler_ConfiguredProjection(f *testing.F) {
	f.Add("public", "alias", "native", int64(1024))
	f.Fuzz(func(t *testing.T, id, alias, model string, limit int64) {
		if len(id)+len(alias)+len(model) > 8192 || limit < 1 || limit > 8192 {
			t.Skip()
		}
		info := configuredInfo()
		info.ID = id
		info.Aliases = []string{alias}
		info.Candidates[0].ModelID = model
		data, err := (&handler{limit: limit}).encode([]catalog.ModelInfo{info})
		if err != nil {
			assert.Nil(t, data)
			return
		}
		assert.LessOrEqual(t, int64(len(data)), limit)
		assert.True(t, json.Valid(data))
	})
}
