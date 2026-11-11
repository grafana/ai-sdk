package v4

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	"github.com/grafana/ai-sdk/ai-gateway/internal/host"
)

type discoveryModel struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description,omitempty"`
	Specification discoverySpecification `json:"specification"`
	Gateway       *configuredRoute       `json:"gateway,omitempty"`
}

type configuredRoute struct {
	Aliases   []string              `json:"aliases"`
	Primary   configuredCandidate   `json:"primary"`
	Fallbacks []configuredCandidate `json:"fallbacks"`
}

type configuredCandidate struct {
	ProviderInstance string `json:"providerInstance"`
	Provider         string `json:"provider"`
	ProviderModelID  string `json:"providerModelId"`
}

type discoverySpecification struct {
	SpecificationVersion string `json:"specificationVersion"`
	Provider             string `json:"provider"`
	ModelID              string `json:"modelId"`
}

// DiscoveryProjection encodes listed models as the ProviderWire V4 discovery
// document: canonical rows sorted by ID, with configured route facts when the
// catalog supplies candidates.
type DiscoveryProjection struct{}

var _ host.DiscoveryProjection = DiscoveryProjection{}

// ProjectDiscovery encodes models without revalidating them.
func (DiscoveryProjection) ProjectDiscovery(models []catalog.ModelInfo) ([]byte, error) {
	rows := make([]discoveryModel, 0, len(models))
	for _, info := range models {
		rows = append(rows, discoveryRow(info))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(struct {
		Models []discoveryModel `json:"models"`
	}{rows}); err != nil {
		return nil, fmt.Errorf("providerwire v4: encoding discovery: %w", err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func discoveryRow(info catalog.ModelInfo) discoveryModel {
	value := discoveryModel{
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Specification: discoverySpecification{
			SpecificationVersion: "v4",
			Provider:             "grafana",
			ModelID:              info.ID,
		},
	}
	if len(info.Candidates) != 0 {
		aliases := info.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		candidates := make([]configuredCandidate, len(info.Candidates))
		for index, candidate := range info.Candidates {
			candidates[index] = configuredCandidate{ProviderInstance: candidate.ProviderInstance, Provider: candidate.Provider, ProviderModelID: candidate.ModelID}
		}
		value.Gateway = &configuredRoute{Aliases: aliases, Primary: candidates[0], Fallbacks: candidates[1:]}
	}
	return value
}
