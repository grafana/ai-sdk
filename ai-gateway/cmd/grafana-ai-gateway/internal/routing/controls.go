// Package routing decodes request routing controls and composes the attempt
// chain they select from authorized model groups.
package routing

import (
	"encoding/json"
	"strconv"

	"github.com/grafana/ai-sdk/ai-gateway/internal/execution"
)

// Access selects the controls a caller may send.
type Access uint8

const (
	// ConfiguredAccess accepts models, order and only.
	ConfiguredAccess Access = iota + 1
	// RequestAccountAccess also accepts byok and providerTimeouts.byok.
	RequestAccountAccess
)

const (
	keyModels           = "models"
	keyOrder            = "order"
	keyOnly             = "only"
	keyBYOK             = "byok"
	keyProviderTimeouts = "providerTimeouts"

	// MaxProviderTimeout is the largest accepted provider start timeout, in
	// milliseconds.
	MaxProviderTimeout = 600_000
)

var allowedControls = map[Access]map[string]bool{
	ConfiguredAccess:     {keyModels: true, keyOrder: true, keyOnly: true},
	RequestAccountAccess: {keyModels: true, keyOrder: true, keyOnly: true, keyBYOK: true, keyProviderTimeouts: true},
}

// Controls are the decoded call-level gateway controls of one request.
type Controls struct {
	// Models are the fallback model IDs, in order.
	Models []string
	// Order is the provider preference.
	Order []string
	// Only is the exclusive provider filter, nil when absent. An empty,
	// non-nil filter excludes every candidate.
	Only *[]string
	// BYOK is the raw request-account map, nil when absent.
	BYOK json.RawMessage
	// ProviderTimeouts maps provider slugs to start timeouts in milliseconds.
	ProviderTimeouts map[string]int
}

// Routed reports whether the controls change the attempt chain.
func (c Controls) Routed() bool {
	return len(c.Models) != 0 || len(c.Order) != 0 || c.Only != nil
}

// Decode decodes raw call-level gateway controls for access. Control names are
// matched exactly. It returns [execution.ErrUnsupportedGatewayControl] for a
// control access does not accept, [execution.ErrRoutingControls] for malformed
// models, order or only, and [execution.ErrProviderTimeouts] for a malformed
// providerTimeouts map. Absent or null controls decode to zero Controls.
func Decode(raw json.RawMessage, access Access) (Controls, error) {
	if raw == nil {
		return Controls{}, nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return Controls{}, execution.ErrRoutingControls
	}
	allowed := allowedControls[access]
	for name := range members {
		if !allowed[name] {
			return Controls{}, execution.ErrUnsupportedGatewayControl
		}
	}
	if timeouts, ok := members[keyProviderTimeouts]; ok && hasUnsupportedTimeoutMember(timeouts) {
		return Controls{}, execution.ErrUnsupportedGatewayControl
	}
	var controls Controls
	var err error
	if controls.Models, err = decodeList(members[keyModels]); err != nil {
		return Controls{}, err
	}
	if controls.Order, err = decodeList(members[keyOrder]); err != nil {
		return Controls{}, err
	}
	if only, ok := members[keyOnly]; ok && !isNull(only) {
		list, err := decodeList(only)
		if err != nil {
			return Controls{}, err
		}
		if list == nil {
			list = []string{}
		}
		controls.Only = &list
	}
	if timeouts, ok := members[keyProviderTimeouts]; ok {
		if controls.ProviderTimeouts, err = decodeProviderTimeouts(timeouts); err != nil {
			return Controls{}, err
		}
	}
	if accounts, ok := members[keyBYOK]; ok {
		controls.BYOK = accounts
	}
	return controls, nil
}

func decodeList(raw json.RawMessage) ([]string, error) {
	if raw == nil || isNull(raw) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, execution.ErrRoutingControls
	}
	list := make([]string, 0, len(entries))
	for _, entry := range entries {
		var value string
		if isNull(entry) || json.Unmarshal(entry, &value) != nil || value == "" {
			return nil, execution.ErrRoutingControls
		}
		list = append(list, value)
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list, nil
}

func decodeProviderTimeouts(raw json.RawMessage) (map[string]int, error) {
	if isNull(raw) {
		return nil, nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, execution.ErrProviderTimeouts
	}
	byok, ok := members[keyBYOK]
	if !ok || isNull(byok) {
		return nil, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(byok, &values); err != nil {
		return nil, execution.ErrProviderTimeouts
	}
	timeouts := make(map[string]int, len(values))
	for name, value := range values {
		milliseconds, err := strconv.Atoi(string(value))
		if err != nil || milliseconds < 1 || milliseconds > MaxProviderTimeout {
			return nil, execution.ErrProviderTimeouts
		}
		timeouts[name] = milliseconds
	}
	return timeouts, nil
}

// hasUnsupportedTimeoutMember reports whether a providerTimeouts object has a
// member other than byok. A value that is not an object is left to
// [decodeProviderTimeouts].
func hasUnsupportedTimeoutMember(raw json.RawMessage) bool {
	var members map[string]json.RawMessage
	if json.Unmarshal(raw, &members) != nil {
		return false
	}
	for name := range members {
		if name != keyBYOK {
			return true
		}
	}
	return false
}

func isNull(raw json.RawMessage) bool {
	return string(raw) == "null"
}
