# Gateway model catalog

Use `gateway/catalog` when an API lets clients discover or select models through
stable public names. A catalog can expose names such as `default`, `fast`, or
`balanced` while the service changes provider IDs, deployments, or fallback
order behind them.

The catalog resolves those names to models and returns listing metadata for a
model picker or API. The application still decides which authenticated users or
tenants may discover and invoke each model.

## Map public names to models

A catalog keeps three names separate:

```text
requested alias "default"
          ↓
canonical public ID "balanced"
          ↓
model-reported ID "claude-sonnet-5"
```

`ResolveModel` returns the canonical public ID and constructed model:

```go
resolved, err := models.ResolveModel(ctx, "default")
if err != nil {
	return err
}

fmt.Println(resolved.ID)              // balanced
fmt.Println(resolved.Model.ModelID()) // model-reported ID
```

The model-reported ID is separate from the canonical catalog ID. Middleware and
composite models can change what `ModelID()` reports. Use provider response
metadata to identify the provider and model that served a call.

## Combine catalog components

A gateway can combine several independent capabilities:

- The catalog owns public IDs, aliases, metadata, resolution, and listing.
- A registry constructs models from provider-oriented IDs such as
  `bedrock:model-id`.
- A fallback model tries ordered candidates after eligible failures.
- The application host owns authentication, authorization, and model visibility.

Applications that construct one model can pass it directly to the generation
APIs. Services that only resolve provider-oriented IDs can use a
[registry](fallback-and-registry.md).

## Create a static catalog

`NewStatic` stores models constructed during application startup:

```go
models, err := catalog.NewStatic([]catalog.StaticEntry{
	{
		Info: catalog.ModelInfo{
			ID:           "balanced",
			Name:         "Balanced",
			Description:  "General-purpose model",
			Aliases:      []string{"default"},
			Capabilities: []catalog.ModelCapability{"tools"},
		},
		Model: balancedModel,
	},
})
if err != nil {
	return err
}
```

Both `balanced` and `default` resolve to the same model and return `balanced` as
the canonical ID.

`ListModels` returns one row per canonical ID in sorted order:

```go
available, err := models.ListModels(ctx)
```

Aliases and capabilities appear as metadata on the canonical row. Catalog
construction and listing use defensive copies, keeping the namespace immutable.

## Construct models through a registry

`NewRegistry` maps each public route to an opaque ID accepted by an existing
`registry.Provider`:

```go
models, err := catalog.NewRegistry(
	providerRegistry,
	[]catalog.RegistryRoute{
		{
			Info: catalog.ModelInfo{
				ID:      "balanced",
				Aliases: []string{"default"},
			},
			ProviderModelID: "anthropic:claude-sonnet-5",
		},
	},
)
if err != nil {
	return err
}
```

Resolving `default` calls:

```go
providerRegistry.LanguageModel("anthropic:claude-sonnet-5")
```

The catalog returns `balanced` as the canonical public ID. Registry errors stay
in the returned error chain.

A public route can also hold a `fallback.Model`. Construct the fallback first,
then add it to a static catalog entry. See
[Fallback and registry](fallback-and-registry.md) for candidate selection, usage,
and provider attribution.

## Apply host policy and catalog guarantees

Authenticate requests before catalog access. Apply the same tenant,
entitlement, and feature rules to both operations:

- `ResolveModel` accepts IDs the caller can invoke.
- `ListModels` returns entries the caller can discover.

Both interfaces receive `context.Context`, allowing a host wrapper to read
request-scoped identity and policy. The built-in catalogs expose the configured
namespace; the host applies request-specific visibility.

`ModelInfo.Capabilities` contains labels defined by the public API. A route
backed by fallback models should advertise capabilities supported by every
candidate.

An unknown public ID returns an error matching `catalog.ErrUnknownModel` and
records the requested ID in `*catalog.UnknownModelError`. The error does not
list available models.

Catalog constructors validate:

- required canonical IDs;
- non-empty aliases and namespace collisions;
- non-nil static models;
- a non-nil registry provider and non-empty provider model IDs.

A registry-backed catalog resolves its configured provider model ID when
`ResolveModel` is called, so provider lookup errors occur at request time.

## Connect the catalog to a host transport

A host-owned transport adapter can pass its request context and public model ID
to `ResolveModel`, execute the returned `ResolvedModel.Model`, and retain
`ResolvedModel.ID` as the canonical public identity for policy and logging.

When resolution returns an error matching `catalog.ErrUnknownModel`, map it to
the transport's not-found response. Other catalog or registry failures pass
through to the host's normal error handling. The catalog does not own HTTP
status codes, response envelopes, or another transport's lifecycle.

## Reference

- [`gateway/catalog` package](https://pkg.go.dev/github.com/grafana/ai-sdk/gateway/catalog)
- [`registry` package](https://pkg.go.dev/github.com/grafana/ai-sdk/registry)

---

← [Writing a provider](../providers/writing-a-provider.md) · [Docs index](../README.md) · [Legacy ProviderWire retirement →](provider-wire-retirement.md)
