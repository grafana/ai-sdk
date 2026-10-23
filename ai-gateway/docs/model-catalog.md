# Gateway model catalog

Use `ai-gateway/catalog` when an API lets clients discover or select models
through stable public names. A catalog can expose names such as `default`, `fast`, or
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
[registry](../../docs/guides/fallback-and-registry.md).

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

Use each row's aliases and capabilities to build a model picker. If you want
users to inspect provider choices, supply candidate metadata when creating the
catalog. You can modify a returned listing without changing the catalog.

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
[Fallback and registry](../../docs/guides/fallback-and-registry.md) for candidate selection, usage,
and provider attribution.

## Apply host policy and catalog guarantees

Authenticate requests before catalog access. Apply the same tenant,
entitlement, and feature rules to both operations:

- `ResolveModel` accepts IDs the caller can invoke.
- `ListModels` returns entries the caller can discover.

Both interfaces receive `context.Context`, allowing a host wrapper to read
request-scoped identity and policy. The built-in catalogs expose the configured
namespace; the host applies request-specific visibility.

Use `ModelInfo.Capabilities` to tell callers what they can do with a public model.
If that model has fallbacks, advertise only features that every candidate supports,
such as function tools or file inputs. Otherwise, a request that works on the
primary model may fail when a backup is needed.

Choose provider-specific settings that are valid for the models in the route.
Fallback does not adapt those settings for a different provider.

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

## Connect the catalog to a transport

Your service passes the request context and the requested model ID to
`ResolveModel`, calls the returned model, and uses `ResolvedModel.ID` in its
policy checks and logs. Map errors matching `catalog.ErrUnknownModel` to your
not-found response, and handle other errors as usual. The catalog does not
produce HTTP responses itself.

## Let clients see provider choices

A catalog can describe which providers and models stand behind each public
model. For example, `balanced` might use an Anthropic model first and an OpenAI
model as a fallback. Supply this candidate metadata when you create the
catalog; models without it can still be listed and selected.

Grafana AI Gateway returns these choices when clients list models. Clients
still select `balanced` or one of its aliases, not a candidate's model. The
list describes what the Gateway can try, not which provider answered a past
request, and it does not check that providers are available. Every client that
can list models sees these choices, so check that they may. See
[discovering models from Go and TypeScript](../../docs/providers/grafana-gateway.md#discover-and-select-public-models)
for client examples.

## Reference

- [`ai-gateway/catalog` package](https://pkg.go.dev/github.com/grafana/ai-sdk/ai-gateway/catalog)
- [`registry` package](https://pkg.go.dev/github.com/grafana/ai-sdk/registry)

---

← [AI Gateway](../README.md) · [SDK documentation](../../docs/README.md)
