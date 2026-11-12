package extension

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/ai-sdk/schema"
)

// Scope names who configures an extension.
type Scope string

const (
	// ScopeOperator extensions are configured by the operator, through the
	// Gateway's extensions setting.
	ScopeOperator Scope = "operator"
	// ScopeTenant extensions are configured per namespace, by Middleware
	// resources named after the extension. A tenant-scope definition must
	// bind to a public type in package tenanttype, which declares its
	// configuration schema and default. A namespace without a resource uses
	// the extension's default state, and a host that gives the plugin no API
	// access applies the default state everywhere.
	ScopeTenant Scope = "tenant"
)

// FailurePolicy selects how a call continues when one of the extension's
// phases fails.
type FailurePolicy string

const (
	// FailClosed fails the call with [ErrUnavailable].
	FailClosed FailurePolicy = "fail-closed"
	// FailSkip discards the failed phase's effect and continues the call.
	FailSkip FailurePolicy = "skip"
)

// Definition declares an extension. It is data plus a factory; the host
// validates it at startup and fails startup when it is invalid.
type Definition struct {
	// Name identifies the extension in configuration, logs and metrics. It is
	// 1 to 63 lowercase letters, digits or hyphens, starting and ending with a
	// letter or digit, and unique within a distribution.
	Name  string
	Scope Scope
	// Keys lists the key ownerships of the calls the extension applies to,
	// whatever their source. Nil applies to both.
	Keys          []KeyOwnership
	FailurePolicy FailurePolicy
	// ConfigSchema is the JSON Schema the extension's configuration must match.
	// The zero value accepts only an empty configuration.
	ConfigSchema schema.Schema
	// Secrets names the secrets the extension needs. An operator configuration
	// entry supplies each one as <name>Env, the environment variable holding
	// it. Names are 1 to 63 ASCII letters or digits, starting with a lowercase
	// letter.
	Secrets []string
	// EnabledByDefault enables the extension when it has no configuration
	// entry.
	EnabledByDefault bool
	// New constructs the extension's one instance, once per process at
	// startup, with its dependencies. An operator-scope extension gets its
	// validated configuration from the operator setting. A tenant-scope
	// extension gets an empty configuration and receives each namespace's
	// configuration per call in [Call.Config]. The instance must implement at
	// least one phase interface: [Admitter], [RequestTransformer],
	// [RequestInspector], [ResponseInspector] or [Observer]. It may also
	// implement [Closer], which runs once at shutdown.
	New func(config json.RawMessage, deps Dependencies) (any, error)
	// Decode turns a tenant-scope extension's stored configuration, a JSON
	// object its ConfigSchema accepts, into the value its calls receive in
	// [Call.Config]. The host runs it once each time a namespace's stored
	// configuration changes, never per call, and once at startup on the
	// empty configuration of a type enabled by default. Nil passes the
	// configuration through as its canonical [json.RawMessage]. An error or
	// panic applies the extension's failure policy in that namespace, or
	// fails startup for the default configuration. It may run concurrently
	// for different namespaces. The returned value is shared by concurrent
	// calls and must not be modified. A type without a
	// configuration schema never decodes. Operator-scope definitions must
	// leave it nil.
	Decode func(config json.RawMessage) (any, error)
}

// Dependencies are the process services an extension instance may use.
type Dependencies struct {
	// Logger writes through the Gateway's log protocol, with the extension's
	// name attached. It is not filtered by the Gateway's log allowlist.
	Logger *slog.Logger
	// Registerer registers the extension's collectors, with the extension's
	// name as the constant label "extension".
	Registerer prometheus.Registerer
	// Clock returns the current time.
	Clock func() time.Time
	// Secrets holds each declared secret by name.
	Secrets map[string]string
}
