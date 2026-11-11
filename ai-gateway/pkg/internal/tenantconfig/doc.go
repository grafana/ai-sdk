// Package tenantconfig validates tenant extension configuration: the
// Middleware resources a stack stores to configure the distribution's
// tenant-scope extensions.
//
// Validator is the Middleware admission validator. A resource is named after
// the extension it configures, and its configuration must match the
// extension's schema. A disable-only update is accepted whatever the stored
// type, so a build that no longer registers an extension never strands its
// stored resources.
//
// Refusals name field paths, the type and schema locations. They never
// contain configuration values or the schema validator's message.
package tenantconfig
