package customertelemetry

import (
	"errors"
	"maps"
	"net/url"
	"strings"
	"time"

	"github.com/grafana/ai-sdk/ai-gateway/cmd/grafana-ai-gateway/internal/auth"
)

// Signal identifies an independently routed telemetry signal.
type Signal string

const (
	// SignalGenerations identifies model generation records.
	SignalGenerations Signal = "generations"
	// SignalTraces identifies customer trace spans.
	SignalTraces Signal = "traces"
	// SignalMetrics identifies customer model metrics.
	SignalMetrics Signal = "metrics"
	// SignalLogs reserves a destination for a future customer log producer.
	SignalLogs Signal = "logs"
)

var (
	// ErrStack indicates missing, refused, or mismatched stack attribution.
	ErrStack = errors.New("customertelemetry: invalid stack attribution")
	// ErrSnapshot indicates missing revision or invalid validity information.
	ErrSnapshot = errors.New("customertelemetry: invalid snapshot metadata")
	// ErrDestination indicates an invalid signal destination or credential reference.
	ErrDestination = errors.New("customertelemetry: invalid destination")
)

// Stack carries authorized namespace attribution, not an ingestion identity.
// Its zero value cannot authorize customer recording.
type Stack struct{ namespace string }

// StackFromCaller retains only the namespace from an authorized caller.
// It relies on the host having authenticated and authorized that caller.
func StackFromCaller(caller auth.Caller) (Stack, error) {
	if caller.Namespace == "" {
		return Stack{}, ErrStack
	}
	switch caller.AccountAccess() {
	case auth.ConfiguredAccounts, auth.RequestBYOK:
		return Stack{namespace: caller.Namespace}, nil
	default:
		return Stack{}, ErrStack
	}
}

// Namespace returns the exact authorized namespace, without normalization.
func (s Stack) Namespace() string { return s.namespace }

// CredentialRef names deployment-managed ingestion authority and its version.
// Neither field may contain secret material. No authentication scheme is implied.
type CredentialRef struct {
	Name    string
	Version string
}

// Destination describes one signal's ingestion target. It contains no secret.
// IngestionIdentity is opaque and need not equal the stack or another signal ID.
type Destination struct {
	Endpoint          string
	IngestionIdentity string
	Credential        CredentialRef
}

// DestinationInput is discovery output awaiting validation. Missing signal entries
// explicitly mean unavailable; they never select another signal's destination.
type DestinationInput struct {
	Namespace  string
	Revision   string
	ValidUntil time.Time
	Signals    map[Signal]Destination
}

// Destinations is an immutable, stack-scoped snapshot of available destinations.
type Destinations struct {
	stack      Stack
	revision   string
	validUntil time.Time
	signals    map[Signal]Destination
}

// NewDestinations validates discovery output against the requesting stack.
// Errors intentionally omit input values. Egress allowlisting is an adapter duty.
func NewDestinations(stack Stack, input DestinationInput, now time.Time) (Destinations, error) {
	if stack.namespace == "" || input.Namespace != stack.namespace {
		return Destinations{}, ErrStack
	}
	if !validMetadata(input.Revision, input.ValidUntil, now) {
		return Destinations{}, ErrSnapshot
	}
	for signal, destination := range input.Signals {
		if !validSignal(signal) || !validDestination(destination) {
			return Destinations{}, ErrDestination
		}
	}
	return Destinations{stack, input.Revision, input.ValidUntil, maps.Clone(input.Signals)}, nil
}

// Revision identifies destination configuration independently of recording policy.
func (d Destinations) Revision() string { return d.revision }

// ValidUntil returns the exclusive validity deadline.
func (d Destinations) ValidUntil() time.Time { return d.validUntil }

// Lookup returns a value copy only for the matching stack and a current snapshot.
// Availability does not imply recording permission; use RecordingDestination
// to combine this result with an effective policy.
func (d Destinations) Lookup(stack Stack, signal Signal, now time.Time) (Destination, bool) {
	if stack.namespace == "" || d.stack != stack || !validMetadata(d.revision, d.validUntil, now) {
		return Destination{}, false
	}
	destination, ok := d.signals[signal]
	return destination, ok
}

func validMetadata(revision string, until, now time.Time) bool {
	return strings.TrimSpace(revision) != "" && !now.IsZero() && now.Before(until)
}

func validSignal(signal Signal) bool {
	switch signal {
	case SignalGenerations, SignalTraces, SignalMetrics, SignalLogs:
		return true
	default:
		return false
	}
}

func validDestination(d Destination) bool {
	if strings.TrimSpace(d.IngestionIdentity) == "" || strings.TrimSpace(d.Credential.Name) == "" || strings.TrimSpace(d.Credential.Version) == "" {
		return false
	}
	u, err := url.Parse(d.Endpoint)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.Opaque == "" && !strings.ContainsAny(d.Endpoint, "?#")
}
