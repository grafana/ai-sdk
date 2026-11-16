package extensionhost

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/grafana/ai-sdk/ai-gateway/pkg/extension"
)

// phaseConfigure labels failures of a stored tenant configuration, which is
// not a call phase.
const phaseConfigure extension.Phase = "configure"

// namespaceConfig is one namespace configuration revision: the lists each
// key ownership's calls use, with each tenant extension's decoded
// configuration. It is immutable once built, so calls keep using a revision
// the cache has since replaced or evicted.
type namespaceConfig struct {
	lists map[extension.KeyOwnership]*lists
}

// buildNamespace builds the configuration of a namespace's resources. A
// tenant extension without a resource uses its default state; a disabled one
// is off; an enabled one whose member its schema accepts runs with the
// member's decoded value. Any other resource, and a decoding that fails,
// apply the extension's failure policy, logged and counted once per build.
// An enabled resource of a type this build does not implement is skipped,
// logged and counted once per build; the cache builds only when a
// namespace's resources change.
func (h *Host) buildNamespace(namespace string, resources []TenantResource) *namespaceConfig {
	byName := make(map[string]TenantResource, len(resources))
	for _, resource := range resources {
		byName[resource.Name] = resource
		if !resource.Invalid && resource.Enabled && !h.tenantDefinition(resource.Name) {
			h.unimplementedTypes.Inc()
			h.logger.Warn("gateway tenant configuration has a type this build does not implement", "namespace", namespace, "type", resource.Name)
		}
	}
	members := h.operatorMembers()
	var unavailable []extension.Definition
	failed := func(definition extension.Definition, class failureClass) {
		h.recordConfigurationFailure(definition, namespace, class)
		if definition.FailurePolicy == extension.FailClosed {
			unavailable = append(unavailable, definition)
		}
	}
	for _, definition := range h.tenant {
		running := h.tenantInstances[definition.Name]
		resource, found := byName[definition.Name]
		if !found {
			if fallback, ok := h.defaults[definition.Name]; ok {
				members = append(members, member{instance: running, config: fallback})
			}
			continue
		}
		if !resource.Invalid && resource.Type == resource.Name && !resource.Enabled {
			continue
		}
		configuration, ok := configurationOf(definition, resource)
		if !ok {
			failed(definition, classInvalid)
			continue
		}
		decoded, class, err := decode(definition, configuration)
		if err != nil {
			failed(definition, class)
			continue
		}
		members = append(members, member{instance: running, config: decoded})
	}
	slices.SortFunc(members, func(a, b member) int { return strings.Compare(a.instance.definition.Name, b.instance.definition.Name) })
	built := &namespaceConfig{lists: make(map[extension.KeyOwnership]*lists, len(keyOwnerships))}
	for _, keys := range keyOwnerships {
		built.lists[keys] = newLists(members, keys, unavailable)
	}
	return built
}

// configurationOf returns the configuration an enabled resource gives its
// definition, and whether the definition accepts it: a type with a schema
// needs its member, which must match the schema, and a type without one
// takes no member and gets an empty configuration.
func configurationOf(definition extension.Definition, resource TenantResource) (json.RawMessage, bool) {
	switch {
	case resource.Invalid || resource.Type != resource.Name:
		return nil, false
	case definition.ConfigSchema.JSON() == nil:
		return emptyConfig, resource.Config == nil
	case resource.Config == nil:
		return nil, false
	default:
		return resource.Config, CheckConfig(definition, resource.Config) == nil
	}
}

// failureConfig returns the configuration of a namespace without a usable
// one: operator extensions run, and every tenant extension applies its
// failure policy. It is shared.
func (h *Host) failureConfig() *namespaceConfig {
	var unavailable []extension.Definition
	for _, definition := range h.tenant {
		if definition.FailurePolicy == extension.FailClosed {
			unavailable = append(unavailable, definition)
		}
	}
	failed := &namespaceConfig{lists: make(map[extension.KeyOwnership]*lists, len(keyOwnerships))}
	operators := h.operatorMembers()
	for _, keys := range keyOwnerships {
		failed.lists[keys] = newLists(operators, keys, unavailable)
	}
	return failed
}

// recordConfigurationFailure counts and logs one tenant configuration
// failure without configuration values or error text.
func (h *Host) recordConfigurationFailure(definition extension.Definition, namespace string, class failureClass) {
	h.events.WithLabelValues(definition.Name, string(phaseConfigure), eventFailed).Inc()
	h.logger.Warn("gateway tenant extension configuration failed", "extension", definition.Name, "namespace", namespace, "policy", string(definition.FailurePolicy), "class", string(class))
}
