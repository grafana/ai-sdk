package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	providerv4 "github.com/grafana/ai-sdk/gateway/providerwire/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProviderWireV4Mount_UnaryAliasUsesCanonicalIdentity(t *testing.T) {
	mux := http.NewServeMux()
	require.NoError(t, mountProviderWireV4(mux))

	request := httptest.NewRequest(http.MethodPost, providerWireV4Prefix+providerv4.PathLanguageModel, strings.NewReader(providerWireV4UnaryRequestBody))
	request.Header.Set("Content-Type", providerv4.MIMEJSON)
	request.Header.Set(providerv4.HeaderModelID, providerWireV4UnaryAlias)
	request.Header.Set(providerv4.HeaderSpecVersion, providerv4.SpecVersionV4)
	request.Header.Set(providerv4.HeaderStreaming, "false")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, providerv4.MIMEJSON, response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), `"warnings":[]`)
	assert.Contains(t, response.Body.String(), `"modelId":"`+providerWireV4UnaryID+`"`)
	assert.NotContains(t, response.Body.String(), "private-integration")
}

const providerWireV4UnaryRequestBody = `{
	"prompt":[
		{"role":"system","content":""},
		{"role":"user","content":[{"type":"text","text":"hello"},{"type":"text","text":""}]},
		{"role":"assistant","content":[{"type":"text","text":"reply"}]}
	],
	"maxOutputTokens":0,
	"temperature":0,
	"topP":0.5,
	"topK":7,
	"presencePenalty":0,
	"frequencyPenalty":-0.5,
	"stopSequences":[],
	"seed":42,
	"reasoning":"high"
}`
