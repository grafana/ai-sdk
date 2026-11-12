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
	// ScopeTenant extensions are configured per namespace. Until namespace
	// configuration exists, they run with their default state and an empty
	// configuration.
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
	// New constructs an instance from its validated configuration, a JSON
	// object, and its dependencies. The instance must implement at least one
	// phase interface: [Admitter], [RequestTransformer], [RequestInspector],
	// [ResponseInspector] or [Observer]. It may also implement [Closer]. New
	// may run more than once per process, once per configuration it is given.
	New func(config json.RawMessage, deps Dependencies) (any, error)
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
