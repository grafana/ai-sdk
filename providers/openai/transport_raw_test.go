package openai

import (
	"testing"

	"github.com/grafana/ai-sdk/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoStream_RawEvents(t *testing.T) {
	const valid = `{"type":"response.output_text.delta","item_id":"msg_1","delta":"hello"}`
	const unknown = `{"type":"future.event","payload":1}`
	const apiError = `{"error":{"message":"failed","type":"server_error"}}`
	for _, tc := range []struct {
		name    string
		options provider.CallOptions
		wantRaw bool
	}{
		{name: "omitted"},
		{name: "false", options: provider.CallOptions{IncludeRawChunks: false}},
		{name: "true", options: provider.CallOptions{IncludeRawChunks: true}, wantRaw: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parts := collectHTTPStream(t, []string{valid, unknown, `not JSON`, apiError}, tc.options)
			var rawValues []string
			var sequence []provider.StreamPartType
			for _, part := range parts {
				if part.Type == provider.PartRaw || part.Type == provider.PartError || part.Type == provider.PartTextDelta {
					sequence = append(sequence, part.Type)
				}
				if part.Type == provider.PartRaw {
					rawValues = append(rawValues, string(part.RawValue))
				}
			}
			if tc.wantRaw {
				require.Equal(t, []string{valid, unknown, "", apiError}, rawValues)
				assert.Equal(t, []provider.StreamPartType{
					provider.PartRaw, provider.PartTextDelta,
					provider.PartRaw,
					provider.PartRaw, provider.PartError,
					provider.PartRaw, provider.PartError,
				}, sequence)
			} else {
				assert.Empty(t, rawValues)
				assert.Equal(t, []provider.StreamPartType{provider.PartTextDelta, provider.PartError, provider.PartError}, sequence)
			}
		})
	}
}
