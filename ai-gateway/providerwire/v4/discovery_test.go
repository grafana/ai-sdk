package v4

import (
	_ "embed"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/schema"
)

//go:embed schema/discovery.json
var discoverySchemaJSON []byte

type discoveryDocument struct {
	Models []discoveryModel `json:"models"`
}

func projectDiscovery(t *testing.T, models []catalog.ModelInfo) []byte {
	t.Helper()
	document, err := DiscoveryProjection{}.ProjectDiscovery(models)
	require.NoError(t, err)
	return document
}

func configuredInfo() catalog.ModelInfo {
	return catalog.ModelInfo{ID: "public", Name: "Model", Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "instance", Provider: "anthropic", ModelID: "native"}}}
}

func TestDiscoveryProjection_ClosedSortedCanonicalProjection(t *testing.T) {
	document := string(projectDiscovery(t, []catalog.ModelInfo{
		{ID: "zeta", Name: "Zeta", Description: "Private-safe description", Aliases: []string{"alpha"}},
		{ID: "middle", Name: "Middle"},
	}))
	assert.JSONEq(t, `{
		"models":[
			{"id":"middle","name":"Middle","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"middle"}},
			{"id":"zeta","name":"Zeta","description":"Private-safe description","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"zeta"}}
		]
	}`, document)
	for _, private := range []string{"anthropic-primary", "anthropic", "backend-private", "ANTHROPIC_API_KEY", "secret-api-key", "https://provider.example", "aliases"} {
		assert.NotContains(t, document, private)
	}
}

func TestDiscoveryProjection_EmptyCatalog(t *testing.T) {
	assert.Equal(t, `{"models":[]}`, string(projectDiscovery(t, nil)))
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

func TestDiscoveryProjection_ConfiguredProjection(t *testing.T) {
	compiled, err := schema.CompileSchema(discoverySchemaJSON)
	require.NoError(t, err)
	data := projectDiscovery(t, []catalog.ModelInfo{{
		ID: "public", Name: "sk-ordinary-display", Aliases: []string{"z-alias", "a-alias"},
		Candidates: []catalog.ConfiguredCandidate{
			{ProviderInstance: "primary", Provider: "anthropic", ModelID: "native-primary"},
			{ProviderInstance: "backup", Provider: "openai", ModelID: "native-backup"},
		},
	}})
	assert.JSONEq(t, `{"models":[{"id":"public","name":"sk-ordinary-display","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"public"},"gateway":{"aliases":["z-alias","a-alias"],"primary":{"providerInstance":"primary","provider":"anthropic","providerModelId":"native-primary"},"fallbacks":[{"providerInstance":"backup","provider":"openai","providerModelId":"native-backup"}]}}]}`, string(data))
	var document discoveryDocument
	require.NoError(t, json.Unmarshal(data, &document))
	require.Len(t, document.Models, 1)
	row := document.Models[0]
	assert.Equal(t, "public", row.ID)
	assert.Equal(t, "public", row.Specification.ModelID)
	assert.Equal(t, "grafana", row.Specification.Provider)
	require.NotNil(t, row.Gateway)
	assert.Equal(t, []string{"z-alias", "a-alias"}, row.Gateway.Aliases)
	assert.Equal(t, configuredCandidate{ProviderInstance: "primary", Provider: "anthropic", ProviderModelID: "native-primary"}, row.Gateway.Primary)
	assert.Equal(t, []configuredCandidate{{ProviderInstance: "backup", Provider: "openai", ProviderModelID: "native-backup"}}, row.Gateway.Fallbacks)
	require.NoError(t, compiled.Validate(json.RawMessage(data)))

	t.Run("direct route has empty collections", func(t *testing.T) {
		data := projectDiscovery(t, []catalog.ModelInfo{configuredInfo()})
		assert.JSONEq(t, `{"models":[{"id":"public","name":"Model","specification":{"specificationVersion":"v4","provider":"grafana","modelId":"public"},"gateway":{"aliases":[],"primary":{"providerInstance":"instance","provider":"anthropic","providerModelId":"native"},"fallbacks":[]}}]}`, string(data))
		require.NoError(t, compiled.Validate(json.RawMessage(data)))
	})
}

func TestDiscoveryProjection_ProjectsWithoutRevalidatingCatalog(t *testing.T) {
	info := configuredInfo()
	info.Aliases = []string{"public", "duplicate", "duplicate"}
	info.Candidates = append(info.Candidates, info.Candidates[0])
	info.Name = " "
	var document discoveryDocument
	require.NoError(t, json.Unmarshal(projectDiscovery(t, []catalog.ModelInfo{info}), &document))
	require.Len(t, document.Models, 1)
	row := document.Models[0]
	assert.Equal(t, " ", row.Name)
	assert.Equal(t, info.Aliases, row.Gateway.Aliases)
	assert.Equal(t, row.Gateway.Primary, row.Gateway.Fallbacks[0])
}

func TestDiscoveryProjection_StandardJSONEscaping(t *testing.T) {
	compiled, err := schema.CompileSchema(discoverySchemaJSON)
	require.NoError(t, err)
	for _, text := range []string{`quote" slash\ controls` + "\x00\n<>&", "line\u2028paragraph\u2029end", strings.Repeat("\x00", 2048), strings.Repeat("é", 1024)} {
		info := configuredInfo()
		info.Name = text
		info.Description = text
		info.Candidates[0].ModelID = text
		info.Aliases = []string{"a", "z" + strings.Repeat("-", 127)}
		data := projectDiscovery(t, []catalog.ModelInfo{info})
		require.NoError(t, compiled.Validate(json.RawMessage(data)))
		var document discoveryDocument
		require.NoError(t, json.Unmarshal(data, &document))
		require.Len(t, document.Models, 1)
		for _, row := range document.Models {
			assert.Equal(t, text, row.Name)
			assert.Equal(t, text, row.Description)
			assert.Equal(t, text, row.Gateway.Primary.ProviderModelID)
			assert.Equal(t, []configuredCandidate{}, row.Gateway.Fallbacks)
		}
	}
}
