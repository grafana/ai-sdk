package discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed schema.json
var discoverySchemaJSON []byte

func TestHandler_ClosedSortedCanonicalAndAliasProjection(t *testing.T) {
	lister := &testLister{models: []catalog.ModelInfo{
		{ID: "zeta", Name: "Zeta", Description: "Private-safe description", Aliases: []string{"alpha"}},
		{ID: "middle", Name: "Middle"},
	}}
	handler := newTestHandler(t, lister, 1<<20)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	assert.JSONEq(t, `{
		"models":[
			{"id":"alpha","name":"Zeta","description":"Private-safe description","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"alpha"}},
			{"id":"middle","name":"Middle","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"middle"}},
			{"id":"zeta","name":"Zeta","description":"Private-safe description","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"zeta"}}
		]
	}`, response.Body.String())
	for _, private := range []string{"anthropic-primary", "anthropic", "backend-private", "ANTHROPIC_API_KEY", "secret-api-key", "https://provider.example", "aliases"} {
		assert.NotContains(t, response.Body.String(), private)
	}
}

func TestHandler_ResponseBoundariesAndPrecommitFailures(t *testing.T) {
	lister := &testLister{models: []catalog.ModelInfo{{ID: "model", Name: "Model", Aliases: []string{"alias"}}}}
	large := newTestHandler(t, lister, 1<<20)
	document, err := large.encode(lister.models)
	require.NoError(t, err)

	for _, tc := range []struct {
		name   string
		limit  int64
		status int
	}{
		{name: "below", limit: int64(len(document) + 1), status: http.StatusOK},
		{name: "exact", limit: int64(len(document)), status: http.StatusOK},
		{name: "over", limit: int64(len(document) - 1), status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := newTestHandler(t, lister, tc.limit)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
			assert.Equal(t, tc.status, response.Code)
			if tc.status == http.StatusOK {
				assert.LessOrEqual(t, int64(response.Body.Len()), tc.limit)
			} else {
				assert.Equal(t, `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`, response.Body.String())
				assert.NotContains(t, response.Body.String(), `"models"`)
			}
		})
	}

	t.Run("listing error", func(t *testing.T) {
		lister := &testLister{err: errors.New("private backend listing failure")}
		assertInternalOnly(t, newTestHandler(t, lister, 1<<20))
	})

	t.Run("listing panic", func(t *testing.T) {
		lister := &testLister{panic: true}
		assertInternalOnly(t, newTestHandler(t, lister, 1<<20))
	})

	t.Run("invalid UTF-8", func(t *testing.T) {
		lister := &testLister{models: []catalog.ModelInfo{{ID: "model", Name: string([]byte{0xff})}}}
		assertInternalOnly(t, newTestHandler(t, lister, 1<<20))
	})

	for _, id := range []string{"bad id", "grafaná", "a" + strings.Repeat("-", 128)} {
		t.Run("invalid public ID "+id[:1], func(t *testing.T) {
			lister := &testLister{models: []catalog.ModelInfo{{ID: id, Name: "Model"}}}
			assertInternalOnly(t, newTestHandler(t, lister, 1<<20))
		})
	}

}

func TestHandler_EncodingStopsAtBoundAcrossRepeatedAliases(t *testing.T) {
	aliases := make([]string, 1024)
	for index := range aliases {
		aliases[index] = fmt.Sprintf("alias-%04d", index)
	}
	lister := &testLister{models: []catalog.ModelInfo{{
		ID:          "model",
		Name:        "Model",
		Description: strings.Repeat("description", 1<<13),
		Aliases:     aliases,
	}}}
	rows, err := prepareRows(lister.models, 1024)
	require.ErrorIs(t, err, errResponseLimit)
	assert.Nil(t, rows, "impossible row counts must fail before alias rows are materialized")
	measure := func(infos []catalog.ModelInfo, limit int64) float64 {
		invalidResult := false
		allocations := testing.AllocsPerRun(100, func() {
			rows, err := prepareRows(infos, limit)
			if rows != nil || !errors.Is(err, errResponseLimit) {
				invalidResult = true
			}
		})
		require.False(t, invalidResult)
		return allocations
	}
	baseline := []catalog.ModelInfo{{ID: "model", Name: "Model"}}
	assert.Equal(t, measure(baseline, 1), measure(lister.models, 1024), "preflight allocation must not grow with aliases")

	handler := newTestHandler(t, lister, 1024)
	document, err := handler.encode(lister.models)
	require.ErrorIs(t, err, errResponseLimit)
	assert.Nil(t, document)

	buffer := newBoundedBuffer(1025)
	encodeModel(&buffer, discoveryModel("alias", "Model", lister.models[0].Description))
	assert.True(t, buffer.overflow)
	assert.LessOrEqual(t, len(buffer.data), 1025)
}

func TestDiscoverySchema_ClosedDraft202012(t *testing.T) {
	compiled, err := schema.CompileSchema(discoverySchemaJSON)
	require.NoError(t, err)
	valid := `{"models":[{"id":"model","name":"Model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"model"}}]}`
	require.NoError(t, compiled.Validate(json.RawMessage(valid)))

	invalid := []string{
		`{}`,
		`{"models":[],"private":"value"}`,
		`{"models":[{"name":"Model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"model"}}]}`,
		`{"models":[{"id":"model","name":"Model","private":"value","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"model"}}]}`,
		`{"models":[{"id":"model","name":"Model","specification":{"specificationVersion":"v3","provider":"grafana","modelId":"model"}}]}`,
		`{"models":[{"id":"model","name":"Model","specification":{"specificationVersion":"v4","provider":"anthropic","modelId":"model"}}]}`,
		`{"models":[{"id":"model","name":"Model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"model","backend":"private"}}]}`,
		`{"models":[{"id":"bad id","name":"Model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"bad id"}}]}`,
		`{"models":[{"id":"model","name":"Model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"grafaná"}}]}`,
	}
	for _, document := range invalid {
		assert.Error(t, compiled.Validate(json.RawMessage(document)), document)
	}
}

func newTestHandler(t *testing.T, lister catalog.ModelLister, limit int64) *handler {
	t.Helper()
	errorWriter := providerv4.NewHostErrorWriter()
	created, err := New(lister, errorWriter, limit)
	require.NoError(t, err)
	result, ok := created.(*handler)
	require.True(t, ok)
	return result
}

func assertInternalOnly(t *testing.T, handler http.Handler) {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`, response.Body.String())
	assert.NotContains(t, response.Body.String(), "private")
}

type testLister struct {
	models []catalog.ModelInfo
	err    error
	panic  bool
}

func (lister *testLister) ListModels(context.Context) ([]catalog.ModelInfo, error) {
	if lister.panic {
		panic("private panic")
	}
	return lister.models, lister.err
}
