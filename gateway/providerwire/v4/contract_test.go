package providerwirev4

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	validateschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func loadContractRegistry(t *testing.T) *schemaRegistry {
	t.Helper()
	registry, err := loadEmbeddedContractRegistry()
	require.NoError(t, err)
	return registry
}

func readPositiveCorpus(t *testing.T) corpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "positive.json"))
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
	positive := readPositiveCorpus(t)
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
	compiler := validateschema.NewCompiler()
	compiler.UseLoader(validateschema.SchemeURLLoader{})
	id := embeddedSchemaIDPrefix + "outside-registry.json"
	require.NoError(t, compiler.AddResource(id, map[string]any{"$ref": "https://example.test/unknown.json"}))
	_, err := compiler.Compile(id)
	require.Error(t, err)
	var loadError *validateschema.LoadURLError
	assert.ErrorAs(t, err, &loadError)
}

func TestCapturedRequests_ValidateAgainstRequestSchema(t *testing.T) {
	registry := loadContractRegistry(t)
	path := os.Getenv("PROVIDERWIRE_V4_CAPTURE_PATH")
	if path == "" {
		path = "../../../test/interop/providerwire-v4/captures/requests.json"
	}
	raw, err := os.ReadFile(path)
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
	fixtures := readPositiveCorpus(t)
	require.NotEmpty(t, fixtures.Cases)

	for _, fixture := range fixtures.Cases {
		t.Run(fixture.Name, func(t *testing.T) {
			if fixture.Schema == "error" {
				require.NoError(t, registry.validateErrorEnvelope(fixture.Document, fixture.Status))
				return
			}
			require.NoError(t, registry.validate(fixture.Schema, fixture.Document))
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
	fixture := findCorpusCase(t, readPositiveCorpus(t), "error rate limit")
	mismatched := applyFixtureMutations(t, fixture.Document, []fixtureMutation{{
		Operation: "set",
		Path:      "/error/statusCode",
		Value:     json.RawMessage("500"),
	}})

	err := registry.validateErrorEnvelope(mismatched, fixture.Status)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status correlation")
}

func TestContractCorpus_EveryStreamArmHasNegativeCoverage(t *testing.T) {
	registry := loadContractRegistry(t)
	fixtures := readPositiveCorpus(t)
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

func validationErrorContainsPath(err *validateschema.ValidationError, want string) bool {
	if safeJSONPointer(err.InstanceLocation) == want {
		return true
	}
	for _, cause := range err.Causes {
		if validationErrorContainsPath(cause, want) {
			return true
		}
	}
	return false
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

type unaryResultArtifact struct {
	Result json.RawMessage `json:"result"`
}

func validateUnaryResultArtifact(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading unary result artifact: %w", err)
	}
	if _, err := validateStrictJSON(raw); err != nil {
		return fmt.Errorf("validating unary result artifact syntax: %w", err)
	}
	var artifact unaryResultArtifact
	if err := json.Unmarshal(raw, &artifact); err != nil {
		return fmt.Errorf("decoding unary result artifact: %w", err)
	}
	if len(artifact.Result) == 0 {
		return errors.New("unary result artifact is missing result")
	}
	registry, err := loadEmbeddedContractRegistry()
	if err != nil {
		return err
	}
	if err := registry.validate("generate-result", artifact.Result); err != nil {
		return fmt.Errorf("validating unary result artifact: %w", err)
	}
	return nil
}

func TestProviderWireV4UnaryResultArtifact(t *testing.T) {
	path := os.Getenv("PROVIDERWIRE_V4_UNARY_RESULT_PATH")
	if path == "" {
		t.Skip("PROVIDERWIRE_V4_UNARY_RESULT_PATH is not set")
	}
	require.NoError(t, validateUnaryResultArtifact(path))
}

func TestValidateUnaryResultArtifact(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "valid", content: `{"result":{"content":[],"finishReason":{"unified":"other"},"usage":{"inputTokens":{},"outputTokens":{}},"warnings":[]}}`},
		{name: "missing result", content: `{}`, wantErr: true},
		{name: "invalid result", content: `{"result":{"content":[]}}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "artifact.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.content), 0o600))
			err := validateUnaryResultArtifact(path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
