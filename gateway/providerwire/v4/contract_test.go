package providerwirev4

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	validateschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const schemaIDPrefix = "https://grafana.com/ai-sdk/providerwire/v4/schema/"

var streamArmNegativeFields = map[string]string{
	"custom":                "kind",
	"error":                 "error",
	"file":                  "data",
	"finish":                "usage",
	"raw":                   "rawValue",
	"reasoning-delta":       "delta",
	"reasoning-end":         "id",
	"reasoning-file":        "data",
	"reasoning-start":       "id",
	"response-metadata":     "warnings",
	"source:document":       "id",
	"source:url":            "id",
	"stream-start":          "warnings",
	"text-delta":            "delta",
	"text-end":              "id",
	"text-start":            "id",
	"tool-approval-request": "approvalId",
	"tool-call":             "input",
	"tool-input-delta":      "delta",
	"tool-input-end":        "id",
	"tool-input-start":      "toolName",
	"tool-result":           "result",
}

type contractRegistry struct {
	compiled map[string]*validateschema.Schema
}

type corpus struct {
	Cases []corpusCase `json:"cases"`
}

type corpusCase struct {
	Name     string          `json:"name"`
	Schema   string          `json:"schema"`
	Path     string          `json:"path"`
	Status   int             `json:"status"`
	Document json.RawMessage `json:"document"`
}

type corpusRecipeFile struct {
	Cases []corpusRecipe `json:"cases"`
}

type corpusRecipe struct {
	Name      string            `json:"name"`
	Base      string            `json:"base"`
	Path      string            `json:"path"`
	Mutations []fixtureMutation `json:"mutations"`
}

type captureArtifact struct {
	Captures []struct {
		Scenario string `json:"scenario"`
		Sequence int    `json:"sequence"`
		Request  struct {
			Method  string            `json:"method"`
			Path    string            `json:"path"`
			Headers map[string]string `json:"headers"`
			Body    json.RawMessage   `json:"body"`
		} `json:"request"`
	} `json:"captures"`
}

func loadContractRegistry(t *testing.T) *contractRegistry {
	t.Helper()

	entries, err := os.ReadDir("schema")
	require.NoError(t, err)

	type resource struct {
		id  string
		doc any
	}
	resources := make([]resource, 0)
	known := make(map[string]struct{})
	var documents []any

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("schema", entry.Name()))
		require.NoError(t, err)
		_, err = validateStrictJSON(raw)
		require.NoError(t, err)

		var metadata struct {
			ID string `json:"$id"`
		}
		require.NoError(t, json.Unmarshal(raw, &metadata))
		require.True(t, strings.HasPrefix(metadata.ID, schemaIDPrefix), "unexpected schema id %q", metadata.ID)
		_, duplicate := known[metadata.ID]
		require.False(t, duplicate, "duplicate schema id %q", metadata.ID)
		known[metadata.ID] = struct{}{}

		doc, err := validateschema.UnmarshalJSON(bytes.NewReader(raw))
		require.NoError(t, err)
		resources = append(resources, resource{id: metadata.ID, doc: doc})
		documents = append(documents, doc)
	}

	require.Len(t, resources, 5)
	for i, document := range documents {
		require.NoError(t, validateReferences(resources[i].id, document, known))
	}

	compiler := validateschema.NewCompiler()
	compiler.DefaultDraft(validateschema.Draft2020)
	compiler.AssertFormat()
	for _, resource := range resources {
		require.NoError(t, compiler.AddResource(resource.id, resource.doc))
	}

	compiled := make(map[string]*validateschema.Schema)
	for _, name := range []string{"request", "generate-result", "stream-part", "error"} {
		id := schemaIDPrefix + name + ".json"
		schema, err := compiler.Compile(id)
		require.NoError(t, err)
		compiled[name] = schema
	}
	return &contractRegistry{compiled: compiled}
}

