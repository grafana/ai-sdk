package discovery

import "github.com/grafana/ai-sdk/ai-gateway/catalog"

const (
	// MaxModelRows bounds canonical and alias rows in one document.
	MaxModelRows = 1024
	// MaxCandidates bounds configured primary and fallback destinations per route.
	MaxCandidates = 16
	// MaxAliases bounds aliases configured for one route.
	MaxAliases = 128
	// MaxStringBytes bounds each recognized UTF-8 identity or display value.
	MaxStringBytes = 2048
)

// Validate checks complete configured discovery feasibility before readiness.
func Validate(infos []catalog.ModelInfo, limit int64) error {
	if limit <= 0 || limit == int64(^uint64(0)>>1) {
		return errResponseLimit
	}
	_, err := (&handler{limit: limit}).encode(infos)
	return err
}
