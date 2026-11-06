package models

import (
	"encoding/json"
	"testing"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectionEmptyAndPublicFields(t *testing.T) {
	data, err := (Projection{}).ProjectDiscovery(nil)
	require.NoError(t, err)
	assert.JSONEq(t, `{"object":"list","data":[]}`, string(data))
	info := []catalog.ModelInfo{{ID: "public", Name: "private-display", Description: "private-description", Aliases: []string{"alias", "alias"}, Candidates: []catalog.ConfiguredCandidate{{ProviderInstance: "secret-account", Provider: "private-provider", ModelID: "private-native"}}}}
	data, err = (Projection{}).ProjectDiscovery(info)
	require.NoError(t, err)
	assert.JSONEq(t, `{"object":"list","data":[{"id":"alias","object":"model","created":0,"owned_by":"grafana"},{"id":"public","object":"model","created":0,"owned_by":"grafana"}]}`, string(data))
	var fields map[string]json.RawMessage
	data, found, err := (Projection{}).ProjectDiscoveryResource(info, "alias")
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, json.Unmarshal(data, &fields))
	assert.Len(t, fields, 4)
	_, found, err = (Projection{}).ProjectDiscoveryResource(info, "private-native")
	require.NoError(t, err)
	assert.False(t, found)
}
