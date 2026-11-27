## Decision

Create an independent admission instance for discovery and reuse the bounded listing transport. Cache filtered inventory IDs per authorized namespace, catalog, Grafana resource version, and provider instance. Evict expired entries before evicting the oldest when full. Without a verified revision, always list live. Writer actions neither use the cache nor contend with discovery for admission.

The pinned Vercel Gateway client reads the same model rows from `/config`. This cache is server-side Grafana policy, not a wire or conformance change.
