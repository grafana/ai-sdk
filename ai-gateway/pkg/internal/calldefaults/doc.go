// Package calldefaults implements the shipped tenant extensions that give a
// stack's model calls default instructions and default settings, bound to
// the public default-instructions and default-settings types.
//
// Both reuse the SDK's model middleware, so they keep the upstream
// semantics: default instructions are prepended only to calls without a
// system message, and a default setting fills only a field the call leaves
// unset. Both are request transforms that apply to every key ownership and
// source, skip on failure, and register no collectors.
package calldefaults
