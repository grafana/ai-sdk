// Package schema provides JSON Schema generation, compilation, and validation
// for the AI SDK. It is the Go equivalent of the upstream TypeScript SDK's
// Schema type from @ai-sdk/provider-utils.
//
// The central type is [Schema], which bundles a JSON Schema definition with a
// pre-compiled validator. Schema values are created via constructors:
//
//   - [SchemaFor] generates a Schema from a Go struct type using struct tags
//   - [SchemaFromJSON] creates a Schema from raw JSON Schema bytes
//   - [SchemaFromFile] loads a Schema from a JSON Schema file on disk
//
// Schema generation uses invopop/jsonschema for rich struct tag support
// (enum, pattern, title, description, format, minimum, maximum, etc.).
// Validation uses santhosh-tekuri/jsonschema for full JSON Schema compliance.
//
// This package is a leaf with no dependencies on other packages in the module,
// allowing it to be imported by both the root aisdk package and the output
// package without creating import cycles.
package schema
