# Legacy ProviderWire retirement

The legacy `gateway/providerwire` server and `providers/grafana` client have
been removed. The repository does not provide a compatibility shim, replacement
transport, or strict ProviderWire V4 implementation in this release.

## Roll back a server deployment

Root-module deployments that still require the legacy server can pin:

```text
github.com/grafana/ai-sdk@v0.1.0-alpha.1
```

That tag is rollback guidance only for the root module and server package. Test
the pinned deployment with its existing client before rolling it out.

## Inspect the former Grafana client

The Grafana client source remains available at repository tag
`v0.1.0-alpha.1` and in Git history. The root tag is not a nested-module tag, so
do not treat `github.com/grafana/ai-sdk/providers/grafana@v0.1.0-alpha.1` as a
published module version.

This retirement does not publish an independently versioned Grafana client or
provide migration guidance for a strict replacement. Existing consumers should
remain on their known working module revision until a separately specified
client and protocol are available.

## Current supported boundaries

The provider interfaces, concrete non-Grafana providers, gateway model catalog,
fallback, registry, middleware, and UI-message SSE helpers remain supported.
The catalog is transport-neutral and does not replace the retired remote-model
transport.

---

← [Gateway model catalog](gateway-model-catalog.md) · [Docs index](../README.md) · [Production checklist →](../best-practices/production.md)
