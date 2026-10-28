// Package host is the contract between the Gateway plugin host and the API
// adapters it serves.
//
// An adapter supplies:
//   - a route handler that maps its requests onto the shared
//     [github.com/grafana/ai-sdk/ai-gateway/internal/execution.Runtime] and
//     encodes the outcomes in its own wire format;
//   - an [ErrorWriter] that writes its fixed document for each [ErrorCategory];
//   - optionally, a [DiscoveryProjection] that encodes listed models as its
//     discovery document;
//   - optionally, an execution.Translator passed with each call, which maps its
//     API semantics to each candidate's native options.
//
// The host provides:
//   - the access decision and request telemetry for every route, refusing
//     callers with the adapter's permission document before the body is read;
//   - one access-aware selector, so configured and BYOK callers share one
//     runtime;
//   - discovery listing, its access decision and its internal failures;
//   - per-attempt translation: every configured candidate and BYOK account is
//     wrapped once, inside fallback and outside native option validation, and
//     translated options are re-checked by host policy.
//
// Registering an adapter is one route-table entry that names the method, the
// path under the namespaced route prefix, a telemetry label, the handler and
// the error writer. Production entries must match the committed manifest.
package host
