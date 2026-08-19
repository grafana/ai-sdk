package providerwirev4

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type syntaxCorpus struct {
	Invalid []syntaxCase `json:"invalid"`
	Valid   []syntaxCase `json:"valid"`
}

type syntaxCase struct {
	Name     string `json:"name"`
	Encoding string `json:"encoding"`
	Value    string `json:"value"`
	Category string `json:"category"`
}

func TestStrictJSON_Corpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/syntax.json")
	require.NoError(t, err)
	var fixtures syntaxCorpus
	require.NoError(t, json.Unmarshal(raw, &fixtures))

	for _, fixture := range fixtures.Invalid {
		t.Run("reject "+fixture.Name, func(t *testing.T) {
			source := decodeSyntaxFixture(t, fixture)
			preserved := append([]byte(nil), source...)
			_, err := validateStrictJSON(source)
			require.Error(t, err)
			assert.Contains(t, err.Error(), fixture.Category)
			assert.Equal(t, preserved, source)
		})
	}
	for _, fixture := range fixtures.Valid {
		t.Run("accept "+fixture.Name, func(t *testing.T) {
			source := decodeSyntaxFixture(t, fixture)
			preserved := append([]byte(nil), source...)
			validated, err := validateStrictJSON(source)
			require.NoError(t, err)
			assert.Equal(t, preserved, validated)
			assert.Equal(t, preserved, source)
		})
	}
}

func TestStrictJSON_SyntaxPrecedesSchema(t *testing.T) {
	registry := loadContractRegistry(t)
	source := []byte(`{"prompt":[],"prompt":null,"unknown":true}`)

	_, err := validateStrictJSON(source)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid-json-syntax")
	assert.Error(t, registry.validate("request", source))
}

func decodeSyntaxFixture(t *testing.T, fixture syntaxCase) []byte {
	t.Helper()
	switch fixture.Encoding {
	case "utf8":
		return []byte(fixture.Value)
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(fixture.Value)
		require.NoError(t, err)
		return decoded
	default:
		t.Fatalf("unknown syntax fixture encoding %q", fixture.Encoding)
		return nil
	}
}
