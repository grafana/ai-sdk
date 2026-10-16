package provider

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratedOriginalURL(t *testing.T) {
	for _, raw := range []string{`{"type":"url","url":"opaque:value"}`, `{"type":"url","url":"opaque:value","originalUrl":""}`, `{"type":"url","url":"opaque:value","originalUrl":"opaque:VALUE?token=application"}`} {
		for _, target := range []any{&DataContent{}, &StreamFileData{}} {
			require.NoError(t, json.Unmarshal([]byte(raw), target))
			encoded, err := json.Marshal(target)
			require.NoError(t, err)
			require.JSONEq(t, raw, string(encoded))
		}
	}
}
