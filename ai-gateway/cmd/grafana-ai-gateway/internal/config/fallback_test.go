package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFile_OrderedDirectFallback(t *testing.T) {
	valid := strings.Replace(minimalConfigYAML, "models:", "  backup:\n    type: anthropic\n    apiKeyEnv: BACKUP_KEY\nmodels:", 1)
	valid += "    fallback:\n      - provider: backup\n        model: second\n      - provider: anthropic-primary\n        model: third\n"
	file, err := LoadFile(writeConfigFile(t, valid), 1<<20)
	require.NoError(t, err)
	require.NoError(t, file.Validate())
	for _, tc := range []struct{ name, body string }{
		{"missing provider", "      - model: second\n"},
		{"unknown provider", "      - provider: missing\n        model: second\n"},
		{"empty model", "      - provider: backup\n        model: ''\n"},
		{"duplicate primary", "      - provider: anthropic-primary\n        model: claude-test\n"},
		{"duplicate secondary", "      - provider: backup\n        model: second\n      - provider: backup\n        model: second\n"},
		{"route reference", "      - route: grafana/assistant\n"},
		{"nested fallback", "      - provider: backup\n        model: second\n        fallback: []\n"},
		{"duplicate field", "      - provider: backup\n        provider: backup\n        model: second\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := valid[:strings.Index(valid, "    fallback:")]
			_, err := LoadFile(writeConfigFile(t, prefix+"    fallback:\n"+tc.body), 1<<20)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "BACKUP_KEY")
		})
	}
}
