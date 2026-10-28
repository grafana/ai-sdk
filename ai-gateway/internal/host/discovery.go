package host

import "github.com/grafana/ai-sdk/ai-gateway/catalog"

// DiscoveryProjection encodes the models the host listed for a caller as an
// adapter's discovery document. It writes nothing, so the host can still
// answer a failed projection with [ErrorInternal].
type DiscoveryProjection interface {
	ProjectDiscovery(models []catalog.ModelInfo) ([]byte, error)
}
