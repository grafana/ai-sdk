## Why

An operator has to edit `model_catalog` and redeploy for each new backend model an application wants to call. Stack catalogs already let an operator of a catalog opt a provider into serving unlisted models while excluding exact backend IDs.

## What Changes

- Add optional `serveAllModels` and `modelBlocklist` fields to each operator provider. Omitted settings preserve listed-only serving.
- On both selection paths, resolve listed IDs and aliases before trying `<instance>/<model>` against that instance's serving policy and startup-resolved account. An unlisted selector supplies one candidate, with no inherited fallback or native option grants.
- Allow an explicit empty `models: {}` with at least one opted-in provider. Refuse blocklist conflicts with listed primaries, Gateway fallback targets, or explicit Anthropic server-side fallback targets.
- Refuse Anthropic `allowDefault` with a nonempty blocklist. Reuse bounded, cached stack discovery on operator `/config`; both paths consult the cache before admitting new provider work.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `gateway-provider-configuration`: operator YAML and startup checks.
- `gateway-request-routing`: qualified selectors on the prebuilt and routed selection paths.
- `gateway-configured-discovery`: best-effort qualified rows on operator `/config` and cache-first admission on both routes.
- `gateway-text-observability`: keep model metric cardinality bounded for unlisted operator IDs.
- `gateway-model-catalog-kind`: remove statements that the operator schema stays unchanged and always requires a listed model. Stack admission and serving do not change.

## Impact

Gateway startup configuration, configured selection, the operator catalog reference, and discovery. Hosted-tool model options, BYOK, stack selection and admission policy, and client wire formats are unchanged.