func validateReferences(baseID string, value any, known map[string]struct{}) error {
	switch value := value.(type) {
	case map[string]any:
		if reference, ok := value["$ref"].(string); ok {
			base, err := url.Parse(baseID)
			if err != nil {
				return err
			}
			ref, err := url.Parse(reference)
			if err != nil {
				return fmt.Errorf("invalid reference %q: %w", reference, err)
			}
			resolved := base.ResolveReference(ref)
			resolved.Fragment = ""
			if resolved.String() != baseID {
				if _, ok := known[resolved.String()]; !ok {
					return fmt.Errorf("reference %q resolves outside the offline registry", reference)
				}
			}
		}
		for _, child := range value {
			if err := validateReferences(baseID, child, known); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateReferences(baseID, child, known); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *contractRegistry) validate(name string, raw json.RawMessage) error {
	schema, ok := r.compiled[name]
	if !ok {
		return fmt.Errorf("unknown contract schema %q", name)
	}
	if _, err := validateStrictJSON(raw); err != nil {
		return fmt.Errorf("syntax: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("semantic json: %w", err)
	}
	return schema.Validate(value)
}

func readCorpus(t *testing.T, name string) corpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var result corpus
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

func findCorpusCase(t *testing.T, fixtures corpus, name string) corpusCase {
	t.Helper()
	for _, fixture := range fixtures.Cases {
		if fixture.Name == name {
			return fixture
		}
	}
	t.Fatalf("missing corpus case %q", name)
	return corpusCase{}
}

func readNegativeCorpus(t *testing.T) corpus {
	t.Helper()
	positive := readCorpus(t, "positive.json")
	bases := make(map[string]corpusCase, len(positive.Cases))
	for _, fixture := range positive.Cases {
		_, duplicate := bases[fixture.Name]
		require.False(t, duplicate, "duplicate positive fixture %q", fixture.Name)
		bases[fixture.Name] = fixture
	}

	raw, err := os.ReadFile(filepath.Join("testdata", "negative.json"))
	require.NoError(t, err)
	var recipes corpusRecipeFile
	require.NoError(t, json.Unmarshal(raw, &recipes))

	result := corpus{Cases: make([]corpusCase, 0, len(recipes.Cases))}
	for _, recipe := range recipes.Cases {
		base, ok := bases[recipe.Base]
		require.True(t, ok, "unknown positive fixture %q", recipe.Base)
		base.Name = recipe.Name
		base.Path = recipe.Path
		base.Document = applyFixtureMutations(t, base.Document, recipe.Mutations)
		result.Cases = append(result.Cases, base)
	}
	return result
}

func TestContractSchemas_CompileOffline(t *testing.T) {
	registry := loadContractRegistry(t)
	assert.ElementsMatch(t, []string{"request", "generate-result", "stream-part", "error"}, mapKeys(registry.compiled))
}

func TestContractSchemas_RejectReferencesOutsideRegistry(t *testing.T) {
	known := map[string]struct{}{schemaIDPrefix + "request.json": {}}
	err := validateReferences(schemaIDPrefix+"request.json", map[string]any{"$ref": "https://example.test/unknown.json"}, known)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "outside the offline registry")
}

func TestCapturedRequests_ValidateAgainstRequestSchema(t *testing.T) {
	registry := loadContractRegistry(t)
	raw, err := os.ReadFile("../../../test/interop/providerwire-v4/captures/requests.json")
	require.NoError(t, err)
	_, err = validateStrictJSON(raw)
	require.NoError(t, err)
	var artifact captureArtifact
	require.NoError(t, json.Unmarshal(raw, &artifact))
	require.NotEmpty(t, artifact.Captures)
	for _, capture := range artifact.Captures {
		name := fmt.Sprintf("%s/%d", capture.Scenario, capture.Sequence)
		t.Run(name, func(t *testing.T) {
			require.NoError(t, registry.validate("request", capture.Request.Body))
			mediaType, category := validateEnvelope(envelopeCase{
				Method:  capture.Request.Method,
				Path:    capture.Request.Path,
				Headers: capture.Request.Headers,
			})
			require.Empty(t, category)
			if capture.Request.Headers["ai-language-model-streaming"] == "true" {
				assert.Equal(t, "text/event-stream", mediaType)
			} else {
				assert.Equal(t, "application/json", mediaType)
			}
		})
	}
}

func TestContractCorpus_Positive(t *testing.T) {
	registry := loadContractRegistry(t)
	fixtures := readCorpus(t, "positive.json")
	require.NotEmpty(t, fixtures.Cases)

	for _, fixture := range fixtures.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			require.NoError(t, registry.validate(fixture.Schema, fixture.Document))
			if fixture.Schema == "error" {
				require.Equal(t, fixture.Status, nestedErrorStatus(t, fixture.Document))
			}
		})
	}
}

