package byok

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRequest(t *testing.T) {
	const valid = `{"byok":{"openai":[{"apiKey":"first"},{"apiKey":"second"}],"anthropic":[{"apiKey":"unused"}]}}`
	for _, tc := range []struct {
		name, selector, controls string
		wantProvider             providerName
		wantModel                string
		wantKeys                 []string
	}{
		{"ordered", "openai/native/model", valid, openai, "native/model", []string{"first", "second"}},
		{"other selected", "anthropic/not-in-catalog", valid, anthropic, "not-in-catalog", []string{"unused"}},
		{"last member wins", "openai/model", `{"byok":{"openai":[{"apiKey":"discarded","apiKey":"last"}]}}`, openai, "model", []string{"last"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRequest(tc.selector, json.RawMessage(tc.controls))
			require.NoError(t, err)
			assert.Equal(t, tc.wantProvider, got.provider)
			assert.Equal(t, tc.wantModel, got.model)
			assert.Equal(t, tc.wantKeys, got.keys)
		})
	}
	for _, selector := range []string{"", "alias", "/model", "openai/", "OpenAI/model", "bedrock/model", "openai/a b", "openai/a\t", "openai/a\u00a0", string([]byte{'o', '/', 255})} {
		t.Run("invalid selector/"+selector, func(t *testing.T) {
			got, err := parseRequest(selector, json.RawMessage(valid))
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
		})
	}
	for _, raw := range []string{
		`null`, `{}`, `[]`, `{"byok":null}`, `{"byok":{}}`, `{"byok":[]}`,
		`{"byok":{"openai":null}}`, `{"byok":{"openai":[]}}`, `{"byok":{"openai":[null]}}`,
		`{"byok":{"openai":[{}]}}`, `{"byok":{"openai":[{"apiKey":null}]}}`,
		`{"byok":{"openai":[{"apiKey":12}]}}`, `{"byok":{"openai":[{"apiKey":""}]}}`,
		`{"byok":{"openai":[{"apiKey":"dummy-secret marker"}]}}`,
		`{"byok":{"openai":[{"apiKey":"dummy-secret\nmarker"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid","dummy-secret-field":true}]}}`,
		`{"byok":{"openai":[{"APIKey":"dummy-secret"}]}}`,
		`{"byok":{"dummy-secret-provider":[{"apiKey":"valid"}]}}`,
		`{"byok":{"anthropic":[{"apiKey":"valid"}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}],"anthropic":[{}]}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"order":["openai"]}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"modelMappings":{}}`,
		`{"byok":{"openai":[{"apiKey":"valid"}]},"dummy-secret-control":true}`,
	} {
		t.Run("invalid controls/"+raw, func(t *testing.T) {
			got, err := parseRequest("openai/model", json.RawMessage(raw))
			require.ErrorIs(t, err, ErrInvalidRequest)
			assert.Empty(t, got)
			assert.NotContains(t, err.Error(), "dummy-secret")
		})
	}
}

func TestParseRequest_Boundaries(t *testing.T) {
	credential := `{"apiKey":"` + strings.Repeat("k", maxCredentialBytes) + `"}`
	for _, delta := range []int{0, 1} {
		t.Run("selector/"+strings.Repeat("+", delta), func(t *testing.T) {
			selector := "openai/" + strings.Repeat("m", MaxSelectorBytes-len("openai/")+delta)
			_, err := parseRequest(selector, json.RawMessage(`{"byok":{"openai":[`+credential+`]}}`))
			assert.Equal(t, delta != 0, err != nil)
		})
		t.Run("key/"+strings.Repeat("+", delta), func(t *testing.T) {
			raw := `{"byok":{"openai":[{"apiKey":"` + strings.Repeat("k", maxCredentialBytes+delta) + `"}]}}`
			_, err := parseRequest("openai/model", json.RawMessage(raw))
			assert.Equal(t, delta != 0, err != nil)
		})
		t.Run("count/"+strings.Repeat("+", delta), func(t *testing.T) {
			entries := strings.TrimSuffix(strings.Repeat(credential+",", maxCredentials+delta), ",")
			_, err := parseRequest("openai/model", json.RawMessage(`{"byok":{"openai":[`+entries+`]}}`))
			assert.Equal(t, delta != 0, err != nil)
		})
		t.Run("raw bytes/"+strings.Repeat("+", delta), func(t *testing.T) {
			mapBody := `{"openai":[{"apiKey":"valid"}]}`
			mapBody = mapBody[:len(mapBody)-1] + strings.Repeat(" ", maxBYOKBytes-len(mapBody)+delta) + "}"
			_, err := parseRequest("openai/model", json.RawMessage(`{"byok":`+mapBody+`}`))
			assert.Equal(t, delta != 0, err != nil)
		})
	}
}
