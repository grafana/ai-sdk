package providerwirev4

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}
