// Package tenantschema projects public tenant types into the Middleware
// kind's manifest members and checks that the platform stores them as
// written.
//
// [Project] keeps a type schema's JSON types, properties, items, closed
// objects, required fields and descriptions, drops value constraints and
// defaults, and refuses what the platform would prune, what the manifest
// generator cannot emit, typed maps, references and integer fields without
// bounds within ±[MaxSafeInteger]. [Render] writes the members as
// the CUE the kind imports, after checking that each projection is
// structural and keeps the type's example through pruning. [ManifestKind]
// derives the structural schema a kind store builds from a manifest, so
// tests can check the generated manifest the same way.
//
// The package is used by the members generator and by tests, never by the
// plugin backend.
package tenantschema
