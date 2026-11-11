// Package tenantconfig validates and reads tenant extension configuration:
// the Middleware resources a stack stores to configure the distribution's
// tenant-scope extensions.
//
// Validator is the Middleware admission validator. A resource is named after
// the extension it configures, and its configuration must match the
// extension's schema. A disable-only update is accepted whatever the stored
// type, so a build that no longer registers an extension never strands its
// stored resources. Without API access every create and update is refused,
// because the Gateway could not apply what it would store.
//
// Reader lists a namespace's resources through the host's API access
// configuration, as the plugin. It decodes each stored resource in one
// place, keeping its configuration as the stored JSON.
//
// Refusals name field paths, the type and schema locations. They never
// contain configuration values or the schema validator's message.
package tenantconfig
