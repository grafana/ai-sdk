// Package tenantconfig decodes and validates tenant extension configuration:
// the Middleware resources a stack stores to configure the distribution's
// tenant extensions.
//
// A Middleware spec holds the extension's type, whether it is enabled, and at
// most one member: the type's configuration, named after the type (see
// [tenanttype.Type.Member]). [DecodeSpec] decodes a spec from raw JSON
// without decoding members into Go types, so numbers stay exact; admission
// and the read path share it.
//
// [Validator] is the Middleware admission validator, used with [Kind]. A
// resource is named after its type, sets no member but its type's, and its
// member must match the type's schema. A disable-only update is accepted
// whatever the stored type, with the member kept or removed, so a build that
// no longer implements an extension never strands its stored resources.
//
// Refusals name field paths, the type, its member and schema locations. They
// never contain configuration values, undeclared keys or the schema
// validator's message.
package tenantconfig
