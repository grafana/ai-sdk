package discovery

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
	"github.com/grafana/ai-sdk/provider"
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
	handler := newTestHandler(t, lister)
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

func TestHandler_EmptyCatalogAndListingFailures(t *testing.T) {
	response := httptest.NewRecorder()
	newTestHandler(t, &testLister{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.JSONEq(t, `{"models":[]}`, response.Body.String())
	for _, tc := range []struct {
		name   string
		lister *testLister
	}{
		{"error", &testLister{err: errors.New("private backend listing failure")}},
		{"panic", &testLister{panic: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			newTestHandler(t, tc.lister).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
			assert.Equal(t, http.StatusInternalServerError, response.Code)
			assert.Equal(t, `{"error":{"message":"internal error","type":"internal_server_error","param":null,"code":"internal_error"}}`, response.Body.String())
		})
	}
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

func newTestHandler(t *testing.T, lister catalog.ModelLister) *handler {
	t.Helper()
	result, ok := New(lister, providerv4.NewHostErrorWriter()).(*handler)
	require.True(t, ok)
	return result
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

func configuredInfo() catalog.ModelInfo {
	return catalog.ModelInfo{ID: "public", Name: "Model", Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "instance", Provider: "anthropic", ModelID: "native"}}}
}

func TestHandler_ConfiguredProjection(t *testing.T) {
	infos := []catalog.ModelInfo{{
		ID: "public", Name: "sk-ordinary-display", Aliases: []string{"z-alias", "a-alias"},
		Candidates: []catalog.ConfiguredCandidate{
			{ProviderInstance: "primary", Provider: "anthropic", ModelID: "native-primary"},
			{ProviderInstance: "backup", Provider: "openai", ModelID: "native-backup"},
		},
	}}
	response := httptest.NewRecorder()
	newTestHandler(t, &testLister{models: infos}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var document struct {
		Models []model `json:"models"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	require.Len(t, document.Models, 3)
	for i, id := range []string{"a-alias", "public", "z-alias"} {
		row := document.Models[i]
		assert.Equal(t, id, row.ID)
		assert.Equal(t, id, row.Specification.ModelID)
		assert.Equal(t, "grafana", row.Specification.Provider)
		require.NotNil(t, row.Gateway)
		assert.Equal(t, "public", row.Gateway.CanonicalModelID)
		assert.Equal(t, []string{"z-alias", "a-alias"}, row.Gateway.Aliases)
		assert.Equal(t, []configuredCandidate{{ProviderInstance: "primary", Provider: "anthropic", ModelID: "native-primary"}, {ProviderInstance: "backup", Provider: "openai", ModelID: "native-backup"}}, row.Gateway.Candidates)
	}
	assert.Contains(t, response.Body.String(), "sk-ordinary-display")
}

func TestHandler_ProjectsWithoutRevalidatingCatalog(t *testing.T) {
	info := configuredInfo()
	info.Aliases = []string{"public", "duplicate", "duplicate"}
	info.Candidates = append(info.Candidates, info.Candidates[0])
	info.Name = " "
	response := httptest.NewRecorder()
	newTestHandler(t, &testLister{models: []catalog.ModelInfo{info}}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var document struct {
		Models []model `json:"models"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	require.Len(t, document.Models, 4)
	for _, row := range document.Models {
		assert.Equal(t, " ", row.Name)
		assert.Equal(t, info.Aliases, row.Gateway.Aliases)
		assert.Len(t, row.Gateway.Candidates, 2)
	}
}

func TestHandler_EncodesCompleteCatalogWithoutResponseLimit(t *testing.T) {
	info := configuredInfo()
	info.Aliases = []string{}
	for i := range 64 {
		info.Aliases = append(info.Aliases, fmt.Sprintf("alias-%d", i))
	}
	info.Candidates = nil
	for i := range 16 {
		info.Candidates = append(info.Candidates, catalog.ConfiguredCandidate{ProviderInstance: "instance", Provider: "anthropic", ModelID: fmt.Sprintf("%d-", i) + strings.Repeat("a", 1048)})
	}
	response := httptest.NewRecorder()
	newTestHandler(t, &testLister{models: []catalog.ModelInfo{info}}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Greater(t, response.Body.Len(), 1<<20)
	var document struct {
		Models []model `json:"models"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &document))
	require.Len(t, document.Models, 65)
	for _, row := range document.Models {
		assert.Len(t, row.Gateway.Candidates, 16)
		assert.Equal(t, info.Aliases, row.Gateway.Aliases)
	}
}

func TestHandler_StandardJSONEscaping(t *testing.T) {
	compiled, err := schema.CompileSchema(discoverySchemaJSON)
	require.NoError(t, err)
	for _, text := range []string{`quote" slash\ controls` + "\x00\n<>&", "line\u2028paragraph\u2029end", strings.Repeat("\x00", 2048), strings.Repeat("é", 1024)} {
		info := configuredInfo()
		info.Name = text
		info.Description = text
		info.Candidates[0].ModelID = text
		info.Aliases = []string{"a", "z" + strings.Repeat("-", 127)}
		data, err := newTestHandler(t, &testLister{}).encode([]catalog.ModelInfo{info})
		require.NoError(t, err)
		require.NoError(t, compiled.Validate(json.RawMessage(data)))
		var document struct {
			Models []model `json:"models"`
		}
		require.NoError(t, json.Unmarshal(data, &document))
		require.Len(t, document.Models, 3)
		for _, row := range document.Models {
			assert.Equal(t, text, row.Name)
			assert.Equal(t, text, row.Description)
			assert.Equal(t, text, row.Gateway.Candidates[0].ModelID)
		}
	}
}

type scopeKey struct{}

type scopedTestCatalog struct{ scopes map[string]catalog.Catalog }

func (c scopedTestCatalog) ListModels(ctx context.Context) ([]catalog.ModelInfo, error) {
	scope, _ := ctx.Value(scopeKey{}).(string)
	return c.scopes[scope].ListModels(ctx)
}
func (c scopedTestCatalog) ResolveModel(ctx context.Context, id string) (catalog.ResolvedModel, error) {
	scope, _ := ctx.Value(scopeKey{}).(string)
	return c.scopes[scope].ResolveModel(ctx, id)
}

type discoveryOnlyModel struct{ calls atomic.Int32 }

func (*discoveryOnlyModel) SpecificationVersion() string               { return "v4" }
func (*discoveryOnlyModel) Provider() string                           { return "response-provider-not-configured" }
func (*discoveryOnlyModel) ModelID() string                            { return "response-model-not-configured" }
func (*discoveryOnlyModel) SupportedURLs() map[string][]*regexp.Regexp { return nil }
func (m *discoveryOnlyModel) DoGenerate(context.Context, provider.CallOptions) (*provider.GenerateResult, error) {
	m.calls.Add(1)
	return nil, assert.AnError
}
func (m *discoveryOnlyModel) DoStream(context.Context, provider.CallOptions) (*provider.StreamResult, error) {
	m.calls.Add(1)
	return nil, assert.AnError
}

func TestHandler_ScopedListingMatchesResolution(t *testing.T) {
	model := &discoveryOnlyModel{}
	scoped := scopedTestCatalog{scopes: map[string]catalog.Catalog{}}
	for _, scope := range []string{"first", "second"} {
		created, err := catalog.NewStatic([]catalog.StaticEntry{{Info: catalog.ModelInfo{ID: scope, Name: "sk-ordinary-" + scope, Aliases: []string{scope + "-alias"}, Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: scope + "-instance", Provider: "anthropic", ModelID: scope + "-native"}}}, Model: model}})
		require.NoError(t, err)
		scoped.scopes[scope] = created
	}
	handler := newTestHandler(t, scoped)
	for _, scope := range []string{"first", "second", "first"} {
		ctx := context.WithValue(context.Background(), scopeKey{}, scope)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil).WithContext(ctx))
		require.Equal(t, 200, response.Code)
		assert.Contains(t, response.Body.String(), scope+"-instance")
		assert.Contains(t, response.Body.String(), "sk-ordinary-"+scope)
		other := "first"
		if scope == other {
			other = "second"
		}
		assert.NotContains(t, response.Body.String(), other+"-instance")
		assert.NotContains(t, response.Body.String(), "response-model-not-configured")
		resolved, err := scoped.ResolveModel(ctx, scope+"-alias")
		require.NoError(t, err)
		assert.Equal(t, scope, resolved.ID)
		_, err = scoped.ResolveModel(ctx, other)
		require.ErrorIs(t, err, catalog.ErrUnknownModel)
	}
	assert.Zero(t, model.calls.Load())
}
