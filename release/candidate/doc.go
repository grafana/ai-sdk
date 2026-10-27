// Package candidate generates and verifies the first-party requirement files
// of a combined library release pull request.
//
// It builds module archives from a candidate tree with the same content the Go
// command and public proxy produce for the resulting tags, rewrites first-party
// requirements of released modules and Gateway, and records their checksums in
// dependency order. The module is local-only and is never published.
package candidate
