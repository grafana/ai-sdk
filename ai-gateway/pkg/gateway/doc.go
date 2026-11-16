// Package gateway runs the Grafana AI Gateway plugin backend. A
// distribution's main calls [Run], which owns everything the plugin host
// relies on, so no distribution can miss or change it: the plugin runtime, the
// plugin ID, the platform authenticator, the process logger, environment and
// metrics registerer, the app provider that receives the host's API access
// configuration, the lifecycle events and the final telemetry flush.
//
// [Options] selects what a distribution adds to the shipped Gateway; its zero
// value runs the shipped Gateway. Importing the package registers nothing.
//
// # Stability
//
// This API is v0 and experimental. It may change or break in any minor
// release, and is not a stable extension contract.
package gateway
