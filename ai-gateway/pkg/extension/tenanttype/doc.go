// Package tenanttype is the public registry of tenant extension types: the
// contracts that a stack's Middleware resources configure.
//
// A tenant extension has two halves. Its [Type] is public: the name, the
// configuration schema, the default state and an example. The Gateway's
// manifest declares one typed member per type, so every build, public or
// private, ships the same manifest. Its implementation may be private: a
// distribution binds an [Implementation] to a public type with [Type.Bind]
// and passes the resulting definition to the Gateway. Startup fails when a
// tenant-scope definition has no public type, or differs from its type in
// schema or default.
//
// Private extensions should be operator-scope. Use a tenant type only when
// tenants need to configure the extension, and add the type here first.
//
// A type's configuration is stored in the spec member named [Type.Member].
// The member is hand-written in the Middleware kind's CUE and mirrors the
// type's schema structurally: JSON types, properties, items, closed objects,
// required fields and descriptions, without value constraints, which
// admission checks instead. A schema uses closed objects
// and arrays only: every object declares properties with additionalProperties
// false, and every node has exactly one JSON type. Typed maps, $ref and
// combinators are not supported. A bound an integer field declares must be
// within ±(2^53−1), the safe integers of a JavaScript number. As upstream,
// integers beyond that range are not compared exactly.
//
// The tests of this package check the registry: names, unique and
// unreserved members, schema shapes, and examples the schemas accept. A
// manifest test checks that each member mirrors its type.
//
// The platform validates every write against the current schema, including
// fields the write leaves unchanged. A type's schema therefore changes only
// additively, with new optional fields. An incompatible change, such as a new
// required field or a narrower type, needs a new type name.
//
// This package is v0 and experimental: its API may change in any minor
// release.
package tenanttype
