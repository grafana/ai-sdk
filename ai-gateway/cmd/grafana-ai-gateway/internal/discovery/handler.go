package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/grafana/ai-sdk/ai-gateway/catalog"
	providerv4 "github.com/grafana/ai-sdk/ai-gateway/providerwire/v4"
)

type model struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Description   string           `json:"description,omitempty"`
	Specification specification    `json:"specification"`
	Gateway       *configuredRoute `json:"gateway,omitempty"`
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

type specification struct {
	SpecificationVersion string `json:"specificationVersion"`
	Provider             string `json:"provider"`
	ModelID              string `json:"modelId"`
}

type handler struct {
	lister catalog.ModelLister
	errors *providerv4.HostErrorWriter
}

// New constructs a discovery handler for a validated catalog.
func New(lister catalog.ModelLister, errorWriter *providerv4.HostErrorWriter) http.Handler {
	return &handler{lister: lister, errors: errorWriter}
}

func (handler *handler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	models, err := handler.safeList(request.Context())
	if err != nil {
		handler.errors.Write(w, providerv4.HostErrorInternal)
		return
	}
	document, err := handler.encode(models)
	if err != nil {
		handler.errors.Write(w, providerv4.HostErrorInternal)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(document)
}

func (handler *handler) safeList(ctx context.Context) (models []catalog.ModelInfo, err error) {
	defer func() {
		if recover() != nil {
			models = nil
			err = fmt.Errorf("gateway discovery: listing models panicked")
		}
	}()
	return handler.lister.ListModels(ctx)
}

func (handler *handler) encode(infos []catalog.ModelInfo) ([]byte, error) {
	rows := make([]model, 0, len(infos))
	for _, info := range infos {
		rows = append(rows, discoveryModel(info))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(struct {
		Models []model `json:"models"`
	}{rows}); err != nil {
		return nil, fmt.Errorf("gateway discovery: encoding catalog: %w", err)
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func discoveryModel(info catalog.ModelInfo) model {
	value := model{
		ID:          info.ID,
		Name:        info.Name,
		Description: info.Description,
		Specification: specification{
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
