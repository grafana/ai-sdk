# Proposal

## Why

Org admins can store validated `Middleware` resources, but the Gateway ignores them: a tenant extension runs everywhere it applies when it is enabled by default, with an empty configuration. Each namespace's stored configuration has to reach its own calls without trusting the caller's permissions, without a failed read turning into a more permissive default, and without giving up the host's lifecycle guarantees for extension instances.

## What Changes

- **Namespace configuration.** Each call's tenant extensions follow its namespace's `Middleware` resources: no resource means the extension's default, `enabled: false` means off, and `enabled: true` runs it with the stored configuration. Configuration is decoded as the JSON the platform stores, so numbers stay exact, and it is checked against the extension's schema again at read time. Invalid configuration or a failing factory applies the extension's failure policy. Enabled resources of types the build doesn't register are skipped, logged once per namespace and type, and counted. Disabled ones are ignored.
- **Reads as the plugin.** The app provider callback gives the host's API access configuration to a namespace reader. The reader lists a namespace's resources once per load, on a context with no caller identity or cancellation and its own timeout.
- **Per-namespace cache.** Each entry is fresh, stale, absent or failed. Concurrent first loads share one read, and the cache refreshes stale entries in the background. A failed refresh backs off. The cache is bounded by LRU, and its constants can be overridden in tests. A failed read is never treated as absence. The last-known-good configuration is served for up to 10 minutes, then the failure policy applies. With nothing loaded, the failure policy applies immediately.
- **Pipeline.** Per call, the pipeline resolves the namespace's configuration revision. It uses the ordered extension lists cached for that revision and the call's source. Operator extensions are unchanged. No read happens when no tenant extension applies to the call's source, so the shipped build, which registers no tenant extension, makes no API calls.
- **Instance lifecycle.** Each configured namespace gets its own instances, built once per configuration revision. Instances of a definition share one metrics registerer, which accepts the same collector registered again. An instance is closed after replacement, eviction or expiry, once no call or observation still holds it, and at shutdown.
- **Self-hosted without API access.** Tenant extensions run with their defaults, and the Gateway logs this once. Admission refuses `Middleware` creates and updates with `400`, because stored configuration wouldn't be applied. app-sdk `v0.60.16` can't return admission warnings (see design.md, Decision: admission without API access). Deletes stay unreviewed.
- **Telemetry and docs.** Counters for loads and unknown types. Configuration failures are counted as extension events. Read-failure logs carry no response body, error text or configuration values. The extension guide, architecture walkthrough and glossary cover the namespace configuration path, propagation within the TTL, replica convergence, roles, self-hosted behavior, and what tenant extensions see: the caller UID, plus the provider instances and models of operator-catalog candidates and attempts.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-extensions`: namespace tenant configuration, its read-time checks, unknown types, read failures, the cache, reads as the plugin, per-namespace instances and shared metrics, behavior without API access, and updated admission, documentation and evidence. It replaces "Tenant extensions without namespace configuration" and "Stored tenant configuration has no runtime effect".
- `gateway-plugin-host`: the app provider callback hands API access to the namespace reader, and the Middleware validator knows whether the plugin has API access.
- `gateway-plugin-telemetry`: tenant configuration load, unknown-type and failure telemetry.

## Impact

- **Code:** `ai-gateway/pkg/internal/tenantconfig` (namespace reader, admission without API access), `ai-gateway/pkg/internal/extensionhost` (tenant cache, per-namespace snapshots and lists, lifecycle, shared registerer), `ai-gateway/pkg/internal/gateway` (callback wiring), `ai-gateway/pkg/extension` (godoc), and `ai-gateway/test/platformhost` (namespace list for kinds).
- **Dependencies:** `k8s.io/client-go` becomes a direct requirement of the Gateway module. It is already in the build list at `v0.37.1`.
- **APIs:** no change to ProviderWire, the namespace routes, `gateway.Options` or the extension contract's types. The `Dependencies.Registerer` and factory godoc state the multi-instance contract.
- **Platform:** the plugin identity needs `list` on `middlewares` in every stack namespace. The development deployment check is a manual follow-up (see design.md).
- **Not parity-sensitive:** the Vercel AI SDK has no tenant configuration, and nothing about streams, chunks, providers or the wire format changes.
