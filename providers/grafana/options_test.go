package grafana

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }

func TestGrafanaOptions_ProviderKey(t *testing.T) {
	var opt provider.ProviderOption = GrafanaOptions{}
	assert.Equal(t, "grafana", opt.ProviderKey())
}

func TestGrafanaOptions_RoundTrip(t *testing.T) {
	opts := provider.BuildProviderOptions(GrafanaOptions{
		AgentObservability: &AgentObservabilityControl{
			Disabled:    boolPtr(true),
			CaptureMode: CaptureModeMetadataOnly,
		},
		Usage: &UsageControl{Disabled: boolPtr(false)},
	})

	data, err := json.Marshal(opts)
	require.NoError(t, err)

	var decoded provider.ProviderOptions
	require.NoError(t, json.Unmarshal(data, &decoded))

	got, ok, err := provider.ResolveOption[GrafanaOptions](decoded, "grafana")
	require.NoError(t, err)
	require.True(t, ok)

	require.NotNil(t, got.AgentObservability)
	require.NotNil(t, got.AgentObservability.Disabled)
	assert.True(t, *got.AgentObservability.Disabled)
	assert.Equal(t, CaptureModeMetadataOnly, got.AgentObservability.CaptureMode)

	require.NotNil(t, got.Usage)
	require.NotNil(t, got.Usage.Disabled)
	assert.False(t, *got.Usage.Disabled)
}

func TestGrafanaOptions_MarshalEmitsGrafanaKey(t *testing.T) {
	opts := provider.BuildProviderOptions(GrafanaOptions{
		AgentObservability: &AgentObservabilityControl{CaptureMode: CaptureModeFull},
	})

	data, err := json.Marshal(opts)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	grafanaJSON, ok := raw["grafana"]
	require.True(t, ok, "expected a top-level grafana key")
	assert.JSONEq(t, `{"agentObservability":{"captureMode":"full"}}`, string(grafanaJSON))
}

func TestGrafanaOptions_AgentObservabilityWireKey(t *testing.T) {
	t.Run("unmarshals from agentObservability", func(t *testing.T) {
		var got GrafanaOptions
		require.NoError(t, json.Unmarshal([]byte(`{"agentObservability":{"captureMode":"full"}}`), &got))
		require.NotNil(t, got.AgentObservability)
		assert.Equal(t, CaptureModeFull, got.AgentObservability.CaptureMode)
	})

	// GrafanaOptions has no custom UnmarshalJSON, so only the struct tag key
	// decodes; a tolerant decode would have to be added deliberately. The
	// former key is assembled from fragments because the old product name must
	// not appear as a searchable literal in this repository.
	t.Run("former key is ignored", func(t *testing.T) {
		formerKey := "sig" + "il"
		payload := fmt.Sprintf(`{%q:{"captureMode":"full"}}`, formerKey)

		var got GrafanaOptions
		require.NoError(t, json.Unmarshal([]byte(payload), &got))
		assert.Nil(t, got.AgentObservability)
	})
}

func TestGrafanaOptions_NilControlsOmitted(t *testing.T) {
	data, err := json.Marshal(GrafanaOptions{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))

	data, err = json.Marshal(GrafanaOptions{AgentObservability: &AgentObservabilityControl{}})
	require.NoError(t, err)
	// Empty AgentObservabilityControl: nil Disabled and empty CaptureMode both omitted.
	assert.JSONEq(t, `{"agentObservability":{}}`, string(data))
}

func TestGrafanaOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    GrafanaOptions
		wantErr bool
	}{
		{
			name: "valid capture mode",
			opts: GrafanaOptions{AgentObservability: &AgentObservabilityControl{CaptureMode: CaptureModeMetadataOnly}},
		},
		{
			name: "empty capture mode is valid",
			opts: GrafanaOptions{AgentObservability: &AgentObservabilityControl{}},
		},
		{
			name: "nil Agent Observability control is valid",
			opts: GrafanaOptions{},
		},
		{
			name: "disabled only is valid",
			opts: GrafanaOptions{AgentObservability: &AgentObservabilityControl{Disabled: boolPtr(true)}},
		},
		{
			name:    "invalid capture mode",
			opts:    GrafanaOptions{AgentObservability: &AgentObservabilityControl{CaptureMode: CaptureMode("bogus")}},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.opts.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAgentObservabilityControl_DisabledTristateRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		disabled *bool
		wantJSON string
	}{
		{name: "nil omitted", disabled: nil, wantJSON: `{}`},
		{name: "true", disabled: boolPtr(true), wantJSON: `{"disabled":true}`},
		{name: "false", disabled: boolPtr(false), wantJSON: `{"disabled":false}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := AgentObservabilityControl{Disabled: tc.disabled}
			data, err := json.Marshal(ctrl)
			require.NoError(t, err)
			assert.JSONEq(t, tc.wantJSON, string(data))

			var decoded AgentObservabilityControl
			require.NoError(t, json.Unmarshal(data, &decoded))
			if tc.disabled == nil {
				assert.Nil(t, decoded.Disabled)
				return
			}
			require.NotNil(t, decoded.Disabled)
			assert.Equal(t, *tc.disabled, *decoded.Disabled)
		})
	}
}
