package providerwirev4

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	qvaluePattern     = regexp.MustCompile(`^(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)$`)
	rawQMarkerPattern = regexp.MustCompile(`(?i)(?:^|;)[\t ]*q[\t ]*=`)
	rawQvaluePattern  = regexp.MustCompile(`(?i)(?:^|;)[\t ]*q=(?:0(?:\.[0-9]{0,3})?|1(?:\.0{0,3})?)[\t ]*(?:;|$)`)
)

type envelopeCorpus struct {
	Seeds []envelopeSeed   `json:"seeds"`
	Cases []envelopeRecipe `json:"cases"`
}

type envelopeSeed struct {
	Name     string          `json:"name"`
	Document json.RawMessage `json:"document"`
}

type envelopeRecipe struct {
	Name          string            `json:"name"`
	Base          string            `json:"base"`
	Mutations     []fixtureMutation `json:"mutations"`
	WantMediaType string            `json:"wantMediaType"`
	WantCategory  string            `json:"wantCategory"`
}

type envelopeCase struct {
	Name          string            `json:"name"`
	Method        string            `json:"method"`
	Path          string            `json:"path"`
	Headers       map[string]string `json:"headers"`
	WantMediaType string            `json:"wantMediaType"`
	WantCategory  string            `json:"wantCategory"`
}

func validateEnvelope(fixture envelopeCase) (string, string) {
	if fixture.Method != http.MethodPost {
		return "", "method"
	}
	if fixture.Path != "/language-model" {
		return "", "path"
	}

	headers := make(http.Header)
	for name, value := range fixture.Headers {
		headers.Set(name, value)
	}
	modelID := headers.Get("ai-language-model-id")
	if modelID == "" || strings.TrimSpace(modelID) != modelID {
		return "", "model-id"
	}
	if headers.Get("ai-language-model-specification-version") != "4" {
		return "", "specification-version"
	}

	var responseMediaType string
	switch headers.Get("ai-language-model-streaming") {
	case "false":
		responseMediaType = "application/json"
	case "true":
		responseMediaType = "text/event-stream"
	default:
		return "", "streaming"
	}

	contentType, _, err := mime.ParseMediaType(headers.Get("content-type"))
	if err != nil || contentType != "application/json" {
		return "", "content-type"
	}
	if acceptValues, present := headers[http.CanonicalHeaderKey("accept")]; present {
		accept := strings.Join(acceptValues, ",")
		compatible, valid := acceptsRepresentation(accept, responseMediaType)
		if !valid || !compatible {
			return "", "accept"
		}
	}
	return responseMediaType, ""
}

func acceptsRepresentation(header, target string) (compatible, valid bool) {
	targetType, _, ok := strings.Cut(target, "/")
	if !ok {
		return false, false
	}
	bestSpecificity := -1
	bestQuality := 0.0
	for _, entry := range strings.Split(header, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return false, false
		}
		if rawQMarkerPattern.MatchString(entry) && !rawQvaluePattern.MatchString(entry) {
			return false, false
		}
		mediaType, params, err := mime.ParseMediaType(entry)
		if err != nil {
			return false, false
		}
		quality := 1.0
		if rawQuality, present := params["q"]; present {
			if !qvaluePattern.MatchString(rawQuality) {
				return false, false
			}
			quality, err = strconv.ParseFloat(rawQuality, 64)
			if err != nil {
				return false, false
			}
		}

		specificity := -1
		switch mediaType {
		case target:
			specificity = 2
		case targetType + "/*":
			specificity = 1
		}
		if specificity > bestSpecificity || specificity == bestSpecificity && quality > bestQuality {
			bestSpecificity = specificity
			bestQuality = quality
		}
	}
	return bestSpecificity >= 1 && bestQuality > 0, true
}

func TestHTTPEnvelope_Corpus(t *testing.T) {
	raw, err := os.ReadFile("testdata/envelopes.json")
	require.NoError(t, err)
	var corpus envelopeCorpus
	require.NoError(t, json.Unmarshal(raw, &corpus))

	seeds := make(map[string]json.RawMessage, len(corpus.Seeds))
	for _, seed := range corpus.Seeds {
		_, duplicate := seeds[seed.Name]
		require.False(t, duplicate, "duplicate envelope seed %q", seed.Name)
		seeds[seed.Name] = seed.Document
	}
	for _, recipe := range corpus.Cases {
		t.Run(recipe.Name, func(t *testing.T) {
			seed, ok := seeds[recipe.Base]
			require.True(t, ok, "unknown envelope seed %q", recipe.Base)
			raw := applyFixtureMutations(t, seed, recipe.Mutations)
			var fixture envelopeCase
			require.NoError(t, json.Unmarshal(raw, &fixture))
			fixture.Name = recipe.Name
			fixture.WantCategory = recipe.WantCategory
			fixture.WantMediaType = recipe.WantMediaType
			mediaType, category := validateEnvelope(fixture)
			assert.Equal(t, fixture.WantCategory, category)
			assert.Equal(t, fixture.WantMediaType, mediaType)
		})
	}
}
