// Package routing decodes request routing controls and composes the attempt
// chain they select from authorized model groups.
package routing

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

// Access selects the controls a caller may send.
type Access uint8

const (
	// ConfiguredAccess accepts models, order and only.
	ConfiguredAccess Access = iota + 1
	// RequestAccountAccess also accepts byok.
	RequestAccountAccess
)

var allowedControls = map[Access][]string{
	ConfiguredAccess:     {"models", "order", "only"},
	RequestAccountAccess: {"models", "order", "only", "byok"},
}

// Controls are the decoded call-level gateway controls of one request.
type Controls struct {
	// Models are the fallback model IDs, in order.
	Models []string `json:"models"`
	// Order is the provider preference.
	Order []string `json:"order"`
	// Only is the exclusive provider filter, nil when absent or null. An empty,
	// non-nil filter excludes every candidate.
	Only *[]string `json:"only"`
	// BYOK is the raw request-account map, nil when absent.
	BYOK json.RawMessage `json:"byok"`
}

// Routed reports whether the controls change the attempt chain.
func (c Controls) Routed() bool {
	return len(c.Models) != 0 || len(c.Order) != 0 || c.Only != nil
}

// Decode decodes raw call-level gateway controls for access with ordinary Go
// JSON decoding. It returns [execution.ErrUnsupportedGatewayControl] for a
// control access does not accept and [execution.ErrRoutingControls] for
// malformed controls or an empty or null list entry. Absent or null controls
// decode to zero Controls.
func Decode(raw json.RawMessage, access Access) (Controls, error) {
	var members map[string]json.RawMessage
	if raw == nil {
		return Controls{}, nil
	}
	if err := json.Unmarshal(raw, &members); err != nil {
		return Controls{}, execution.ErrRoutingControls
	}
	for name := range members {
		if !slices.ContainsFunc(allowedControls[access], func(control string) bool { return strings.EqualFold(name, control) }) {
			return Controls{}, execution.ErrUnsupportedGatewayControl
		}
	}
	var controls Controls
	if err := json.Unmarshal(raw, &controls); err != nil {
		return Controls{}, execution.ErrRoutingControls
	}
	if slices.Contains(controls.Models, "") || slices.Contains(controls.Order, "") || (controls.Only != nil && slices.Contains(*controls.Only, "")) {
		return Controls{}, execution.ErrRoutingControls
	}
	return controls, nil
}
