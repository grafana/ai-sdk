// Package registry provides provider management for the AI SDK.
//
// It includes a Provider interface for string-based model resolution,
// a ProviderRegistry for multi-provider routing via composite IDs
// (e.g., "anthropic:claude-sonnet-4-6"), and a CustomProvider for model
// aliasing, fallback delegation, and access control.
//
// The registry mirrors the upstream Vercel AI SDK's provider registry
// and custom provider functionality, adapted to Go idioms.
package registry