func TestContractCorpus_Negative(t *testing.T) {
	registry := loadContractRegistry(t)
	fixtures := readNegativeCorpus(t)
	require.NotEmpty(t, fixtures.Cases)

	for _, fixture := range fixtures.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			err := registry.validate(fixture.Schema, fixture.Document)
			require.Error(t, err)
			if fixture.Path != "" {
				var validationError *validateschema.ValidationError
				require.True(t, errors.As(err, &validationError))
				assert.True(t, validationErrorContainsPath(validationError, fixture.Path), "validation tree did not contain %s: %v", fixture.Path, err)
			}
		})
	}
}

func TestContractCorpus_ErrorStatusCorrelation(t *testing.T) {
	registry := loadContractRegistry(t)
	fixture := findCorpusCase(t, readCorpus(t, "positive.json"), "error rate limit")
	mismatched := applyFixtureMutations(t, fixture.Document, []fixtureMutation{{
		Operation: "set",
		Path:      "/error/statusCode",
		Value:     json.RawMessage("500"),
	}})

	require.NoError(t, registry.validate(fixture.Schema, mismatched))
	assert.NotEqual(t, fixture.Status, nestedErrorStatus(t, mismatched))
}

func TestContractCorpus_EveryStreamArmHasNegativeCoverage(t *testing.T) {
	registry := loadContractRegistry(t)
	fixtures := readCorpus(t, "positive.json")
	seen := make(map[string]struct{})

	for _, fixture := range fixtures.Cases {
		if fixture.Schema != "stream-part" {
			continue
		}
		var value map[string]any
		require.NoError(t, json.Unmarshal(fixture.Document, &value))
		key := streamCaseKey(value)
		field, ok := streamArmNegativeFields[key]
		require.True(t, ok, "missing negative coverage definition for stream arm %q", key)
		seen[key] = struct{}{}
		if key == "response-metadata" {
			value[field] = []any{}
		} else {
			delete(value, field)
		}
		invalid, err := json.Marshal(value)
		require.NoError(t, err)
		t.Run(fixture.Name, func(t *testing.T) {
			assert.Error(t, registry.validate("stream-part", invalid))
		})
	}

	assert.ElementsMatch(t, mapKeys(streamArmNegativeFields), mapKeys(seen))
}

func nestedErrorStatus(t *testing.T, raw json.RawMessage) int {
	t.Helper()
	var payload struct {
		Error struct {
			StatusCode int `json:"statusCode"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload.Error.StatusCode
}

func validationErrorContainsPath(err *validateschema.ValidationError, want string) bool {
	if jsonPointer(err.InstanceLocation) == want {
		return true
	}
	for _, cause := range err.Causes {
		if validationErrorContainsPath(cause, want) {
			return true
		}
	}
	return false
}

func jsonPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(strings.ReplaceAll(segment, "~", "~0"), "/", "~1")
	}
	return "/" + strings.Join(segments, "/")
}

func streamCaseKey(value map[string]any) string {
	kind, _ := value["type"].(string)
	if kind == "source" {
		sourceType, _ := value["sourceType"].(string)
		return kind + ":" + sourceType
	}
	return kind
}

func mapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
