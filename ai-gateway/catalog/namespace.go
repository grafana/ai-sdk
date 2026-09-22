package catalog

import (
	"fmt"
	"reflect"
	"sort"
)

type modelNamespace struct {
	models  map[string]struct{}
	aliases map[string]string
	infos   []ModelInfo
}

func newModelNamespace(infos []ModelInfo) (modelNamespace, error) {
	namespace := modelNamespace{
		models:  make(map[string]struct{}, len(infos)),
		aliases: make(map[string]string),
		infos:   make([]ModelInfo, len(infos)),
	}

	for i, info := range infos {
		if info.ID == "" {
			return modelNamespace{}, fmt.Errorf("catalog: model ID is required")
		}
		if _, exists := namespace.models[info.ID]; exists {
			return modelNamespace{}, fmt.Errorf("catalog: duplicate model ID %q", info.ID)
		}
		namespace.models[info.ID] = struct{}{}
		namespace.infos[i] = cloneModelInfo(info)
	}

	for _, info := range namespace.infos {
		for _, alias := range info.Aliases {
			if alias == "" {
				return modelNamespace{}, fmt.Errorf("catalog: alias is required for model %q", info.ID)
			}
			if _, exists := namespace.models[alias]; exists {
				return modelNamespace{}, fmt.Errorf("catalog: alias %q collides with a model ID", alias)
			}
			if existingID, exists := namespace.aliases[alias]; exists {
				return modelNamespace{}, fmt.Errorf("catalog: duplicate alias %q for models %q and %q", alias, existingID, info.ID)
			}
			namespace.aliases[alias] = info.ID
		}
	}

	sort.Slice(namespace.infos, func(i, j int) bool {
		return namespace.infos[i].ID < namespace.infos[j].ID
	})

	return namespace, nil
}

func (n modelNamespace) canonicalID(modelID string) (string, bool) {
	if _, exists := n.models[modelID]; exists {
		return modelID, true
	}
	canonicalID, exists := n.aliases[modelID]
	return canonicalID, exists
}

func (n modelNamespace) list() []ModelInfo {
	infos := make([]ModelInfo, len(n.infos))
	for i, info := range n.infos {
		infos[i] = cloneModelInfo(info)
	}
	return infos
}

func cloneModelInfo(info ModelInfo) ModelInfo {
	cloned := info
	cloned.Aliases = append([]string(nil), info.Aliases...)
	cloned.Capabilities = append([]ModelCapability(nil), info.Capabilities...)
	return cloned
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
