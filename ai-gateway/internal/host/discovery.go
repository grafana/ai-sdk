package host

import "github.com/grafana/ai-sdk/ai-gateway/catalog"

// DiscoveryProjection encodes the models the host listed for a caller as an
// adapter's discovery document. It writes nothing, so the host can still
// answer a failed projection with [ErrorInternal].
type DiscoveryProjection interface {
	ProjectDiscovery(models []catalog.ModelInfo) ([]byte, error)
}

// DiscoveryResourceProjection selects a public model from the caller-visible
// catalog. A missing resource is distinct from a projection failure.
type DiscoveryResourceProjection interface {
	ProjectDiscoveryResource(models []catalog.ModelInfo, id string) (document []byte, found bool, err error)
}
