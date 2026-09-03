// Package catalog provides a finite public model namespace for hosted gateways.
//
// A catalog is distinct from a provider registry. Registry providers construct
// models from provider-oriented IDs, while a catalog exposes canonical public
// IDs, aliases, and discovery metadata. Registry-backed catalogs bridge the two
// namespaces through explicit routes without changing registry behavior.
//
// Resolution and listing both accept a context so application-owned decorators
// can apply one request-scoped visibility policy to both operations. Catalog
// implementations do not define authentication, entitlements, profiles,
// provider construction, fallback policy, or HTTP behavior.
//
// Aliases resolve to the canonical ID stored in ResolvedModel. Listing emits one
// row per canonical model in stable ID order and returns defensive metadata
// copies. Capabilities describe behavior guaranteed by the public route; routes
// with multiple possible backends should advertise only their shared behavior.
package catalog
