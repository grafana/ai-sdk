// Package output provides structured output capabilities for the AI SDK.
//
// It enables LLM responses to be parsed and validated against JSON schemas,
// returning typed Go values instead of raw text. The package supports four
// output modes:
//
//   - Object: Generate a single typed object matching a JSON schema
//   - Array: Generate an array of typed elements
//   - Choice: Select from a predefined set of string options
//   - JSON: Generate unstructured but valid JSON
//
// The primary entry points are the factory functions (Object, Array, Choice,
// JSON) which create Output implementations for use with StreamText/GenerateText,
// and the convenience wrappers GenerateObject and StreamObject for type-safe
// end-to-end usage.
//
// Schema generation and validation are provided by the [github.com/grafana/ai-sdk/schema]
// package. Use [schema.SchemaFor] to generate schemas from Go types and
// [schema.SchemaFromJSON] to create schemas from raw JSON bytes.
package output
