// Package stackcatalog validates and converts stack model catalogs: the
// ModelCatalog resources a stack stores with its provider keys as Grafana
// secure values.
//
// Parse applies the stack catalog policy, converts the spec into the
// operator catalog's config.File, runs the shared shape checks and checks
// that secure keys and providers match one to one. Validator applies Parse
// at admission, after merging the stored secure values with the update the
// way the platform will store them.
//
// Errors name field paths, provider names, secure keys and catalog
// identifiers such as model IDs and aliases. They never contain secure
// values.
package stackcatalog
