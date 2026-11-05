// Package models projects the public gateway catalog in OpenAI model format.
package models

import (
	"encoding/json"
	"sort"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
)

const Path = "/v1/models"

// Projection exposes only public model IDs and aliases. The catalog has no
// creation timestamp; zero is an explicit unknown-time convention for virtual
// gateway models, not the creation time of an upstream model.
type Projection struct{}

type model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func row(id string) model { return model{ID: id, Object: "model", OwnedBy: "grafana"} }

func (Projection) ProjectDiscovery(infos []catalog.ModelInfo) ([]byte, error) {
	ids := map[string]struct{}{}
	for _, info := range infos {
		ids[info.ID] = struct{}{}
		for _, id := range info.Aliases {
			ids[id] = struct{}{}
		}
	}
	rows := make([]model, 0, len(ids))
	for id := range ids {
		rows = append(rows, row(id))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return json.Marshal(struct {
		Object string  `json:"object"`
		Data   []model `json:"data"`
	}{"list", rows})
}

func (Projection) ProjectDiscoveryResource(infos []catalog.ModelInfo, id string) ([]byte, bool, error) {
	for _, info := range infos {
		found := info.ID == id
		for _, alias := range info.Aliases {
			found = found || alias == id
		}
		if found {
			data, err := json.Marshal(row(id))
			return data, true, err
		}
	}
	return nil, false, nil
}
