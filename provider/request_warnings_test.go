package provider

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRejectUnsupportedIsExplicitAndLocal(t *testing.T) {
	warnings := []Warning{{Type: WarnUnsupported, Feature: "seed", Details: "sensitive input"}}
	require.NoError(t, (CallOptions{}).CheckUnsupportedWarnings(warnings))
	options := CallOptions{RejectUnsupported: true}
	require.ErrorIs(t, options.CheckUnsupportedWarnings(warnings), ErrUnsupportedFunctionality)
	require.NoError(t, options.CheckUnsupportedWarnings([]Warning{{Type: WarnCompatibility, Feature: "normalization"}}))
	raw, err := json.Marshal(options)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "RejectUnsupported")
	require.NotContains(t, string(raw), "rejectUnsupported")
}
